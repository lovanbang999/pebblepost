package autoupdate

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultRepository information
const (
	DefaultRepoOwner = "lovanbang999"
	DefaultRepoName  = "pebblepost"
	DefaultAPIBase   = "https://api.github.com"
)

// Service manages update checks, settings, and signature verifications.
type Service struct {
	mu                sync.RWMutex
	currentVersion    string
	repoOwner         string
	repoName          string
	apiBaseURL        string
	publicKey         ed25519.PublicKey
	settingsPath      string
	settings          UpdateSettings
	httpClient        *http.Client
	allowInsecureHTTP bool
}

// Option configures Service.
type Option func(*Service)

// WithAllowInsecureHTTP allows HTTP download URLs (intended only for local tests).
func WithAllowInsecureHTTP(allow bool) Option {
	return func(s *Service) {
		s.allowInsecureHTTP = allow
	}
}

// WithAPIBase sets a custom GitHub API base URL (useful for testing).
func WithAPIBase(url string) Option {
	return func(s *Service) {
		s.apiBaseURL = strings.TrimRight(url, "/")
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(s *Service) {
		s.httpClient = client
	}
}

// WithRepo sets the repository owner and name.
func WithRepo(owner, name string) Option {
	return func(s *Service) {
		s.repoOwner = owner
		s.repoName = name
	}
}

// WithPublicKey sets the trusted Ed25519/Minisign public key for verification.
func WithPublicKey(pubKey ed25519.PublicKey) Option {
	return func(s *Service) {
		s.publicKey = pubKey
	}
}

// NewService instantiates an autoupdate service.
func NewService(currentVersion, dataDir string, opts ...Option) *Service {
	s := &Service{
		currentVersion: currentVersion,
		repoOwner:      DefaultRepoOwner,
		repoName:       DefaultRepoName,
		apiBaseURL:     DefaultAPIBase,
		settingsPath:   filepath.Join(dataDir, "update-settings.json"),
		settings: UpdateSettings{
			CheckOnStartup: true,
			Channel:        "stable",
		},
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	for _, opt := range opts {
		opt(s)
	}

	s.loadSettings()
	return s
}

func (s *Service) loadSettings() {
	if s.settingsPath == "" {
		return
	}
	data, err := os.ReadFile(s.settingsPath)
	if err != nil {
		return
	}
	var loaded UpdateSettings
	if err := json.Unmarshal(data, &loaded); err == nil {
		s.settings = loaded
	}
}

// GetSettings returns current update settings.
func (s *Service) GetSettings() UpdateSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings
}

// UpdateSettings updates user preferences.
func (s *Service) UpdateSettings(newSettings UpdateSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.settings = newSettings
	if s.settingsPath == "" {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(s.settingsPath), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(newSettings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.settingsPath, data, 0644)
}

// CheckForUpdate queries GitHub Releases to determine if a newer version is available.
func (s *Service) CheckForUpdate(ctx context.Context, force bool) (*UpdateInfo, error) {
	s.mu.RLock()
	settings := s.settings
	currentVer := s.currentVersion
	owner := s.repoOwner
	repo := s.repoName
	apiBase := s.apiBaseURL
	s.mu.RUnlock()

	if !settings.CheckOnStartup && !force {
		return &UpdateInfo{
			CurrentVersion: currentVer,
			LatestVersion:  currentVer,
			HasUpdate:      false,
		}, nil
	}

	reqURL := fmt.Sprintf("%s/repos/%s/%s/releases/latest", apiBase, owner, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create update check request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "PebblePost-Updater/"+currentVer)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query GitHub releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// No releases published yet
		return &UpdateInfo{
			CurrentVersion: currentVer,
			LatestVersion:  currentVer,
			HasUpdate:      false,
		}, nil
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("GitHub API returned status %d: %s", resp.StatusCode, string(body))
	}

	var ghRelease GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&ghRelease); err != nil {
		return nil, fmt.Errorf("failed to decode GitHub release response: %w", err)
	}

	latestVer := strings.TrimPrefix(ghRelease.TagName, "v")
	cleanCurrent := strings.TrimPrefix(currentVer, "v")
	hasUpdate := CompareSemver(latestVer, cleanCurrent) > 0

	info := &UpdateInfo{
		CurrentVersion: currentVer,
		LatestVersion:  latestVer,
		HasUpdate:      hasUpdate,
		ReleaseNotes:   ghRelease.Body,
		ReleaseURL:     ghRelease.HTMLURL,
		PublishedAt:    ghRelease.PublishedAt,
	}

	// Match platform binary asset and signature
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	for _, asset := range ghRelease.Assets {
		name := strings.ToLower(asset.Name)
		if strings.HasSuffix(name, ".minisig") || strings.HasSuffix(name, ".sig") {
			info.SignatureURL = asset.BrowserDownloadURL
		}

		// Detect platform-matching asset
		if strings.Contains(name, goos) && (strings.Contains(name, goarch) || strings.Contains(name, "universal") || strings.Contains(name, "all")) {
			if info.AssetURL == "" || (!strings.HasSuffix(name, ".sig") && !strings.HasSuffix(name, ".minisig")) {
				info.AssetURL = asset.BrowserDownloadURL
			}
		}
	}

	return info, nil
}

