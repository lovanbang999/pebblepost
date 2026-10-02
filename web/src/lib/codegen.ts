import type { RequestDefinition, KeyValue } from '../types'

// Escape a shell argument (single-quote safe)
function shellEscape(val: string): string {
  return val.replace(/'/g, "'\\''")
}

// Filter only enabled KV pairs
function enabled(kvs: KeyValue[] | undefined): KeyValue[] {
  return (kvs || []).filter((kv) => kv.enabled && kv.key.trim() !== '')
}

// Build query string from params
function buildQueryString(req: RequestDefinition): string {
  const params = enabled(req.params)
  if (!params.length) return ''
  return '?' + params.map((p) => `${encodeURIComponent(p.key)}=${encodeURIComponent(p.value)}`).join('&')
}

// Build full URL with params
function fullUrl(req: RequestDefinition): string {
  return req.url + buildQueryString(req)
}

// Collect all effective headers including auth header
function allHeaders(req: RequestDefinition): KeyValue[] {
  const headers = [...enabled(req.headers)]
  if (req.auth.type === 'bearer') {
    headers.unshift({ key: 'Authorization', value: `Bearer ${req.auth.token || ''}`, enabled: true })
  } else if (req.auth.type === 'basic') {
    const encoded = btoa(`${req.auth.username || ''}:${req.auth.password || ''}`)
    headers.unshift({ key: 'Authorization', value: `Basic ${encoded}`, enabled: true })
  } else if (req.auth.type === 'apiKey' && req.auth.addTo === 'header') {
    headers.unshift({ key: req.auth.key || 'X-API-Key', value: req.auth.value || '', enabled: true })
  }
  return headers
}

// ─── cURL ────────────────────────────────────────────────────────────────────

export function generateCURL(req: RequestDefinition): string {
  const lines: string[] = [`curl -X ${req.method} '${shellEscape(fullUrl(req))}'`]

  for (const h of allHeaders(req)) {
    lines.push(`  -H '${shellEscape(h.key)}: ${shellEscape(h.value)}'`)
  }

  if (req.body.type === 'json' || req.body.type === 'raw') {
    lines.push(`  -H 'Content-Type: application/json'`)
    lines.push(`  --data-raw '${shellEscape(req.body.raw || '')}'`)
  } else if (req.body.type === 'urlEncoded') {
    const parts = enabled(req.body.urlEncoded).map((kv) => `${kv.key}=${kv.value}`)
    lines.push(`  --data-urlencode '${shellEscape(parts.join('&'))}'`)
  } else if (req.body.type === 'formData') {
    for (const kv of enabled(req.body.formData)) {
      if (kv.type === 'file') {
        lines.push(`  -F '${shellEscape(kv.key)}=@${shellEscape(kv.value)}'`)
      } else {
        lines.push(`  -F '${shellEscape(kv.key)}=${shellEscape(kv.value)}'`)
      }
    }
  }

  if (!req.settings.verifySSL) lines.push('  --insecure')
  if (!req.settings.followRedirects) lines.push('  --no-location')

  return lines.join(' \\\n')
}

// ─── Go net/http ─────────────────────────────────────────────────────────────

export function generateGo(req: RequestDefinition): string {
  const headers = allHeaders(req)
  const url = fullUrl(req)

  let bodySetup = ''
  let bodyArg = 'nil'

  if (req.body.type === 'json' || req.body.type === 'raw') {
    bodySetup = `\tbody := strings.NewReader(\`${req.body.raw || ''}\`)\n`
    bodyArg = 'body'
  }

  const headerLines = headers
    .map((h) => `\treq.Header.Set("${h.key}", "${h.value}")`)
    .join('\n')

  const clientTimeout = req.settings.timeoutMs
    ? `\tclient := &http.Client{Timeout: ${req.settings.timeoutMs / 1000} * time.Second}`
    : '\tclient := &http.Client{}'

  return `package main

import (
\t"fmt"
\t"io"
\t"net/http"
\t"strings"
\t"time"
)

func main() {
${bodySetup}\treq, err := http.NewRequest("${req.method}", "${url}", ${bodyArg})
\tif err != nil {
\t\tpanic(err)
\t}

${headerLines}

${clientTimeout}
\tresp, err := client.Do(req)
\tif err != nil {
\t\tpanic(err)
\t}
\tdefer resp.Body.Close()

\tbody2, _ := io.ReadAll(resp.Body)
\tfmt.Printf("Status: %s\\n%s\\n", resp.Status, body2)
}`
}

// ─── Node.js fetch ────────────────────────────────────────────────────────────

export function generateNodeFetch(req: RequestDefinition): string {
  const headers = allHeaders(req)
  const url = fullUrl(req)

  const headersObj = Object.fromEntries(headers.map((h) => [h.key, h.value]))

  let bodyPart = ''
  if (req.body.type === 'json' || req.body.type === 'raw') {
    bodyPart = `\n  body: \`${(req.body.raw || '').replace(/`/g, '\\`')}\`,`
    headersObj['Content-Type'] = 'application/json'
  }

  const headersStr = JSON.stringify(headersObj, null, 2).replace(/^/gm, '  ')

  return `// Node.js 18+ fetch (built-in)
const response = await fetch("${url}", {
  method: "${req.method}",
  headers: ${headersStr},${bodyPart}
});

const data = await response.json();
console.log(response.status, data);`
}

