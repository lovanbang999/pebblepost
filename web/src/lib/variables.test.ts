import { describe, it, expect } from "vitest";
import {
  getAvailableVariables,
  tokenizeVariables,
  findUndefinedVariablesInRequest,
  DYNAMIC_VARIABLES,
} from "./variables";
import type { EnvironmentDefinition, FolderDefinition, RequestDefinition } from "../types";

describe("Variables Utilities", () => {
  it("includes all dynamic built-in variables", () => {
    const keys = DYNAMIC_VARIABLES.map((v) => v.key);
    expect(keys).toContain("$uuid");
    expect(keys).toContain("$timestamp");
    expect(keys).toContain("$isoTimestamp");
    expect(keys).toContain("$randomInt");
    expect(keys).toContain("$randomEmail");
  });

  it("merges dynamic, environment, and folder variables with correct precedence", () => {
    const envs: EnvironmentDefinition[] = [
      {
        name: "dev",
        variables: [
          { key: "HOST", value: "dev.api.io", enabled: true, secret: false },
          { key: "API_KEY", value: "secret-token", enabled: true, secret: true },
          { key: "OVERRIDDEN", value: "env-val", enabled: true },
          { key: "DISABLED", value: "skip-me", enabled: false },
        ],
      },
    ];

    const folder: FolderDefinition = {
      name: "users",
      variables: [
        { key: "OVERRIDDEN", value: "folder-val", enabled: true },
        { key: "FOLDER_KEY", value: "users-scope", enabled: true },
      ],
    };

    const map = getAvailableVariables(envs, "dev", folder);

    // Dynamic variable present
    expect(map.get("$uuid")?.isDynamic).toBe(true);

    // Environment variables present
    expect(map.get("HOST")?.value).toBe("dev.api.io");
    expect(map.get("API_KEY")?.secret).toBe(true);
    expect(map.has("DISABLED")).toBe(false);

    // Folder variables override environment variables
    expect(map.get("OVERRIDDEN")?.value).toBe("folder-val");
    expect(map.get("OVERRIDDEN")?.source).toContain("Folder");
    expect(map.get("FOLDER_KEY")?.value).toBe("users-scope");
  });

  it("tokenizes string into literals and variable tokens with defined flags", () => {
    const map = getAvailableVariables(
      [
        {
          name: "dev",
          variables: [{ key: "NAME", value: "Alice", enabled: true }],
        },
      ],
      "dev"
    );

    const input = "Hello {{NAME}}, your balance is {{BALANCE}} and id is {{$uuid}}.";
    const tokens = tokenizeVariables(input, map);

    expect(tokens.length).toBe(7);
    expect(tokens[0]).toEqual({ type: "text", content: "Hello " });
    expect(tokens[1]).toMatchObject({
      type: "variable",
      content: "{{NAME}}",
      varKey: "NAME",
      isDefined: true,
    });
    expect(tokens[3]).toMatchObject({
      type: "variable",
      content: "{{BALANCE}}",
      varKey: "BALANCE",
      isDefined: false,
    });
    expect(tokens[5]).toMatchObject({
      type: "variable",
      content: "{{$uuid}}",
      varKey: "$uuid",
      isDefined: true,
    });
  });

  it("finds all undefined variables across request fields without duplicates", () => {
    const map = getAvailableVariables(
      [
        {
          name: "dev",
          variables: [
            { key: "BASE_URL", value: "https://api.io", enabled: true },
            { key: "AUTH_TOKEN", value: "xyz", enabled: true },
          ],
        },
      ],
      "dev"
    );

    const request = {
      url: "{{BASE_URL}}/users/{{USER_ID}}",
      headers: [
        { key: "Authorization", value: "Bearer {{AUTH_TOKEN}}", enabled: true },
        { key: "X-Trace-Id", value: "{{TRACE_ID}}", enabled: true },
        { key: "X-Disabled", value: "{{IGNORED}}", enabled: false }, // disabled, should be ignored
      ],
      params: [
        { key: "query", value: "{{SEARCH_QUERY}}", enabled: true },
      ],
      auth: {
        token: "{{AUTH_TOKEN}}",
        apiKey: "{{API_KEY}}",
      },
      body: {
        type: "json",
        raw: JSON.stringify({
          userId: "{{USER_ID}}", // duplicate
          note: "{{MISSING_BODY_VAR}}",
        }),
      },
    };

    const undefinedVars = findUndefinedVariablesInRequest(request, map);

    expect(undefinedVars).toContain("USER_ID");
    expect(undefinedVars).toContain("TRACE_ID");
    expect(undefinedVars).toContain("SEARCH_QUERY");
    expect(undefinedVars).toContain("API_KEY");
    expect(undefinedVars).toContain("MISSING_BODY_VAR");

    // Defined variables should not be in the list
    expect(undefinedVars).not.toContain("BASE_URL");
    expect(undefinedVars).not.toContain("AUTH_TOKEN");

    // Disabled header variable should not be in the list
    expect(undefinedVars).not.toContain("IGNORED");

    // Check no duplicates for USER_ID
    const userIdCount = undefinedVars.filter((v) => v === "USER_ID").length;
    expect(userIdCount).toBe(1);
  });

  it("registers extractor variables and formats their source with scope and JSONPath", () => {
    const request = {
      name: "Login Endpoint",
      extractors: [
        {
          id: "1",
          name: "SESSION_TOKEN",
          path: "$.data.token",
          scope: "runtime" as const,
          enabled: true,
        },
        {
          id: "2",
          name: "DISABLED_EXTRACTOR",
          path: "$.data.ignore",
          scope: "environment" as const,
          enabled: false,
        },
      ],
    };

    const runtimeVars = {
      SESSION_TOKEN: "live_jwt_777",
    };

    const map = getAvailableVariables([], undefined, null, request as unknown as RequestDefinition, runtimeVars);

    expect(map.has("SESSION_TOKEN")).toBe(true);
    const info = map.get("SESSION_TOKEN");
    expect(info?.value).toBe("live_jwt_777");
    expect(info?.source).toContain("Extractor (Login Endpoint)");
    expect(info?.source).toContain("runtime");
    expect(info?.source).toContain("$.data.token");

    // Disabled extractor should not be present
    expect(map.has("DISABLED_EXTRACTOR")).toBe(false);
  });
});
