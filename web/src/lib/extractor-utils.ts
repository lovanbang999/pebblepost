import { JSONPath } from 'jsonpath-plus'

/**
 * Constructs a normalized JSONPath string from an array of object keys and array indices.
 * Example: ['data', 'items', 0, 'id'] -> '$.data.items[0].id'
 * Handles special characters by wrapping keys in bracket notation: ['data', 'user-name'] -> '$.data["user-name"]'
 */
export function buildJsonPathFromKeyPath(segments: (string | number)[]): string {
  if (!segments || segments.length === 0) {
    return '$'
  }

  let result = '$'
  for (const seg of segments) {
    if (typeof seg === 'number') {
      result += `[${seg}]`
    } else {
      const key = String(seg)
      // Check if key is a simple JS identifier
      if (/^[a-zA-Z_$][a-zA-Z0-9_$]*$/.test(key)) {
        result += `.${key}`
      } else {
        result += `[${JSON.stringify(key)}]`
      }
    }
  }

  return result
}

/**
 * Suggests an idiomatic variable name based on the target path segments.
 * Example: ['users', 0, 'id'] -> 'id' or 'user_id'
 */
export function suggestVariableName(segments: (string | number)[]): string {
  if (!segments || segments.length === 0) {
    return 'extracted_val'
  }

  const last = segments[segments.length - 1]
  if (typeof last === 'string' && last.trim() !== '') {
    // Sanitize non-alphanumeric chars to underscores
    const clean = last.trim().replace(/[^a-zA-Z0-9_]/g, '_')
    if (clean) return clean
  }

  // If last was index (number), inspect parent segment
  if (segments.length >= 2) {
    const parent = segments[segments.length - 2]
    if (typeof parent === 'string') {
      const singular = parent.endsWith('s') ? parent.slice(0, -1) : parent
      return `${singular}_item`
    }
  }

  return 'var_' + Date.now().toString(36).slice(-4)
}

/**
 * Evaluates a JSONPath query against a JSON string in the browser using jsonpath-plus.
 */
export function evaluateJsonPath(
  jsonString: string,
  path: string
): { success: boolean; value: string; raw: unknown; warning?: string } {
  const trimmed = jsonString.trim()
  if (!trimmed) {
    return { success: false, value: '', raw: null, warning: 'Response body is empty' }
  }

  let parsed: unknown
  try {
    parsed = JSON.parse(trimmed)
  } catch (e) {
    return {
      success: false,
      value: '',
      raw: null,
      warning: `Response is not valid JSON: ${e instanceof Error ? e.message : String(e)}`,
    }
  }

  try {
    const results = JSONPath({ path, json: parsed as object })
    if (results === undefined || results === null || (Array.isArray(results) && results.length === 0)) {
      return { success: false, value: '', raw: null, warning: `JSONPath "${path}" not found in response` }
    }

    // JSONPath typically wraps results in an array
    const target = Array.isArray(results) && results.length === 1 ? results[0] : results

    let formatted: string
    if (target === null || target === undefined) {
      formatted = ''
    } else if (typeof target === 'string') {
      formatted = target
    } else if (typeof target === 'number' || typeof target === 'boolean') {
      formatted = String(target)
    } else {
      formatted = JSON.stringify(target)
    }

    return { success: true, value: formatted, raw: target }
  } catch (e) {
    return {
      success: false,
      value: '',
      raw: null,
      warning: `Invalid JSONPath expression: ${e instanceof Error ? e.message : String(e)}`,
    }
  }
}
