package docs

import (
	"fmt"
	"html"
	"strings"
)

// GenerateHTML generates an offline-ready, standalone, modern HTML documentation page.
func GenerateHTML(col *DocCollection) string {
	title := html.EscapeString(col.Title)
	if title == "" {
		title = "API Documentation"
	}
	desc := html.EscapeString(col.Description)

	var sidebarItems strings.Builder
	var contentItems strings.Builder

	writeHTMLTree(&sidebarItems, &contentItems, col.Items, 0)

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>%s - PebblePost</title>
  <style>
    :root {
      --bg: #ffffff;
      --fg: #09090b;
      --muted: #71717a;
      --border: #e4e4e7;
      --sidebar-bg: #f4f4f5;
      --card-bg: #ffffff;
      --code-bg: #18181b;
      --code-fg: #f4f4f5;
      --accent: #2563eb;
      --accent-hover: #1d4ed8;
      --font-sans: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
      --font-mono: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
    }
    @media (prefers-color-scheme: dark) {
      :root {
        --bg: #09090b;
        --fg: #fafafa;
        --muted: #a1a1aa;
        --border: #27272a;
        --sidebar-bg: #111113;
        --card-bg: #18181b;
        --code-bg: #000000;
        --code-fg: #f4f4f5;
        --accent: #3b82f6;
        --accent-hover: #60a5fa;
      }
    }
    [data-theme="light"] {
      --bg: #ffffff;
      --fg: #09090b;
      --muted: #71717a;
      --border: #e4e4e7;
      --sidebar-bg: #f4f4f5;
      --card-bg: #ffffff;
      --code-bg: #18181b;
      --code-fg: #f4f4f5;
      --accent: #2563eb;
    }
    [data-theme="dark"] {
      --bg: #09090b;
      --fg: #fafafa;
      --muted: #a1a1aa;
      --border: #27272a;
      --sidebar-bg: #111113;
      --card-bg: #18181b;
      --code-bg: #000000;
      --code-fg: #f4f4f5;
      --accent: #3b82f6;
    }
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      font-family: var(--font-sans);
      background-color: var(--bg);
      color: var(--fg);
      display: flex;
      height: 100vh;
      overflow: hidden;
      line-height: 1.5;
    }
    /* Sidebar */
    .sidebar {
      width: 300px;
      background-color: var(--sidebar-bg);
      border-right: 1px solid var(--border);
      display: flex;
      flex-direction: column;
      flex-shrink: 0;
    }
    .sidebar-header {
      padding: 16px;
      border-bottom: 1px solid var(--border);
    }
    .sidebar-title {
      font-size: 16px;
      font-weight: 700;
      margin-bottom: 4px;
    }
    .sidebar-desc {
      font-size: 12px;
      color: var(--muted);
    }
    .search-box {
      padding: 12px 16px;
      border-bottom: 1px solid var(--border);
    }
    .search-input {
      width: 100%%;
      padding: 6px 10px;
      border: 1px solid var(--border);
      border-radius: 6px;
      background: var(--card-bg);
      color: var(--fg);
      font-size: 13px;
    }
    .sidebar-nav {
      flex: 1;
      overflow-y: auto;
      padding: 12px 8px;
    }
    .nav-folder {
      font-size: 12px;
      font-weight: 600;
      color: var(--muted);
      text-transform: uppercase;
      letter-spacing: 0.05em;
      padding: 8px 8px 4px 8px;
      margin-top: 8px;
    }
    .nav-link {
      display: flex;
      align-items: center;
      gap: 8px;
      padding: 6px 8px;
      border-radius: 6px;
      text-decoration: none;
      color: var(--fg);
      font-size: 13px;
      transition: background 0.15s;
    }
    .nav-link:hover {
      background-color: rgba(125, 125, 125, 0.1);
    }
    .theme-toggle-container {
      padding: 12px 16px;
      border-top: 1px solid var(--border);
      display: flex;
      justify-content: space-between;
      align-items: center;
      font-size: 12px;
    }
    .theme-btn {
      background: none;
      border: 1px solid var(--border);
      padding: 4px 8px;
      border-radius: 4px;
      color: var(--fg);
      cursor: pointer;
    }
    /* Main Content */
    .content-area {
      flex: 1;
      overflow-y: auto;
      padding: 32px 48px;
      max-width: 1000px;
    }
    .doc-section {
      margin-bottom: 48px;
      border-bottom: 1px solid var(--border);
      padding-bottom: 32px;
    }
    .section-title {
      font-size: 24px;
      font-weight: 700;
      margin-bottom: 8px;
    }
    .endpoint-header {
      display: flex;
      align-items: center;
      gap: 12px;
      margin-bottom: 12px;
    }
    .endpoint-url {
      font-family: var(--font-mono);
      font-size: 14px;
      background: var(--sidebar-bg);
      padding: 6px 12px;
      border-radius: 6px;
      border: 1px solid var(--border);
      display: inline-block;
      margin-bottom: 16px;
    }
    .badge {
      font-family: var(--font-mono);
      font-size: 11px;
      font-weight: 700;
      padding: 2px 6px;
      border-radius: 4px;
      text-transform: uppercase;
    }
    .badge-get { background: #dcfce7; color: #15803d; }
    .badge-post { background: #dbeafe; color: #1d4ed8; }
    .badge-put { background: #fef9c3; color: #a16207; }
    .badge-patch { background: #f3e8ff; color: #7e22ce; }
    .badge-delete { background: #fee2e2; color: #b91c1c; }
    .badge-grpc { background: #fce7f3; color: #be185d; }
    .badge-ws { background: #e0e7ff; color: #4338ca; }
    .table {
      width: 100%%;
      border-collapse: collapse;
      margin: 16px 0;
      font-size: 13px;
    }
    .table th, .table td {
      border: 1px solid var(--border);
      padding: 8px 12px;
      text-align: left;
    }
    .table th {
      background: var(--sidebar-bg);
      font-weight: 600;
    }
    /* Tabs & Code Snippets */
    .tabs-container {
      margin: 16px 0;
      background: var(--code-bg);
      border-radius: 8px;
      border: 1px solid var(--border);
      overflow: hidden;
    }
    .tab-header {
      display: flex;
      background: rgba(255, 255, 255, 0.05);
      border-bottom: 1px solid rgba(255, 255, 255, 0.1);
      padding: 0 8px;
    }
    .tab-btn {
      background: none;
      border: none;
      color: #a1a1aa;
      padding: 8px 12px;
      font-size: 12px;
      cursor: pointer;
      border-bottom: 2px solid transparent;
      font-family: var(--font-sans);
    }
    .tab-btn.active {
      color: #fff;
      border-bottom-color: var(--accent);
      font-weight: 600;
    }
    .code-wrapper {
      position: relative;
    }
    .copy-btn {
      position: absolute;
      top: 8px;
      right: 8px;
      background: rgba(255, 255, 255, 0.15);
      border: none;
      color: #fff;
      padding: 4px 8px;
      border-radius: 4px;
      font-size: 11px;
      cursor: pointer;
    }
    .copy-btn:hover { background: rgba(255, 255, 255, 0.25); }
    pre {
      padding: 16px;
      overflow-x: auto;
      font-family: var(--font-mono);
      font-size: 12.5px;
      color: var(--code-fg);
      line-height: 1.45;
    }
  </style>
</head>
<body>
  <div class="sidebar">
    <div class="sidebar-header">
      <div class="sidebar-title">%s</div>
      <div class="sidebar-desc">%s</div>
    </div>
    <div class="search-box">
      <input type="text" class="search-input" id="search" placeholder="Search endpoints..." onkeyup="filterNav()">
    </div>
    <div class="sidebar-nav" id="sidebar-nav">
%s
    </div>
    <div class="theme-toggle-container">
      <span>Theme</span>
      <button class="theme-btn" onclick="toggleTheme()">Toggle Dark/Light</button>
    </div>
  </div>

  <div class="content-area">
%s
  </div>

  <script>
    function toggleTheme() {
      const current = document.documentElement.getAttribute('data-theme');
      const next = current === 'dark' ? 'light' : 'dark';
      document.documentElement.setAttribute('data-theme', next);
    }
    function filterNav() {
      const q = document.getElementById('search').value.toLowerCase();
      const links = document.querySelectorAll('.nav-link');
      links.forEach(l => {
        const text = l.innerText.toLowerCase();
        l.style.display = text.includes(q) ? 'flex' : 'none';
      });
    }
    function selectTab(btn, targetId) {
      const container = btn.closest('.tabs-container');
      container.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
      container.querySelectorAll('.tab-pane').forEach(p => p.style.display = 'none');
      btn.classList.add('active');
      const target = container.querySelector('#' + targetId);
      if (target) target.style.display = 'block';
    }
    function copySnippet(btn) {
      const pre = btn.nextElementSibling;
      if (pre) {
        navigator.clipboard.writeText(pre.innerText).then(() => {
          const orig = btn.innerText;
          btn.innerText = 'Copied!';
          setTimeout(() => btn.innerText = orig, 1500);
        });
      }
    }
  </script>
</body>
</html>`, title, title, desc, sidebarItems.String(), contentItems.String())
}

func getMethodClass(method string) string {
	switch strings.ToUpper(method) {
	case "GET":
		return "badge-get"
	case "POST":
		return "badge-post"
	case "PUT":
		return "badge-put"
	case "PATCH":
		return "badge-patch"
	case "DELETE":
		return "badge-delete"
	case "GRPC":
		return "badge-grpc"
	case "WS", "SSE":
		return "badge-ws"
	default:
		return "badge-get"
	}
}

func writeHTMLTree(sidebar, content *strings.Builder, items []*DocItem, depth int) {
	for _, item := range items {
		if item.IsFolder && item.Folder != nil {
			folder := item.Folder
			sidebar.WriteString(fmt.Sprintf("<div class=\"nav-folder\">%s</div>\n", html.EscapeString(folder.Name)))
			writeHTMLTree(sidebar, content, folder.Items, depth+1)
		} else if !item.IsFolder && item.Request != nil {
			req := item.Request
			method := req.Method
			if method == "" {
				method = "GET"
			}
			badgeCls := getMethodClass(method)
			id := slugify(fmt.Sprintf("%s-%s", method, req.Name))

			// Sidebar link
			sidebar.WriteString(fmt.Sprintf("<a href=\"#%s\" class=\"nav-link\"><span class=\"badge %s\">%s</span><span>%s</span></a>\n",
				id, badgeCls, method, html.EscapeString(req.Name)))

			// Content section
			content.WriteString(fmt.Sprintf("<div class=\"doc-section\" id=\"%s\">\n", id))
			content.WriteString(fmt.Sprintf("<div class=\"endpoint-header\"><span class=\"badge %s\">%s</span><h2 class=\"section-title\">%s</h2></div>\n",
				badgeCls, method, html.EscapeString(req.Name)))

			if req.Description != "" {
				content.WriteString(fmt.Sprintf("<p style=\"margin-bottom: 16px; color: var(--muted);\">%s</p>\n", html.EscapeString(req.Description)))
			}

			content.WriteString(fmt.Sprintf("<div class=\"endpoint-url\">%s %s</div>\n", method, html.EscapeString(req.URL)))

			// Parameters table
			params := enabledKVs(req.Params)
			if len(params) > 0 {
				content.WriteString("<h4 style=\"margin-top: 16px;\">Query Parameters</h4>\n")
				content.WriteString("<table class=\"table\"><thead><tr><th>Parameter</th><th>Value</th></tr></thead><tbody>\n")
				for _, p := range params {
					content.WriteString(fmt.Sprintf("<tr><td><code>%s</code></td><td>%s</td></tr>\n", html.EscapeString(p.Key), html.EscapeString(p.Value)))
				}
				content.WriteString("</tbody></table>\n")
			}

			// Headers table
			headers := enabledKVs(req.Headers)
			if len(headers) > 0 {
				content.WriteString("<h4 style=\"margin-top: 16px;\">Headers</h4>\n")
				content.WriteString("<table class=\"table\"><thead><tr><th>Header</th><th>Value</th></tr></thead><tbody>\n")
				for _, h := range headers {
					content.WriteString(fmt.Sprintf("<tr><td><code>%s</code></td><td>%s</td></tr>\n", html.EscapeString(h.Key), html.EscapeString(h.Value)))
				}
				content.WriteString("</tbody></table>\n")
			}

			// Body
			if req.Body.Type == "json" || req.Body.Type == "raw" {
				if strings.TrimSpace(req.Body.Raw) != "" {
					content.WriteString("<h4 style=\"margin-top: 16px;\">Request Body</h4>\n")
					content.WriteString(fmt.Sprintf("<div class=\"tabs-container\"><pre><code>%s</code></pre></div>\n", html.EscapeString(req.Body.Raw)))
				}
			}

			// Code snippet tabs
			uid := id
			content.WriteString("<h4 style=\"margin-top: 24px;\">Code Snippets</h4>\n")
			content.WriteString("<div class=\"tabs-container\">\n")
			content.WriteString("  <div class=\"tab-header\">\n")
			content.WriteString(fmt.Sprintf("    <button class=\"tab-btn active\" onclick=\"selectTab(this, '%s_curl')\">cURL</button>\n", uid))
			content.WriteString(fmt.Sprintf("    <button class=\"tab-btn\" onclick=\"selectTab(this, '%s_go')\">Go</button>\n", uid))
			content.WriteString(fmt.Sprintf("    <button class=\"tab-btn\" onclick=\"selectTab(this, '%s_node')\">Node.js</button>\n", uid))
			content.WriteString(fmt.Sprintf("    <button class=\"tab-btn\" onclick=\"selectTab(this, '%s_python')\">Python</button>\n", uid))
			content.WriteString(fmt.Sprintf("    <button class=\"tab-btn\" onclick=\"selectTab(this, '%s_csharp')\">C#</button>\n", uid))
			content.WriteString("  </div>\n")

			writeSnippetPane(content, uid+"_curl", req.Snippets.CURL, true)
			writeSnippetPane(content, uid+"_go", req.Snippets.Go, false)
			writeSnippetPane(content, uid+"_node", req.Snippets.Node, false)
			writeSnippetPane(content, uid+"_python", req.Snippets.Python, false)
			writeSnippetPane(content, uid+"_csharp", req.Snippets.CSharp, false)

			content.WriteString("</div>\n")

			// Response Examples
			if len(req.Examples) > 0 {
				content.WriteString("<h4 style=\"margin-top: 24px;\">Saved Response Examples</h4>\n")
				for _, ex := range req.Examples {
					name := html.EscapeString(ex.Name)
					if name == "" {
						name = fmt.Sprintf("%d %s", ex.StatusCode, ex.StatusText)
					}
					statusBadge := "badge-get"
					if ex.StatusCode >= 400 {
						statusBadge = "badge-delete"
					}
					content.WriteString("<div style=\"margin: 12px 0; border: 1px solid var(--border); border-radius: 8px; padding: 12px;\">\n")
					content.WriteString(fmt.Sprintf("<div style=\"display: flex; align-items: center; gap: 8px; font-weight: 600; font-size: 13px;\"><span class=\"badge %s\">%d</span> %s</div>\n",
						statusBadge, ex.StatusCode, name))
					if ex.Body != "" {
						content.WriteString(fmt.Sprintf("<div class=\"tabs-container\" style=\"margin-top: 8px;\"><div class=\"code-wrapper\"><button class=\"copy-btn\" onclick=\"copySnippet(this)\">Copy</button><pre><code>%s</code></pre></div></div>\n", html.EscapeString(ex.Body)))
					}
					content.WriteString("</div>\n")
				}
			}

			content.WriteString("</div>\n")
		}
	}
}

func writeSnippetPane(sb *strings.Builder, paneId, code string, active bool) {
	disp := "none"
	if active {
		disp = "block"
	}
	sb.WriteString(fmt.Sprintf("  <div class=\"tab-pane code-wrapper\" id=\"%s\" style=\"display: %s;\">\n", paneId, disp))
	sb.WriteString("    <button class=\"copy-btn\" onclick=\"copySnippet(this)\">Copy</button>\n")
	sb.WriteString(fmt.Sprintf("    <pre><code>%s</code></pre>\n", html.EscapeString(code)))
	sb.WriteString("  </div>\n")
}
