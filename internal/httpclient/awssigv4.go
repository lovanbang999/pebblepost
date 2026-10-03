package httpclient

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"pebblepost/internal/types"
)

// SignAWSSigV4 applies AWS Signature v4 headers to the given HTTP request.
func SignAWSSigV4(req *http.Request, auth types.AuthDefinition, bodyBytes []byte, t time.Time) error {
	accessKey := strings.TrimSpace(auth.AccessKey)
	secretKey := strings.TrimSpace(auth.SecretKey)
	region := strings.TrimSpace(auth.Region)
	service := strings.TrimSpace(auth.Service)
	sessionToken := strings.TrimSpace(auth.SessionToken)

	if accessKey == "" || secretKey == "" {
		return fmt.Errorf("AWS access key and secret key are required")
	}
	if region == "" {
		region = "us-east-1"
	}
	if service == "" {
		service = "execute-api"
	}

	dateISO := t.UTC().Format("20060102T150405Z")
	dateShort := t.UTC().Format("20060102")

	// Set required AWS headers
	req.Header.Set("X-Amz-Date", dateISO)
	if sessionToken != "" {
		req.Header.Set("X-Amz-Security-Token", sessionToken)
	}

	// 1. Calculate Payload Hash
	payloadHash := sha256Hex(bodyBytes)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	// 2. Canonical URI
	canonicalURI := req.URL.EscapedPath()
	if canonicalURI == "" {
		canonicalURI = "/"
	}

	// 3. Canonical Query String
	canonicalQuery := buildCanonicalQuery(req.URL.Query())

	// 4. Canonical Headers & Signed Headers
	canonicalHeaders, signedHeaders := buildCanonicalHeaders(req)

	// 5. Canonical Request
	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalURI,
		canonicalQuery,
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	// 6. String to Sign
	credentialScope := fmt.Sprintf("%s/%s/%s/aws4_request", dateShort, region, service)
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		dateISO,
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	// 7. Calculate Signature Key & Signature
	signingKey := getSignatureKey(secretKey, dateShort, region, service)
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	// 8. Construct Authorization Header
	authHeader := fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKey,
		credentialScope,
		signedHeaders,
		signature,
	)

	req.Header.Set("Authorization", authHeader)
	return nil
}

func buildCanonicalQuery(query url.Values) string {
	if len(query) == 0 {
		return ""
	}

	keys := make([]string, 0, len(query))
	for k := range query {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		escapedKey := url.QueryEscape(k)
		vals := query[k]
		sort.Strings(vals)
		for _, v := range vals {
			escapedVal := url.QueryEscape(v)
			parts = append(parts, fmt.Sprintf("%s=%s", escapedKey, escapedVal))
		}
	}
	return strings.Join(parts, "&")
}

func buildCanonicalHeaders(req *http.Request) (string, string) {
	// Standardize headers
	headers := make(map[string]string)
	headers["host"] = req.URL.Host

	for k, v := range req.Header {
		lowerKey := strings.ToLower(strings.TrimSpace(k))
		if lowerKey == "host" || strings.HasPrefix(lowerKey, "x-amz-") || lowerKey == "content-type" {
			headers[lowerKey] = strings.TrimSpace(strings.Join(v, ","))
		}
	}

	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var canonicalLines []string
	for _, k := range keys {
		canonicalLines = append(canonicalLines, fmt.Sprintf("%s:%s", k, headers[k]))
	}
	canonicalHeaders := strings.Join(canonicalLines, "\n") + "\n"
	signedHeaders := strings.Join(keys, ";")

	return canonicalHeaders, signedHeaders
}

func getSignatureKey(secretKey, date, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secretKey), []byte(date))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))
	return kSigning
}

func hmacSHA256(key []byte, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
