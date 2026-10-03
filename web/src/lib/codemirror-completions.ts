import { CompletionContext, CompletionResult, snippetCompletion } from '@codemirror/autocomplete'

// Autocomplete definitions for PebblePost Scripting API
const PB_COMPLETIONS = [
  // Top-level namespaces
  { label: 'pb', type: 'variable', detail: 'PebblePost global API object' },
  { label: 'pm', type: 'variable', detail: 'Postman compatibility alias' },
  { label: 'console', type: 'variable', detail: 'Console logger (log, info, warn, error)' },

  // pb.test
  snippetCompletion('pb.test("${1:Test name}", function() {\n\t${2:// assertions}\n});', {
    label: 'pb.test',
    detail: 'Define a test case assertion block',
    type: 'function',
  }),

  // pb.expect assertions
  snippetCompletion('pb.expect(${1:actual}).to.have.status(${2:200});', {
    label: 'pb.expect.status',
    detail: 'Assert response status code (e.g. 200)',
    type: 'function',
  }),
  snippetCompletion('pb.expect(${1:actual}).to.equal(${2:expected});', {
    label: 'pb.expect.equal',
    detail: 'Assert strict equality',
    type: 'function',
  }),
  snippetCompletion('pb.expect(${1:actual}).to.deep.equal(${2:expected});', {
    label: 'pb.expect.deep.equal',
    detail: 'Assert deep structural equality',
    type: 'function',
  }),
  snippetCompletion('pb.expect(${1:actual}).to.have.property("${2:propName}");', {
    label: 'pb.expect.property',
    detail: 'Assert object property exists',
    type: 'function',
  }),
  snippetCompletion('pb.expect(${1:actual}).to.have.length(${2:length});', {
    label: 'pb.expect.length',
    detail: 'Assert array or string length',
    type: 'function',
  }),
  snippetCompletion('pb.expect(${1:actual}).to.exist;', {
    label: 'pb.expect.exist',
    detail: 'Assert value is neither null nor undefined',
    type: 'keyword',
  }),
  snippetCompletion('pb.expect(${1:actual}).to.be.oneOf([${2:items}]);', {
    label: 'pb.expect.oneOf',
    detail: 'Assert value is contained in array',
    type: 'function',
  }),
  snippetCompletion('pb.expect(${1:actual}).to.have.jsonSchema(${2:schema});', {
    label: 'pb.expect.jsonSchema',
    detail: 'Assert JSON payload matches schema',
    type: 'function',
  }),

  // pb.response
  { label: 'pb.response.status', type: 'property', detail: 'HTTP status code (e.g. 200)' },
  { label: 'pb.response.statusText', type: 'property', detail: 'HTTP status text (e.g. "OK")' },
  { label: 'pb.response.time', type: 'property', detail: 'Response total roundtrip duration in ms' },
  { label: 'pb.response.duration', type: 'property', detail: 'Response duration in ms' },
  { label: 'pb.response.size', type: 'property', detail: 'Response body size in bytes' },
  snippetCompletion('pb.response.json()', {
    label: 'pb.response.json',
    detail: 'Parse response body as JSON object',
    type: 'function',
  }),
  snippetCompletion('pb.response.text()', {
    label: 'pb.response.text',
    detail: 'Get raw response body string',
    type: 'function',
  }),
  snippetCompletion('pb.response.headers.get("${1:Content-Type}")', {
    label: 'pb.response.headers.get',
    detail: 'Get response header value',
    type: 'function',
  }),
  snippetCompletion('pb.response.headers.has("${1:Header-Name}")', {
    label: 'pb.response.headers.has',
    detail: 'Check if response header exists',
    type: 'function',
  }),

  // pb.request
  { label: 'pb.request.url', type: 'property', detail: 'Target request URL string' },
  { label: 'pb.request.method', type: 'property', detail: 'HTTP method (GET, POST, etc.)' },
  snippetCompletion('pb.request.headers.set("${1:Header}", "${2:Value}");', {
    label: 'pb.request.headers.set',
    detail: 'Set or overwrite a request header',
    type: 'function',
  }),
  snippetCompletion('pb.request.headers.add("${1:Header}", "${2:Value}");', {
    label: 'pb.request.headers.add',
    detail: 'Append a new request header',
    type: 'function',
  }),
  snippetCompletion('pb.request.headers.get("${1:Header}")', {
    label: 'pb.request.headers.get',
    detail: 'Read outgoing request header',
    type: 'function',
  }),
  snippetCompletion('pb.request.headers.remove("${1:Header}");', {
    label: 'pb.request.headers.remove',
    detail: 'Remove outgoing request header',
    type: 'function',
  }),
  snippetCompletion('pb.request.body.setRaw(${1:bodyString});', {
    label: 'pb.request.body.setRaw',
    detail: 'Overwrite raw request body string',
    type: 'function',
  }),

  // pb.environment
  snippetCompletion('pb.environment.get("${1:VAR_NAME}")', {
    label: 'pb.environment.get',
    detail: 'Read active workspace environment variable',
    type: 'function',
  }),
  snippetCompletion('pb.environment.set("${1:VAR_NAME}", "${2:value}");', {
    label: 'pb.environment.set',
    detail: 'Set persistent workspace environment variable',
    type: 'function',
  }),
  snippetCompletion('pb.environment.has("${1:VAR_NAME}")', {
    label: 'pb.environment.has',
    detail: 'Check if environment variable exists',
    type: 'function',
  }),
  snippetCompletion('pb.environment.unset("${1:VAR_NAME}");', {
    label: 'pb.environment.unset',
    detail: 'Delete workspace environment variable',
    type: 'function',
  }),

  // pb.variables (run-local)
  snippetCompletion('pb.variables.get("${1:VAR_NAME}")', {
    label: 'pb.variables.get',
    detail: 'Read run-local variable',
    type: 'function',
  }),
  snippetCompletion('pb.variables.set("${1:VAR_NAME}", "${2:value}");', {
    label: 'pb.variables.set',
    detail: 'Set run-local execution variable',
    type: 'function',
  }),
  snippetCompletion('pb.variables.has("${1:VAR_NAME}")', {
    label: 'pb.variables.has',
    detail: 'Check if run-local variable exists',
    type: 'function',
  }),
  snippetCompletion('pb.variables.unset("${1:VAR_NAME}");', {
    label: 'pb.variables.unset',
    detail: 'Unset run-local variable',
    type: 'function',
  }),

  // pb.uuid
  snippetCompletion('pb.uuid()', {
    label: 'pb.uuid',
    detail: 'Generate random RFC 4122 v4 UUID string',
    type: 'function',
  }),

  // pb.base64
  snippetCompletion('pb.base64.encode(${1:string})', {
    label: 'pb.base64.encode',
    detail: 'Base64 encode a string',
    type: 'function',
  }),
  snippetCompletion('pb.base64.decode(${1:base64String})', {
    label: 'pb.base64.decode',
    detail: 'Decode Base64 string',
    type: 'function',
  }),

  // pb.hash
  snippetCompletion('pb.hash.sha256(${1:string})', {
    label: 'pb.hash.sha256',
    detail: 'Compute SHA256 hex hash',
    type: 'function',
  }),
  snippetCompletion('pb.hash.md5(${1:string})', {
    label: 'pb.hash.md5',
    detail: 'Compute MD5 hex hash',
    type: 'function',
  }),

  // pb.hmac
  snippetCompletion('pb.hmac.sha256(${1:secret}, ${2:message})', {
    label: 'pb.hmac.sha256',
    detail: 'Compute HMAC-SHA256 signature',
    type: 'function',
  }),

  // pb.jwt
  snippetCompletion('pb.jwt.decode(${1:jwtToken})', {
    label: 'pb.jwt.decode',
    detail: 'Decode unverified JWT header and payload claims',
    type: 'function',
  }),

  // pb.date
  snippetCompletion('pb.date.now()', {
    label: 'pb.date.now',
    detail: 'Current Unix timestamp in milliseconds',
    type: 'function',
  }),
  snippetCompletion('pb.date.nowISO()', {
    label: 'pb.date.nowISO',
    detail: 'Current UTC time in ISO format',
    type: 'function',
  }),
  snippetCompletion('pb.date.format(${1:dateVal}, "${2:YYYY-MM-DD}")', {
    label: 'pb.date.format',
    detail: 'Format date with layout tokens',
    type: 'function',
  }),
  snippetCompletion('pb.date.add(${1:dateVal}, ${2:amount}, "${3:days}")', {
    label: 'pb.date.add',
    detail: 'Add duration to date (units: ms, s, m, h, d)',
    type: 'function',
  }),

  // pb.random
  snippetCompletion('pb.random.int(${1:min}, ${2:max})', {
    label: 'pb.random.int',
    detail: 'Generate random integer between min and max',
    type: 'function',
  }),
  snippetCompletion('pb.random.string(${1:16})', {
    label: 'pb.random.string',
    detail: 'Generate random alphanumeric string of length N',
    type: 'function',
  }),

  // pb.sendRequest
  snippetCompletion(
    'pb.sendRequest({\n\turl: "${1:https://api.example.com}",\n\tmethod: "${2:GET}",\n\theaders: { ${3} },\n\ttimeoutMs: 5000\n});',
    {
      label: 'pb.sendRequest',
      detail: 'Synchronous auxiliary HTTP request',
      type: 'function',
    }
  ),

  // Console methods
  snippetCompletion('console.log(${1:message});', {
    label: 'console.log',
    detail: 'Log message to script console',
    type: 'function',
  }),
  snippetCompletion('console.info(${1:message});', {
    label: 'console.info',
    detail: 'Log info message to script console',
    type: 'function',
  }),
  snippetCompletion('console.warn(${1:message});', {
    label: 'console.warn',
    detail: 'Log warning message to script console',
    type: 'function',
  }),
  snippetCompletion('console.error(${1:message});', {
    label: 'console.error',
    detail: 'Log error message to script console',
    type: 'function',
  }),
]

/**
 * Custom CodeMirror CompletionSource for PebblePost scripts.
 */
export function pebbleScriptCompletions(context: CompletionContext): CompletionResult | null {
  const word = context.matchBefore(/[\w.]*/)
  if (!word || (word.from === word.to && !context.explicit)) {
    return null
  }

  return {
    from: word.from,
    options: PB_COMPLETIONS,
    validFor: /^[\w.]*$/,
  }
}
