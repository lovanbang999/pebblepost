package impexp

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"pebblepost/internal/types"
)

// insomniaExport represents the root structure of an Insomnia v4 export file.
type insomniaExport struct {
	Type         string             `json:"_type"`
	ExportFormat int                `json:"__export_format"`
	Resources    []insomniaResource `json:"resources"`
}

type insomniaResource struct {
	ID             string           `json:"_id"`
	ParentID       string           `json:"parentId"`
	Type           string           `json:"_type"` // "workspace", "request_group", "request", "environment"
	Name           string           `json:"name"`
	Description    string           `json:"description,omitempty"`
	Method         string           `json:"method,omitempty"`
	URL            string           `json:"url,omitempty"`
	Headers        []insomniaHeader `json:"headers,omitempty"`
	Parameters     []insomniaParam  `json:"parameters,omitempty"`
	Body           *insomniaBody    `json:"body,omitempty"`
	Authentication *insomniaAuth    `json:"authentication,omitempty"`
	Data           map[string]any   `json:"data,omitempty"` // For environment resources
}

type insomniaHeader struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Disabled bool   `json:"disabled,omitempty"`
}

type insomniaParam struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Disabled bool   `json:"disabled,omitempty"`
}

type insomniaBody struct {
	MimeType string             `json:"mimeType,omitempty"`
	Text     string             `json:"text,omitempty"`
	Params   []insomniaBodyPart `json:"params,omitempty"`
}

type insomniaBodyPart struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Disabled bool   `json:"disabled,omitempty"`
	Type     string `json:"type,omitempty"` // "file" or "text"
	FileName string `json:"fileName,omitempty"`
}

