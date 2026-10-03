package impexp

import (
	"encoding/json"
	"net/url"
	"path/filepath"
	"strings"

	"pebblepost/internal/types"
)

// ExportPostman converts a collection of requests, folders, and variables into Postman v2.1 JSON.
func ExportPostman(collectionName string, items []ImportedItem, folders []ImportedFolder, variables []types.KeyValue) ([]byte, error) {
	if collectionName == "" {
		collectionName = "PebblePost Export"
	}

	pmCol := pmCollection{
		Info: pmInfo{
			Name:   collectionName,
			Schema: "https://schema.getpostman.com/json/collection/v2.1.0/collection.json",
		},
	}

	// Variables
	for _, v := range variables {
		pmCol.Variable = append(pmCol.Variable, pmVariable{
			Key:      v.Key,
			Value:    v.Value,
			Disabled: !v.Enabled,
		})
	}

	// Build folder hierarchy
	type folderNode struct {
		name       string
		relPath    string
		definition types.FolderDefinition
		subfolders map[string]*folderNode
		requests   []*types.RequestDefinition
	}

	rootNode := &folderNode{
		subfolders: make(map[string]*folderNode),
	}

	// Register explicit folders
	for _, f := range folders {
		if f.RelPath == "." || f.RelPath == "" {
			// Root folder settings
			if f.Definition.Auth.Type != "" && f.Definition.Auth.Type != "none" {
				pmCol.Auth = convertToPostmanAuth(f.Definition.Auth)
			}
			pmCol.Event = convertToPostmanEvents(f.Definition.Scripts)
			continue
		}

		parts := strings.Split(filepath.ToSlash(f.RelPath), "/")
		curr := rootNode
		for _, part := range parts {
			if part == "" || part == "." {
				continue
			}
			if curr.subfolders[part] == nil {
				curr.subfolders[part] = &folderNode{
					name:       part,
					subfolders: make(map[string]*folderNode),
				}
			}
			curr = curr.subfolders[part]
		}
		curr.name = f.Name
		curr.definition = f.Definition
	}

	// Place requests into folders
	for _, it := range items {
		req := it.Request
		if req == nil {
			continue
		}

		relDir := filepath.Dir(filepath.ToSlash(it.RelPath))
		if relDir == "." || relDir == "" {
			rootNode.requests = append(rootNode.requests, req)
		} else {
			parts := strings.Split(relDir, "/")
			curr := rootNode
			for _, part := range parts {
				if part == "" || part == "." {
					continue
				}
				if curr.subfolders[part] == nil {
					curr.subfolders[part] = &folderNode{
						name:       part,
						subfolders: make(map[string]*folderNode),
					}
				}
				curr = curr.subfolders[part]
			}
			curr.requests = append(curr.requests, req)
		}
	}

	// Convert tree into Postman items
	var convertNodeToPmItems func(node *folderNode) []pmItem
	convertNodeToPmItems = func(node *folderNode) []pmItem {
		var res []pmItem

		// 1. Subfolders
		for _, sub := range node.subfolders {
			fItem := pmItem{
				Name:        sub.name,
				Description: sub.definition.Description,
				Auth:        convertToPostmanAuth(sub.definition.Auth),
				Event:       convertToPostmanEvents(sub.definition.Scripts),
				Item:        convertNodeToPmItems(sub),
			}
			res = append(res, fItem)
		}

		// 2. Requests
		for _, r := range node.requests {
			res = append(res, convertRequestToPostmanItem(r))
		}

		return res
	}

	pmCol.Item = convertNodeToPmItems(rootNode)

	return json.MarshalIndent(pmCol, "", "  ")
}

