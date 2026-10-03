/// <reference types="node" />
import { test, describe, beforeEach } from "node:test";
import assert from "node:assert/strict";
import { useTabStore } from "./tabStore.ts";
import type { RequestDefinition } from "../types/index.ts";

// Provide minimal browser polyfills for headless node:test environment
const mockStorage: Record<string, string> = {};
globalThis.localStorage = {
  getItem: (key: string) => mockStorage[key] ?? null,
  setItem: (key: string, val: string) => {
    mockStorage[key] = val;
  },
  removeItem: (key: string) => {
    delete mockStorage[key];
  },
  clear: () => {
    for (const k of Object.keys(mockStorage)) delete mockStorage[k];
  },
  key: (i: number) => Object.keys(mockStorage)[i] ?? null,
  length: Object.keys(mockStorage).length,
};
Object.assign(globalThis, { window: globalThis });

const sampleReq: RequestDefinition = {
  schemaVersion: 1,
  id: "req-1",
  name: "Get User",
  method: "GET",
  url: "https://api.example.com/user",
  headers: [],
  params: [],
  body: { type: "none" },
  auth: { type: "none" },
  scripts: {},
  settings: { followRedirects: true, verifySSL: true, timeoutMs: 30000 },
};

const sampleReq2: RequestDefinition = {
  schemaVersion: 1,
  id: "req-2",
  name: "Create User",
  method: "POST",
  url: "https://api.example.com/user",
  headers: [],
  params: [],
  body: { type: "none" },
  auth: { type: "none" },
  scripts: {},
  settings: { followRedirects: true, verifySSL: true, timeoutMs: 30000 },
};

