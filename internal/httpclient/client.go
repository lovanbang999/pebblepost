package httpclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/proxy"
	"pebblepost/internal/grpcclient"
	"pebblepost/internal/types"
)

// bodyDisplayThreshold is the maximum number of bytes sent to the frontend as inline body text.
// Bodies larger than this are streamed to a temp file and the field BodyTruncated is set to true.
const bodyDisplayThreshold = 5 * 1024 * 1024 // 5 MB

// Client defines the interface for executing HTTP requests.
type Client interface {
	Execute(ctx context.Context, req *types.RequestDefinition) (*types.ExecutionResult, error)
	SetWorkspace(wsPath string)
	GetCookieJar() *PersistentJar
	GetGrpcClient() *grpcclient.Client
}

// DefaultClient implements the Client interface using Go's net/http.
type DefaultClient struct {
	defaultTimeout time.Duration
	cookieJar      *PersistentJar
	oauth2Manager  *OAuth2Manager
	grpcClient     *grpcclient.Client
}

// NewClient creates a new HTTP Client instance.
func NewClient() *DefaultClient {
	return &DefaultClient{
		defaultTimeout: 30 * time.Second,
		cookieJar:      NewPersistentJar(""),
		oauth2Manager:  GetOAuth2Manager(),
		grpcClient:     grpcclient.NewClient(""),
	}
}

// SetWorkspace updates the workspace path for the client's cookie jar and grpc client.
func (c *DefaultClient) SetWorkspace(wsPath string) {
	if c.cookieJar == nil {
		c.cookieJar = NewPersistentJar(wsPath)
	} else {
		c.cookieJar.SetWorkspace(wsPath)
	}
	if c.grpcClient == nil {
		c.grpcClient = grpcclient.NewClient(wsPath)
	} else {
		c.grpcClient.SetWorkspace(wsPath)
	}
}

// GetCookieJar returns the persistent cookie jar attached to this client.
func (c *DefaultClient) GetCookieJar() *PersistentJar {
	return c.cookieJar
}

// GetGrpcClient returns the dynamic gRPC client.
func (c *DefaultClient) GetGrpcClient() *grpcclient.Client {
	return c.grpcClient
}

