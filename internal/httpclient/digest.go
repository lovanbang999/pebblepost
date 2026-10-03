package httpclient

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// DigestChallenge represents parsed WWW-Authenticate Digest challenge parameters.
type DigestChallenge struct {
	Realm     string
	Nonce     string
	Qop       string
	Algorithm string
	Opaque    string
	Domain    string
}

// ParseDigestChallenge extracts parameters from a WWW-Authenticate header.
func ParseDigestChallenge(header string) (*DigestChallenge, error) {
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(header)), "digest") {
		return nil, fmt.Errorf("not a digest challenge")
	}

	raw := strings.TrimSpace(header)[6:]
	params := make(map[string]string)

	// Tokenize comma-separated key=value pairs, handling quoted strings
	for len(raw) > 0 {
		raw = strings.TrimSpace(raw)
		eqIdx := strings.Index(raw, "=")
		if eqIdx == -1 {
			break
		}

		key := strings.ToLower(strings.TrimSpace(raw[:eqIdx]))
		raw = strings.TrimSpace(raw[eqIdx+1:])

		var val string
		if strings.HasPrefix(raw, "\"") {
			raw = raw[1:]
			endQuote := strings.Index(raw, "\"")
			if endQuote == -1 {
				val = raw
				raw = ""
			} else {
				val = raw[:endQuote]
				raw = raw[endQuote+1:]
				if comma := strings.Index(raw, ","); comma != -1 {
					raw = raw[comma+1:]
				} else {
					raw = ""
				}
			}
		} else {
			comma := strings.Index(raw, ",")
			if comma == -1 {
				val = strings.TrimSpace(raw)
				raw = ""
			} else {
				val = strings.TrimSpace(raw[:comma])
				raw = raw[comma+1:]
			}
		}
		params[key] = val
	}

	challenge := &DigestChallenge{
		Realm:     params["realm"],
		Nonce:     params["nonce"],
		Qop:       params["qop"],
		Algorithm: params["algorithm"],
		Opaque:    params["opaque"],
		Domain:    params["domain"],
	}

	if challenge.Algorithm == "" {
		challenge.Algorithm = "MD5"
	}

	return challenge, nil
}

// BuildDigestAuthorization creates the Authorization header value for Digest authentication.
func BuildDigestAuthorization(
	method string,
	uri string,
	username string,
	password string,
	challenge *DigestChallenge,
	bodyBytes []byte,
) (string, error) {
	algorithm := strings.ToUpper(strings.TrimSpace(challenge.Algorithm))
	if algorithm == "" {
		algorithm = "MD5"
	}

	hashFn := md5Hash
	if strings.HasPrefix(algorithm, "SHA-256") {
		hashFn = sha256Hash
	}

	// 1. Calculate HA1 = H(username:realm:password)
	ha1 := hashFn(fmt.Sprintf("%s:%s:%s", username, challenge.Realm, password))

	// 2. Client nonce (cnonce) and Nonce Count (nc)
	cnonceBytes := make([]byte, 8)
	_, _ = io.ReadFull(rand.Reader, cnonceBytes)
	cnonce := hex.EncodeToString(cnonceBytes)
	nc := "00000001"

	// 3. Determine QOP mode
	qop := ""
	if challenge.Qop != "" {
		qopList := strings.Split(challenge.Qop, ",")
		for _, q := range qopList {
			qTrim := strings.ToLower(strings.TrimSpace(q))
			if qTrim == "auth" {
				qop = "auth"
				break
			}
			if qTrim == "auth-int" && qop == "" {
				qop = "auth-int"
			}
		}
		if qop == "" && len(qopList) > 0 {
			qop = strings.ToLower(strings.TrimSpace(qopList[0]))
		}
	}

	// 4. Calculate HA2
	var ha2 string
	if qop == "auth-int" {
		bodyHash := hashFn(string(bodyBytes))
		ha2 = hashFn(fmt.Sprintf("%s:%s:%s", method, uri, bodyHash))
	} else {
		ha2 = hashFn(fmt.Sprintf("%s:%s", method, uri))
	}

	// 5. Calculate Response
	var response string
	if qop == "auth" || qop == "auth-int" {
		response = hashFn(fmt.Sprintf("%s:%s:%s:%s:%s:%s", ha1, challenge.Nonce, nc, cnonce, qop, ha2))
	} else {
		response = hashFn(fmt.Sprintf("%s:%s:%s", ha1, challenge.Nonce, ha2))
	}

	// 6. Format header string
	headerParts := []string{
		fmt.Sprintf(`Digest username="%s"`, escapeQuotes(username)),
		fmt.Sprintf(`realm="%s"`, escapeQuotes(challenge.Realm)),
		fmt.Sprintf(`nonce="%s"`, escapeQuotes(challenge.Nonce)),
		fmt.Sprintf(`uri="%s"`, escapeQuotes(uri)),
		fmt.Sprintf(`response="%s"`, response),
		fmt.Sprintf(`algorithm="%s"`, algorithm),
	}

	if challenge.Opaque != "" {
		headerParts = append(headerParts, fmt.Sprintf(`opaque="%s"`, escapeQuotes(challenge.Opaque)))
	}
	if qop != "" {
		headerParts = append(headerParts, fmt.Sprintf(`qop=%s`, qop))
		headerParts = append(headerParts, fmt.Sprintf(`nc=%s`, nc))
		headerParts = append(headerParts, fmt.Sprintf(`cnonce="%s"`, cnonce))
	}

	return strings.Join(headerParts, ", "), nil
}

func md5Hash(text string) string {
	hasher := md5.New()
	_, _ = hasher.Write([]byte(text))
	return hex.EncodeToString(hasher.Sum(nil))
}

func sha256Hash(text string) string {
	hasher := sha256.New()
	_, _ = hasher.Write([]byte(text))
	return hex.EncodeToString(hasher.Sum(nil))
}

func escapeQuotes(val string) string {
	return strings.ReplaceAll(val, `"`, `\"`)
}

// IsDigestChallenge checks if a response asks for digest authentication.
func IsDigestChallenge(resp *http.Response) bool {
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		return false
	}
	authHeader := resp.Header.Get("WWW-Authenticate")
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(authHeader)), "digest")
}
