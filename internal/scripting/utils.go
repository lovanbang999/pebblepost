package scripting

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"
)

// DateModule provides date arithmetic and formatting for scripts.
type DateModule struct{}

// NewDateModule creates a new DateModule.
func NewDateModule() *DateModule {
	return &DateModule{}
}

// Now returns the current Unix timestamp in milliseconds.
func (d *DateModule) Now() int64 {
	return time.Now().UnixMilli()
}

// NowISO returns the current time in RFC3339 format.
func (d *DateModule) NowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// Format formats a date (timestamp in ms, ISO string, or time.Time) using standard or friendly tokens.
func (d *DateModule) Format(dateVal any, formatStr string) (string, error) {
	t, err := parseDateValue(dateVal)
	if err != nil {
		return "", err
	}

	if formatStr == "" {
		return t.Format(time.RFC3339), nil
	}

	// Translate common tokens to Go layout format if needed
	goLayout := translateDateTokens(formatStr)
	return t.Format(goLayout), nil
}

// Add adds a duration of given unit to a date and returns the updated time in RFC3339.
func (d *DateModule) Add(dateVal any, amount int64, unit string) (string, error) {
	t, err := parseDateValue(dateVal)
	if err != nil {
		return "", err
	}

	var dur time.Duration
	u := strings.ToLower(strings.TrimSpace(unit))
	switch u {
	case "ms", "millisecond", "milliseconds":
		dur = time.Duration(amount) * time.Millisecond
	case "s", "sec", "second", "seconds":
		dur = time.Duration(amount) * time.Second
	case "m", "min", "minute", "minutes":
		dur = time.Duration(amount) * time.Minute
	case "h", "hr", "hour", "hours":
		dur = time.Duration(amount) * time.Hour
	case "d", "day", "days":
		dur = time.Duration(amount) * 24 * time.Hour
	default:
		return "", fmt.Errorf("unknown date unit '%s' (supported: ms, s, m, h, d)", unit)
	}

	return t.Add(dur).UTC().Format(time.RFC3339), nil
}

func parseDateValue(v any) (time.Time, error) {
	if v == nil {
		return time.Now().UTC(), nil
	}
	switch val := v.(type) {
	case time.Time:
		return val, nil
	case int64:
		return time.UnixMilli(val).UTC(), nil
	case int:
		return time.UnixMilli(int64(val)).UTC(), nil
	case float64:
		return time.UnixMilli(int64(val)).UTC(), nil
	case string:
		if val == "" || strings.EqualFold(val, "now") {
			return time.Now().UTC(), nil
		}
		// Try RFC3339
		if t, err := time.Parse(time.RFC3339, val); err == nil {
			return t, nil
		}
		// Try RFC3339Nano
		if t, err := time.Parse(time.RFC3339Nano, val); err == nil {
			return t, nil
		}
		// Try common formats
		layouts := []string{
			"2006-01-02 15:04:05",
			"2006-01-02",
			time.RFC1123,
			time.RFC1123Z,
		}
		for _, l := range layouts {
			if t, err := time.Parse(l, val); err == nil {
				return t, nil
			}
		}
		return time.Time{}, fmt.Errorf("unable to parse date string: %s", val)
	default:
		return time.Time{}, fmt.Errorf("unsupported date value type: %T", v)
	}
}

func translateDateTokens(tokenStr string) string {
	// Simple mapping for common JS/Moment date format tokens
	replacements := []struct {
		token  string
		layout string
	}{
		{"YYYY", "2006"},
		{"YY", "06"},
		{"MM", "01"},
		{"DD", "02"},
		{"HH", "15"},
		{"mm", "04"},
		{"ss", "05"},
		{"SSS", "000"},
	}
	res := tokenStr
	for _, r := range replacements {
		res = strings.ReplaceAll(res, r.token, r.layout)
	}
	return res
}

// RandomModule provides pseudo and cryptographic random utilities for testing.
type RandomModule struct{}

// NewRandomModule creates a new RandomModule.
func NewRandomModule() *RandomModule {
	return &RandomModule{}
}

// Int returns a cryptographically secure random integer between min and max (inclusive).
func (r *RandomModule) Int(min, max int64) (int64, error) {
	if min >= max {
		return min, nil
	}
	diff := max - min + 1
	n, err := rand.Int(rand.Reader, big.NewInt(diff))
	if err != nil {
		return 0, err
	}
	return min + n.Int64(), nil
}

const defaultCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// String generates a random string of length n from charset.
func (r *RandomModule) String(length int, charset string) (string, error) {
	if length <= 0 {
		return "", nil
	}
	if charset == "" {
		charset = defaultCharset
	}
	charsetRunes := []rune(charset)
	charsetLen := big.NewInt(int64(len(charsetRunes)))

	res := make([]rune, length)
	for i := 0; i < length; i++ {
		idx, err := rand.Int(rand.Reader, charsetLen)
		if err != nil {
			return "", err
		}
		res[i] = charsetRunes[idx.Int64()]
	}
	return string(res), nil
}

// AuxiliaryClient handles synchronous auxiliary HTTP requests (pb.sendRequest).
type AuxiliaryClient struct {
	client *http.Client
}

// NewAuxiliaryClient creates a new AuxiliaryClient.
func NewAuxiliaryClient() *AuxiliaryClient {
	return &AuxiliaryClient{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SendRequest performs a synchronous auxiliary HTTP call bounded by maxTimeout.
func (ac *AuxiliaryClient) SendRequest(config map[string]any, maxTimeout time.Duration) (map[string]any, error) {
	rawURL, _ := config["url"].(string)
	if strings.TrimSpace(rawURL) == "" {
		return nil, fmt.Errorf("pb.sendRequest requires a valid 'url'")
	}

	method, _ := config["method"].(string)
	if method == "" {
		method = "GET"
	}
	method = strings.ToUpper(method)

	var reqBody io.Reader
	if b, ok := config["body"]; ok && b != nil {
		switch v := b.(type) {
		case string:
			reqBody = strings.NewReader(v)
		case []byte:
			reqBody = strings.NewReader(string(v))
		default:
			// Attempt to JSON encode
			if jsonBytes, err := json.Marshal(v); err == nil {
				reqBody = strings.NewReader(string(jsonBytes))
			}
		}
	}

	timeout := maxTimeout
	if tMs, ok := config["timeoutMs"].(float64); ok && tMs > 0 {
		userTimeout := time.Duration(tMs) * time.Millisecond
		if userTimeout < timeout {
			timeout = userTimeout
		}
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, method, rawURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create auxiliary request: %w", err)
	}

	// Apply headers
	if headers, ok := config["headers"].(map[string]any); ok {
		for k, v := range headers {
			httpReq.Header.Set(k, fmt.Sprintf("%v", v))
		}
	}

	start := time.Now()
	resp, err := ac.client.Do(httpReq)
	duration := time.Since(start)

	if err != nil {
		return nil, fmt.Errorf("auxiliary request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10MB limit
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	bodyStr := string(bodyBytes)

	headersMap := make(map[string]string)
	for k, vals := range resp.Header {
		if len(vals) > 0 {
			headersMap[k] = vals[0]
		}
	}

	return map[string]any{
		"status":     resp.StatusCode,
		"statusText": resp.Status,
		"headers":    headersMap,
		"body":       bodyStr,
		"duration":   float64(duration.Milliseconds()),
		"time":       float64(duration.Milliseconds()),
	}, nil
}
