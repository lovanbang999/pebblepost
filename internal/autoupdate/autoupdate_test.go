package autoupdate

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCompareSemver(t *testing.T) {
	tests := []struct {
		v1       string
		v2       string
		expected int
	}{
		{"0.2.1", "0.2.0", 1},
		{"0.2.0", "0.2.1", -1},
		{"v0.2.0", "0.2.0", 0},
		{"v1.0.0", "0.9.9", 1},
		{"1.0.0", "1.0.0-rc1", 0},
		{"0.1.9", "0.2.0", -1},
		{"0.2.0", "0.2.0", 0},
		{"v2.1.3", "v2.1.3", 0},
		{"2.2.0", "2.1.9", 1},
	}

	for _, tt := range tests {
		got := CompareSemver(tt.v1, tt.v2)
		if got != tt.expected {
			t.Errorf("CompareSemver(%q, %q) = %d; want %d", tt.v1, tt.v2, got, tt.expected)
		}
	}
}

func TestSignatureVerification(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ed25519 key: %v", err)
	}

	message := []byte("PebblePost binary content for sha256 checksums")
	sig := ed25519.Sign(priv, message)

	// Test 1: Direct 64-byte signature
	if !VerifySignature(message, sig, pub) {
		t.Errorf("expected direct signature verification to pass")
	}

	// Test 2: Base64-encoded signature
	b64Sig := base64.StdEncoding.EncodeToString(sig)
	if !VerifySignature(message, []byte(b64Sig), pub) {
		t.Errorf("expected base64 signature verification to pass")
	}

	// Test 3: Minisign format signature
	minisignSigHeader := []byte{0x45, 0x64}         // "Ed"
	keyID := []byte{1, 2, 3, 4, 5, 6, 7, 8}         // 8-byte key id
	combined := append(minisignSigHeader, keyID...) // 10 bytes
	combined = append(combined, sig...)             // 74 bytes total
	minisignB64 := base64.StdEncoding.EncodeToString(combined)
	minisignFormatted := fmt.Sprintf("untrusted comment: signature from test key\n%s\ntrusted comment: timestamp:123\n%s\n",
		minisignB64,
		base64.StdEncoding.EncodeToString(sig),
	)

	if !VerifySignature(message, []byte(minisignFormatted), pub) {
		t.Errorf("expected minisign formatted signature verification to pass")
	}

	// Test 4: Tampered message
	tampered := []byte("Tampered binary payload")
	if VerifySignature(tampered, sig, pub) {
		t.Errorf("expected tampered message verification to fail")
	}

	// Test 5: Wrong public key
	pub2, _, _ := ed25519.GenerateKey(rand.Reader)
	if VerifySignature(message, sig, pub2) {
		t.Errorf("expected verification with wrong public key to fail")
	}
}

