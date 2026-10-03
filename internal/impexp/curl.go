package impexp

import (
	"fmt"
	"strings"
	"unicode"

	"pebblepost/internal/types"
)

// ParseCURL parses a cURL command string into an ImportResult with a single RequestDefinition.
func ParseCURL(curlStr string) (*ImportResult, error) {
	curlStr = strings.TrimSpace(curlStr)
	if !strings.HasPrefix(curlStr, "curl") && !strings.HasPrefix(curlStr, "curl.exe") {
		return nil, fmt.Errorf("input must start with 'curl'")
	}

	tokens, err := tokenizeCURL(curlStr)
	if err != nil {
		return nil, fmt.Errorf("failed to tokenize cURL: %w", err)
	}

	req := &types.RequestDefinition{
		Method: "GET",
		Auth:   types.AuthDefinition{Type: "none"},
		Body:   types.BodyDefinition{Type: "none"},
		Settings: types.SettingDefinition{
			FollowRedirects: true,
			VerifySSL:       true,
			TimeoutMs:       30000,
		},
	}

	i := 1 // skip "curl"
	for i < len(tokens) {
		tok := tokens[i]
		switch tok {
		case "-X", "--request":
			if i+1 < len(tokens) {
				req.Method = strings.ToUpper(tokens[i+1])
				i += 2
			} else {
				i++
			}
		case "-H", "--header":
			if i+1 < len(tokens) {
				raw := tokens[i+1]
				if colonIdx := strings.Index(raw, ":"); colonIdx != -1 {
					key := strings.TrimSpace(raw[:colonIdx])
					val := strings.TrimSpace(raw[colonIdx+1:])
					if strings.EqualFold(key, "Authorization") {
						parts := strings.SplitN(val, " ", 2)
						if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
							req.Auth = types.AuthDefinition{Type: "bearer", Token: parts[1]}
						} else if len(parts) == 2 && strings.EqualFold(parts[0], "Basic") {
							req.Auth = types.AuthDefinition{Type: "basic", Token: parts[1]}
						} else {
							req.Headers = append(req.Headers, types.KeyValue{Key: key, Value: val, Enabled: true})
						}
					} else {
						req.Headers = append(req.Headers, types.KeyValue{Key: key, Value: val, Enabled: true})
					}
				}
				i += 2
			} else {
				i++
			}
		case "-d", "--data", "--data-raw", "--data-binary":
			if i+1 < len(tokens) {
				body := tokens[i+1]
				body = strings.TrimPrefix(body, "$'")
				body = strings.TrimSuffix(body, "'")
				trimmed := strings.TrimSpace(body)
				if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
					req.Body = types.BodyDefinition{Type: "json", Raw: body}
				} else {
					req.Body = types.BodyDefinition{Type: "raw", Raw: body}
				}
				if req.Method == "GET" {
					req.Method = "POST"
				}
				i += 2
			} else {
				i++
			}
		case "--form", "-F":
			if i+1 < len(tokens) {
				raw := tokens[i+1]
				if eqIdx := strings.Index(raw, "="); eqIdx != -1 {
					k := raw[:eqIdx]
					v := raw[eqIdx+1:]
					if req.Body.Type != "formData" {
						req.Body = types.BodyDefinition{Type: "formData", FormData: []types.KeyValue{}}
					}
					t := "text"
					if strings.HasPrefix(v, "@") {
						t = "file"
					}
					req.Body.FormData = append(req.Body.FormData, types.KeyValue{Key: k, Value: v, Enabled: true, Type: t})
				}
				i += 2
			} else {
				i++
			}
		case "-u", "--user":
			if i+1 < len(tokens) {
				parts := strings.SplitN(tokens[i+1], ":", 2)
				if len(parts) == 2 {
					req.Auth = types.AuthDefinition{Type: "basic", Username: parts[0], Password: parts[1]}
				}
				i += 2
			} else {
				i++
			}
		case "--compressed", "-L", "--location", "-s", "--silent", "-v", "--verbose", "-i", "--include":
			if tok == "-L" || tok == "--location" {
				req.Settings.FollowRedirects = true
			}
			i++
		case "--insecure", "-k":
			req.Settings.VerifySSL = false
			i++
		case "--max-time":
			if i+1 < len(tokens) {
				var ms int
				fmt.Sscanf(tokens[i+1], "%d", &ms)
				req.Settings.TimeoutMs = ms * 1000
				i += 2
			} else {
				i++
			}
		default:
			if !strings.HasPrefix(tok, "-") && req.URL == "" {
				req.URL = tok
				if qIdx := strings.Index(req.URL, "?"); qIdx != -1 {
					query := req.URL[qIdx+1:]
					req.URL = req.URL[:qIdx]
					for _, part := range strings.Split(query, "&") {
						if eqIdx := strings.Index(part, "="); eqIdx != -1 {
							req.Params = append(req.Params, types.KeyValue{
								Key:     part[:eqIdx],
								Value:   part[eqIdx+1:],
								Enabled: true,
							})
						}
					}
				}
			}
			i++
		}
	}

	if req.URL == "" {
		return nil, fmt.Errorf("no URL found in cURL command")
	}

	urlParts := strings.Split(strings.TrimSuffix(req.URL, "/"), "/")
	req.Name = req.Method + " " + urlParts[len(urlParts)-1]
	if req.Name == req.Method+" " {
		req.Name = req.Method + " Request"
	}

	fileName := sanitizeFilename(req.Name) + ".pebble.json"

	return &ImportResult{
		Format:         FormatCURL,
		CollectionName: "cURL Import",
		Requests: []ImportedItem{
			{
				Name:    req.Name,
				RelPath: fileName,
				Request: req,
			},
		},
	}, nil
}

func tokenizeCURL(s string) ([]string, error) {
	var tokens []string
	var current strings.Builder
	inSingle := false
	inDouble := false

	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		c := runes[i]
		switch {
		case c == '\'' && !inDouble:
			inSingle = !inSingle
		case c == '"' && !inSingle:
			inDouble = !inDouble
		case c == '\\' && (inDouble || inSingle):
			if i+1 < len(runes) {
				i++
				current.WriteRune(runes[i])
			}
		case unicode.IsSpace(c) && !inSingle && !inDouble:
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			for i+1 < len(runes) && runes[i+1] == '\\' && i+2 < len(runes) && runes[i+2] == '\n' {
				i += 2
				for i+1 < len(runes) && unicode.IsSpace(runes[i+1]) {
					i++
				}
				break
			}
		default:
			current.WriteRune(c)
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	if inSingle || inDouble {
		return nil, fmt.Errorf("unclosed quote in cURL command")
	}
	return tokens, nil
}