// ─── Python requests ──────────────────────────────────────────────────────────

export function generatePython(req: RequestDefinition): string {
  const headers = allHeaders(req)
  const url = fullUrl(req)

  const headersStr = headers
    .map((h) => `    "${h.key}": "${h.value}"`)
    .join(',\n')

  let bodySection = ''
  if (req.body.type === 'json' || req.body.type === 'raw') {
    bodySection = `\ndata = """
${req.body.raw || ''}
"""\n\nresponse = requests.${req.method.toLowerCase()}(url, headers=headers, data=data)`
  } else if (req.body.type === 'urlEncoded') {
    const parts = Object.fromEntries(enabled(req.body.urlEncoded).map((kv) => [kv.key, kv.value]))
    bodySection = `\npayload = ${JSON.stringify(parts, null, 2)}\n\nresponse = requests.${req.method.toLowerCase()}(url, headers=headers, data=payload)`
  } else if (req.body.type === 'formData') {
    const parts = Object.fromEntries(enabled(req.body.formData).map((kv) => [kv.key, kv.value]))
    bodySection = `\nfiles = ${JSON.stringify(parts, null, 2)}\n\nresponse = requests.${req.method.toLowerCase()}(url, headers=headers, files=files)`
  } else {
    bodySection = `\n\nresponse = requests.${req.method.toLowerCase()}(url, headers=headers)`
  }

  return `import requests

url = "${url}"
headers = {
${headersStr}
}
${bodySection}

print(response.status_code)
print(response.json())`
}

// ─── C# HttpClient ────────────────────────────────────────────────────────────

export function generateCSharp(req: RequestDefinition): string {
  const headers = allHeaders(req)
  const url = fullUrl(req)

  const headerLines = headers
    .map((h) => `        client.DefaultRequestHeaders.Add("${h.key}", "${h.value}");`)
    .join('\n')

  let bodySetup = ''
  let sendCall = `await client.${toPascal(req.method)}Async(url);`

  if (req.body.type === 'json' || req.body.type === 'raw') {
    bodySetup = `        var content = new StringContent(@"${(req.body.raw || '').replace(/"/g, '""')}", System.Text.Encoding.UTF8, "application/json");\n`
    sendCall = `await client.${toPascal(req.method)}Async(url, content);`
  } else if (req.body.type === 'urlEncoded') {
    const pairs = enabled(req.body.urlEncoded)
      .map((kv) => `            { "${kv.key}", "${kv.value}" }`)
      .join(',\n')
    bodySetup = `        var content = new FormUrlEncodedContent(new Dictionary<string, string>\n        {\n${pairs}\n        });\n`
    sendCall = `await client.${toPascal(req.method)}Async(url, content);`
  }

  return `using System.Net.Http;
using System.Threading.Tasks;

var client = new HttpClient();
var url = "${url}";

${headerLines}

${bodySetup}var response = ${sendCall}
var responseBody = await response.Content.ReadAsStringAsync();

Console.WriteLine($"Status: {(int)response.StatusCode}");
Console.WriteLine(responseBody);`
}

function toPascal(method: string): string {
  return method.charAt(0).toUpperCase() + method.slice(1).toLowerCase()
}

// ─── Export all generators ────────────────────────────────────────────────────

export const CODE_GENERATORS: Record<string, { label: string; language: string; fn: (req: RequestDefinition) => string }> = {
  curl:   { label: 'cURL',         language: 'shell',      fn: generateCURL },
  go:     { label: 'Go',           language: 'go',         fn: generateGo },
  node:   { label: 'Node.js',      language: 'javascript', fn: generateNodeFetch },
  python: { label: 'Python',       language: 'python',     fn: generatePython },
  csharp: { label: 'C# HttpClient', language: 'csharp',   fn: generateCSharp },
}
