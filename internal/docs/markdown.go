package docs

import (
	"fmt"
	"strings"
)

// GenerateMarkdown generates clean GitHub Flavored Markdown from a DocCollection.
func GenerateMarkdown(col *DocCollection) string {
	var sb strings.Builder

	title := col.Title
	if title == "" {
		title = "API Documentation"
	}
	sb.WriteString(fmt.Sprintf("# %s\n\n", title))

	if col.Description != "" {
		sb.WriteString(col.Description)
		sb.WriteString("\n\n")
	}

	// ── Table of Contents ─────────────────────────────────────────────────────
	sb.WriteString("## Table of Contents\n\n")
	writeTOC(&sb, col.Items, 0)
	sb.WriteString("\n---\n\n")

	// ── Items ─────────────────────────────────────────────────────────────────
	writeItemsMarkdown(&sb, col.Items, 2)

	return sb.String()
}

func slugify(text string) string {
	clean := strings.ToLower(text)
	clean = strings.ReplaceAll(clean, " ", "-")
	clean = strings.ReplaceAll(clean, "/", "-")
	clean = strings.ReplaceAll(clean, ".", "-")
	clean = strings.ReplaceAll(clean, "(", "")
	clean = strings.ReplaceAll(clean, ")", "")
	clean = strings.ReplaceAll(clean, ":", "")
	return clean
}

func writeTOC(sb *strings.Builder, items []*DocItem, depth int) {
	indent := strings.Repeat("  ", depth)
	for _, item := range items {
		if item.IsFolder && item.Folder != nil {
			anchor := slugify(item.Folder.Name)
			sb.WriteString(fmt.Sprintf("%s- [%s](#%s)\n", indent, item.Folder.Name, anchor))
			writeTOC(sb, item.Folder.Items, depth+1)
		} else if !item.IsFolder && item.Request != nil {
			req := item.Request
			method := req.Method
			if method == "" {
				method = "GET"
			}
			anchor := slugify(fmt.Sprintf("%s-%s", method, req.Name))
			sb.WriteString(fmt.Sprintf("%s- [`%s` %s](#%s)\n", indent, method, req.Name, anchor))
		}
	}
}

func writeItemsMarkdown(sb *strings.Builder, items []*DocItem, headingLevel int) {
	for _, item := range items {
		if item.IsFolder && item.Folder != nil {
			folder := item.Folder
			prefix := strings.Repeat("#", headingLevel)
			sb.WriteString(fmt.Sprintf("%s %s\n\n", prefix, folder.Name))
			if folder.Description != "" {
				sb.WriteString(folder.Description)
				sb.WriteString("\n\n")
			}
			writeItemsMarkdown(sb, folder.Items, headingLevel+1)
		} else if !item.IsFolder && item.Request != nil {
			writeRequestMarkdown(sb, item.Request, headingLevel)
		}
	}
}

func writeRequestMarkdown(sb *strings.Builder, req *DocRequest, headingLevel int) {
	prefix := strings.Repeat("#", headingLevel)
	method := req.Method
	if method == "" {
		method = "GET"
	}
	sb.WriteString(fmt.Sprintf("%s %s %s\n\n", prefix, method, req.Name))

	if req.Description != "" {
		sb.WriteString(req.Description)
		sb.WriteString("\n\n")
	}

	sb.WriteString(fmt.Sprintf("**Endpoint**: `%s %s`\n\n", method, req.URL))

	// Parameters
	params := enabledKVs(req.Params)
	if len(params) > 0 {
		sb.WriteString("#### Query Parameters\n\n")
		sb.WriteString("| Parameter | Example Value |\n")
		sb.WriteString("| :--- | :--- |\n")
		for _, p := range params {
			sb.WriteString(fmt.Sprintf("| `%s` | `%s` |\n", p.Key, p.Value))
		}
		sb.WriteString("\n")
	}

	// Headers
	headers := enabledKVs(req.Headers)
	if len(headers) > 0 {
		sb.WriteString("#### Headers\n\n")
		sb.WriteString("| Header | Value |\n")
		sb.WriteString("| :--- | :--- |\n")
		for _, h := range headers {
			sb.WriteString(fmt.Sprintf("| `%s` | `%s` |\n", h.Key, h.Value))
		}
		sb.WriteString("\n")
	}

	// Body
	switch req.Body.Type {
	case "json", "raw":
		if strings.TrimSpace(req.Body.Raw) != "" {
			sb.WriteString("#### Request Body\n\n```json\n")
			sb.WriteString(req.Body.Raw)
			sb.WriteString("\n```\n\n")
		}
	case "urlEncoded":
		formKVs := enabledKVs(req.Body.UrlEncoded)
		if len(formKVs) > 0 {
			sb.WriteString("#### Form URL-Encoded\n\n| Key | Value |\n| :--- | :--- |\n")
			for _, kv := range formKVs {
				sb.WriteString(fmt.Sprintf("| `%s` | `%s` |\n", kv.Key, kv.Value))
			}
			sb.WriteString("\n")
		}
	}

	// Code Samples
	sb.WriteString("#### Code Samples\n\n")
	sb.WriteString("<details><summary><b>cURL</b></summary>\n\n```bash\n")
	sb.WriteString(req.Snippets.CURL)
	sb.WriteString("\n```\n\n</details>\n\n")

	sb.WriteString("<details><summary><b>Go</b></summary>\n\n```go\n")
	sb.WriteString(req.Snippets.Go)
	sb.WriteString("\n```\n\n</details>\n\n")

	sb.WriteString("<details><summary><b>Node.js (Fetch)</b></summary>\n\n```javascript\n")
	sb.WriteString(req.Snippets.Node)
	sb.WriteString("\n```\n\n</details>\n\n")

	sb.WriteString("<details><summary><b>Python</b></summary>\n\n```python\n")
	sb.WriteString(req.Snippets.Python)
	sb.WriteString("\n```\n\n</details>\n\n")

	sb.WriteString("<details><summary><b>C#</b></summary>\n\n```csharp\n")
	sb.WriteString(req.Snippets.CSharp)
	sb.WriteString("\n```\n\n</details>\n\n")

	// Response Examples
	if len(req.Examples) > 0 {
		sb.WriteString("#### Example Responses\n\n")
		for _, ex := range req.Examples {
			name := ex.Name
			if name == "" {
				name = fmt.Sprintf("%d %s", ex.StatusCode, ex.StatusText)
			}
			sb.WriteString(fmt.Sprintf("##### %s (`%d %s`)\n\n", name, ex.StatusCode, ex.StatusText))
			if len(ex.Headers) > 0 {
				sb.WriteString("<details><summary>Response Headers</summary>\n\n")
				sb.WriteString("| Header | Value |\n| :--- | :--- |\n")
				for _, h := range ex.Headers {
					sb.WriteString(fmt.Sprintf("| `%s` | `%s` |\n", h.Key, h.Value))
				}
				sb.WriteString("\n</details>\n\n")
			}
			if ex.Body != "" {
				lang := "json"
				if !strings.Contains(ex.ContentType, "json") && !strings.HasPrefix(strings.TrimSpace(ex.Body), "{") && !strings.HasPrefix(strings.TrimSpace(ex.Body), "[") {
					lang = "text"
				}
				sb.WriteString(fmt.Sprintf("```%s\n%s\n```\n\n", lang, ex.Body))
			}
		}
	}

	sb.WriteString("---\n\n")
}
