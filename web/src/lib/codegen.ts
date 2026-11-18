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

// Build GraphQL JSON body payload string
function buildGraphQLPayload(req: RequestDefinition): string {
  const payload: Record<string, unknown> = {
    query: req.body.graphql?.query || '',
  }
  if (req.body.graphql?.variables?.trim()) {
    try {
      payload.variables = JSON.parse(req.body.graphql.variables)
    } catch {
      payload.variables = req.body.graphql.variables
    }
  }
  if (req.body.graphql?.operationName?.trim()) {
    payload.operationName = req.body.graphql.operationName.trim()
  }
  return JSON.stringify(payload)
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
  } else if (req.body.type === 'graphql') {
    lines.push(`  -H 'Content-Type: application/json'`)
    lines.push(`  --data-raw '${shellEscape(buildGraphQLPayload(req))}'`)
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

// ─── Rust reqwest ────────────────────────────────────────────────────────────

function rustRawString(value: string): string {
  let hashes = '#'
  while (value.includes(`"${hashes}`)) hashes += '#'
  return `r${hashes}"${value}"${hashes}`
}

export function generateRust(req: RequestDefinition): string {
  const headers = allHeaders(req)
  if (req.body.type === 'graphql' && !headers.some((h) => h.key.toLowerCase() === 'content-type')) {
    headers.unshift({ key: 'Content-Type', value: 'application/json', enabled: true })
  }

  const clientOptions = [
    req.settings.timeoutMs ? `.timeout(std::time::Duration::from_millis(${req.settings.timeoutMs}))` : '',
    req.settings.verifySSL ? '' : '.danger_accept_invalid_certs(true)',
    req.settings.followRedirects ? '' : '.redirect(reqwest::redirect::Policy::none())',
  ].filter(Boolean).join('\n        ')
  const headerCalls = headers
    .map((header) => `    request = request.header(${rustRawString(header.key)}, ${rustRawString(header.value)});`)
    .join('\n')

  let bodySetup = ''
  let bodyCall = ''
  if (req.body.type === 'json' || req.body.type === 'raw') {
    bodyCall = `request = request.body(${rustRawString(req.body.raw || '')});`
  } else if (req.body.type === 'graphql') {
    bodyCall = `request = request.body(${rustRawString(buildGraphQLPayload(req))});`
  } else if (req.body.type === 'urlEncoded') {
    const pairs = enabled(req.body.urlEncoded)
      .map((pair) => `        (${rustRawString(pair.key)}, ${rustRawString(pair.value)})`)
      .join(',\n')
    bodyCall = `request = request.form(&[\n${pairs}\n    ]);`
  } else if (req.body.type === 'formData') {
    const fields = enabled(req.body.formData)
    const lines = fields.map((field) => {
      if (field.type === 'file') {
        const fileName = field.value.split(/[\\/]/).pop() || 'upload.bin'
        return `form = form.part(${rustRawString(field.key)}, reqwest::multipart::Part::bytes(std::fs::read(${rustRawString(field.value)})?).file_name(${rustRawString(fileName)}));`
      }
      return `form = form.text(${rustRawString(field.key)}, ${rustRawString(field.value)});`
    })
    bodySetup = `    let mut form = reqwest::multipart::Form::new();\n${lines.map((line) => `    ${line}`).join('\n')}\n`
    bodyCall = 'request = request.multipart(form);'
  } else if (req.body.type === 'file') {
    bodyCall = `request = request.body(std::fs::read(${rustRawString(req.body.filePath || '')})?);`
  }

  return `use reqwest::Client;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let client = Client::builder()
        ${clientOptions || ''}
        .build()?;
    let mut request = client.request(
        reqwest::Method::from_bytes(${rustRawString(req.method)}.as_bytes())?,
        ${rustRawString(fullUrl(req))},
    );
${headerCalls}
${bodySetup}${bodyCall ? `    ${bodyCall}\n` : ''}    let response = request.send().await?;
    println!("Status: {}", response.status());
    println!("{}", response.text().await?);
    Ok(())
}

// Add reqwest (features: json, multipart, rustls-tls) and tokio (features: macros, rt-multi-thread) to Cargo.toml.`
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
  } else if (req.body.type === 'graphql') {
    bodyPart = `\n  body: JSON.stringify(${buildGraphQLPayload(req)}),`
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

export function generatePython(req: RequestDefinition): string {
  const headers = allHeaders(req)
  const url = fullUrl(req)

  const effectiveHeaders = [...headers]
  if (req.body.type === 'graphql' && !effectiveHeaders.some((h) => h.key.toLowerCase() === 'content-type')) {
    effectiveHeaders.unshift({ key: 'Content-Type', value: 'application/json', enabled: true })
  }

  const headersStr = effectiveHeaders
    .map((h) => `    "${h.key}": "${h.value}"`)
    .join(',\n')

  let bodySection = ''
  if (req.body.type === 'json' || req.body.type === 'raw') {
    bodySection = `\ndata = """
${req.body.raw || ''}
"""\n\nresponse = requests.${req.method.toLowerCase()}(url, headers=headers, data=data)`
  } else if (req.body.type === 'graphql') {
    bodySection = `\njson_data = ${buildGraphQLPayload(req)}\n\nresponse = requests.${req.method.toLowerCase()}(url, headers=headers, json=json_data)`
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
  } else if (req.body.type === 'graphql') {
    bodySetup = `        var content = new StringContent(@"${buildGraphQLPayload(req).replace(/"/g, '""')}", System.Text.Encoding.UTF8, "application/json");\n`
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
  rust:   { label: 'Rust reqwest', language: 'rust',      fn: generateRust },
  node:   { label: 'Node.js',      language: 'javascript', fn: generateNodeFetch },
  python: { label: 'Python',       language: 'python',     fn: generatePython },
  csharp: { label: 'C# HttpClient', language: 'csharp',   fn: generateCSharp },
}