type insomniaAuth struct {
	Type     string `json:"type,omitempty"` // "bearer", "basic", "apikey"
	Token    string `json:"token,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Key      string `json:"key,omitempty"`
	Value    string `json:"value,omitempty"`
	AddTo    string `json:"addTo,omitempty"`
}

// ParseInsomnia parses Insomnia v4 export JSON data into an ImportResult.
func ParseInsomnia(data []byte) (*ImportResult, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("insomnia data is empty")
	}

	var export insomniaExport
	if err := json.Unmarshal(data, &export); err != nil {
		return nil, fmt.Errorf("failed to parse Insomnia export JSON: %w", err)
	}

	collectionName := "Insomnia Collection"
	groupMap := make(map[string]*insomniaResource)
	var rawRequests []*insomniaResource
	var envData map[string]any

	// Categorize resources
	for i := range export.Resources {
		res := &export.Resources[i]
		switch res.Type {
		case "workspace":
			if res.Name != "" {
				collectionName = res.Name
			}
		case "request_group":
			groupMap[res.ID] = res
		case "request":
			rawRequests = append(rawRequests, res)
		case "environment":
			if len(res.Data) > 0 && envData == nil {
				envData = res.Data
			}
		}
	}

	result := &ImportResult{
		Format:         FormatInsomnia,
		CollectionName: collectionName,
	}

	// Map environment data to collection variables
	for k, v := range envData {
		strVal := fmt.Sprintf("%v", v)
		result.Variables = append(result.Variables, types.KeyValue{
			Key:     k,
			Value:   strVal,
			Enabled: true,
		})
	}

	// Build folder path resolver
	buildFolderPath := func(parentID string) string {
		var segments []string
		curr := parentID
		for curr != "" {
			group, ok := groupMap[curr]
			if !ok {
				break
			}
			folderName := sanitizeFilename(group.Name)
			segments = append([]string{folderName}, segments...)
			curr = group.ParentID
		}
		return filepath.Join(segments...)
	}

	// Track created folders
	folderSet := make(map[string]bool)

	for _, res := range rawRequests {
		if strings.TrimSpace(res.URL) == "" && strings.TrimSpace(res.Name) == "" {
			result.Skipped = append(result.Skipped, SkippedItem{
				Name:   res.Name,
				Reason: "Empty request URL and name",
			})
			continue
		}

		reqName := res.Name
		if reqName == "" {
			reqName = res.Method + " Request"
		}

		method := strings.ToUpper(res.Method)
		if method == "" {
			method = "GET"
		}

		req := &types.RequestDefinition{
			Name:   reqName,
			Method: method,
			URL:    res.URL,
			Auth:   types.AuthDefinition{Type: "none"},
			Body:   types.BodyDefinition{Type: "none"},
			Settings: types.SettingDefinition{
				FollowRedirects: true,
				VerifySSL:       true,
				TimeoutMs:       30000,
			},
		}

		// Headers
		for _, h := range res.Headers {
			req.Headers = append(req.Headers, types.KeyValue{
				Key:     h.Name,
				Value:   h.Value,
				Enabled: !h.Disabled,
			})
		}

		// Query params
		for _, p := range res.Parameters {
			req.Params = append(req.Params, types.KeyValue{
				Key:     p.Name,
				Value:   p.Value,
				Enabled: !p.Disabled,
			})
		}

		// Auth
		if res.Authentication != nil {
			switch res.Authentication.Type {
			case "bearer":
				req.Auth = types.AuthDefinition{
					Type:  "bearer",
					Token: res.Authentication.Token,
				}
			case "basic":
				req.Auth = types.AuthDefinition{
					Type:     "basic",
					Username: res.Authentication.Username,
					Password: res.Authentication.Password,
				}
			case "apikey":
				req.Auth = types.AuthDefinition{
					Type:  "apiKey",
					Key:   res.Authentication.Key,
					Value: res.Authentication.Value,
					AddTo: res.Authentication.AddTo,
				}
			}
		}

		// Body
		if res.Body != nil {
			mime := strings.ToLower(res.Body.MimeType)
			switch {
			case strings.Contains(mime, "application/json"):
				req.Body = types.BodyDefinition{
					Type: "json",
					Raw:  res.Body.Text,
				}
			case strings.Contains(mime, "application/x-www-form-urlencoded"):
				var kvs []types.KeyValue
				for _, p := range res.Body.Params {
					kvs = append(kvs, types.KeyValue{
						Key:     p.Name,
						Value:   p.Value,
						Enabled: !p.Disabled,
					})
				}
				req.Body = types.BodyDefinition{
					Type:       "urlEncoded",
					UrlEncoded: kvs,
				}
			case strings.Contains(mime, "multipart/form-data"):
				var kvs []types.KeyValue
				for _, p := range res.Body.Params {
					entryType := "text"
					if p.Type == "file" || p.FileName != "" {
						entryType = "file"
					}
					kvs = append(kvs, types.KeyValue{
						Key:     p.Name,
						Value:   p.Value,
						Enabled: !p.Disabled,
						Type:    entryType,
					})
				}
				req.Body = types.BodyDefinition{
					Type:     "formData",
					FormData: kvs,
				}
			case strings.Contains(mime, "graphql"):
				req.Body = types.BodyDefinition{
					Type: "graphql",
					Raw:  res.Body.Text,
				}
			default:
				if res.Body.Text != "" {
					req.Body = types.BodyDefinition{
						Type: "raw",
						Raw:  res.Body.Text,
					}
				}
			}
		}

		folderRel := buildFolderPath(res.ParentID)
		if folderRel != "" && !folderSet[folderRel] {
			folderSet[folderRel] = true
			result.Folders = append(result.Folders, ImportedFolder{
				Name:    filepath.Base(folderRel),
				RelPath: folderRel,
				Definition: types.FolderDefinition{
					SchemaVersion: 1,
					Name:          filepath.Base(folderRel),
				},
			})
		}

		fileName := sanitizeFilename(req.Name) + ".pebble.json"
		var destRel string
		if folderRel != "" {
			destRel = filepath.Join(folderRel, fileName)
		} else {
			destRel = fileName
		}

		result.Requests = append(result.Requests, ImportedItem{
			Name:    req.Name,
			RelPath: destRel,
			Request: req,
		})
	}

	if len(result.Requests) == 0 && len(result.Skipped) == 0 {
		return nil, fmt.Errorf("no requests found in Insomnia export")
	}

	return result, nil
}
