import { useState, useEffect } from "react";
import { Send, Loader2, Plus, Trash2, Save, Check, Folder } from "lucide-react";
import { CodeGeneratorDialog } from "../common/CodeGeneratorDialog";
import { ImportDialog } from "../common/ImportDialog";
import { FolderSettingsPanel } from "../folder/FolderSettingsPanel";
import CodeMirror from "@uiw/react-codemirror";
import { json } from "@codemirror/lang-json";
import { javascript } from "@codemirror/lang-javascript";
import { useWorkspaceStore } from "../../store/workspaceStore";
import { useTabStore } from "../../store/tabStore";
import { TabBar } from "./TabBar";
import { getMethodColor, cn } from "../../lib/utils";
import type { KeyValue, ResolvedRequestResult } from "../../types";
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
import { RadioGroup, RadioGroupItem } from "../ui/radio-group";

export function RequestPanel() {
  const { theme, isExecuting, setIsExecuting } = useWorkspaceStore();
  const {
    tabs,
    activeTabId,
    updateActiveRequest,
    setActiveSubTab,
    setLastResult,
    saveCurrentTab,
  } = useTabStore();

  const currentTab = tabs.find((t) => t.id === activeTabId);
  const activeRequest = currentTab?.request || null;
  const activeFilePath = currentTab?.filePath || null;
  const activeTab = currentTab?.activeSubTab || "params";

  const [isSaved, setIsSaved] = useState(false);
  const [resolvedInfo, setResolvedInfo] = useState<ResolvedRequestResult | null>(null);

  useEffect(() => {
    if (!activeFilePath || currentTab?.type === "folder") {
      setResolvedInfo(null);
      return;
    }
    const { workspacePath, activeEnv } = useWorkspaceStore.getState();
    if (!workspacePath) return;

    let isMounted = true;
    const url = new URL("/api/request/resolved", window.location.origin);
    url.searchParams.set("workspacePath", workspacePath);
    url.searchParams.set("path", activeFilePath);
    if (activeEnv) url.searchParams.set("environmentName", activeEnv);

    fetch(url.toString())
      .then((r) => (r.ok ? r.json() : null))
      .then((data) => {
        if (isMounted) setResolvedInfo(data);
      })
      .catch(() => {
        if (isMounted) setResolvedInfo(null);
      });

    return () => {
      isMounted = false;
    };
  }, [activeFilePath, currentTab?.type, activeRequest?.auth?.type, activeRequest?.headers]);

  const handleSave = async () => {
    const success = await saveCurrentTab();
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
  }, [activeRequest, activeFilePath, currentTab]);

  if (!currentTab) {
    return (
      <div className="flex-1 flex flex-col h-full bg-white dark:bg-zinc-950 overflow-hidden">
        <TabBar />
        <div className="flex-1 flex flex-col items-center justify-center text-zinc-400 dark:text-zinc-500 text-xs gap-2 p-6 select-none">
          <p className="font-semibold text-zinc-700 dark:text-zinc-300 text-sm">No Request Open</p>
          <p className="text-zinc-400 dark:text-zinc-500">
            Click a request in the sidebar to preview, or double-click to pin a tab.
          </p>
        </div>
      </div>
    );
  }

  if (currentTab.type === "folder" || currentTab.folder) {
    return (
      <div className="flex-1 flex flex-col h-full bg-white dark:bg-zinc-950 overflow-hidden">
        <TabBar />
        <FolderSettingsPanel currentTab={currentTab} />
      </div>
    );
  }

  if (!activeRequest) {
    return (
      <div className="flex-1 flex flex-col h-full bg-white dark:bg-zinc-950 overflow-hidden">
        <TabBar />
        <div className="flex-1 flex flex-col items-center justify-center text-zinc-400 dark:text-zinc-500 text-xs gap-2 p-6 select-none">
          <p className="font-semibold text-zinc-700 dark:text-zinc-300 text-sm">No Request Open</p>
          <p className="text-zinc-400 dark:text-zinc-500">
            Click a request in the sidebar to preview, or double-click to pin a tab.
          </p>
        </div>
      </div>
    );
  }

  const handleSend = async () => {
    setIsExecuting(true);
    const { workspacePath, activeEnv, isWorkspaceTrusted, setWorkspaceTrusted } = useWorkspaceStore.getState();
    const hasScripts = Boolean(
      activeRequest.scripts?.preRequest?.trim() ||
      activeRequest.scripts?.postResponse?.trim()
    );

    let isTrusted = isWorkspaceTrusted(workspacePath || undefined);
    if (hasScripts && !isTrusted) {
      const confirmTrust = window.confirm(
        "This request contains JavaScript pre-request or test scripts.\n\nDo you trust this workspace to execute scripts?"
      );
      if (confirmTrust) {
        setWorkspaceTrusted(true, workspacePath || undefined);
        isTrusted = true;
      }
    }

    const startTime = performance.now();

    try {
      const res = await fetch("/api/request/execute", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          workspacePath: workspacePath || undefined,
          environmentName: activeEnv || undefined,
          path: activeFilePath || undefined,
          request: activeRequest,
          trusted: isTrusted,
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
      <TabBar />
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
        onValueChange={(val) => setActiveSubTab(val as any)}
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
                          <div className="flex items-center gap-1.5">
                            <input
                              type="text"
                              value={header.key}
                              onChange={(e) =>
                                handleUpdateHeader(idx, "key", e.target.value)
                              }
                              placeholder="Header Name"
                              className="flex-1 bg-transparent px-2 py-1 text-zinc-800 dark:text-zinc-200 focus:outline-none placeholder:text-zinc-400 dark:placeholder:text-zinc-600"
                            />
                            {header.key.trim() && resolvedInfo?.overriddenHeaders?.[header.key.trim().toLowerCase()] && (
                              <Badge
                                variant="outline"
                                className="text-[9px] px-1.5 py-0 h-4 bg-amber-500/10 text-amber-600 dark:text-amber-400 border-amber-500/20 font-normal shrink-0"
                              >
                                overrides {resolvedInfo.overriddenHeaders[header.key.trim().toLowerCase()].sourceFolder}
                              </Badge>
                            )}
                          </div>
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

              {/* Inherited Headers section */}
              {resolvedInfo && (
                <div className="pt-4 border-t border-zinc-200 dark:border-zinc-800 space-y-2">
                  <div className="flex items-center justify-between text-xs text-zinc-500 dark:text-zinc-400">
                    <div className="flex items-center gap-1.5 font-medium text-zinc-700 dark:text-zinc-300">
                      <Folder className="w-3.5 h-3.5 text-amber-500" />
                      <span>Inherited Headers</span>
                      <span className="text-[11px] text-zinc-400">
                        ({Object.keys(resolvedInfo.inheritedHeaders || {}).length})
                      </span>
                    </div>
                    <span className="text-[10px] text-zinc-400">
                      Inherited along directory tree
                    </span>
                  </div>

                  {Object.keys(resolvedInfo.inheritedHeaders || {}).length === 0 ? (
                    <p className="text-xs text-zinc-400 dark:text-zinc-500 italic p-3 border border-dashed border-zinc-200 dark:border-zinc-800 rounded-md">
                      No inherited headers active from parent folders.
                    </p>
                  ) : (
                    <div className="border border-zinc-200 dark:border-zinc-800 rounded-md overflow-hidden bg-zinc-50/50 dark:bg-zinc-900/30">
                      <Table>
                        <TableHeader>
                          <TableRow>
                            <TableHead className="w-1/3">Key</TableHead>
                            <TableHead>Value</TableHead>
                            <TableHead className="w-48">Source</TableHead>
                            <TableHead className="w-20 text-right">Action</TableHead>
                          </TableRow>
                        </TableHeader>
                        <TableBody>
                          {Object.entries(resolvedInfo.inheritedHeaders || {}).map(([lowerKey, prov]) => {
                            const resolvedHeader = resolvedInfo.request.headers?.find(
                              (h) => h.key.toLowerCase() === lowerKey
                            );
                            return (
                              <TableRow key={lowerKey} className="opacity-90">
                                <TableCell className="p-2 font-mono text-xs text-zinc-700 dark:text-zinc-300 font-semibold">
                                  {resolvedHeader?.key || lowerKey}
                                </TableCell>
                                <TableCell className="p-2 font-mono text-xs text-zinc-600 dark:text-zinc-400 truncate max-w-xs">
                                  {resolvedHeader?.value || ""}
                                </TableCell>
                                <TableCell className="p-2">
                                  <Badge
                                    variant="secondary"
                                    className="text-[10px] bg-amber-500/10 text-amber-600 dark:text-amber-400 border border-amber-500/20 font-medium"
                                  >
                                    inherited from {prov.sourceFolder}
                                  </Badge>
                                </TableCell>
                                <TableCell className="p-2 text-right">
                                  <Button
                                    variant="ghost"
                                    size="sm"
                                    className="h-6 text-[11px] text-blue-500 hover:text-blue-600 px-2"
                                    onClick={() => {
                                      updateActiveRequest((prev) => ({
                                        ...prev,
                                        headers: [
                                          ...(prev.headers || []),
                                          {
                                            key: resolvedHeader?.key || lowerKey,
                                            value: resolvedHeader?.value || "",
                                            enabled: true,
                                          },
                                        ],
                                      }));
                                    }}
                                  >
                                    Override
                                  </Button>
                                </TableCell>
                              </TableRow>
                            );
                          })}
                        </TableBody>
                      </Table>
                    </div>
                  )}
                </div>
              )}
            </div>
          </TabsContent>

          <TabsContent value="body">
            <div className="h-full flex flex-col space-y-2">
              <div className="flex items-center justify-between border-b border-zinc-200 dark:border-zinc-800 pb-2 mb-1">
                <RadioGroup
                  value={activeRequest.body?.type || "none"}
                  onValueChange={(val) =>
                    updateActiveRequest((prev) => ({
                      ...prev,
                      body: { ...prev.body, type: val as any },
                    }))
                  }
                  className="flex items-center gap-4 text-xs"
                >
                  {(
                    [
                      { id: "none", label: "None" },
                      { id: "json", label: "JSON" },
                      { id: "formData", label: "Form Data" },
                      { id: "raw", label: "Raw" },
                      { id: "graphql", label: "GraphQL" },
                      { id: "file", label: "File Reference" },
                    ] as const
                  ).map(({ id, label }) => {
                    const isSelected =
                      (activeRequest.body?.type || "none") === id;
                    return (
                      <label
                        key={id}
                        htmlFor={`body-type-${id}`}
                        className={cn(
                          "flex items-center gap-2 cursor-pointer select-none py-1 px-1.5 rounded-md transition-colors",
                          "hover:text-zinc-900 dark:hover:text-zinc-100",
                          isSelected
                            ? "text-zinc-900 dark:text-zinc-100 font-medium"
                            : "text-zinc-500 dark:text-zinc-400",
                        )}
                      >
                        <RadioGroupItem value={id} id={`body-type-${id}`} />
                        <span className="text-xs leading-none">{label}</span>
                      </label>
                    );
                  })}
                </RadioGroup>
              </div>

              {activeRequest.body?.type === "file" ? (
                <div className="space-y-3 p-4 border border-zinc-200 dark:border-zinc-800 rounded-md bg-zinc-50/50 dark:bg-zinc-900/30">
                  <div className="space-y-1.5">
                    <label className="text-xs font-semibold text-zinc-700 dark:text-zinc-300">
                      Relative File Path
                    </label>
                    <Input
                      type="text"
                      value={activeRequest.body?.filePath || ""}
                      onChange={(e) =>
                        updateActiveRequest((prev) => ({
                          ...prev,
                          body: { ...prev.body, filePath: e.target.value },
                        }))
                      }
                      placeholder="e.g. data/large-payload.json or uploads/sample.bin"
                      className="font-mono text-xs bg-white dark:bg-zinc-950"
                    />
                  </div>
                  <div className="text-[11px] text-zinc-500 dark:text-zinc-400 space-y-1">
                    <p>
                      Stored in <code className="font-mono text-zinc-700 dark:text-zinc-300">.pebble.json</code> as a relative file reference (<code className="font-mono text-zinc-700 dark:text-zinc-300">"filePath"</code>).
                    </p>
                    <p className="text-emerald-600 dark:text-emerald-400 font-medium">
                      ✓ Zero base64 embedding in JSON keeps Git repositories clean and diffs readable.
                    </p>
                  </div>
                </div>
              ) : (
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
              )}
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
                {resolvedInfo && (resolvedInfo.folderPreScripts?.length || 0) > 0 && (
                  <div className="mb-2 p-2.5 rounded border border-amber-500/20 bg-amber-500/5 text-xs flex items-center justify-between">
                    <div className="flex items-center gap-1.5">
                      <Folder className="w-3.5 h-3.5 text-amber-500 shrink-0" />
                      <span className="text-zinc-600 dark:text-zinc-400">
                        Folder pre-request scripts run before this script:
                      </span>
                      <span className="font-semibold text-zinc-800 dark:text-zinc-200">
                        {resolvedInfo.folderPreScripts?.join(" → ")}
                      </span>
                    </div>
                    <Badge variant="outline" className="text-[9px] bg-amber-500/10 text-amber-600 dark:text-amber-400 border-amber-500/20">
                      root → leaf
                    </Badge>
                  </div>
                )}
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
                {resolvedInfo && (resolvedInfo.folderPostScripts?.length || 0) > 0 && (
                  <div className="mb-2 p-2.5 rounded border border-amber-500/20 bg-amber-500/5 text-xs flex items-center justify-between">
                    <div className="flex items-center gap-1.5">
                      <Folder className="w-3.5 h-3.5 text-amber-500 shrink-0" />
                      <span className="text-zinc-600 dark:text-zinc-400">
                        Folder test scripts run after this script:
                      </span>
                      <span className="font-semibold text-zinc-800 dark:text-zinc-200">
                        {resolvedInfo.folderPostScripts?.join(" → ")}
                      </span>
                    </div>
                    <Badge variant="outline" className="text-[9px] bg-amber-500/10 text-amber-600 dark:text-amber-400 border-amber-500/20">
                      leaf → root
                    </Badge>
                  </div>
                )}
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
                  value={activeRequest.auth.type || "inherit"}
                  onValueChange={(val) =>
                    typeof val === "string" &&
                    updateActiveRequest((prev) => ({
                      ...prev,
                      auth: { type: val as any },
                    }))
                  }
                >
                  <SelectTrigger className="w-64 h-8 bg-zinc-50 dark:bg-zinc-900 border-zinc-200 dark:border-zinc-800">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="inherit">Inherit from parent</SelectItem>
                    <SelectItem value="none">No Auth</SelectItem>
                    <SelectItem value="bearer">Bearer Token</SelectItem>
                    <SelectItem value="basic">
                      Basic Auth (Username / Password)
                    </SelectItem>
                    <SelectItem value="apiKey">API Key</SelectItem>
                  </SelectContent>
                </Select>

                {activeRequest.auth.type && activeRequest.auth.type !== "inherit" && resolvedInfo?.parentAuthSource && (
                  <Badge
                    variant="outline"
                    className="text-[10px] bg-amber-500/10 text-amber-600 dark:text-amber-400 border-amber-500/20 font-normal"
                  >
                    overrides {resolvedInfo.parentAuthSource.sourceFolder}
                  </Badge>
                )}
              </div>

              {(!activeRequest.auth.type || activeRequest.auth.type === "inherit") && (
                <div className="space-y-3 border border-zinc-200 dark:border-zinc-800 rounded-md p-4 bg-zinc-50/50 dark:bg-zinc-900/30">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-2">
                      <Folder className="w-4 h-4 text-amber-500" />
                      <span className="font-semibold text-zinc-800 dark:text-zinc-200">
                        Inherited Authentication
                      </span>
                    </div>
                    {resolvedInfo?.inheritedAuth ? (
                      <Badge
                        variant="secondary"
                        className="text-[10px] bg-amber-500/10 text-amber-600 dark:text-amber-400 border border-amber-500/20 font-medium"
                      >
                        inherited from {resolvedInfo.inheritedAuth.sourceFolder}
                      </Badge>
                    ) : (
                      <Badge variant="outline" className="text-[10px] text-zinc-400">
                        No parent auth found
                      </Badge>
                    )}
                  </div>

                  {resolvedInfo?.inheritedAuth && resolvedInfo.request?.auth ? (
                    <div className="p-3 bg-white dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 rounded text-xs space-y-2">
                      <div className="flex items-center gap-2">
                        <span className="text-zinc-500 dark:text-zinc-400">Effective Type:</span>
                        <span className="font-semibold uppercase font-mono text-zinc-800 dark:text-zinc-200">
                          {resolvedInfo.request.auth.type}
                        </span>
                      </div>
                      {resolvedInfo.request.auth.type === "bearer" && (
                        <div className="flex items-center gap-2 text-zinc-600 dark:text-zinc-400">
                          <span className="text-zinc-500">Token:</span>
                          <code className="font-mono text-xs bg-zinc-100 dark:bg-zinc-900 px-1.5 py-0.5 rounded">
                            {resolvedInfo.request.auth.token || "(empty)"}
                          </code>
                        </div>
                      )}
                      {resolvedInfo.request.auth.type === "basic" && (
                        <div className="flex items-center gap-2 text-zinc-600 dark:text-zinc-400">
                          <span className="text-zinc-500">Username:</span>
                          <code className="font-mono text-xs bg-zinc-100 dark:bg-zinc-900 px-1.5 py-0.5 rounded">
                            {resolvedInfo.request.auth.username || "(empty)"}
                          </code>
                        </div>
                      )}
                      {resolvedInfo.request.auth.type === "apiKey" && (
                        <div className="flex items-center gap-2 text-zinc-600 dark:text-zinc-400">
                          <span className="text-zinc-500">Key:</span>
                          <code className="font-mono text-xs bg-zinc-100 dark:bg-zinc-900 px-1.5 py-0.5 rounded">
                            {resolvedInfo.request.auth.key}={resolvedInfo.request.auth.value} ({resolvedInfo.request.auth.addTo || "header"})
                          </code>
                        </div>
                      )}
                      <p className="text-[11px] text-zinc-400 pt-1 border-t border-zinc-100 dark:border-zinc-900">
                        This request automatically uses authentication configured in <span className="font-semibold text-zinc-700 dark:text-zinc-300">{resolvedInfo.inheritedAuth.sourceFolder}</span>.
                      </p>
                    </div>
                  ) : (
                    <p className="text-zinc-500 text-xs">
                      No folder along the directory tree defines authentication. This request executes without auth unless configured directly.
                    </p>
                  )}
                </div>
              )}

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
            <div className="p-4 border border-zinc-200 dark:border-zinc-800 rounded-lg bg-zinc-50/50 dark:bg-zinc-900/30 divide-y divide-zinc-200/80 dark:divide-zinc-800/80 max-w-2xl">
              <label
                htmlFor="setting-follow-redirects"
                className="flex items-start gap-3 py-3 first:pt-0 last:pb-0 cursor-pointer group select-none"
              >
                <div className="pt-0.5">
                  <Checkbox
                    id="setting-follow-redirects"
                    checked={activeRequest.settings.followRedirects}
                    onCheckedChange={(checked) =>
                      updateActiveRequest((prev) => ({
                        ...prev,
                        settings: {
                          ...prev.settings,
                          followRedirects: !!checked,
                        },
                      }))
                    }
                  />
                </div>
                <div className="flex flex-col gap-0.5">
                  <span className="text-xs font-medium text-zinc-800 dark:text-zinc-200 group-hover:text-blue-600 dark:group-hover:text-blue-400 transition-colors">
                    Follow HTTP Redirects
                  </span>
                  <span className="text-[11px] text-zinc-500 dark:text-zinc-400">
                    Automatically follow 3xx redirect status codes returned by the server.
                  </span>
                </div>
              </label>

              <label
                htmlFor="setting-verify-ssl"
                className="flex items-start gap-3 py-3 first:pt-0 last:pb-0 cursor-pointer group select-none"
              >
                <div className="pt-0.5">
                  <Checkbox
                    id="setting-verify-ssl"
                    checked={activeRequest.settings.verifySSL}
                    onCheckedChange={(checked) =>
                      updateActiveRequest((prev) => ({
                        ...prev,
                        settings: {
                          ...prev.settings,
                          verifySSL: !!checked,
                        },
                      }))
                    }
                  />
                </div>
                <div className="flex flex-col gap-0.5">
                  <span className="text-xs font-medium text-zinc-800 dark:text-zinc-200 group-hover:text-blue-600 dark:group-hover:text-blue-400 transition-colors">
                    Verify SSL/TLS Certificates
                  </span>
                  <span className="text-[11px] text-zinc-500 dark:text-zinc-400">
                    Validate SSL/TLS certificates when sending HTTPS requests. Disable only when testing with self-signed development certificates.
                  </span>
                </div>
              </label>
            </div>
          </TabsContent>
        </div>
      </Tabs>
    </div>
  );
}
