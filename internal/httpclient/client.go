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
	"net/http"
	"net/http/httptrace"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pebblepost/internal/types"
)

// Client defines the interface for executing HTTP requests.
type Client interface {
	Execute(ctx context.Context, req *types.RequestDefinition) (*types.ExecutionResult, error)
}

// DefaultClient implements the Client interface using Go's net/http.
type DefaultClient struct {
	defaultTimeout time.Duration
}

// NewClient creates a new HTTP Client instance.
func NewClient() *DefaultClient {
	return &DefaultClient{
		defaultTimeout: 30 * time.Second,
	}
}

// Execute performs the HTTP request based on the provided RequestDefinition.
func (c *DefaultClient) Execute(ctx context.Context, req *types.RequestDefinition) (*types.ExecutionResult, error) {
	if req == nil {
		return nil, fmt.Errorf("request definition cannot be nil")
	}

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

	// 2. Prepare Body & Content-Type
	bodyReader, contentType, err := c.buildBody(req.Body)
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
		dnsStart, dnsDone       time.Time
		connStart, connDone     time.Time
		tlsStart, tlsDone       time.Time
		firstByteTime           time.Time
		reqStartTime            = time.Now()
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
	httpReq, err := http.NewRequestWithContext(reqCtx, method, targetURL.String(), bodyReader)
	if err != nil {
		result.Error = fmt.Sprintf("failed to create http request: %v", err)
		return result, err
	}

	// 7. Apply Headers
	c.applyHeaders(httpReq, req.Headers, contentType)

	// 8. Apply Authentication
	c.applyAuth(httpReq, targetURL, req.Auth)

	// 9. Build http.Client with Settings
	httpClient := c.buildHTTPClient(req.Settings)

	// 10. Execute Request
	reqStartTime = time.Now()
	resp, err := httpClient.Do(httpReq)
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

	// Limit response size to 50MB to prevent memory exhaustion
	limitedReader := io.LimitReader(resp.Body, 50*1024*1024)
	respBytes, err := io.ReadAll(limitedReader)
	downloadDone := time.Now()

	if err != nil {
		result.Error = fmt.Sprintf("failed to read response body: %v", err)
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
	result.Body = string(respBytes)
	result.Size = int64(len(respBytes))

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

func (c *DefaultClient) buildBody(body types.BodyDefinition) (io.Reader, string, error) {
	bodyType := strings.ToLower(strings.TrimSpace(body.Type))

	switch bodyType {
	case "", "none":
		return nil, "", nil

	case "json":
		raw := strings.TrimSpace(body.Raw)
		if raw == "" {
			return nil, "application/json", nil
		}
		return strings.NewReader(raw), "application/json", nil

	case "raw":
		return strings.NewReader(body.Raw), "text/plain", nil

	case "urlencoded":
		form := url.Values{}
		for _, kv := range body.UrlEncoded {
			if kv.Enabled && kv.Key != "" {
				form.Add(kv.Key, kv.Value)
			}
		}
		return strings.NewReader(form.Encode()), "application/x-www-form-urlencoded", nil

	case "formdata":
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)

		for _, kv := range body.FormData {
			if !kv.Enabled || kv.Key == "" {
				continue
			}

			if strings.ToLower(kv.Type) == "file" {
				// Handle file upload
				filePath := kv.Value
				if filePath == "" {
					continue
				}

				fileName := filepath.Base(filePath)
				fileData, err := os.ReadFile(filePath)
				if err != nil {
					// Fallback to sending value as raw content if file does not exist on disk
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
				// Normal form text field
				if err := writer.WriteField(kv.Key, kv.Value); err != nil {
					return nil, "", err
				}
			}
		}

		if err := writer.Close(); err != nil {
			return nil, "", err
		}
		return &buf, writer.FormDataContentType(), nil

	case "graphql":
		if body.GraphQL == nil {
			return strings.NewReader("{}"), "application/json", nil
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
		return bytes.NewReader(data), "application/json", nil

	default:
		return strings.NewReader(body.Raw), "text/plain", nil
	}
}

func (c *DefaultClient) applyHeaders(req *http.Request, headers []types.KeyValue, computedContentType string) {
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
		req.Header.Set("User-Agent", "PebblePost/1.0")
	}
	if !hasAccept {
		req.Header.Set("Accept", "*/*")
	}
}

func (c *DefaultClient) applyAuth(req *http.Request, u *url.URL, auth types.AuthDefinition) {
	authType := strings.ToLower(strings.TrimSpace(auth.Type))

	switch authType {
	case "bearer", "oauth2":
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
	}
}

func (c *DefaultClient) buildHTTPClient(settings types.SettingDefinition) *http.Client {
	// TLS config
	tlsConfig := &tls.Config{
		InsecureSkipVerify: !settings.VerifySSL,
	}

	transport := &http.Transport{
		TLSClientConfig:       tlsConfig,
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
	}

	// Redirect policy
	if !settings.FollowRedirects {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	return client
}
