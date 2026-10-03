package impexp

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"pebblepost/internal/types"
)

// Postman JSON structures
type pmCollection struct {
	Info     pmInfo       `json:"info"`
	Item     []pmItem     `json:"item"`
	Variable []pmVariable `json:"variable,omitempty"`
	Auth     *pmAuth      `json:"auth,omitempty"`
	Event    []pmEvent    `json:"event,omitempty"`
}

type pmInfo struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Schema      string `json:"schema,omitempty"`
}

type pmVariable struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	Type     string `json:"type,omitempty"`
	Disabled bool   `json:"disabled,omitempty"`
}

type pmItem struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Item        []pmItem   `json:"item,omitempty"` // Non-empty = Folder
	Request     *pmRequest `json:"request,omitempty"`
	Auth        *pmAuth    `json:"auth,omitempty"`
	Event       []pmEvent  `json:"event,omitempty"`
}

type pmRequest struct {
	Method      string     `json:"method"`
	URL         any        `json:"url"` // string or pmURL object
	Header      []pmHeader `json:"header,omitempty"`
	Body        *pmBody    `json:"body,omitempty"`
	Auth        *pmAuth    `json:"auth,omitempty"`
	Description string     `json:"description,omitempty"`
}

type pmURL struct {
	Raw   string        `json:"raw"`
	Query []pmQueryItem `json:"query,omitempty"`
}

type pmQueryItem struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	Disabled bool   `json:"disabled,omitempty"`
}

type pmHeader struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	Disabled bool   `json:"disabled,omitempty"`
}

type pmBody struct {
	Mode       string         `json:"mode"` // raw, urlencoded, formdata, graphql
	Raw        string         `json:"raw,omitempty"`
	URLEncoded []pmURLEncoded `json:"urlencoded,omitempty"`
	FormData   []pmFormData   `json:"formdata,omitempty"`
	GraphQL    *pmGraphQL     `json:"graphql,omitempty"`
	Options    *pmBodyOptions `json:"options,omitempty"`
}

type pmBodyOptions struct {
	Raw struct {
		Language string `json:"language"`
	} `json:"raw"`
}

type pmGraphQL struct {
	Query     string `json:"query"`
	Variables string `json:"variables,omitempty"`
}

type pmURLEncoded struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	Disabled bool   `json:"disabled,omitempty"`
}

type pmFormData struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	Src      string `json:"src,omitempty"`
	Type     string `json:"type,omitempty"` // "file" or "text"
	Disabled bool   `json:"disabled,omitempty"`
}

type pmAuth struct {
	Type   string `json:"type"` // "bearer", "basic", "apikey", "inherit", "noauth"
	Bearer []pmKV `json:"bearer,omitempty"`
	Basic  []pmKV `json:"basic,omitempty"`
	Apikey []pmKV `json:"apikey,omitempty"`
}

type pmKV struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type pmEvent struct {
	Listen string   `json:"listen"` // "prerequest" or "test"
	Script pmScript `json:"script"`
}

type pmScript struct {
	Type string `json:"type"`
	Exec any    `json:"exec"` // string or []string
}

// contextChain holds inherited attributes while walking nested Postman items.
type postmanContext struct {
	parentAuth    types.AuthDefinition
	parentHeaders []types.KeyValue
	folderRelPath string
}

// ParsePostman parses a Postman Collection v2.1 JSON into an ImportResult.
// It resolves folder inheritance (auth, headers, scripts), collection variables,
// and converts `pm.*` scripts into `pb.*` scripts.
func ParsePostman(data []byte) (*ImportResult, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("postman data is empty")
	}

	var col pmCollection
	if err := json.Unmarshal(data, &col); err != nil {
		return nil, fmt.Errorf("failed to parse Postman collection JSON: %w", err)
	}

	colName := strings.TrimSpace(col.Info.Name)
	if colName == "" {
		colName = "Postman Collection"
	}

	result := &ImportResult{
		Format:         FormatPostman,
		CollectionName: colName,
	}

	// 1. Collection variables
	for _, v := range col.Variable {
		if v.Key != "" {
			result.Variables = append(result.Variables, types.KeyValue{
				Key:     v.Key,
				Value:   v.Value,
				Enabled: !v.Disabled,
			})
		}
	}

	// 2. Collection root auth
	rootAuth := parsePostmanAuth(col.Auth)

	// 3. Collection root scripts
	rootScripts, rootWarnings := extractScriptsFromEvents(col.Event)
	if len(rootWarnings) > 0 {
		result.Warnings = append(result.Warnings, rootWarnings...)
	}

	// If root has variables, auth, or scripts, define root folder definition
	if len(result.Variables) > 0 || rootAuth.Type != "none" || rootScripts.PreRequest != "" || rootScripts.PostResponse != "" {
		result.Folders = append(result.Folders, ImportedFolder{
			Name:    colName,
			RelPath: ".",
			Definition: types.FolderDefinition{
				SchemaVersion: 1,
				Name:          colName,
				Description:   col.Info.Description,
				Auth:          rootAuth,
				Variables:     result.Variables,
				Scripts:       rootScripts,
			},
		})
	}

	initialCtx := postmanContext{
		parentAuth:    rootAuth,
		folderRelPath: "",
	}

	seenNames := make(map[string]int)

	walkPostmanItems(col.Item, initialCtx, result, seenNames)

	if len(result.Requests) == 0 && len(result.Skipped) == 0 {
		return nil, fmt.Errorf("no requests found in Postman collection")
	}

	return result, nil
}

