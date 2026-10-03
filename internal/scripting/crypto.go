package scripting

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"strings"
)

// CryptoModule provides cryptographic utilities to the JavaScript sandbox.
type CryptoModule struct{}

// NewCryptoModule creates a new CryptoModule.
func NewCryptoModule() *CryptoModule {
	return &CryptoModule{}
}

// MD5 computes MD5 hash of input string and returns hex string.
func (c *CryptoModule) MD5(input string) string {
	hasher := md5.New()
	hasher.Write([]byte(input))
	return hex.EncodeToString(hasher.Sum(nil))
}

// SHA256 computes SHA256 hash of input string and returns hex string.
func (c *CryptoModule) SHA256(input string) string {
	hasher := sha256.New()
	hasher.Write([]byte(input))
	return hex.EncodeToString(hasher.Sum(nil))
}

// SHA512 computes SHA512 hash of input string and returns hex string.
func (c *CryptoModule) SHA512(input string) string {
	hasher := sha512.New()
	hasher.Write([]byte(input))
	return hex.EncodeToString(hasher.Sum(nil))
}

// HMAC computes HMAC with the specified algorithm ("sha256", "sha512", "md5"), secret, and message.
func (c *CryptoModule) HMAC(algo, secret, message string) (string, error) {
	var h func() hash.Hash
	switch strings.ToLower(algo) {
	case "sha256":
		h = sha256.New
	case "sha512":
		h = sha512.New
	case "md5":
		h = md5.New
	default:
		return "", fmt.Errorf("unsupported HMAC algorithm: %s", algo)
	}

	mac := hmac.New(h, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// UUID generates a random RFC 4122 v4 UUID.
func (c *CryptoModule) UUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // Variant RFC4122
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// Base64Encode encodes a string into standard base64 format.
func (c *CryptoModule) Base64Encode(input string) string {
	return base64.StdEncoding.EncodeToString([]byte(input))
}

// Base64Decode decodes a base64 encoded string.
func (c *CryptoModule) Base64Decode(input string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(input)
	if err != nil {
		// Try URL unpadded
		decoded, err = base64.RawURLEncoding.DecodeString(input)
		if err != nil {
			return "", err
		}
	}
	return string(decoded), nil
}

// JWTDecode parses unverified header and payload claims from a JWT string.
func (c *CryptoModule) JWTDecode(token string) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid JWT: expected at least 2 dot-separated segments")
	}

	decodeSegment := func(seg string) (any, error) {
		seg = strings.TrimSpace(seg)
		decoded, err := base64.RawURLEncoding.DecodeString(seg)
		if err != nil {
			decoded, err = base64.URLEncoding.DecodeString(seg)
			if err != nil {
				decoded, err = base64.StdEncoding.DecodeString(seg)
				if err != nil {
					return nil, fmt.Errorf("failed to base64-decode segment: %w", err)
				}
			}
		}

		var parsed any
		if err := json.Unmarshal(decoded, &parsed); err != nil {
			return string(decoded), nil
		}
		return parsed, nil
	}

	header, err := decodeSegment(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid JWT header: %w", err)
	}

	payload, err := decodeSegment(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid JWT payload: %w", err)
	}

	return map[string]any{
		"header":  header,
		"payload": payload,
	}, nil
}