func convertRequestToPostmanItem(r *types.RequestDefinition) pmItem {
	item := pmItem{
		Name:        r.Name,
		Description: r.Description,
		Event:       convertToPostmanEvents(r.Scripts),
	}

	req := &pmRequest{
		Method:      r.Method,
		Description: r.Description,
		Auth:        convertToPostmanAuth(r.Auth),
	}

	// URL and Query params
	pURL := pmURL{
		Raw: r.URL,
	}
	for _, p := range r.Params {
		pURL.Query = append(pURL.Query, pmQueryItem{
			Key:      p.Key,
			Value:    p.Value,
			Disabled: !p.Enabled,
		})
	}
	req.URL = pURL

	// Headers
	for _, h := range r.Headers {
		req.Header = append(req.Header, pmHeader{
			Key:      h.Key,
			Value:    h.Value,
			Disabled: !h.Enabled,
		})
	}

	// Body
	switch r.Body.Type {
	case "json":
		req.Body = &pmBody{
			Mode: "raw",
			Raw:  r.Body.Raw,
			Options: &pmBodyOptions{
				Raw: struct {
					Language string `json:"language"`
				}{Language: "json"},
			},
		}
	case "raw":
		req.Body = &pmBody{
			Mode: "raw",
			Raw:  r.Body.Raw,
		}
	case "urlEncoded":
		var urlenc []pmURLEncoded
		for _, kv := range r.Body.UrlEncoded {
			urlenc = append(urlenc, pmURLEncoded{
				Key:      kv.Key,
				Value:    kv.Value,
				Disabled: !kv.Enabled,
			})
		}
		req.Body = &pmBody{
			Mode:       "urlencoded",
			URLEncoded: urlenc,
		}
	case "formData":
		var form []pmFormData
		for _, kv := range r.Body.FormData {
			form = append(form, pmFormData{
				Key:      kv.Key,
				Value:    kv.Value,
				Type:     kv.Type,
				Disabled: !kv.Enabled,
			})
		}
		req.Body = &pmBody{
			Mode:     "formdata",
			FormData: form,
		}
	case "graphql":
		if r.Body.GraphQL != nil {
			req.Body = &pmBody{
				Mode: "graphql",
				GraphQL: &pmGraphQL{
					Query:     r.Body.GraphQL.Query,
					Variables: r.Body.GraphQL.Variables,
				},
			}
		}
	}

	item.Request = req
	return item
}

func convertToPostmanAuth(auth types.AuthDefinition) *pmAuth {
	switch auth.Type {
	case "bearer":
		return &pmAuth{
			Type: "bearer",
			Bearer: []pmKV{
				{Key: "token", Value: auth.Token},
			},
		}
	case "basic":
		return &pmAuth{
			Type: "basic",
			Basic: []pmKV{
				{Key: "username", Value: auth.Username},
				{Key: "password", Value: auth.Password},
			},
		}
	case "apiKey":
		return &pmAuth{
			Type: "apikey",
			Apikey: []pmKV{
				{Key: "key", Value: auth.Key},
				{Key: "value", Value: auth.Value},
				{Key: "in", Value: auth.AddTo},
			},
		}
	case "inherit":
		return &pmAuth{Type: "inherit"}
	default:
		return nil
	}
}

func convertToPostmanEvents(scripts types.ScriptDefinition) []pmEvent {
	var events []pmEvent

	if strings.TrimSpace(scripts.PreRequest) != "" {
		code := convertPebbleScriptToPostman(scripts.PreRequest)
		events = append(events, pmEvent{
			Listen: "prerequest",
			Script: pmScript{
				Type: "text/javascript",
				Exec: strings.Split(code, "\n"),
			},
		})
	}

	if strings.TrimSpace(scripts.PostResponse) != "" {
		code := convertPebbleScriptToPostman(scripts.PostResponse)
		events = append(events, pmEvent{
			Listen: "test",
			Script: pmScript{
				Type: "text/javascript",
				Exec: strings.Split(code, "\n"),
			},
		})
	}

	return events
}

func convertPebbleScriptToPostman(script string) string {
	res := script
	res = strings.ReplaceAll(res, "pb.environment.get(", "pm.environment.get(")
	res = strings.ReplaceAll(res, "pb.environment.set(", "pm.environment.set(")
	res = strings.ReplaceAll(res, "pb.variables.get(", "pm.variables.get(")
	res = strings.ReplaceAll(res, "pb.response.json()", "pm.response.json()")
	res = strings.ReplaceAll(res, "pb.response.text()", "pm.response.text()")
	res = strings.ReplaceAll(res, "pb.response.status", "pm.response.code")
	res = strings.ReplaceAll(res, "pb.test(", "pm.test(")
	res = strings.ReplaceAll(res, "pb.console.", "console.")
	res = strings.ReplaceAll(res, "pb.expect(", "pm.expect(")
	res = strings.ReplaceAll(res, ").toEqual(", ").to.eql(")
	res = strings.ReplaceAll(res, ").toBe(", ").to.equal(")
	return res
}

// buildURLHostAndPath parses a raw URL into host segments and path segments for Postman format.
func buildURLHostAndPath(rawURL string) ([]string, []string) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, nil
	}
	var hosts []string
	if u.Host != "" {
		hosts = strings.Split(u.Host, ".")
	}
	var paths []string
	cleanPath := strings.Trim(u.Path, "/")
	if cleanPath != "" {
		paths = strings.Split(cleanPath, "/")
	}
	return hosts, paths
}