// Execute performs the HTTP or gRPC request based on the provided RequestDefinition.
func (c *DefaultClient) Execute(ctx context.Context, req *types.RequestDefinition) (*types.ExecutionResult, error) {
	if req == nil {
		return nil, fmt.Errorf("request definition cannot be nil")
	}

	// Dispatch to gRPC dynamic client when protocol is grpc or method is GRPC
	if req.Protocol == "grpc" || req.Method == "GRPC" || req.Grpc != nil {
		if req.Grpc == nil {
			req.Grpc = &types.GrpcDefinition{}
		}
		if req.Grpc.Address == "" && req.URL != "" {
			req.Grpc.Address = req.URL
		}
		return c.grpcClient.Execute(ctx, req)
	}

	// Redirect chain is populated by the CheckRedirect hook in buildHTTPClient.
	var (
		redirectMu    sync.Mutex
		redirectChain []types.RedirectHop
	)

	result := &types.ExecutionResult{
		ExecutedAt: time.Now().UTC(),
		Headers:    make(map[string][]string),
		Tests:      make([]types.TestAssertionResult, 0),
		Logs:       make([]string, 0),
	}

	// 1. Validate & Parse URL
	targetURL, err := c.buildURL(req.URL, req.Params)
	if err != nil {
		result.Error = fmt.Sprintf("invalid URL: %v", err)
		return result, err
	}

	// 2. Prepare Body Bytes & Content-Type
	bodyBytes, contentType, err := c.buildBodyBytes(req.Body)
	if err != nil {
		result.Error = fmt.Sprintf("failed to build request body: %v", err)
		return result, err
	}

	// 3. Normalize HTTP Method
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = http.MethodGet
	}

	// 4. Setup Context & Timeout
	timeout := c.defaultTimeout
	if req.Settings.TimeoutMs > 0 {
		timeout = time.Duration(req.Settings.TimeoutMs) * time.Millisecond
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 5. Setup Network Trace for Timing Metrics
	var (
		dnsStart, dnsDone   time.Time
		connStart, connDone time.Time
		tlsStart, tlsDone   time.Time
		firstByteTime       time.Time
		reqStartTime        = time.Now()
	)

	trace := &httptrace.ClientTrace{
		DNSStart: func(info httptrace.DNSStartInfo) {
			dnsStart = time.Now()
		},
		DNSDone: func(info httptrace.DNSDoneInfo) {
			dnsDone = time.Now()
		},
		ConnectStart: func(network, addr string) {
			connStart = time.Now()
		},
		ConnectDone: func(network, addr string, err error) {
			connDone = time.Now()
		},
		TLSHandshakeStart: func() {
			tlsStart = time.Now()
		},
		TLSHandshakeDone: func(state tls.ConnectionState, err error) {
			tlsDone = time.Now()
		},
		GotFirstResponseByte: func() {
			firstByteTime = time.Now()
		},
	}

	reqCtx = httptrace.WithClientTrace(reqCtx, trace)

	// 6. Create HTTP Request
	httpReq, err := http.NewRequestWithContext(reqCtx, method, targetURL.String(), bytes.NewReader(bodyBytes))
	if err != nil {
		result.Error = fmt.Sprintf("failed to create http request: %v", err)
		return result, err
	}

	// 7. Apply Headers
	c.applyHeaders(httpReq, req.Headers, contentType, req.Settings.UserAgent)

	// 8. Apply Authentication
	c.applyAuth(reqCtx, httpReq, targetURL, &req.Auth, bodyBytes)

	// 9. Build http.Client with Settings (redirect chain is captured inside)
	httpClient := c.buildHTTPClient(req.Settings, &redirectMu, &redirectChain)

	// 10. Execute Request
	reqStartTime = time.Now()
	resp, err := httpClient.Do(httpReq)

	// Digest Auth 401 Challenge Retry
	if err == nil && IsDigestChallenge(resp) && strings.EqualFold(req.Auth.Type, "digest") {
		challengeHeader := resp.Header.Get("WWW-Authenticate")
		challenge, parseErr := ParseDigestChallenge(challengeHeader)
		if parseErr == nil {
			_ = resp.Body.Close()
			digestAuthHeader, buildErr := BuildDigestAuthorization(
				method,
				targetURL.RequestURI(),
				req.Auth.Username,
				req.Auth.Password,
				challenge,
				bodyBytes,
			)
			if buildErr == nil {
				retryReq, retryErr := http.NewRequestWithContext(reqCtx, method, targetURL.String(), bytes.NewReader(bodyBytes))
				if retryErr == nil {
					c.applyHeaders(retryReq, req.Headers, contentType, req.Settings.UserAgent)
					retryReq.Header.Set("Authorization", digestAuthHeader)
					resp, err = httpClient.Do(retryReq)
				}
			}
		}
	}

	if err != nil {
		totalDuration := time.Since(reqStartTime)
		result.Timing.TotalDurationMs = float64(totalDuration.Microseconds()) / 1000.0
		result.Error = fmt.Sprintf("request execution failed: %v", err)
		return result, err
	}
	defer resp.Body.Close()

	// 11. Read Response Body & Track Download Duration
	downloadStart := time.Now()
	if !firstByteTime.IsZero() {
		downloadStart = firstByteTime
	}

	// Capture sent-request summary from the final httpReq (after auth/headers applied).
	sentSummary := &types.SentRequestSummary{
		Method:  httpReq.Method,
		URL:     httpReq.URL.String(),
		Headers: make(map[string][]string),
	}
	for k, vs := range httpReq.Header {
		sentSummary.Headers[k] = vs
	}
	if len(bodyBytes) > 0 && len(bodyBytes) <= 4096 {
		sentSummary.Body = string(bodyBytes)
	}
	result.SentRequest = sentSummary

	// Attach the redirect chain captured during redirect hook.
	redirectMu.Lock()
	result.RedirectChain = redirectChain
	redirectMu.Unlock()

	// Stream large responses to a temp file; only send the first 5MB inline.
	const readLimit = 50 * 1024 * 1024 // 50 MB hard cap
	limitedReader := io.LimitReader(resp.Body, readLimit)
	respBytes, err := io.ReadAll(limitedReader)
	downloadDone := time.Now()

	if err != nil {
		result.Error = fmt.Sprintf("failed to read response body: %v", err)
	}

	actualSize := int64(len(respBytes))
	result.BodySizeBytes = actualSize

	if actualSize > bodyDisplayThreshold {
		// Write full body to temp file and truncate the inline preview.
		tmpFile, tmpErr := os.CreateTemp("", "pebble-body-*.bin")
		if tmpErr == nil {
			_, _ = tmpFile.Write(respBytes)
			_ = tmpFile.Close()
			result.TempBodyFile = tmpFile.Name()
		}
		result.Body = string(respBytes[:bodyDisplayThreshold])
		result.BodyTruncated = true
	} else {
		result.Body = string(respBytes)
	}

	totalDuration := time.Since(reqStartTime)

	// 12. Populate Timing Metrics
	if !dnsStart.IsZero() && !dnsDone.IsZero() {
		result.Timing.DNSLookupMs = float64(dnsDone.Sub(dnsStart).Microseconds()) / 1000.0
	}
	if !connStart.IsZero() && !connDone.IsZero() {
		result.Timing.TCPConnectMs = float64(connDone.Sub(connStart).Microseconds()) / 1000.0
	}
	if !tlsStart.IsZero() && !tlsDone.IsZero() {
		result.Timing.TLSHandshakeMs = float64(tlsDone.Sub(tlsStart).Microseconds()) / 1000.0
	}
	if !firstByteTime.IsZero() {
		result.Timing.TTFBMs = float64(firstByteTime.Sub(reqStartTime).Microseconds()) / 1000.0
	}
	if !downloadStart.IsZero() && !downloadDone.IsZero() {
		result.Timing.DownloadMs = float64(downloadDone.Sub(downloadStart).Microseconds()) / 1000.0
	}
	result.Timing.TotalDurationMs = float64(totalDuration.Microseconds()) / 1000.0

	// 13. Populate Execution Result
	result.StatusCode = resp.StatusCode
	result.StatusText = resp.Status
	result.Headers = resp.Header
	result.Size = actualSize

	return result, nil
}