func TestCheckForUpdate(t *testing.T) {
	// Mock GitHub API server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/testowner/testrepo/releases/latest" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(GitHubRelease{
				TagName:     "v0.3.0",
				Name:        "Release v0.3.0",
				Body:        "- Added new features\n- Security hardening",
				HTMLURL:     "https://github.com/testowner/testrepo/releases/tag/v0.3.0",
				PublishedAt: time.Now(),
				Assets: []GitHubAsset{
					{
						Name:               "pebblepost_0.3.0_linux_amd64.tar.gz",
						BrowserDownloadURL: "https://github.com/downloads/pebblepost_linux_amd64.tar.gz",
						Size:               15000000,
					},
					{
						Name:               "checksums.txt.minisig",
						BrowserDownloadURL: "https://github.com/downloads/checksums.txt.minisig",
						Size:               256,
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer mockServer.Close()

	tempDir := t.TempDir()
	svc := NewService("0.2.0", tempDir,
		WithAPIBase(mockServer.URL),
		WithRepo("testowner", "testrepo"),
	)

	// Check when update is available
	info, err := svc.CheckForUpdate(context.Background(), true)
	if err != nil {
		t.Fatalf("CheckForUpdate failed: %v", err)
	}

	if !info.HasUpdate {
		t.Errorf("expected HasUpdate = true, got false")
	}
	if info.LatestVersion != "0.3.0" {
		t.Errorf("expected LatestVersion = 0.3.0, got %s", info.LatestVersion)
	}
	if info.SignatureURL != "https://github.com/downloads/checksums.txt.minisig" {
		t.Errorf("unexpected signature url: %s", info.SignatureURL)
	}

	// When current version is equal or newer
	svcEqual := NewService("0.3.0", tempDir,
		WithAPIBase(mockServer.URL),
		WithRepo("testowner", "testrepo"),
	)
	infoEqual, err := svcEqual.CheckForUpdate(context.Background(), true)
	if err != nil {
		t.Fatalf("CheckForUpdate equal failed: %v", err)
	}
	if infoEqual.HasUpdate {
		t.Errorf("expected HasUpdate = false for identical version")
	}

	// When checkOnStartup is disabled and force is false
	_ = svc.UpdateSettings(UpdateSettings{CheckOnStartup: false, Channel: "stable"})
	infoDisabled, err := svc.CheckForUpdate(context.Background(), false)
	if err != nil {
		t.Fatalf("CheckForUpdate disabled failed: %v", err)
	}
	if infoDisabled.HasUpdate {
		t.Errorf("expected disabled auto-check to return HasUpdate = false")
	}
}

func TestHandlerEndpoints(t *testing.T) {
	tempDir := t.TempDir()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519 key gen failed: %v", err)
	}

	svc := NewService("0.2.0", tempDir, WithPublicKey(pub))
	handler := NewHandler(svc)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// 1. GET /api/update/settings
	req := httptest.NewRequest(http.MethodGet, "/api/update/settings", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("GET /api/update/settings status = %d; want %d", rr.Code, http.StatusOK)
	}

	// 2. POST /api/update/settings
	newSettingsPayload := `{"checkOnStartup": false, "channel": "stable"}`
	req = httptest.NewRequest(http.MethodPost, "/api/update/settings", bytes.NewBufferString(newSettingsPayload))
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("POST /api/update/settings status = %d; want %d", rr.Code, http.StatusOK)
	}
	if svc.GetSettings().CheckOnStartup != false {
		t.Errorf("expected checkOnStartup to be false")
	}

	// 3. POST /api/update/verify
	msg := "test artifact payload"
	sig := ed25519.Sign(priv, []byte(msg))
	verifyPayload, _ := json.Marshal(map[string]string{
		"message":   msg,
		"signature": base64.StdEncoding.EncodeToString(sig),
	})

	req = httptest.NewRequest(http.MethodPost, "/api/update/verify", bytes.NewBuffer(verifyPayload))
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("POST /api/update/verify status = %d; want %d", rr.Code, http.StatusOK)
	}

	var verifyResp struct {
		Valid bool `json:"valid"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&verifyResp); err != nil {
		t.Fatalf("failed to decode verify response: %v", err)
	}
	if !verifyResp.Valid {
		t.Errorf("expected valid signature verification")
	}

	// 4. GET /api/update/check
	req = httptest.NewRequest(http.MethodGet, "/api/update/check?force=true", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	// Server returns 200 or 502 depending on network, both verify handler path
	if rr.Code != http.StatusOK && rr.Code != http.StatusBadGateway {
		t.Errorf("GET /api/update/check status = %d", rr.Code)
	}
}

func TestService_OptionsAndParseKey(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	// Test ParsePublicKey with raw and hex
	parsed, err := ParsePublicKey(pub)
	if err != nil || len(parsed) != ed25519.PublicKeySize {
		t.Fatalf("ParsePublicKey raw failed: %v", err)
	}

	b64Key := base64.StdEncoding.EncodeToString(pub)
	parsedB64, err := ParsePublicKey([]byte(b64Key))
	if err != nil || len(parsedB64) != ed25519.PublicKeySize {
		t.Fatalf("ParsePublicKey b64 failed: %v", err)
	}

	// Test WithHTTPClient
	customClient := &http.Client{}
	svc := NewService("1.0.0", t.TempDir(), WithHTTPClient(customClient))
	if svc.httpClient != customClient {
		t.Errorf("expected custom HTTP client")
	}
}
