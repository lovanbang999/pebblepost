package impexp

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"pebblepost/internal/types"
)

type harRoot struct {
	Log harLog `json:"log"`
}

type harLog struct {
	Version string     `json:"version"`
	Creator harCreator `json:"creator"`
	Entries []harEntry `json:"entries"`
}

type harCreator struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type harEntry struct {
	StartedDateTime string     `json:"startedDateTime"`
	Request         harRequest `json:"request"`
}

type harRequest struct {
	Method      string         `json:"method"`
	URL         string         `json:"url"`
	Headers     []harHeader    `json:"headers"`
	QueryString []harQueryItem `json:"queryString"`
	PostData    *harPostData   `json:"postData"`
}

type harHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type harQueryItem struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type harPostData struct {
	MimeType string         `json:"mimeType"`
	Text     string         `json:"text"`
	Params   []harPostParam `json:"params"`
}

type harPostParam struct {
	Name        string `json:"name"`
	Value       string `json:"value"`
	FileName    string `json:"fileName"`
	ContentType string `json:"contentType"`
}

// ParseHAR parses an HTTP Archive 1.2 JSON file into an ImportResult.
func ParseHAR(data []byte) (*ImportResult, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("HAR data is empty")
	}

	var root harRoot
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("failed to parse HAR JSON: %w", err)
	}

	if len(root.Log.Entries) == 0 {
		return nil, fmt.Errorf("no entries found in HAR log")
	}

	collectionName := "HAR Import"
	if root.Log.Creator.Name != "" {
		collectionName = fmt.Sprintf("HAR (%s)", root.Log.Creator.Name)
	}

	result := &ImportResult{
		Format:         FormatHAR,
		CollectionName: collectionName,
	}

	seenNames := make(map[string]int)

	for i, entry := range root.Log.Entries {
		hr := entry.Request
		rawURL := strings.TrimSpace(hr.URL)
		if rawURL == "" {
			result.Skipped = append(result.Skipped, SkippedItem{
				Name:   fmt.Sprintf("Entry #%d", i+1),
				Reason: "Missing URL in HAR entry",
			})
			continue
		}

		method := strings.ToUpper(strings.TrimSpace(hr.Method))
		if method == "" {
			method = "GET"
		}

		// Clean URL and derive name
		parsedURL, err := url.Parse(rawURL)
		urlPath := ""
		if err == nil {
			urlPath = parsedURL.Path
		}
		if urlPath == "" || urlPath == "/" {
			if parsedURL != nil && parsedURL.Host != "" {
				urlPath = "/" + parsedURL.Host
			} else {
				urlPath = "/request"
			}
		}

		baseName := fmt.Sprintf("%s %s", method, filepath.Base(urlPath))
		if baseName == method+" /" || baseName == method+" ." {
			baseName = fmt.Sprintf("%s Request", method)
		}

		seenNames[baseName]++
		finalName := baseName
		if seenNames[baseName] > 1 {
			finalName = fmt.Sprintf("%s (%d)", baseName, seenNames[baseName])
		}

		req := &types.RequestDefinition{
			Name:   finalName,
			Method: method,
			URL:    rawURL,
			Auth:   types.AuthDefinition{Type: "none"},
			Body:   types.BodyDefinition{Type: "none"},
			Settings: types.SettingDefinition{
				FollowRedirects: true,
				VerifySSL:       true,
				TimeoutMs:       30000,
			},
		}

		// Strip query parameters from URL if we have structured query items or parse them
		if parsedURL != nil && len(hr.QueryString) > 0 {
			// Clean URL without query
			cleanURL := *parsedURL
			cleanURL.RawQuery = ""
			cleanURL.Fragment = ""
			req.URL = cleanURL.String()

			for _, q := range hr.QueryString {
				req.Params = append(req.Params, types.KeyValue{
					Key:     q.Name,
					Value:   q.Value,
					Enabled: true,
				})
			}
		}

		// Process headers
		for _, h := range hr.Headers {
			// Skip HTTP/2 pseudo-headers (:method, :authority, :path, :scheme)
			if strings.HasPrefix(h.Name, ":") {
				continue
			}

			// Extract Auth header if bearer or basic
			if strings.EqualFold(h.Name, "Authorization") {
				parts := strings.SplitN(h.Value, " ", 2)
				if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
					req.Auth = types.AuthDefinition{
						Type:  "bearer",
						Token: parts[1],
					}
					continue
				} else if len(parts) == 2 && strings.EqualFold(parts[0], "Basic") {
					req.Auth = types.AuthDefinition{
						Type:  "basic",
						Token: parts[1],
					}
					continue
				}
			}

			req.Headers = append(req.Headers, types.KeyValue{
				Key:     h.Name,
				Value:   h.Value,
				Enabled: true,
			})
		}

		// Process PostData (Body)
		if hr.PostData != nil {
			mime := strings.ToLower(hr.PostData.MimeType)
			switch {
			case strings.Contains(mime, "application/json"):
				req.Body = types.BodyDefinition{
					Type: "json",
					Raw:  hr.PostData.Text,
				}
			case strings.Contains(mime, "application/x-www-form-urlencoded"):
				var kvs []types.KeyValue
				if len(hr.PostData.Params) > 0 {
					for _, p := range hr.PostData.Params {
						kvs = append(kvs, types.KeyValue{
							Key:     p.Name,
							Value:   p.Value,
							Enabled: true,
						})
					}
				} else if hr.PostData.Text != "" {
					// Parse query string form
					vals, err := url.ParseQuery(hr.PostData.Text)
					if err == nil {
						for k, vs := range vals {
							for _, v := range vs {
								kvs = append(kvs, types.KeyValue{
									Key:     k,
									Value:   v,
									Enabled: true,
								})
							}
						}
					}
				}
				req.Body = types.BodyDefinition{
					Type:       "urlEncoded",
					UrlEncoded: kvs,
				}
			case strings.Contains(mime, "multipart/form-data"):
				var kvs []types.KeyValue
				for _, p := range hr.PostData.Params {
					t := "text"
					if p.FileName != "" {
						t = "file"
					}
					kvs = append(kvs, types.KeyValue{
						Key:     p.Name,
						Value:   p.Value,
						Enabled: true,
						Type:    t,
					})
				}
				req.Body = types.BodyDefinition{
					Type:     "formData",
					FormData: kvs,
				}
			default:
				if hr.PostData.Text != "" {
					req.Body = types.BodyDefinition{
						Type: "raw",
						Raw:  hr.PostData.Text,
					}
				}
			}
		}

		fileName := sanitizeFilename(finalName) + ".pebble.json"
		result.Requests = append(result.Requests, ImportedItem{
			Name:    finalName,
			RelPath: fileName,
			Request: req,
		})
	}

	return result, nil
}