func walkPostmanItems(items []pmItem, ctx postmanContext, result *ImportResult, seenNames map[string]int) {
	for _, item := range items {
		// A folder is indicated by having sub-items or having no request
		isFolder := len(item.Item) > 0 || (item.Request == nil && item.Item != nil)

		if isFolder {
			folderName := sanitizeFilename(item.Name)
			folderRel := filepath.Join(ctx.folderRelPath, folderName)

			// Folder Auth
			folderAuth := ctx.parentAuth
			if item.Auth != nil {
				parsedAuth := parsePostmanAuth(item.Auth)
				if parsedAuth.Type != "none" && parsedAuth.Type != "inherit" {
					folderAuth = parsedAuth
				}
			}

			// Folder Scripts
			folderScripts, warnings := extractScriptsFromEvents(item.Event)
			if len(warnings) > 0 {
				result.Warnings = append(result.Warnings, warnings...)
			}

			result.Folders = append(result.Folders, ImportedFolder{
				Name:    item.Name,
				RelPath: folderRel,
				Definition: types.FolderDefinition{
					SchemaVersion: 1,
					Name:          item.Name,
					Description:   item.Description,
					Auth:          folderAuth,
					Scripts:       folderScripts,
				},
			})

			childCtx := postmanContext{
				parentAuth:    folderAuth,
				folderRelPath: folderRel,
			}

			walkPostmanItems(item.Item, childCtx, result, seenNames)
			continue
		}

		// Request Item
		if item.Request == nil {
			result.Skipped = append(result.Skipped, SkippedItem{
				Name:   item.Name,
				Path:   ctx.folderRelPath,
				Reason: "Item has no request definition",
			})
			continue
		}

		r := item.Request
		reqURL, queryParams := parsePostmanURL(r.URL)
		if reqURL == "" && item.Name == "" {
			result.Skipped = append(result.Skipped, SkippedItem{
				Name:   "Unnamed request",
				Path:   ctx.folderRelPath,
				Reason: "Empty request URL and name",
			})
			continue
		}

		reqName := item.Name
		if reqName == "" {
			reqName = r.Method + " Request"
		}

		pathKey := filepath.Join(ctx.folderRelPath, reqName)
		seenNames[pathKey]++
		finalName := reqName
		if seenNames[pathKey] > 1 {
			finalName = fmt.Sprintf("%s (%d)", reqName, seenNames[pathKey])
		}

		req := &types.RequestDefinition{
			Name:        finalName,
			Description: r.Description,
			Method:      strings.ToUpper(strings.TrimSpace(r.Method)),
			URL:         reqURL,
			Auth:        types.AuthDefinition{Type: "inherit"}, // Default inherit
			Body:        types.BodyDefinition{Type: "none"},
			Settings: types.SettingDefinition{
				FollowRedirects: true,
				VerifySSL:       true,
				TimeoutMs:       30000,
			},
		}
		if req.Method == "" {
			req.Method = "GET"
		}

		// Headers
		for _, h := range r.Header {
			req.Headers = append(req.Headers, types.KeyValue{
				Key:     h.Key,
				Value:   h.Value,
				Enabled: !h.Disabled,
			})
		}

		// Query Params
		req.Params = queryParams

		// Auth: If explicitly configured, use it; otherwise inherit from parent folder
		if r.Auth != nil && r.Auth.Type != "" && r.Auth.Type != "noauth" && r.Auth.Type != "inherit" {
			req.Auth = parsePostmanAuth(r.Auth)
		} else if ctx.parentAuth.Type != "" && ctx.parentAuth.Type != "none" {
			req.Auth = ctx.parentAuth
		} else {
			req.Auth = types.AuthDefinition{Type: "none"}
		}

		// Body
		if r.Body != nil {
			switch strings.ToLower(r.Body.Mode) {
			case "raw":
				bodyType := "raw"
				trimmed := strings.TrimSpace(r.Body.Raw)
				if (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) ||
					(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) {
					bodyType = "json"
				} else if r.Body.Options != nil && r.Body.Options.Raw.Language == "json" {
					bodyType = "json"
				}
				req.Body = types.BodyDefinition{Type: bodyType, Raw: r.Body.Raw}

			case "urlencoded":
				var kvs []types.KeyValue
				for _, kv := range r.Body.URLEncoded {
					kvs = append(kvs, types.KeyValue{
						Key:     kv.Key,
						Value:   kv.Value,
						Enabled: !kv.Disabled,
					})
				}
				req.Body = types.BodyDefinition{Type: "urlEncoded", UrlEncoded: kvs}

			case "formdata":
				var kvs []types.KeyValue
				for _, kv := range r.Body.FormData {
					t := "text"
					if kv.Type == "file" || kv.Src != "" {
						t = "file"
					}
					kvs = append(kvs, types.KeyValue{
						Key:     kv.Key,
						Value:   kv.Value,
						Enabled: !kv.Disabled,
						Type:    t,
					})
				}
				req.Body = types.BodyDefinition{Type: "formData", FormData: kvs}

			case "graphql":
				if r.Body.GraphQL != nil {
					req.Body = types.BodyDefinition{
						Type: "graphql",
						GraphQL: &types.GraphQL{
							Query:     r.Body.GraphQL.Query,
							Variables: r.Body.GraphQL.Variables,
						},
					}
				}
			}
		}

		// Scripts
		scripts, scriptWarnings := extractScriptsFromEvents(item.Event)
		req.Scripts = scripts
		if len(scriptWarnings) > 0 {
			result.Warnings = append(result.Warnings, scriptWarnings...)
		}

		fileName := sanitizeFilename(finalName) + ".pebble.json"
		var destRel string
		if ctx.folderRelPath != "" {
			destRel = filepath.Join(ctx.folderRelPath, fileName)
		} else {
			destRel = fileName
		}

		result.Requests = append(result.Requests, ImportedItem{
			Name:    finalName,
			RelPath: destRel,
			Request: req,
		})
	}
}