// CompareSemver compares semver strings v1 and v2.
// Returns 1 if v1 > v2, -1 if v1 < v2, and 0 if v1 == v2.
func CompareSemver(v1, v2 string) int {
	parts1 := parseSemverParts(v1)
	parts2 := parseSemverParts(v2)

	for i := 0; i < 3; i++ {
		if parts1[i] > parts2[i] {
			return 1
		}
		if parts1[i] < parts2[i] {
			return -1
		}
	}
	return 0
}

func parseSemverParts(v string) [3]int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	// Strip metadata / prerelease suffix
	if idx := strings.IndexAny(v, "-+"); idx != -1 {
		v = v[:idx]
	}
	segments := strings.Split(v, ".")
	var parts [3]int
	for i := 0; i < len(segments) && i < 3; i++ {
		if val, err := strconv.Atoi(segments[i]); err == nil {
			parts[i] = val
		}
	}
	return parts
}

// ExtractSignature extracts 64 raw Ed25519 signature bytes from raw bytes, base64,
// or a Minisign .minisig file format.
func ExtractSignature(sigData []byte) ([]byte, error) {
	sigStr := strings.TrimSpace(string(sigData))
	lines := strings.Split(sigStr, "\n")

	// Minisign formatted signature file
	if len(lines) >= 2 && strings.HasPrefix(strings.ToLower(strings.TrimSpace(lines[0])), "untrusted comment") {
		rawB64 := strings.TrimSpace(lines[1])
		decoded, err := base64.StdEncoding.DecodeString(rawB64)
		if err != nil {
			return nil, fmt.Errorf("failed to decode minisign base64 signature: %w", err)
		}
		// Minisign format: 2 bytes header ("Ed") + 8 bytes key ID + 64 bytes signature = 74 bytes
		if len(decoded) == 74 {
			return decoded[10:74], nil
		}
		if len(decoded) == 64 {
			return decoded, nil
		}
		return nil, fmt.Errorf("invalid minisign signature payload length: %d", len(decoded))
	}

	// Raw base64 string
	if decoded, err := base64.StdEncoding.DecodeString(sigStr); err == nil && len(decoded) == 64 {
		return decoded, nil
	}

	if len(sigData) == 64 {
		return sigData, nil
	}

	return nil, errors.New("unrecognized signature format")
}

// ParsePublicKey parses an Ed25519 public key from 32 raw bytes, base64 string,
// or Minisign public key format (with untrusted comment).
func ParsePublicKey(keyData []byte) (ed25519.PublicKey, error) {
	keyStr := strings.TrimSpace(string(keyData))
	lines := strings.Split(keyStr, "\n")
	if len(lines) >= 2 && strings.HasPrefix(strings.ToLower(strings.TrimSpace(lines[0])), "untrusted comment") {
		keyStr = strings.TrimSpace(lines[1])
	}

	if decoded, err := base64.StdEncoding.DecodeString(keyStr); err == nil {
		if len(decoded) == 32 {
			return ed25519.PublicKey(decoded), nil
		}
		// Minisign format: 2 bytes header ("Ed") + 8 bytes key ID + 32 bytes pubkey = 42 bytes
		if len(decoded) == 42 {
			return ed25519.PublicKey(decoded[10:42]), nil
		}
	}

	if len(keyData) == 32 {
		return ed25519.PublicKey(keyData), nil
	}

	return nil, errors.New("invalid public key format: expected 32-byte Ed25519 key or Minisign public key")
}

// VerifySignature verifies message data against signature using public key.
func VerifySignature(message []byte, signatureData []byte, pubKey ed25519.PublicKey) bool {
	sig, err := ExtractSignature(signatureData)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return false
	}
	if len(pubKey) != ed25519.PublicKeySize {
		return false
	}
	return ed25519.Verify(pubKey, message, sig)
}

