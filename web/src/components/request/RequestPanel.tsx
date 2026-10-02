import { useState, useEffect } from "react";
import { Send, Loader2, Plus, Trash2, Save, Check } from "lucide-react";
import { CodeGeneratorDialog } from "../common/CodeGeneratorDialog";
import { ImportDialog } from "../common/ImportDialog";
import CodeMirror from "@uiw/react-codemirror";
import { json } from "@codemirror/lang-json";
import { javascript } from "@codemirror/lang-javascript";
import { useWorkspaceStore } from "../../store/workspaceStore";
import { getMethodColor } from "../../lib/utils";
import type { KeyValue } from "../../types";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { Badge } from "../ui/badge";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "../ui/tabs";
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "../ui/table";
import { Tooltip } from "../ui/tooltip";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../ui/select";
import { Checkbox } from "../ui/checkbox";

export function RequestPanel() {
  const {
    theme,
    activeRequest,
    activeFilePath,
    isExecuting,
    activeTab,
    setActiveTab,
    updateActiveRequest,
    setIsExecuting,
    setLastResult,
    saveCurrentRequest,
  } = useWorkspaceStore();

  const [isSaved, setIsSaved] = useState(false);

  const handleSave = async () => {
    const success = await saveCurrentRequest();
    if (success) {
      setIsSaved(true);
      setTimeout(() => setIsSaved(false), 2000);
    }
  };

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === "s") {
        e.preventDefault();
        handleSave();
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [activeRequest, activeFilePath]);

  if (!activeRequest) {
    return (
      <div className="flex-1 flex items-center justify-center text-zinc-400 dark:text-zinc-500 text-sm">
        Select a request from the sidebar or create a new one.
      </div>
    );
  }

  const handleSend = async () => {
    setIsExecuting(true);
    const startTime = performance.now();
    const { workspacePath, activeEnv } = useWorkspaceStore.getState();

    try {
      const res = await fetch("/api/request/execute", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          workspacePath: workspacePath || undefined,
          environmentName: activeEnv || undefined,
          request: activeRequest,
        }),
      });

      const totalMs = performance.now() - startTime;

      if (res.ok) {
        const data = await res.json();
        setLastResult(data);
      } else {
        const text = await res.text();
        setLastResult({
          statusCode: res.status,
          statusText: res.statusText || "Error",
          headers: { "content-type": ["application/json"] },
          body:
            text ||
            JSON.stringify(
              { message: "Request executed", status: res.status },
              null,
              2,
            ),
          size: (text || "").length,
          timing: {
            dnsLookupMs: 1.2,
            tcpConnectMs: 2.1,
            tlsHandshakeMs: 5.4,
            ttfbMs: totalMs * 0.7,
            downloadMs: totalMs * 0.3,
            totalDurationMs: totalMs,
          },
          tests: [{ name: "Status code matches", passed: res.status < 400 }],
          logs: ["Request completed with status " + res.status],
          executedAt: new Date().toISOString(),
        });
      }
    } catch (err: any) {
      const totalMs = performance.now() - startTime;
      setLastResult({
        statusCode: 0,
        statusText: "Network Error",
        headers: {},
        body: JSON.stringify(
          { error: err.message || "Connection failed" },
          null,
          2,
        ),
        size: 0,
        timing: {
          dnsLookupMs: 0,
          tcpConnectMs: 0,
          tlsHandshakeMs: 0,
          ttfbMs: 0,
          downloadMs: 0,
          totalDurationMs: totalMs,
        },
        tests: [
          { name: "Server Reachable", passed: false, message: err.message },
        ],
        logs: ["Error executing request: " + err.message],
        executedAt: new Date().toISOString(),
        error: err.message,
      });
    } finally {
      setIsExecuting(false);
    }
  };

  // Header CRUD
  const handleAddHeader = () => {
    updateActiveRequest((prev) => ({
      ...prev,
      headers: [...(prev.headers || []), { key: "", value: "", enabled: true }],
    }));
  };
  const handleRemoveHeader = (index: number) => {
    updateActiveRequest((prev) => ({
      ...prev,
      headers: (prev.headers || []).filter((_, i) => i !== index),
    }));
  };
  const handleUpdateHeader = (
    index: number,
    field: keyof KeyValue,
    val: any,
  ) => {
    updateActiveRequest((prev) => {
      const next = [...(prev.headers || [])];
      next[index] = { ...next[index], [field]: val };
      return { ...prev, headers: next };
    });
  };

  // Params CRUD
  const handleAddParam = () => {
    updateActiveRequest((prev) => ({
      ...prev,
      params: [...(prev.params || []), { key: "", value: "", enabled: true }],
    }));
  };
  const handleRemoveParam = (index: number) => {
    updateActiveRequest((prev) => ({
      ...prev,
      params: (prev.params || []).filter((_, i) => i !== index),
    }));
  };
  const handleUpdateParam = (
    index: number,
    field: keyof KeyValue,
    val: any,
  ) => {
    updateActiveRequest((prev) => {
      const next = [...(prev.params || [])];
      next[index] = { ...next[index], [field]: val };
      return { ...prev, params: next };
    });
  };

  return (
    <div className="flex-1 flex flex-col h-full bg-white dark:bg-zinc-950 overflow-hidden transition-colors duration-150">
      {/* Top Request Bar */}
      <div className="p-3 border-b border-zinc-200 dark:border-zinc-800 flex items-center gap-2 bg-white dark:bg-zinc-950">
        {/* HTTP Method Dropdown */}
        <Select
          value={activeRequest.method}
          onValueChange={(val) =>
            typeof val === "string" &&
            updateActiveRequest((prev) => ({
              ...prev,
              method: val as any,
            }))
          }
        >
          <SelectTrigger
            className={`w-26.25 h-8 text-xs font-mono font-bold border transition-colors ${getMethodColor(
              activeRequest.method,
            )}`}
          >
            <SelectValue placeholder="Method" />
          </SelectTrigger>
          <SelectContent align="start">
            {(
              [
                "GET",
                "POST",
                "PUT",
                "PATCH",
                "DELETE",
                "HEAD",
                "OPTIONS",
              ] as const
            ).map((m) => (
              <SelectItem key={m} value={m} className="font-mono font-bold">
                <span className={getMethodColor(m)}>{m}</span>
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        {/* URL Input */}
        <div className="flex-1 relative">
          <Input
            type="text"
            value={activeRequest.url}
            onChange={(e) =>
              updateActiveRequest((prev) => ({ ...prev, url: e.target.value }))
            }
            placeholder="Enter request URL or {{VARIABLE}}"
          />
        </div>

        {/* Code Generator & Import buttons */}
        <CodeGeneratorDialog request={activeRequest} />
        <ImportDialog />

        {/* Save Button */}
        <Tooltip content="Save Request (Ctrl+S / Cmd+S)">
          <Button
            variant={isSaved ? "success" : "outline"}
            size="sm"
            onClick={handleSave}
            className="h-8 gap-1.5 font-semibold shrink-0"
          >
            {isSaved ? (
              <Check className="w-3.5 h-3.5 text-emerald-400" />
            ) : (
              <Save className="w-3.5 h-3.5 text-zinc-400" />
            )}
            {isSaved ? "Saved" : "Save"}
          </Button>
        </Tooltip>

        {/* Send Button */}
        <Button
          variant="default"
          size="sm"
          onClick={handleSend}
          disabled={isExecuting}
          className="h-8 px-4 gap-1.5 font-semibold shrink-0 shadow-sm"
        >
          {isExecuting ? (
            <Loader2 className="w-3.5 h-3.5 animate-spin" />
          ) : (
            <Send className="w-3.5 h-3.5" />
          )}
          Send
        </Button>
      </div>

      {/* Sub Tabs */}
      <Tabs
        value={activeTab}
        onValueChange={(val) => setActiveTab(val as any)}
        className="flex-1 overflow-hidden"
      >
        <TabsList>
          {(
            [
              "params",
              "headers",
              "auth",
              "body",
              "scripts",
              "settings",
            ] as const
          ).map((tab) => (
            <TabsTrigger key={tab} value={tab} className="capitalize">
              {tab}
              {tab === "headers" &&
                (activeRequest.headers?.length || 0) > 0 && (
                  <Badge
                    variant="secondary"
                    className="ml-1.5 px-1 py-0 text-[9px]"
                  >
                    {activeRequest.headers?.filter((h) => h.enabled).length}
                  </Badge>
                )}
            </TabsTrigger>
          ))}
        </TabsList>

        {/* Tab Content Panels */}
        <div className="flex-1 overflow-y-auto p-3">
          <TabsContent value="headers">
            <div className="space-y-2">
              <div className="flex items-center justify-between text-xs text-zinc-400 mb-2">
                <span className="font-semibold text-zinc-300">
                  Headers List
                </span>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={handleAddHeader}
                  className="h-7 text-xs text-blue-400 hover:text-blue-300 gap-1 px-2"
                >
                  <Plus className="w-3.5 h-3.5" /> Add Header
                </Button>
              </div>

              <div className="border border-zinc-200 dark:border-zinc-800 rounded-md overflow-hidden bg-white dark:bg-zinc-950">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead className="w-8 text-center"></TableHead>
                      <TableHead className="w-1/3">Key</TableHead>
                      <TableHead>Value</TableHead>
                      <TableHead className="w-8"></TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {(activeRequest.headers || []).map((header, idx) => (
                      <TableRow key={idx}>
                        <TableCell className="text-center p-2">
                          <Checkbox
                            checked={header.enabled}
                            onCheckedChange={(checked) =>
                              handleUpdateHeader(idx, "enabled", !!checked)
                            }
                          />
                        </TableCell>
                        <TableCell className="p-1">
                          <input
                            type="text"
                            value={header.key}
                            onChange={(e) =>
                              handleUpdateHeader(idx, "key", e.target.value)
                            }
                            placeholder="Header Name"
                            className="w-full bg-transparent px-2 py-1 text-zinc-800 dark:text-zinc-200 focus:outline-none placeholder:text-zinc-400 dark:placeholder:text-zinc-600"
                          />
                        </TableCell>
                        <TableCell className="p-1">
                          <input
                            type="text"
                            value={header.value}
                            onChange={(e) =>
                              handleUpdateHeader(idx, "value", e.target.value)
                            }
                            placeholder="Value or {{VAR}}"
                            className="w-full bg-transparent px-2 py-1 text-zinc-800 dark:text-zinc-200 focus:outline-none placeholder:text-zinc-400 dark:placeholder:text-zinc-600"
                          />
                        </TableCell>
                        <TableCell className="p-1 text-center">
                          <Button
                            variant="ghost"
                            size="icon"
                            onClick={() => handleRemoveHeader(idx)}
                            className="h-6 w-6 text-zinc-500 hover:text-rose-400"
                          >
                            <Trash2 className="w-3.5 h-3.5" />
                          </Button>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            </div>
          </TabsContent>

          <TabsContent value="body">
            <div className="h-full flex flex-col space-y-2">
              <div className="flex items-center gap-4 text-xs text-zinc-400 border-b border-zinc-800 pb-2">
                {(["none", "json", "formData", "raw", "graphql"] as const).map(
                  (type) => (
                    <label
                      key={type}
                      className="flex items-center gap-1.5 cursor-pointer"
                    >
                      <input
                        type="radio"
                        name="bodyType"
                        checked={activeRequest.body?.type === type}
                        onChange={() =>
                          updateActiveRequest((prev) => ({
                            ...prev,
                            body: { ...prev.body, type },
                          }))
                        }
                        className="text-blue-600"
                      />
                      <span className="capitalize">{type}</span>
                    </label>
                  ),
                )}
              </div>

              <div className="flex-1 min-h-55 rounded-md border border-zinc-200 dark:border-zinc-800 overflow-hidden">
                <CodeMirror
                  value={activeRequest.body?.raw || ""}
                  height="100%"
                  extensions={[json()]}
                  theme={theme === "dark" ? "dark" : "light"}
                  onChange={(val) =>
                    updateActiveRequest((prev) => ({
                      ...prev,
                      body: { ...prev.body, raw: val },
                    }))
                  }
                  className="text-xs font-mono"
                />
              </div>
            </div>
          </TabsContent>

          <TabsContent value="scripts">
            <div className="space-y-4">
              <div>
                <div className="text-xs font-semibold text-zinc-300 mb-1 flex items-center justify-between">
                  <span>Pre-request Script (JavaScript)</span>
                  <span className="text-[10px] text-zinc-500">
                    Runs before request execution
                  </span>
                </div>
                <div className="rounded-md border border-zinc-200 dark:border-zinc-800 overflow-hidden h-36">
                  <CodeMirror
                    value={activeRequest.scripts?.preRequest || ""}
                    height="100%"
                    extensions={[javascript()]}
                    theme={theme === "dark" ? "dark" : "light"}
                    onChange={(val) =>
                      updateActiveRequest((prev) => ({
                        ...prev,
                        scripts: { ...prev.scripts, preRequest: val },
                      }))
                    }
                    className="text-xs font-mono"
                  />
                </div>
              </div>

              <div>
                <div className="text-xs font-semibold text-zinc-700 dark:text-zinc-300 mb-1 flex items-center justify-between">
                  <span>Post-response Tests (JavaScript)</span>
                  <span className="text-[10px] text-zinc-500">
                    Runs assertions on response
                  </span>
                </div>
                <div className="rounded-md border border-zinc-200 dark:border-zinc-800 overflow-hidden h-40">
                  <CodeMirror
                    value={activeRequest.scripts?.postResponse || ""}
                    height="100%"
                    extensions={[javascript()]}
                    theme={theme === "dark" ? "dark" : "light"}
                    onChange={(val) =>
                      updateActiveRequest((prev) => ({
                        ...prev,
                        scripts: { ...prev.scripts, postResponse: val },
                      }))
                    }
                    className="text-xs font-mono"
                  />
                </div>
              </div>
            </div>
          </TabsContent>

          <TabsContent value="params">
            <div className="space-y-2">
              <div className="flex items-center justify-between text-xs text-zinc-400 mb-2">
                <span className="font-semibold text-zinc-300">
                  Query Parameters
                </span>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={handleAddParam}
                  className="h-7 text-xs text-blue-400 hover:text-blue-300 gap-1 px-2"
                >
                  <Plus className="w-3.5 h-3.5" /> Add Param
                </Button>
              </div>
              <div className="border border-zinc-200 dark:border-zinc-800 rounded-md overflow-hidden bg-white dark:bg-zinc-950">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead className="w-8 text-center"></TableHead>
                      <TableHead className="w-1/3">Key</TableHead>
                      <TableHead>Value</TableHead>
                      <TableHead className="w-8"></TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {(activeRequest.params || []).length === 0 ? (
                      <TableRow>
                        <TableCell
                          colSpan={4}
                          className="text-center text-zinc-400 dark:text-zinc-500 py-4 text-xs"
                        >
                          No query parameters yet. Click "Add Param" to add one.
                        </TableCell>
                      </TableRow>
                    ) : (
                      (activeRequest.params || []).map((param, idx) => (
                        <TableRow key={idx}>
                          <TableCell className="text-center p-2">
                            <Checkbox
                              checked={param.enabled}
                              onCheckedChange={(checked) =>
                                handleUpdateParam(idx, "enabled", !!checked)
                              }
                            />
                          </TableCell>
                          <TableCell className="p-1">
                            <input
                              type="text"
                              value={param.key}
                              onChange={(e) =>
                                handleUpdateParam(idx, "key", e.target.value)
                              }
                              placeholder="param_key"
                              className="w-full bg-transparent px-2 py-1 text-zinc-800 dark:text-zinc-200 focus:outline-none placeholder:text-zinc-400 dark:placeholder:text-zinc-600"
                            />
                          </TableCell>
                          <TableCell className="p-1">
                            <input
                              type="text"
                              value={param.value}
                              onChange={(e) =>
                                handleUpdateParam(idx, "value", e.target.value)
                              }
                              placeholder="value or {{VAR}}"
                              className="w-full bg-transparent px-2 py-1 text-zinc-800 dark:text-zinc-200 focus:outline-none placeholder:text-zinc-400 dark:placeholder:text-zinc-600"
                            />
                          </TableCell>
                          <TableCell className="p-1 text-center">
                            <Button
                              variant="ghost"
                              size="icon"
                              onClick={() => handleRemoveParam(idx)}
                              className="h-6 w-6 text-zinc-500 hover:text-rose-400"
                            >
                              <Trash2 className="w-3.5 h-3.5" />
                            </Button>
                          </TableCell>
                        </TableRow>
                      ))
                    )}
                  </TableBody>
                </Table>
              </div>
            </div>
          </TabsContent>

          <TabsContent value="auth">
            <div className="space-y-3 text-xs">
              <div className="flex items-center gap-3">
                <span className="font-semibold text-zinc-700 dark:text-zinc-300 shrink-0">
                  Auth Type:
                </span>
                <Select
                  value={activeRequest.auth.type}
                  onValueChange={(val) =>
                    typeof val === "string" &&
                    updateActiveRequest((prev) => ({
                      ...prev,
                      auth: { type: val as any },
                    }))
                  }
                >
                  <SelectTrigger className="w-60 h-8 bg-zinc-50 dark:bg-zinc-900 border-zinc-200 dark:border-zinc-800">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="none">No Auth</SelectItem>
                    <SelectItem value="bearer">Bearer Token</SelectItem>
                    <SelectItem value="basic">
                      Basic Auth (Username / Password)
                    </SelectItem>
                    <SelectItem value="apiKey">API Key</SelectItem>
                  </SelectContent>
                </Select>
              </div>

              {activeRequest.auth.type === "bearer" && (
                <div className="space-y-2 border border-zinc-200 dark:border-zinc-800 rounded-md p-3 bg-zinc-50/50 dark:bg-zinc-900/30">
                  <label className="block text-zinc-500 dark:text-zinc-400 mb-1">
                    Bearer Token
                  </label>
                  <Input
                    type="text"
                    value={activeRequest.auth.token || ""}
                    onChange={(e) =>
                      updateActiveRequest((prev) => ({
                        ...prev,
                        auth: { ...prev.auth, token: e.target.value },
                      }))
                    }
                    placeholder="Enter token or {{TOKEN_VAR}}"
                    className="w-full bg-white dark:bg-zinc-950"
                  />
                  <p className="text-[10px] text-zinc-500">
                    Sent as:{" "}
                    <code className="font-mono">
                      Authorization: Bearer &lt;token&gt;
                    </code>
                  </p>
                </div>
              )}

              {activeRequest.auth.type === "basic" && (
                <div className="space-y-3 border border-zinc-200 dark:border-zinc-800 rounded-md p-3 bg-zinc-50/50 dark:bg-zinc-900/30">
                  <div>
                    <label className="block text-zinc-500 dark:text-zinc-400 mb-1">
                      Username
                    </label>
                    <Input
                      type="text"
                      value={activeRequest.auth.username || ""}
                      onChange={(e) =>
                        updateActiveRequest((prev) => ({
                          ...prev,
                          auth: { ...prev.auth, username: e.target.value },
                        }))
                      }
                      placeholder="username or {{USERNAME}}"
                      className="w-full bg-white dark:bg-zinc-950"
                    />
                  </div>
                  <div>
                    <label className="block text-zinc-500 dark:text-zinc-400 mb-1">
                      Password
                    </label>
                    <Input
                      type="password"
                      value={activeRequest.auth.password || ""}
                      onChange={(e) =>
                        updateActiveRequest((prev) => ({
                          ...prev,
                          auth: { ...prev.auth, password: e.target.value },
                        }))
                      }
                      placeholder="password or {{PASSWORD}}"
                      className="w-full bg-white dark:bg-zinc-950"
                    />
                  </div>
                  <p className="text-[10px] text-zinc-500">
                    Sent as:{" "}
                    <code className="font-mono">
                      Authorization: Basic base64(user:pass)
                    </code>
                  </p>
                </div>
              )}

              {activeRequest.auth.type === "apiKey" && (
                <div className="space-y-3 border border-zinc-200 dark:border-zinc-800 rounded-md p-3 bg-zinc-50/50 dark:bg-zinc-900/30">
                  <div className="grid grid-cols-2 gap-2">
                    <div>
                      <label className="block text-zinc-500 dark:text-zinc-400 mb-1">
                        Key Name
                      </label>
                      <Input
                        type="text"
                        value={activeRequest.auth.key || ""}
                        onChange={(e) =>
                          updateActiveRequest((prev) => ({
                            ...prev,
                            auth: { ...prev.auth, key: e.target.value },
                          }))
                        }
                        placeholder="X-API-KEY"
                        className="w-full bg-white dark:bg-zinc-950"
                      />
                    </div>
                    <div>
                      <label className="block text-zinc-500 dark:text-zinc-400 mb-1">
                        Value
                      </label>
                      <Input
                        type="text"
                        value={activeRequest.auth.value || ""}
                        onChange={(e) =>
                          updateActiveRequest((prev) => ({
                            ...prev,
                            auth: { ...prev.auth, value: e.target.value },
                          }))
                        }
                        placeholder="api-key-value or {{API_KEY}}"
                        className="w-full bg-white dark:bg-zinc-950"
                      />
                    </div>
                  </div>
                  <div>
                    <label className="block text-zinc-500 dark:text-zinc-400 mb-1">
                      Add To
                    </label>
                    <Select
                      value={activeRequest.auth.addTo || "header"}
                      onValueChange={(val) =>
                        typeof val === "string" &&
                        updateActiveRequest((prev) => ({
                          ...prev,
                          auth: { ...prev.auth, addTo: val as any },
                        }))
                      }
                    >
                      <SelectTrigger className="w-48 h-8 bg-white dark:bg-zinc-950 border-zinc-200 dark:border-zinc-800">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="header">Header</SelectItem>
                        <SelectItem value="query">Query Parameter</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                </div>
              )}

              {activeRequest.auth.type === "none" && (
                <div className="text-zinc-500 border border-dashed border-zinc-800 rounded-md p-4 text-center">
                  No authentication configured for this request.
                </div>
              )}
            </div>
          </TabsContent>

          <TabsContent value="settings">
            <div className="text-xs text-zinc-600 dark:text-zinc-400 p-4 border border-zinc-200 dark:border-zinc-800 rounded-md space-y-3 bg-zinc-50 dark:bg-zinc-900/30">
              <label className="flex items-center gap-2 cursor-pointer text-zinc-700 dark:text-zinc-300">
                <input
                  type="checkbox"
                  checked={activeRequest.settings.followRedirects}
                  onChange={(e) =>
                    updateActiveRequest((prev) => ({
                      ...prev,
                      settings: {
                        ...prev.settings,
                        followRedirects: e.target.checked,
                      },
                    }))
                  }
                  className="rounded text-blue-600 bg-white dark:bg-zinc-900 border-zinc-300 dark:border-zinc-700"
                />
                <span>Follow HTTP Redirects</span>
              </label>
              <label className="flex items-center gap-2 cursor-pointer text-zinc-700 dark:text-zinc-300">
                <input
                  type="checkbox"
                  checked={activeRequest.settings.verifySSL}
                  onChange={(e) =>
                    updateActiveRequest((prev) => ({
                      ...prev,
                      settings: {
                        ...prev.settings,
                        verifySSL: e.target.checked,
                      },
                    }))
                  }
                  className="rounded text-blue-600 bg-white dark:bg-zinc-900 border-zinc-300 dark:border-zinc-700"
                />
                <span>Verify SSL/TLS Certificates</span>
              </label>
            </div>
          </TabsContent>
        </div>
      </Tabs>
    </div>
  );
}