func (c *DefaultClient) buildURL(rawURL string, params []types.KeyValue) (*url.URL, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return nil, fmt.Errorf("URL cannot be empty")
	}

	if !strings.HasPrefix(trimmed, "http://") && !strings.HasPrefix(trimmed, "https://") {
		trimmed = "http://" + trimmed
	}

	parsedURL, err := url.Parse(trimmed)
	if err != nil {
		return nil, err
	}

	// Merge query parameters
	query := parsedURL.Query()
	for _, param := range params {
		if param.Enabled && param.Key != "" {
			query.Add(param.Key, param.Value)
		}
	}
	parsedURL.RawQuery = query.Encode()

	return parsedURL, nil
}

func (c *DefaultClient) buildBodyBytes(body types.BodyDefinition) ([]byte, string, error) {
	bodyType := strings.ToLower(strings.TrimSpace(body.Type))

	// If body references an external file, load content from disk
	if body.FilePath != "" {
		data, err := os.ReadFile(body.FilePath)
		if err != nil {
			return nil, "", fmt.Errorf("failed to read body file %s: %w", body.FilePath, err)
		}
		contentType := "application/octet-stream"
		switch bodyType {
		case "json":
			contentType = "application/json"
		case "raw":
			contentType = "text/plain"
		}
		return data, contentType, nil
	}

	switch bodyType {
	case "", "none":
		return nil, "", nil

	case "file":
		filePath := body.Raw
		if filePath == "" {
			return nil, "", nil
		}
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, "", fmt.Errorf("failed to read body file %s: %w", filePath, err)
		}
		return data, "application/octet-stream", nil

	case "json":
		raw := strings.TrimSpace(body.Raw)
		if raw == "" {
			return nil, "application/json", nil
		}
		return []byte(raw), "application/json", nil

	case "raw":
		return []byte(body.Raw), "text/plain", nil

	case "urlencoded":
		values := url.Values{}
		for _, kv := range body.UrlEncoded {
			if kv.Enabled && kv.Key != "" {
				values.Add(kv.Key, kv.Value)
			}
		}
		return []byte(values.Encode()), "application/x-www-form-urlencoded", nil

	case "formdata":
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)

		for _, kv := range body.FormData {
			if !kv.Enabled || kv.Key == "" {
				continue
			}

			if kv.Type == "file" {
				filePath := kv.Value
				if filePath == "" {
					continue
				}

				fileName := filepath.Base(filePath)
				fileData, err := os.ReadFile(filePath)
				if err != nil {
					part, partErr := writer.CreateFormFile(kv.Key, fileName)
					if partErr != nil {
						return nil, "", partErr
					}
					_, _ = part.Write([]byte(kv.Value))
					continue
				}

				part, err := writer.CreateFormFile(kv.Key, fileName)
				if err != nil {
					return nil, "", err
				}
				if _, err := part.Write(fileData); err != nil {
					return nil, "", err
				}
			} else {
				if err := writer.WriteField(kv.Key, kv.Value); err != nil {
					return nil, "", err
				}
			}
		}

		if err := writer.Close(); err != nil {
			return nil, "", err
		}
		return buf.Bytes(), writer.FormDataContentType(), nil

	case "graphql":
		if body.GraphQL == nil {
			return []byte("{}"), "application/json", nil
		}

		payload := map[string]any{
			"query": body.GraphQL.Query,
		}

		if strings.TrimSpace(body.GraphQL.Variables) != "" {
			var parsedVars map[string]any
			if err := json.Unmarshal([]byte(body.GraphQL.Variables), &parsedVars); err == nil {
				payload["variables"] = parsedVars
			} else {
				payload["variables"] = body.GraphQL.Variables
			}
		}

		data, err := json.Marshal(payload)
		if err != nil {
			return nil, "", err
		}
		return data, "application/json", nil

	default:
		return []byte(body.Raw), "text/plain", nil
	}
}