// ApplyUpdate downloads the binary and signature, verifies the signature against the trusted public key,
// checks semver to prevent downgrades, enforces HTTPS-only downloads, ensures secure temporary file permissions (0600),
// and replaces the binary at targetBinaryPath only if all verification checks pass.
func (s *Service) ApplyUpdate(ctx context.Context, targetBinaryPath string, info *UpdateInfo) error {
	if info == nil {
		return errors.New("update info cannot be nil")
	}

	// 1. Refuse downgrade or same-version reinstall
	if CompareSemver(info.LatestVersion, s.currentVersion) <= 0 {
		return fmt.Errorf("refusing to downgrade or reinstall version %s when current version is %s", info.LatestVersion, s.currentVersion)
	}

	// 2. Enforce HTTPS only (unless explicitly allowed in tests)
	if !s.allowInsecureHTTP {
		if !strings.HasPrefix(strings.ToLower(info.AssetURL), "https://") {
			return fmt.Errorf("insecure asset URL: HTTPS is required (got %s)", info.AssetURL)
		}
		if !strings.HasPrefix(strings.ToLower(info.SignatureURL), "https://") {
			return fmt.Errorf("insecure signature URL: HTTPS is required (got %s)", info.SignatureURL)
		}
	}

	// 3. Verify signature presence and public key configuration
	if strings.TrimSpace(info.SignatureURL) == "" {
		return errors.New("missing signature URL: update artifact cannot be verified")
	}
	s.mu.RLock()
	pubKey := s.publicKey
	s.mu.RUnlock()
	if len(pubKey) == 0 {
		return errors.New("trusted public key not configured: cannot verify update signature")
	}

	// 4. Download signature
	sigReq, err := http.NewRequestWithContext(ctx, http.MethodGet, info.SignatureURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create signature request: %w", err)
	}
	sigResp, err := s.httpClient.Do(sigReq)
	if err != nil {
		return fmt.Errorf("failed to download signature: %w", err)
	}
	defer sigResp.Body.Close()
	if sigResp.StatusCode != http.StatusOK {
		return fmt.Errorf("signature download failed with status %d", sigResp.StatusCode)
	}
	sigBytes, err := io.ReadAll(io.LimitReader(sigResp.Body, 64*1024))
	if err != nil {
		return fmt.Errorf("failed to read signature: %w", err)
	}

	// 5. Download asset to temporary file with secure 0600 permissions in 0700 dir
	tempDir, err := os.MkdirTemp("", "pebblepost-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	_ = os.Chmod(tempDir, 0700)
	defer os.RemoveAll(tempDir)

	tempFile := filepath.Join(tempDir, "update-binary.tmp")
	f, err := os.OpenFile(tempFile, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("failed to create secure temp file: %w", err)
	}

	assetReq, err := http.NewRequestWithContext(ctx, http.MethodGet, info.AssetURL, nil)
	if err != nil {
		f.Close()
		return fmt.Errorf("failed to create asset request: %w", err)
	}
	assetResp, err := s.httpClient.Do(assetReq)
	if err != nil {
		f.Close()
		return fmt.Errorf("failed to download asset: %w", err)
	}
	defer assetResp.Body.Close()
	if assetResp.StatusCode != http.StatusOK {
		f.Close()
		return fmt.Errorf("asset download failed with status %d", assetResp.StatusCode)
	}

	written, err := io.Copy(f, assetResp.Body)
	f.Close()
	if err != nil {
		return fmt.Errorf("failed to write update asset: %w", err)
	}
	if written == 0 {
		return errors.New("downloaded update asset is empty")
	}

	assetBytes, err := os.ReadFile(tempFile)
	if err != nil {
		return fmt.Errorf("failed to read downloaded update asset: %w", err)
	}

	// 6. Signature verification gate: MUST PASS before touching target binary
	if !VerifySignature(assetBytes, sigBytes, pubKey) {
		return errors.New("update signature verification failed: tampered artifact or invalid signature")
	}

	// 7. Make binary executable (0755) and replace targetBinaryPath
	if err := os.Chmod(tempFile, 0755); err != nil {
		return fmt.Errorf("failed to set executable permissions: %w", err)
	}

	if targetBinaryPath == "" {
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("failed to resolve current executable path: %w", err)
		}
		targetBinaryPath = exe
	}

	if err := os.MkdirAll(filepath.Dir(targetBinaryPath), 0755); err != nil {
		return fmt.Errorf("failed to create target binary parent directory: %w", err)
	}

	if err := os.Rename(tempFile, targetBinaryPath); err != nil {
		// Fallback for cross-device moves
		in, errCopy := os.Open(tempFile)
		if errCopy != nil {
			return fmt.Errorf("failed to replace binary: %w", errCopy)
		}
		defer in.Close()
		out, errCreate := os.OpenFile(targetBinaryPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
		if errCreate != nil {
			return fmt.Errorf("failed to replace binary: %w", errCreate)
		}
		if _, errCopyData := io.Copy(out, in); errCopyData != nil {
			out.Close()
			return fmt.Errorf("failed to copy binary: %w", errCopyData)
		}
		out.Close()
	}

	return nil
}