func parsePostmanURL(raw any) (string, []types.KeyValue) {
	if raw == nil {
		return "", nil
	}

	switch v := raw.(type) {
	case string:
		return v, nil
	case map[string]any:
		rawURL, _ := v["raw"].(string)
		var params []types.KeyValue
		if queryRaw, ok := v["query"].([]any); ok {
			for _, q := range queryRaw {
				if qm, ok := q.(map[string]any); ok {
					k, _ := qm["key"].(string)
					val, _ := qm["value"].(string)
					disabled, _ := qm["disabled"].(bool)
					if k != "" {
						params = append(params, types.KeyValue{
							Key:     k,
							Value:   val,
							Enabled: !disabled,
						})
					}
				}
			}
		}
		return rawURL, params
	}

	return "", nil
}

func parsePostmanAuth(auth *pmAuth) types.AuthDefinition {
	if auth == nil {
		return types.AuthDefinition{Type: "none"}
	}

	switch strings.ToLower(auth.Type) {
	case "bearer":
		token := ""
		for _, kv := range auth.Bearer {
			if kv.Key == "token" {
				token = kv.Value
			}
		}
		return types.AuthDefinition{Type: "bearer", Token: token}

	case "basic":
		u, p := "", ""
		for _, kv := range auth.Basic {
			switch kv.Key {
			case "username":
				u = kv.Value
			case "password":
				p = kv.Value
			}
		}
		return types.AuthDefinition{Type: "basic", Username: u, Password: p}

	case "apikey":
		k, v, addTo := "", "", "header"
		for _, kv := range auth.Apikey {
			switch kv.Key {
			case "key":
				k = kv.Value
			case "value":
				v = kv.Value
			case "in":
				addTo = kv.Value
			}
		}
		return types.AuthDefinition{Type: "apiKey", Key: k, Value: v, AddTo: addTo}

	case "inherit":
		return types.AuthDefinition{Type: "inherit"}
	default:
		return types.AuthDefinition{Type: "none"}
	}
}

func extractScriptsFromEvents(events []pmEvent) (types.ScriptDefinition, []string) {
	var scripts types.ScriptDefinition
	var allWarnings []string

	for _, ev := range events {
		rawCode := ""
		switch v := ev.Script.Exec.(type) {
		case string:
			rawCode = v
		case []any:
			var lines []string
			for _, item := range v {
				if s, ok := item.(string); ok {
					lines = append(lines, s)
				}
			}
			rawCode = strings.Join(lines, "\n")
		case []string:
			rawCode = strings.Join(v, "\n")
		}

		if strings.TrimSpace(rawCode) == "" {
			continue
		}

		converted, warnings := ConvertPostmanScript(rawCode)
		if len(warnings) > 0 {
			allWarnings = append(allWarnings, warnings...)
		}

		switch strings.ToLower(ev.Listen) {
		case "prerequest":
			if scripts.PreRequest != "" {
				scripts.PreRequest += "\n\n" + converted
			} else {
				scripts.PreRequest = converted
			}
		case "test":
			if scripts.PostResponse != "" {
				scripts.PostResponse += "\n\n" + converted
			} else {
				scripts.PostResponse = converted
			}
		}
	}

	return scripts, allWarnings
}