func (c *DefaultClient) applyHeaders(req *http.Request, headers []types.KeyValue, computedContentType, customUA string) {
	hasContentType := false
	hasUserAgent := false
	hasAccept := false

	for _, h := range headers {
		if !h.Enabled || strings.TrimSpace(h.Key) == "" {
			continue
		}

		keyLower := strings.ToLower(strings.TrimSpace(h.Key))
		switch keyLower {
		case "content-type":
			hasContentType = true
		case "user-agent":
			hasUserAgent = true
		case "accept":
			hasAccept = true
		}

		req.Header.Add(h.Key, h.Value)
	}

	if !hasContentType && computedContentType != "" {
		req.Header.Set("Content-Type", computedContentType)
	}
	if !hasUserAgent {
		ua := "PebblePost/1.0"
		if strings.TrimSpace(customUA) != "" {
			ua = strings.TrimSpace(customUA)
		}
		req.Header.Set("User-Agent", ua)
	}
	if !hasAccept {
		req.Header.Set("Accept", "*/*")
	}
}

func (c *DefaultClient) applyAuth(ctx context.Context, req *http.Request, u *url.URL, auth *types.AuthDefinition, bodyBytes []byte) {
	if auth == nil {
		return
	}
	authType := strings.ToLower(strings.TrimSpace(auth.Type))

	switch authType {
	case "bearer":
		if token := strings.TrimSpace(auth.Token); token != "" {
			if !strings.HasPrefix(strings.ToLower(token), "bearer ") {
				req.Header.Set("Authorization", "Bearer "+token)
			} else {
				req.Header.Set("Authorization", token)
			}
		}

	case "oauth2":
		// Auto-fetch / auto-refresh OAuth2 token
		if c.oauth2Manager != nil {
			tok, err := c.oauth2Manager.GetToken(ctx, auth)
			if err == nil && tok != nil && tok.AccessToken != "" {
				req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
				return
			}
		}
		// Fallback to static token if present
		if token := strings.TrimSpace(auth.Token); token != "" {
			if !strings.HasPrefix(strings.ToLower(token), "bearer ") {
				req.Header.Set("Authorization", "Bearer "+token)
			} else {
				req.Header.Set("Authorization", token)
			}
		}

	case "basic":
		if auth.Username != "" || auth.Password != "" {
			credentials := fmt.Sprintf("%s:%s", auth.Username, auth.Password)
			encoded := base64.StdEncoding.EncodeToString([]byte(credentials))
			req.Header.Set("Authorization", "Basic "+encoded)
		}

	case "apikey":
		if auth.Key != "" {
			if strings.ToLower(auth.AddTo) == "query" {
				q := u.Query()
				q.Set(auth.Key, auth.Value)
				u.RawQuery = q.Encode()
				req.URL = u
			} else {
				req.Header.Set(auth.Key, auth.Value)
			}
		}

	case "awssigv4":
		_ = SignAWSSigV4(req, *auth, bodyBytes, time.Now())
	}
}