describe("useTabStore", () => {
  beforeEach(() => {
    useTabStore.setState({
      tabs: [],
      activeTabId: null,
      closedTabsHistory: [],
      pendingCloseTab: null,
    });
    globalThis.localStorage.clear();
  });

  test("opens a preview tab on single-click", () => {
    const store = useTabStore.getState();
    store.openTab("collections/user/get-user.pebble.json", sampleReq, true);

    const state = useTabStore.getState();
    assert.equal(state.tabs.length, 1);
    assert.equal(state.tabs[0].isPreview, true);
    assert.equal(state.tabs[0].isDirty, false);
    assert.equal(state.tabs[0].title, "Get User");
    assert.equal(state.activeTabId, "collections/user/get-user.pebble.json");
  });

  test("single-click on another request replaces the existing unpinned preview tab", () => {
    const store = useTabStore.getState();
    store.openTab("collections/user/get-user.pebble.json", sampleReq, true);
    store.openTab("collections/user/create-user.pebble.json", sampleReq2, true);

    const state = useTabStore.getState();
    assert.equal(state.tabs.length, 1);
    assert.equal(state.tabs[0].id, "collections/user/create-user.pebble.json");
    assert.equal(state.tabs[0].title, "Create User");
    assert.equal(state.tabs[0].isPreview, true);
  });

  test("double-click pins the tab and opens additional tabs without replacing", () => {
    const store = useTabStore.getState();
    store.openTab("collections/user/get-user.pebble.json", sampleReq, false);
    store.openTab(
      "collections/user/create-user.pebble.json",
      sampleReq2,
      false,
    );

    const state = useTabStore.getState();
    assert.equal(state.tabs.length, 2);
    assert.equal(state.tabs[0].isPreview, false);
    assert.equal(state.tabs[1].isPreview, false);
  });

  test("pinTab explicitly converts a preview tab to pinned", () => {
    const store = useTabStore.getState();
    store.openTab("collections/user/get-user.pebble.json", sampleReq, true);
    store.pinTab("collections/user/get-user.pebble.json");

    const state = useTabStore.getState();
    assert.equal(state.tabs[0].isPreview, false);
  });

  test("editing a request marks tab as dirty and automatically promotes from preview to pinned", () => {
    const store = useTabStore.getState();
    store.openTab("collections/user/get-user.pebble.json", sampleReq, true);

    store.updateActiveRequest((prev) => ({
      ...prev,
      url: "https://api.example.com/user?modified=1",
    }));

    const state = useTabStore.getState();
    assert.equal(state.tabs[0].isDirty, true);
    assert.equal(state.tabs[0].isPreview, false);
  });

  test("closing an unsaved dirty tab triggers confirmation requirement", () => {
    const store = useTabStore.getState();
    store.openTab("collections/user/get-user.pebble.json", sampleReq, false);
    store.updateActiveRequest((prev) => ({
      ...prev,
      name: "Renamed Query",
    }));

    const closed = store.closeTab("collections/user/get-user.pebble.json");
    assert.equal(closed, false);

    const state = useTabStore.getState();
    assert.equal(state.tabs.length, 1);
    assert.notEqual(state.pendingCloseTab, null);
    assert.equal(state.pendingCloseTab?.title, "Renamed Query");
  });

  test("confirmCloseTab with cancel restores normal state without closing", async () => {
    const store = useTabStore.getState();
    store.openTab("collections/user/get-user.pebble.json", sampleReq, false);
    store.updateActiveRequest((prev) => ({ ...prev, name: "Renamed" }));
    store.closeTab("collections/user/get-user.pebble.json");

    await store.confirmCloseTab("cancel");

    const state = useTabStore.getState();
    assert.equal(state.tabs.length, 1);
    assert.equal(state.pendingCloseTab, null);
  });

  test("confirmCloseTab with discard closes the tab immediately", async () => {
    const store = useTabStore.getState();
    store.openTab("collections/user/get-user.pebble.json", sampleReq, false);
    store.updateActiveRequest((prev) => ({ ...prev, name: "Renamed" }));
    store.closeTab("collections/user/get-user.pebble.json");

    await store.confirmCloseTab("discard");

    const state = useTabStore.getState();
    assert.equal(state.tabs.length, 0);
    assert.equal(state.pendingCloseTab, null);
    assert.equal(state.closedTabsHistory.length, 1);
  });

  test("reopenLastClosedTab brings back the most recently closed tab", () => {
    const store = useTabStore.getState();
    store.openTab("collections/user/get-user.pebble.json", sampleReq, false);
    store.closeTab("collections/user/get-user.pebble.json");

    assert.equal(useTabStore.getState().tabs.length, 0);

    useTabStore.getState().reopenLastClosedTab();
    const state = useTabStore.getState();
    assert.equal(state.tabs.length, 1);
    assert.equal(state.tabs[0].title, "Get User");
  });

  test("cycleTabs navigates forward and backward circularly", () => {
    const store = useTabStore.getState();
    store.openTab("collections/user/get-user.pebble.json", sampleReq, false);
    store.openTab(
      "collections/user/create-user.pebble.json",
      sampleReq2,
      false,
    );

    assert.equal(
      useTabStore.getState().activeTabId,
      "collections/user/create-user.pebble.json",
    );

    useTabStore.getState().cycleTabs("next");
    assert.equal(
      useTabStore.getState().activeTabId,
      "collections/user/get-user.pebble.json",
    );

    useTabStore.getState().cycleTabs("prev");
    assert.equal(
      useTabStore.getState().activeTabId,
      "collections/user/create-user.pebble.json",
    );
  });

  test("onFileRenamed updates open tab path and title", () => {
    const store = useTabStore.getState();
    store.openTab("collections/user/old-name.pebble.json", sampleReq, false);

    store.onFileRenamed(
      "collections/user/old-name.pebble.json",
      "collections/user/new-name.pebble.json",
      "new-name",
    );

    const state = useTabStore.getState();
    assert.equal(state.tabs[0].id, "collections/user/new-name.pebble.json");
    assert.equal(
      state.tabs[0].filePath,
      "collections/user/new-name.pebble.json",
    );
    assert.equal(state.tabs[0].title, "new-name");
    assert.equal(state.activeTabId, "collections/user/new-name.pebble.json");
  });

  test("onFileDeleted removes tab if deleted", () => {
    const store = useTabStore.getState();
    store.openTab("collections/user/get-user.pebble.json", sampleReq, false);
    store.openTab(
      "collections/user/create-user.pebble.json",
      sampleReq2,
      false,
    );

    store.onFileDeleted("collections/user/get-user.pebble.json");

    const state = useTabStore.getState();
    assert.equal(state.tabs.length, 1);
    assert.equal(state.tabs[0].id, "collections/user/create-user.pebble.json");
  });

  test("openFolderTab opens preview tab on single click and pinned tab on double click", () => {
    const store = useTabStore.getState();
    const folderDef = {
      name: "01-auth",
      headers: [{ key: "X-Folder", value: "1", enabled: true }],
      auth: { type: "bearer" as const, token: "secret" },
    };

    // Single-click preview
    store.openFolderTab("collections/01-auth", folderDef, true);
    let state = useTabStore.getState();
    assert.equal(state.tabs.length, 1);
    assert.equal(state.tabs[0].id, "folder:collections/01-auth");
    assert.equal(state.tabs[0].type, "folder");
    assert.equal(state.tabs[0].title, "01-auth");
    assert.equal(state.tabs[0].isPreview, true);

    // Double-click pin
    store.openFolderTab("collections/01-auth", folderDef, false);
    state = useTabStore.getState();
    assert.equal(state.tabs[0].isPreview, false);
  });

  test("updateActiveFolder marks folder tab dirty and modifies folder definition", () => {
    const store = useTabStore.getState();
    const folderDef = {
      name: "02-payments",
      headers: [],
      auth: { type: "none" as const },
    };

    store.openFolderTab("collections/02-payments", folderDef, false);
    assert.equal(useTabStore.getState().tabs[0].isDirty, false);

    store.updateActiveFolder((prev) => ({
      ...prev,
      headers: [{ key: "X-Payment-Gateway", value: "stripe", enabled: true }],
    }));

    const state = useTabStore.getState();
    assert.equal(state.tabs[0].isDirty, true);
    assert.equal(state.tabs[0].folder?.headers?.[0].key, "X-Payment-Gateway");
  });

  test("updateActiveRequest configures new Auth types (Digest, OAuth2, AWSSigV4)", () => {
    const store = useTabStore.getState();
    store.openTab("collections/auth-req.pebble.json", sampleReq, true);

    // 1. Digest Auth
    store.updateActiveRequest((prev) => ({
      ...prev,
      auth: {
        type: "digest",
        username: "digest_user",
        password: "digest_password",
        realm: "my_realm",
      },
    }));

    let tab = useTabStore.getState().tabs[0];
    assert.equal(tab.isDirty, true);
    assert.ok(tab.request);
    assert.equal(tab.request.auth.type, "digest");
    assert.equal(tab.request.auth.username, "digest_user");
    assert.equal(tab.request.auth.realm, "my_realm");

    // 2. OAuth 2.0 Auth
    store.updateActiveRequest((prev) => ({
      ...prev,
      auth: {
        type: "oauth2",
        grantType: "authorization_code",
        authUrl: "https://oauth.example.com/auth",
        tokenUrl: "https://oauth.example.com/token",
        clientId: "client_id_123",
        token: "active_access_token",
      },
    }));

    tab = useTabStore.getState().tabs[0];
    assert.ok(tab.request);
    assert.equal(tab.request.auth.type, "oauth2");
    assert.equal(tab.request.auth.grantType, "authorization_code");
    assert.equal(tab.request.auth.token, "active_access_token");

    // 3. AWS SigV4
    store.updateActiveRequest((prev) => ({
      ...prev,
      auth: {
        type: "awsSigV4",
        accessKey: "AKIA123",
        secretKey: "SECRET123",
        region: "eu-central-1",
        service: "s3",
      },
    }));

    tab = useTabStore.getState().tabs[0];
    assert.ok(tab.request);
    assert.equal(tab.request.auth.type, "awsSigV4");
    assert.equal(tab.request.auth.region, "eu-central-1");
    assert.equal(tab.request.auth.service, "s3");
  });

  test("updateActiveRequest configures Settings (cookies, proxy, mTLS, TLS verify)", () => {
    const store = useTabStore.getState();
    store.openTab("collections/settings-req.pebble.json", sampleReq, true);

    store.updateActiveRequest((prev) => ({
      ...prev,
      settings: {
        ...prev.settings,
        verifySSL: false,
        enableCookies: false,
        proxyUrl: "socks5://127.0.0.1:1080",
        clientCertPath: "/certs/client.pem",
        clientKeyPath: "/certs/client.key",
        userAgent: "CustomAgent/3.0",
        maxRedirects: 5,
        connectTimeoutMs: 5000,
      },
    }));

    const tab = useTabStore.getState().tabs[0];
    assert.equal(tab.isDirty, true);
    assert.ok(tab.request);
    assert.equal(tab.request.settings.verifySSL, false);
    assert.equal(tab.request.settings.enableCookies, false);
    assert.equal(tab.request.settings.proxyUrl, "socks5://127.0.0.1:1080");
    assert.equal(tab.request.settings.clientCertPath, "/certs/client.pem");
    assert.equal(tab.request.settings.userAgent, "CustomAgent/3.0");
    assert.equal(tab.request.settings.maxRedirects, 5);
    assert.equal(tab.request.settings.connectTimeoutMs, 5000);
  });

  test("tab execution results record structured consoleLogs and assertions", () => {
    const store = useTabStore.getState();
    store.openTab("collections/user/get-user.pebble.json", sampleReq, false);

    const execResult = {
      statusCode: 200,
      statusText: "200 OK",
      headers: { "content-type": ["application/json"] },
      body: '{"ok":true}',
      size: 11,
      timing: {
        dnsLookupMs: 1,
        tcpConnectMs: 2,
        tlsHandshakeMs: 3,
        ttfbMs: 10,
        downloadMs: 1,
        totalDurationMs: 17,
      },
      tests: [{ name: "Status is 200", passed: true }],
      logs: ["[10:00:00] [Pre-request] [INFO] Request starting"],
      consoleLogs: [
        {
          timestamp: "10:00:00.123",
          level: "info" as const,
          source: "Pre-request",
          message: "Request starting",
        },
        {
          timestamp: "10:00:00.200",
          level: "log" as const,
          source: "Post-response",
          message: "Response received",
        },
      ],
      executedAt: "2026-10-03T10:00:00Z",
    };

    store.setLastResult(execResult);

    const tab = useTabStore.getState().tabs[0];
    assert.ok(tab.lastResult);
    assert.equal(tab.lastResult.statusCode, 200);
    assert.equal(tab.lastResult.tests.length, 1);
    assert.equal(tab.lastResult.consoleLogs?.length, 2);
    assert.equal(tab.lastResult.consoleLogs?.[0].level, "info");
    assert.equal(tab.lastResult.consoleLogs?.[1].level, "log");
  });

  test("openHistoryTab opens read-only tab with recorded execution result", () => {
    const store = useTabStore.getState();
    const historyEntry = {
      id: 101,
      workspacePath: "/ws",
      requestName: "Get Status",
      method: "GET",
      url: "https://api.example.com/status",
      statusCode: 200,
      durationMs: 45,
      sizeBytes: 150,
      responseBody: '{"status":"healthy"}',
      responseHeaders: { "content-type": ["application/json"] },
      resolvedRequest: {
        method: "GET",
        url: "https://api.example.com/status",
        name: "Get Status",
      },
      executedAt: "2026-10-03T12:00:00Z",
    };

    store.openHistoryTab(historyEntry);

    const state = useTabStore.getState();
    const tab = state.tabs.find((t) => t.id === "history:101");
    assert.ok(tab);
    assert.equal(tab.isReadOnly, true);
    assert.equal(tab.type, "history");
    assert.equal(tab.title, "[History] Get Status");
    assert.equal(tab.lastResult?.statusCode, 200);
    assert.equal(tab.lastResult?.body, '{"status":"healthy"}');
    assert.equal(state.activeTabId, "history:101");
  });

  test("restoreRequestFromHistory restores into open editable tab or creates draft tab", () => {
    const store = useTabStore.getState();
    // Open an editable request tab first
    store.openTab("collections/user/get-user.pebble.json", sampleReq, false);

    const historyEntry = {
      id: 202,
      workspacePath: "/ws",
      requestName: "Restored API Call",
      method: "POST",
      url: "https://api.example.com/v2/users",
      statusCode: 201,
      durationMs: 65,
      sizeBytes: 88,
      responseBody: '{"id":"user-1"}',
      responseHeaders: {},
      resolvedRequest: {
        method: "POST",
        url: "https://api.example.com/v2/users",
        name: "Restored API Call",
        headers: [{ key: "X-Restored", value: "true", enabled: true }],
      },
      executedAt: "2026-10-03T12:30:00Z",
    };

    store.restoreRequestFromHistory(historyEntry);

    const activeTab = useTabStore
      .getState()
      .tabs.find((t) => t.id === "collections/user/get-user.pebble.json");
    assert.ok(activeTab);
    assert.equal(activeTab.request?.method, "POST");
    assert.equal(activeTab.request?.url, "https://api.example.com/v2/users");
    assert.equal(activeTab.request?.headers?.[0].key, "X-Restored");
  });
});
