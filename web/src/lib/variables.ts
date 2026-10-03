import type { AuthDefinition, EnvironmentDefinition, FolderDefinition } from "../types";

export interface VariableInfo {
  key: string;
  value: string;
  secret?: boolean;
  source: string;
  isDynamic?: boolean;
}

export const DYNAMIC_VARIABLES: VariableInfo[] = [
  {
    key: "$uuid",
    value: "Generates a random UUID v4 (e.g. 550e8400-e29b-41d4-a716-446655440000)",
    source: "Dynamic",
    isDynamic: true,
  },
  {
    key: "$timestamp",
    value: "Current Unix epoch timestamp in seconds",
    source: "Dynamic",
    isDynamic: true,
  },
  {
    key: "$isoTimestamp",
    value: "Current UTC timestamp in ISO-8601 format (e.g. 2026-10-03T15:00:00Z)",
    source: "Dynamic",
    isDynamic: true,
  },
  {
    key: "$randomInt",
    value: "Random integer between 0 and 1000",
    source: "Dynamic",
    isDynamic: true,
  },
  {
    key: "$randomEmail",
    value: "Random sample email address (e.g. user_1234@example.com)",
    source: "Dynamic",
    isDynamic: true,
  },
];

/**
 * Collects all variables available in current workspace context:
 * 1. Built-in dynamic variables ($uuid, $timestamp, etc.)
 * 2. Active environment variables (public & secret)
 * 3. Folder-scoped variables (if any)
 */
export function getAvailableVariables(
  environments: EnvironmentDefinition[],
  activeEnvName?: string,
  folder?: FolderDefinition | null
): Map<string, VariableInfo> {
  const map = new Map<string, VariableInfo>();

  // 1. Dynamic variables
  for (const dyn of DYNAMIC_VARIABLES) {
    map.set(dyn.key, dyn);
  }

  // 2. Active Environment variables
  if (activeEnvName) {
    const env = environments.find((e) => e.name === activeEnvName);
    if (env && env.variables) {
      for (const v of env.variables) {
        if (v.enabled !== false && v.key) {
          map.set(v.key, {
            key: v.key,
            value: v.value || "",
            secret: Boolean(v.secret || v.isSecret),
            source: `Environment (${env.name})`,
          });
        }
      }
    }
  }

  // 3. Folder variables (folder variables take precedence over environment variables)
  if (folder && folder.variables) {
    for (const v of folder.variables) {
      if (v.enabled !== false && v.key) {
        map.set(v.key, {
          key: v.key,
          value: v.value || "",
          secret: Boolean(v.secret),
          source: `Folder (${folder.name || "current"})`,
        });
      }
    }
  }

  return map;
}

/**
 * Regex matching {{VARIABLE}} or {{ VARIABLE }}
 */
export const VARIABLE_REGEX = /\{\{\s*([a-zA-Z0-9_$.-]+)\s*\}\}/g;

export interface TokenPart {
  type: "text" | "variable";
  content: string;
  varKey?: string;
  isDefined?: boolean;
  varInfo?: VariableInfo;
}

/**
 * Tokenizes an input string into literal text and {{VAR}} variable tokens.
 */
export function tokenizeVariables(
  input: string,
  availableMap: Map<string, VariableInfo>
): TokenPart[] {
  if (!input) return [];

  const parts: TokenPart[] = [];
  let lastIndex = 0;
  const regex = new RegExp(VARIABLE_REGEX.source, "g");
  let match: RegExpExecArray | null;

  while ((match = regex.exec(input)) !== null) {
    // Literal text before the variable match
    if (match.index > lastIndex) {
      parts.push({
        type: "text",
        content: input.slice(lastIndex, match.index),
      });
    }

    const varKey = match[1].trim();
    const varInfo = availableMap.get(varKey);

    parts.push({
      type: "variable",
      content: match[0],
      varKey,
      isDefined: Boolean(varInfo),
      varInfo,
    });

    lastIndex = regex.lastIndex;
  }

  // Trailing literal text
  if (lastIndex < input.length) {
    parts.push({
      type: "text",
      content: input.slice(lastIndex),
    });
  }

  return parts;
}

/**
 * Scans a request object for all undefined {{VAR}} instances.
 */
export function findUndefinedVariablesInRequest(
  request: {
    url?: string;
    headers?: Array<{ key: string; value: string; enabled?: boolean }>;
    params?: Array<{ key: string; value: string; enabled?: boolean }>;
    auth?: AuthDefinition | Record<string, unknown>;
    body?: { raw?: string; type?: string };
  },
  availableMap: Map<string, VariableInfo>
): string[] {
  const undefinedSet = new Set<string>();

  const checkText = (text?: string) => {
    if (!text || !text.includes("{{")) return;
    const regex = new RegExp(VARIABLE_REGEX.source, "g");
    let match: RegExpExecArray | null;
    while ((match = regex.exec(text)) !== null) {
      const key = match[1].trim();
      if (!availableMap.has(key)) {
        undefinedSet.add(key);
      }
    }
  };

  // Check URL
  checkText(request.url);

  // Check Headers
  if (request.headers) {
    for (const h of request.headers) {
      if (h.enabled !== false) {
        checkText(h.key);
        checkText(h.value);
      }
    }
  }

  // Check Query Params
  if (request.params) {
    for (const p of request.params) {
      if (p.enabled !== false) {
        checkText(p.key);
        checkText(p.value);
      }
    }
  }

  // Check Auth fields
  if (request.auth) {
    for (const val of Object.values(request.auth)) {
      if (typeof val === "string") {
        checkText(val);
      }
    }
  }

  // Check Body
  if (request.body?.raw) {
    checkText(request.body.raw);
  }

  return Array.from(undefinedSet);
}