func (c *DefaultClient) buildHTTPClient(
	settings types.SettingDefinition,
	redirectMu *sync.Mutex,
	redirectChain *[]types.RedirectHop,
) *http.Client {
	// TLS configuration
	tlsConfig := &tls.Config{
		InsecureSkipVerify: !settings.VerifySSL,
	}

	// Client Certificate (mTLS)
	if settings.ClientCertPath != "" && settings.ClientKeyPath != "" {
		if cert, err := tls.LoadX509KeyPair(settings.ClientCertPath, settings.ClientKeyPath); err == nil {
			tlsConfig.Certificates = []tls.Certificate{cert}
		}
	}

	connectTimeout := 10 * time.Second
	if settings.ConnectTimeoutMs > 0 {
		connectTimeout = time.Duration(settings.ConnectTimeoutMs) * time.Millisecond
	}

	dialer := &net.Dialer{
		Timeout:   connectTimeout,
		KeepAlive: 30 * time.Second,
	}

	var proxyFn func(*http.Request) (*url.URL, error) = http.ProxyFromEnvironment
	var dialContextFn func(ctx context.Context, network, addr string) (net.Conn, error) = dialer.DialContext

	// Proxy configuration: HTTP, HTTPS, or SOCKS5
	if strings.TrimSpace(settings.ProxyURL) != "" {
		proxyStr := strings.TrimSpace(settings.ProxyURL)
		if strings.HasPrefix(strings.ToLower(proxyStr), "socks5://") {
			if parsedProxy, err := url.Parse(proxyStr); err == nil {
				var auth *proxy.Auth
				if parsedProxy.User != nil {
					auth = &proxy.Auth{
						User: parsedProxy.User.Username(),
					}
					if pass, ok := parsedProxy.User.Password(); ok {
						auth.Password = pass
					}
				}
				if socksDialer, err := proxy.SOCKS5("tcp", parsedProxy.Host, auth, dialer); err == nil {
					proxyFn = nil
					if cd, ok := socksDialer.(proxy.ContextDialer); ok {
						dialContextFn = cd.DialContext
					} else {
						dialContextFn = func(ctx context.Context, network, addr string) (net.Conn, error) {
							return socksDialer.Dial(network, addr)
						}
					}
				}
			}
		} else {
			if !strings.Contains(proxyStr, "://") {
				proxyStr = "http://" + proxyStr
			}
			if parsedProxy, err := url.Parse(proxyStr); err == nil {
				proxyFn = http.ProxyURL(parsedProxy)
			}
		}
	}

	transport := &http.Transport{
		TLSClientConfig:       tlsConfig,
		Proxy:                 proxyFn,
		DialContext:           dialContextFn,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
	}

	// Cookie Jar integration
	enableCookies := true
	if settings.EnableCookies != nil {
		enableCookies = *settings.EnableCookies
	}
	if enableCookies && c.cookieJar != nil {
		client.Jar = c.cookieJar
	}

	// Redirect policy: limit hops, and strip sensitive headers across origins
	maxRedirects := 10
	if settings.MaxRedirects > 0 {
		maxRedirects = settings.MaxRedirects
	}

	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		// Return early without capturing if we're not following redirects.
		if !settings.FollowRedirects {
			return http.ErrUseLastResponse
		}

		// Capture the hop being followed.
		if redirectMu != nil && redirectChain != nil && len(via) > 0 {
			prev := via[len(via)-1]
			hop := types.RedirectHop{
				Method: prev.Method,
				URL:    prev.URL.String(),
				Headers: map[string]string{
					"Location": req.URL.String(),
				},
			}
			redirectMu.Lock()
			*redirectChain = append(*redirectChain, hop)
			redirectMu.Unlock()
		}

		if len(via) >= maxRedirects {
			return fmt.Errorf("stopped after %d redirects", maxRedirects)
		}
		if len(via) > 0 {
			initial := via[0]
			// Strip sensitive headers on cross-origin redirect.
			if initial.URL.Scheme != req.URL.Scheme || !strings.EqualFold(initial.URL.Host, req.URL.Host) {
				req.Header.Del("Authorization")
				req.Header.Del("Cookie")
				req.Header.Del("Proxy-Authorization")
			}
		}
		return nil
	}

	return client
}
