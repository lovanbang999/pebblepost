import { useState, useEffect, useCallback, useMemo, useRef } from "react";
import { GrpcRequestPanel } from "../grpc/GrpcRequestPanel";
import { StreamRequestPanel } from "../stream/StreamRequestPanel";
import type { GrpcStreamMessage, ExecutionResult, StreamLogEntry, StreamSessionStatus } from "../../types";
import {
  Send,
  Loader2,
  Plus,
  Trash2,
  Save,
  Check,
  Folder,
  Cookie,
  ShieldAlert,
  Key,
  Globe,
  Shield,
  RefreshCw,
  Lock,
  Clock,
  RotateCcw,
  Radio,
  WifiOff,
} from "lucide-react";
import { CodeGeneratorDialog } from "../common/CodeGeneratorDialog";
import { ImportDialog } from "../common/ImportDialog";
import { FolderSettingsPanel } from "../folder/FolderSettingsPanel";
import { RunnerPanel } from "../runner/RunnerPanel";
import { DocsPanel } from "../docs/DocsPanel";
import { RequestDocsTab } from "./RequestDocsTab";
import { RequestExamplesTab } from "./RequestExamplesTab";
import { CookieManagerDialog } from "../cookies/CookieManagerDialog";
import { ScriptTrustDialog } from "./ScriptTrustDialog";
import { VariableInput } from "../common/VariableInput";
import { UndefinedVariablesDialog } from "./UndefinedVariablesDialog";
import { ManageEnvironmentsDialog } from "../environments/ManageEnvironmentsDialog";
import { getAvailableVariables, findUndefinedVariablesInRequest } from "../../lib/variables";
import {
  variableStyles,
  createVariableHighlightExtension,
  createVariableHoverTooltip,
  createVariableAutocomplete,
} from "../../lib/codemirrorVariableExtension";
import CodeMirror from "@uiw/react-codemirror";
import { autocompletion } from "@codemirror/autocomplete";
import { json } from "@codemirror/lang-json";
import { javascript } from "@codemirror/lang-javascript";
import { pebbleScriptCompletions } from "../../lib/codemirror-completions";
import { useWorkspaceStore } from "../../store/workspaceStore";
import { useTabStore, type RequestTab } from "../../store/tabStore";
import { TabBar } from "./TabBar";
import { getMethodColor, cn } from "../../lib/utils";
import type {
  KeyValue,
  ResolvedRequestResult,
  RequestDefinition,
  AuthDefinition,
  BodyDefinition,
} from "../../types";
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
  const [isCookieManagerOpen, setIsCookieManagerOpen] = useState(false);
  const [isTrustDialogOpen, setIsTrustDialogOpen] = useState(false);
  const [isOAuthAuthorizing, setIsOAuthAuthorizing] = useState(false);
  const [oauthError, setOauthError] = useState<string | null>(null);

  const [undefinedDialogOpen, setUndefinedDialogOpen] = useState(false);
  const [pendingUndefinedVars, setPendingUndefinedVars] = useState<string[]>([]);
  const [isEnvManagerOpen, setIsEnvManagerOpen] = useState(false);
  const streamAbortRef = useRef<{ abortController: AbortController; streamId: string } | null>(null);
  const [isStreaming, setIsStreaming] = useState(false);
  const [streamStatus, setStreamStatus] = useState<StreamSessionStatus | null>(null);

  const handleStreamDisconnect = async () => {
    if (streamAbortRef.current) {
      const { streamId, abortController } = streamAbortRef.current;
      abortController.abort();
      try {
        await fetch("/api/stream/disconnect", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ streamId }),
        });
      } catch {
        // ignore
      }
      streamAbortRef.current = null;
    }
    setIsStreaming(false);
  };

  const handleStreamConnect = async () => {
    if (isStreaming) {
      await handleStreamDisconnect();
      return;
    }

    if (!activeRequest) return;

    const undefinedVars = findUndefinedVariablesInRequest(activeRequest, availableMap);
    if (undefinedVars.length > 0) {
      setPendingUndefinedVars(undefinedVars);
      setUndefinedDialogOpen(true);
      return;
    }

    setIsStreaming(true);
    const streamId = activeRequest.id || `stream_${Date.now()}`;
    const abortController = new AbortController();
    streamAbortRef.current = { abortController, streamId };

    const { workspacePath, activeEnv } = useWorkspaceStore.getState();
    const startTime = performance.now();

    try {
      const res = await fetch("/api/stream/connect", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        signal: abortController.signal,
        body: JSON.stringify({
          streamId,
          workspacePath: workspacePath || undefined,
          environmentName: activeEnv || undefined,
          path: activeFilePath || undefined,
          request: activeRequest,
          trusted: true,
        }),
      });

      if (!res.ok) {
        const errText = await res.text();
        throw new Error(errText || `Server responded with ${res.status}`);
      }

      const reader = res.body?.getReader();
      if (!reader) throw new Error("Response body is unreadable");

      const decoder = new TextDecoder();
      let buffer = "";
      let currentLogs: StreamLogEntry[] = [];

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });

        const blocks = buffer.split("\n\n");
        buffer = blocks.pop() || "";

        for (const block of blocks) {
          if (!block.trim()) continue;
          let event = "message";
          let data = "";
          for (const line of block.split("\n")) {
            if (line.startsWith("event: ")) {
              event = line.slice(7).trim();
            } else if (line.startsWith("data: ")) {
              data = line.slice(6).trim();
            }
          }

          if (event === "status" && data) {
            try {
              const status: StreamSessionStatus = JSON.parse(data);
              setStreamStatus(status);
              if (status.state === "disconnected") {
                setIsStreaming(false);
              }
              setLastResult((prev) => ({
                ...(prev || {
                  statusCode: 200,
                  statusText: "Stream Connected",
                  headers: {},
                  body: "",
                  size: 0,
                  timing: {
                    dnsLookupMs: 0,
                    tcpConnectMs: 0,
                    tlsHandshakeMs: 0,
                    ttfbMs: 0,
                    downloadMs: 0,
                    totalDurationMs: performance.now() - startTime,
                  },
                  tests: [],
                  logs: [],
                  executedAt: new Date().toISOString(),
                }),
                streamCloseCode: status.closeCode,
                streamCloseReason: status.closeReason,
                streamEvicted: status.evictedCount,
              }));
            } catch {
              // ignore
            }
          } else if (event === "log" && data) {
            try {
              const logEntry: StreamLogEntry = JSON.parse(data);
              currentLogs = [...currentLogs, logEntry];
              setLastResult((prev) => ({
                ...(prev || {
                  statusCode: 200,
                  statusText: "Streaming",
                  headers: {},
                  body: "",
                  size: 0,
                  timing: {
                    dnsLookupMs: 0,
                    tcpConnectMs: 0,
                    tlsHandshakeMs: 0,
                    ttfbMs: 0,
                    downloadMs: 0,
                    totalDurationMs: performance.now() - startTime,
                  },
                  tests: [],
                  logs: [],
                  executedAt: new Date().toISOString(),
                }),
                streamLogs: currentLogs,
              }));
            } catch {
              // ignore
            }
          }
        }
      }
    } catch (err: unknown) {
      if (err instanceof Error && err.name === "AbortError") {
        return;
      }
      const errMsg = err instanceof Error ? err.message : String(err);
      setLastResult((prev) => ({
        ...(prev || {
          statusCode: 500,
          statusText: "Stream Error",
          headers: {},
          body: "",
          size: 0,
          timing: {
            dnsLookupMs: 0,
            tcpConnectMs: 0,
            tlsHandshakeMs: 0,
            ttfbMs: 0,
            downloadMs: 0,
            totalDurationMs: performance.now() - startTime,
          },
          tests: [{ name: "Stream Connection", passed: false, message: errMsg }],
          logs: ["Error connecting stream: " + errMsg],
          executedAt: new Date().toISOString(),
        }),
        error: errMsg,
      }));
    } finally {
      setIsStreaming(false);
    }
  };

  const handleStreamSend = async (
    payload: string,
    type: "text" | "binary" | "ping" | "pong" = "text",
  ) => {
    const streamId = streamAbortRef.current?.streamId || activeRequest?.id;
    if (!streamId) return;
    try {
      const res = await fetch("/api/stream/send", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          streamId,
          payload,
          type,
        }),
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(data.error || "Failed to send message");
      }
    } catch (err: unknown) {
      console.error("Failed to send stream message:", err);
    }
  };

  const { environments, activeEnv } = useWorkspaceStore();
  const availableMap = useMemo(() => {
    return getAvailableVariables(environments, activeEnv);
  }, [environments, activeEnv]);

  const bodyVariableExtensions = useMemo(() => {
    return [
      variableStyles,
      createVariableHighlightExtension(() => availableMap),
      createVariableHoverTooltip(() => availableMap, () => setIsEnvManagerOpen(true)),
      createVariableAutocomplete(() => availableMap),
    ];
  }, [availableMap]);

  const handleFetchOAuthToken = async () => {
    if (!activeRequest) return;
    setIsOAuthAuthorizing(true);
    setOauthError(null);
    try {
      const isAuthCode = activeRequest.auth.grantType === "authorization_code";
      const endpoint = isAuthCode ? "/api/oauth2/authorize" : "/api/oauth2/token";
      const res = await fetch(endpoint, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ auth: activeRequest.auth }),
      });
      const data = await res.json();
      if (!res.ok) {
        throw new Error(data.error || "Failed to retrieve OAuth2 token");
      }
      updateActiveRequest((prev) => ({
        ...prev,
        auth: {
          ...prev.auth,
          token: data.access_token,
          refreshToken: data.refresh_token || prev.auth.refreshToken,
          tokenExpiresAt: data.expires_at,
        },
      }));
    } catch (err: unknown) {
      setOauthError(err instanceof Error ? err.message : String(err));
    } finally {
      setIsOAuthAuthorizing(false);
    }
  };

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

  const handleSave = useCallback(async () => {
    const success = await saveCurrentTab();
    if (success) {
      setIsSaved(true);
      setTimeout(() => setIsSaved(false), 2000);
    }
  }, [saveCurrentTab]);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === "s") {
        e.preventDefault();
        handleSave();
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [handleSave]);

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

  if (currentTab.type === "runner") {
    return (
      <div className="flex-1 flex flex-col h-full bg-white dark:bg-zinc-950 overflow-hidden">
        <TabBar />
        <RunnerPanel currentTab={currentTab} />
      </div>
    );
  }

  if (currentTab.type === "docs") {
    return (
      <div className="flex-1 flex flex-col h-full bg-white dark:bg-zinc-950 overflow-hidden">
        <TabBar />
        <DocsPanel currentTab={currentTab} />
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
 
  const handleCancelStream = async () => {
    if (streamAbortRef.current) {
      streamAbortRef.current.abortController.abort();
      try {
        await fetch("/api/grpc/stream/cancel", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ streamId: streamAbortRef.current.streamId }),
        });
      } catch {
        // ignore
      }
      streamAbortRef.current = null;
      setIsExecuting(false);
    }
  };

  const executeRequest = async (isTrusted: boolean) => {
    setIsExecuting(true);
    const { workspacePath, activeEnv } = useWorkspaceStore.getState();
    const startTime = performance.now();

    if (activeRequest.protocol === "grpc" || activeRequest.method === "GRPC") {
      const streamId = `stream_${Date.now()}`;
      const abortController = new AbortController();
      streamAbortRef.current = { abortController, streamId };

      try {
        const res = await fetch("/api/grpc/stream", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          signal: abortController.signal,
          body: JSON.stringify({
            streamId,
            workspacePath: workspacePath || undefined,
            environmentName: activeEnv || undefined,
            path: activeFilePath || undefined,
            request: activeRequest,
            trusted: isTrusted,
          }),
        });

        if (!res.ok) {
          const errText = await res.text();
          throw new Error(errText || `Server responded with ${res.status}`);
        }

        const reader = res.body?.getReader();
        if (!reader) throw new Error("Response body is unreadable");

        const decoder = new TextDecoder();
        let buffer = "";
        let currentMessages: GrpcStreamMessage[] = [];

        while (true) {
          const { done, value } = await reader.read();
          if (done) break;
          buffer += decoder.decode(value, { stream: true });

          const lines = buffer.split("\n\n");
          buffer = lines.pop() || "";

          for (const block of lines) {
            if (!block.trim()) continue;
            let event = "message";
            let data = "";
            for (const line of block.split("\n")) {
              if (line.startsWith("event: ")) {
                event = line.slice(7).trim();
              } else if (line.startsWith("data: ")) {
                data = line.slice(6).trim();
              }
            }

            if (event === "message" && data) {
              try {
                const streamMsg: GrpcStreamMessage = JSON.parse(data);
                currentMessages = [...currentMessages, streamMsg];
                setLastResult((prev: ExecutionResult | null) => ({
                  ...(prev || {
                    statusCode: 200,
                    statusText: "Streaming",
                    headers: {},
                    body: "",
                    size: 0,
                    timing: {
                      dnsLookupMs: 0,
                      tcpConnectMs: 0,
                      tlsHandshakeMs: 0,
                      ttfbMs: 0,
                      downloadMs: 0,
                      totalDurationMs: performance.now() - startTime,
                    },
                    tests: [],
                    logs: [],
                    executedAt: new Date().toISOString(),
                  }),
                  grpcMessages: currentMessages,
                }));
              } catch {
                // ignore
              }
            } else if (event === "done" && data) {
              try {
                const finalResult: ExecutionResult = JSON.parse(data);
                setLastResult(finalResult);
              } catch {
                // ignore
              }
            }
          }
        }
      } catch (err: unknown) {
        if (err instanceof Error && err.name === "AbortError") {
          return;
        }
        const errMsg = err instanceof Error ? err.message : String(err);
        setLastResult({
          statusCode: 500,
          statusText: "Stream Error",
          headers: {},
          body: JSON.stringify({ error: errMsg }, null, 2),
          size: 0,
          timing: {
            dnsLookupMs: 0,
            tcpConnectMs: 0,
            tlsHandshakeMs: 0,
            ttfbMs: 0,
            downloadMs: 0,
            totalDurationMs: performance.now() - startTime,
          },
          tests: [{ name: "gRPC Stream Execution", passed: false, message: errMsg }],
          logs: ["Error executing gRPC request: " + errMsg],
          executedAt: new Date().toISOString(),
          error: errMsg,
        });
      } finally {
        streamAbortRef.current = null;
        setIsExecuting(false);
      }
      return;
    }

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
    } catch (err: unknown) {
      const totalMs = performance.now() - startTime;
      const errMsg = err instanceof Error ? err.message : String(err);
      setLastResult({
        statusCode: 0,
        statusText: "Network Error",
        headers: {},
        body: JSON.stringify(
          { error: errMsg || "Connection failed" },
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
          { name: "Server Reachable", passed: false, message: errMsg },
        ],
        logs: ["Error executing request: " + errMsg],
        executedAt: new Date().toISOString(),
        error: errMsg,
      });
    } finally {
      setIsExecuting(false);
    }
  };

  const handleSend = async () => {
    if (!activeRequest) return;
    const { workspacePath, isWorkspaceTrusted } = useWorkspaceStore.getState();
    const hasScripts = Boolean(
      activeRequest.scripts?.preRequest?.trim() ||
      activeRequest.scripts?.postResponse?.trim()
    );

    const isTrusted = isWorkspaceTrusted(workspacePath || undefined);
    if (hasScripts && !isTrusted) {
      setIsTrustDialogOpen(true);
      return;
    }

    // Check for undefined variables before send
    const undefinedVars = findUndefinedVariablesInRequest(activeRequest, availableMap);
    if (undefinedVars.length > 0) {
      setPendingUndefinedVars(undefinedVars);
      setUndefinedDialogOpen(true);
      return;
    }

    await executeRequest(isTrusted);
  };

  const handleConfirmSendAnyway = async () => {
    setUndefinedDialogOpen(false);
    const { workspacePath, isWorkspaceTrusted } = useWorkspaceStore.getState();
    const isTrusted = isWorkspaceTrusted(workspacePath || undefined);
    await executeRequest(isTrusted);
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
  const handleUpdateHeader = <K extends keyof KeyValue>(
    index: number,
    field: K,
    val: KeyValue[K],
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
  const handleUpdateParam = <K extends keyof KeyValue>(
    index: number,
    field: K,
    val: KeyValue[K],
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

      {/* Historical Execution Read-Only Banner */}
      {(currentTab?.type === "history" || currentTab?.isReadOnly) && (
        <div className="px-4 py-2 bg-indigo-50/90 dark:bg-indigo-950/40 border-b border-indigo-200 dark:border-indigo-800/80 flex items-center justify-between text-xs animate-in fade-in duration-150">
          <div className="flex items-center gap-2 text-indigo-800 dark:text-indigo-300">
            <Clock className="w-4 h-4 text-indigo-600 dark:text-indigo-400 shrink-0" />
            <span>
              <strong>Historical Run Snapshot</strong> &bull;{" "}
              {currentTab.historyEntry?.executedAt
                ? new Date(currentTab.historyEntry.executedAt).toLocaleString()
                : "Recorded Run"}{" "}
              (Read-Only)
            </span>
          </div>
          <div className="flex items-center gap-2">
            <Button
              size="sm"
              variant="outline"
              onClick={() => {
                if (currentTab.historyEntry) {
                  useTabStore.getState().restoreRequestFromHistory(currentTab.historyEntry);
                }
              }}
              className="h-6 px-2.5 text-xs font-semibold bg-white dark:bg-zinc-900 border-indigo-300 dark:border-indigo-700 text-indigo-700 dark:text-indigo-300 hover:bg-indigo-50 dark:hover:bg-indigo-900/40 gap-1.5 cursor-pointer"
            >
              <RotateCcw className="w-3 h-3" />
              Restore this request
            </Button>
          </div>
        </div>
      )}

      {/* Top Request Bar */}
      <div className="p-3 border-b border-zinc-200 dark:border-zinc-800 flex items-center gap-2 bg-white dark:bg-zinc-950">
        {/* HTTP Method Dropdown */}
        <Select
          value={activeRequest.method}
          onValueChange={(val) => {
            if (typeof val !== "string") return;
            const isGrpcVal = val === "GRPC";
            const isWsVal = val === "WS";
            const isSseVal = val === "SSE";
            const isStreamVal = isWsVal || isSseVal;

            updateActiveRequest((prev) => ({
              ...prev,
              method: val as RequestDefinition["method"],
              protocol: isGrpcVal
                ? "grpc"
                : isWsVal
                ? "websocket"
                : isSseVal
                ? "sse"
                : (prev.protocol === "grpc" || prev.protocol === "websocket" || prev.protocol === "sse")
                ? "http"
                : prev.protocol,
              url:
                isWsVal && (!prev.url || prev.url.startsWith("http"))
                  ? (prev.url ? prev.url.replace(/^http/, "ws") : "ws://localhost:8080/ws")
                  : isSseVal && (!prev.url || prev.url.startsWith("ws"))
                  ? (prev.url ? prev.url.replace(/^ws/, "http") : "http://localhost:8080/events")
                  : isGrpcVal && !prev.url
                  ? "localhost:50051"
                  : prev.url,
              grpc: isGrpcVal
                ? prev.grpc || {
                    address: prev.url || "localhost:50051",
                    protoSource: "reflection",
                    service: "",
                    method: "",
                    message: "{}",
                    useTls: false,
                  }
                : prev.grpc,
              stream: isStreamVal
                ? prev.stream || {
                    autoReconnect: true,
                    maxReconnectAttempts: 5,
                    reconnectIntervalMs: 1000,
                    pingIntervalMs: isWsVal ? 30000 : 0,
                    maxLogEntries: 1000,
                    maxLogBytes: 5 * 1024 * 1024,
                    outgoingMessages: [],
                    subprotocols: [],
                  }
                : prev.stream,
            }));
            if (isGrpcVal) {
              setActiveSubTab("grpc");
            } else if (isStreamVal) {
              setActiveSubTab("stream");
            }
          }}
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
                "GRPC",
                "WS",
                "SSE",
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
          <VariableInput
            value={activeRequest.url}
            onChange={(val) =>
              updateActiveRequest((prev) => ({ ...prev, url: val }))
            }
            onOpenManageEnvironments={() => setIsEnvManagerOpen(true)}
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

        {/* Send / Cancel / Connect / Disconnect Button */}
        {(() => {
          const isStream =
            activeRequest.method === "WS" ||
            activeRequest.method === "SSE" ||
            activeRequest.protocol === "websocket" ||
            activeRequest.protocol === "sse";

          if (isStream) {
            return isStreaming ? (
              <Button
                variant="destructive"
                size="sm"
                onClick={handleStreamDisconnect}
                className="h-8 px-4 gap-1.5 font-semibold shrink-0 shadow-sm"
              >
                <WifiOff className="w-3.5 h-3.5" />
                Disconnect
              </Button>
            ) : (
              <Button
                variant="default"
                size="sm"
                onClick={handleStreamConnect}
                className="h-8 px-4 gap-1.5 font-semibold shrink-0 shadow-sm bg-cyan-600 hover:bg-cyan-700 text-white"
              >
                <Radio className="w-3.5 h-3.5" />
                Connect
              </Button>
            );
          }

          if (isExecuting && (activeRequest.method === "GRPC" || activeRequest.protocol === "grpc")) {
            return (
              <Button
                variant="destructive"
                size="sm"
                onClick={handleCancelStream}
                className="h-8 px-4 gap-1.5 font-semibold shrink-0 shadow-sm animate-pulse"
              >
                Cancel
              </Button>
            );
          }

          return (
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
          );
        })()}
      </div>

      {/* Sub Tabs */}
      {(() => {
        const isGrpc = activeRequest.method === "GRPC" || activeRequest.protocol === "grpc";
        const isStream =
          activeRequest.method === "WS" ||
          activeRequest.method === "SSE" ||
          activeRequest.protocol === "websocket" ||
          activeRequest.protocol === "sse";
        const subTabs = isGrpc
          ? (["grpc", "scripts", "settings"] as const)
          : isStream
          ? (["stream", "headers", "scripts", "settings"] as const)
          : (["params", "headers", "auth", "body", "scripts", "docs", "examples", "settings"] as const);
        const resolvedActiveTab =
          isGrpc && !(subTabs as readonly string[]).includes(activeTab)
            ? "grpc"
            : isStream && !(subTabs as readonly string[]).includes(activeTab)
            ? "stream"
            : activeTab;

        return (
          <Tabs
            value={resolvedActiveTab}
            onValueChange={(val) =>
              setActiveSubTab(val as RequestTab["activeSubTab"])
            }
            className="flex-1 overflow-hidden"
          >
            <TabsList>
              {subTabs.map((tab) => (
                <TabsTrigger key={tab} value={tab} className="capitalize">
                  {tab === "grpc"
                    ? "gRPC Config"
                    : tab === "stream"
                    ? activeRequest.protocol === "sse" || activeRequest.method === "SSE"
                      ? "SSE Config"
                      : "WebSocket"
                    : tab === "docs"
                    ? "Docs"
                    : tab === "examples"
                    ? "Examples"
                    : tab}
                  {tab === "docs" && Boolean(activeRequest.description?.trim()) && (
                    <span className="w-1.5 h-1.5 rounded-full bg-blue-500 ml-1.5" />
                  )}
                  {tab === "examples" && (activeRequest.examples?.length || 0) > 0 && (
                    <Badge
                      variant="secondary"
                      className="ml-1.5 px-1 py-0 text-[9px]"
                    >
                      {activeRequest.examples?.length}
                    </Badge>
                  )}
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
              {isGrpc && (
                <TabsContent value="grpc" className="h-[calc(100vh-230px)] min-h-112.5 m-0 -m-3">
                  <GrpcRequestPanel
                    request={activeRequest}
                    onChange={updateActiveRequest}
                    isExecuting={isExecuting}
                    onCancel={handleCancelStream}
                  />
                </TabsContent>
              )}
              {isStream && (
                <TabsContent value="stream" className="h-[calc(100vh-230px)] min-h-112.5 m-0 -m-3">
                  <StreamRequestPanel
                    request={activeRequest}
                    onChange={updateActiveRequest}
                    isStreaming={isStreaming}
                    streamStatus={streamStatus}
                    onSend={handleStreamSend}
                  />
                </TabsContent>
              )}
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
                      body: {
                        ...prev.body,
                        type: val as BodyDefinition["type"],
                      },
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
                    extensions={[json(), ...bodyVariableExtensions]}
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
                    extensions={[javascript(), autocompletion({ override: [pebbleScriptCompletions] })]}
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
                    extensions={[javascript(), autocompletion({ override: [pebbleScriptCompletions] })]}
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
            <div className="space-y-4 text-xs">
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
                      auth: {
                        ...prev.auth,
                        type: val as AuthDefinition["type"],
                      },
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
                    <SelectItem value="basic">Basic Auth (Username / Password)</SelectItem>
                    <SelectItem value="apiKey">API Key</SelectItem>
                    <SelectItem value="digest">Digest Auth</SelectItem>
                    <SelectItem value="oauth2">OAuth 2.0</SelectItem>
                    <SelectItem value="awsSigV4">AWS Signature v4</SelectItem>
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

              {/* Inherited Auth Info */}
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

              {/* Bearer Token */}
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
                    className="w-full bg-white dark:bg-zinc-950 font-mono"
                  />
                  <p className="text-[10px] text-zinc-500">
                    Sent as: <code className="font-mono">Authorization: Bearer &lt;token&gt;</code>
                  </p>
                </div>
              )}

              {/* Basic Auth */}
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
                    Sent as: <code className="font-mono">Authorization: Basic base64(user:pass)</code>
                  </p>
                </div>
              )}

              {/* API Key */}
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
                        placeholder="X-API-KEY or {{API_KEY_NAME}}"
                        className="w-full bg-white dark:bg-zinc-950"
                      />
                    </div>
                    <div>
                      <label className="block text-zinc-500 dark:text-zinc-400 mb-1">
                        Value
                      </label>
                      <Input
                        type="password"
                        value={activeRequest.auth.value || ""}
                        onChange={(e) =>
                          updateActiveRequest((prev) => ({
                            ...prev,
                            auth: { ...prev.auth, value: e.target.value },
                          }))
                        }
                        placeholder="secret-key or {{API_KEY_VALUE}}"
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
                          auth: {
                            ...prev.auth,
                            addTo: val as AuthDefinition["addTo"],
                          },
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

              {/* Digest Auth */}
              {activeRequest.auth.type === "digest" && (
                <div className="space-y-3 border border-zinc-200 dark:border-zinc-800 rounded-md p-4 bg-zinc-50/50 dark:bg-zinc-900/30">
                  <div className="flex items-center gap-2 text-zinc-800 dark:text-zinc-200 font-medium">
                    <Lock className="w-4 h-4 text-blue-500" />
                    <span>Digest Authentication (RFC 7616 / RFC 2617)</span>
                  </div>
                  <p className="text-[11px] text-zinc-500">
                    Automatically responds to 401 Unauthorized challenges with MD5 / SHA-256 digest responses.
                  </p>
                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <label className="block text-zinc-500 mb-1">Username</label>
                      <Input
                        type="text"
                        value={activeRequest.auth.username || ""}
                        onChange={(e) =>
                          updateActiveRequest((prev) => ({
                            ...prev,
                            auth: { ...prev.auth, username: e.target.value },
                          }))
                        }
                        placeholder="username or {{DIGEST_USER}}"
                        className="h-8 bg-white dark:bg-zinc-950"
                      />
                    </div>
                    <div>
                      <label className="block text-zinc-500 mb-1">Password</label>
                      <Input
                        type="password"
                        value={activeRequest.auth.password || ""}
                        onChange={(e) =>
                          updateActiveRequest((prev) => ({
                            ...prev,
                            auth: { ...prev.auth, password: e.target.value },
                          }))
                        }
                        placeholder="password or {{DIGEST_PASS}}"
                        className="h-8 bg-white dark:bg-zinc-950"
                      />
                    </div>
                  </div>
                  <div>
                    <label className="block text-zinc-500 mb-1">Realm (Optional)</label>
                    <Input
                      type="text"
                      value={activeRequest.auth.realm || ""}
                      onChange={(e) =>
                        updateActiveRequest((prev) => ({
                          ...prev,
                          auth: { ...prev.auth, realm: e.target.value },
                        }))
                      }
                      placeholder="Leave empty to auto-negotiate with server challenge"
                      className="h-8 bg-white dark:bg-zinc-950"
                    />
                  </div>
                </div>
              )}

              {/* OAuth 2.0 */}
              {activeRequest.auth.type === "oauth2" && (
                <div className="space-y-4 border border-zinc-200 dark:border-zinc-800 rounded-md p-4 bg-zinc-50/50 dark:bg-zinc-900/30">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-2 text-zinc-800 dark:text-zinc-200 font-medium">
                      <Key className="w-4 h-4 text-emerald-500" />
                      <span>OAuth 2.0 Configuration</span>
                    </div>
                    {activeRequest.auth.token && (
                      <Badge
                        variant="outline"
                        className="text-[10px] bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border-emerald-500/20"
                      >
                        Token Active
                      </Badge>
                    )}
                  </div>

                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <label className="block text-zinc-500 mb-1">Grant Type</label>
                      <Select
                        value={activeRequest.auth.grantType || "client_credentials"}
                        onValueChange={(val) =>
                          updateActiveRequest((prev) => ({
                            ...prev,
                            auth: {
                              ...prev.auth,
                              grantType: val as AuthDefinition["grantType"],
                            },
                          }))
                        }
                      >
                        <SelectTrigger className="h-8 bg-white dark:bg-zinc-950">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="client_credentials">Client Credentials</SelectItem>
                          <SelectItem value="authorization_code">
                            Authorization Code (with PKCE)
                          </SelectItem>
                        </SelectContent>
                      </Select>
                    </div>

                    <div>
                      <label className="block text-zinc-500 mb-1">Token URL *</label>
                      <Input
                        type="text"
                        value={activeRequest.auth.tokenUrl || ""}
                        onChange={(e) =>
                          updateActiveRequest((prev) => ({
                            ...prev,
                            auth: { ...prev.auth, tokenUrl: e.target.value },
                          }))
                        }
                        placeholder="https://oauth2.example.com/token or {{TOKEN_URL}}"
                        className="h-8 bg-white dark:bg-zinc-950 font-mono text-xs"
                      />
                    </div>
                  </div>

                  {activeRequest.auth.grantType === "authorization_code" && (
                    <div className="p-3 bg-amber-500/5 border border-amber-500/20 rounded text-xs space-y-2">
                      <label className="block font-medium text-amber-700 dark:text-amber-400">
                        Authorization URL * (PKCE S256)
                      </label>
                      <Input
                        type="text"
                        value={activeRequest.auth.authUrl || ""}
                        onChange={(e) =>
                          updateActiveRequest((prev) => ({
                            ...prev,
                            auth: { ...prev.auth, authUrl: e.target.value },
                          }))
                        }
                        placeholder="https://oauth2.example.com/authorize"
                        className="h-8 bg-white dark:bg-zinc-950 font-mono text-xs"
                      />
                      <p className="text-[10px] text-zinc-500">
                        Clicking "Authorize" opens your system browser and catches the loopback callback automatically.
                      </p>
                    </div>
                  )}

                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <label className="block text-zinc-500 mb-1">Client ID</label>
                      <Input
                        type="text"
                        value={activeRequest.auth.clientId || ""}
                        onChange={(e) =>
                          updateActiveRequest((prev) => ({
                            ...prev,
                            auth: { ...prev.auth, clientId: e.target.value },
                          }))
                        }
                        placeholder="client-id or {{CLIENT_ID}}"
                        className="h-8 bg-white dark:bg-zinc-950 text-xs"
                      />
                    </div>
                    <div>
                      <label className="block text-zinc-500 mb-1">Client Secret</label>
                      <Input
                        type="password"
                        value={activeRequest.auth.clientSecret || ""}
                        onChange={(e) =>
                          updateActiveRequest((prev) => ({
                            ...prev,
                            auth: { ...prev.auth, clientSecret: e.target.value },
                          }))
                        }
                        placeholder="client-secret or {{CLIENT_SECRET}}"
                        className="h-8 bg-white dark:bg-zinc-950 text-xs"
                      />
                    </div>
                  </div>

                  <div>
                    <label className="block text-zinc-500 mb-1">Scope (Optional)</label>
                    <Input
                      type="text"
                      value={activeRequest.auth.scope || ""}
                      onChange={(e) =>
                        updateActiveRequest((prev) => ({
                          ...prev,
                          auth: { ...prev.auth, scope: e.target.value },
                        }))
                      }
                      placeholder="read write email or {{OAUTH_SCOPE}}"
                      className="h-8 bg-white dark:bg-zinc-950 text-xs"
                    />
                  </div>

                  <div className="pt-2 flex items-center gap-3">
                    <Button
                      type="button"
                      size="sm"
                      onClick={handleFetchOAuthToken}
                      disabled={isOAuthAuthorizing}
                      className="h-8 text-xs bg-emerald-600 hover:bg-emerald-700 text-white gap-1.5 cursor-pointer"
                    >
                      <RefreshCw className={`w-3.5 h-3.5 ${isOAuthAuthorizing ? "animate-spin" : ""}`} />
                      {isOAuthAuthorizing
                        ? "Authorizing..."
                        : activeRequest.auth.grantType === "authorization_code"
                        ? "Authorize in Browser"
                        : "Fetch Token"}
                    </Button>

                    {activeRequest.auth.token && (
                      <span className="text-[11px] text-zinc-500">
                        Token stored & will auto-refresh before expiration.
                      </span>
                    )}
                  </div>

                  {oauthError && (
                    <div className="p-2.5 rounded bg-red-500/10 border border-red-500/20 text-red-600 dark:text-red-400 text-xs">
                      {oauthError}
                    </div>
                  )}

                  {activeRequest.auth.token && (
                    <div className="pt-2 space-y-1">
                      <label className="block text-zinc-500 text-[11px]">Current Access Token</label>
                      <Input
                        type="text"
                        value={activeRequest.auth.token}
                        onChange={(e) =>
                          updateActiveRequest((prev) => ({
                            ...prev,
                            auth: { ...prev.auth, token: e.target.value },
                          }))
                        }
                        className="h-8 font-mono text-[11px] bg-white dark:bg-zinc-950"
                      />
                    </div>
                  )}
                </div>
              )}

              {/* AWS Signature v4 */}
              {activeRequest.auth.type === "awsSigV4" && (
                <div className="space-y-3 border border-zinc-200 dark:border-zinc-800 rounded-md p-4 bg-zinc-50/50 dark:bg-zinc-900/30">
                  <div className="flex items-center gap-2 text-zinc-800 dark:text-zinc-200 font-medium">
                    <Shield className="w-4 h-4 text-amber-500" />
                    <span>AWS Signature v4</span>
                  </div>
                  <p className="text-[11px] text-zinc-500">
                    Calculates HMAC-SHA256 request signatures, X-Amz-Date, and canonical headers on send.
                  </p>

                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <label className="block text-zinc-500 mb-1">Access Key ID *</label>
                      <Input
                        type="text"
                        value={activeRequest.auth.accessKey || ""}
                        onChange={(e) =>
                          updateActiveRequest((prev) => ({
                            ...prev,
                            auth: { ...prev.auth, accessKey: e.target.value },
                          }))
                        }
                        placeholder="AKIAIOSFODNN7EXAMPLE"
                        className="h-8 bg-white dark:bg-zinc-950 font-mono text-xs"
                      />
                    </div>
                    <div>
                      <label className="block text-zinc-500 mb-1">Secret Access Key *</label>
                      <Input
                        type="password"
                        value={activeRequest.auth.secretKey || ""}
                        onChange={(e) =>
                          updateActiveRequest((prev) => ({
                            ...prev,
                            auth: { ...prev.auth, secretKey: e.target.value },
                          }))
                        }
                        placeholder="wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
                        className="h-8 bg-white dark:bg-zinc-950 font-mono text-xs"
                      />
                    </div>
                  </div>

                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <label className="block text-zinc-500 mb-1">AWS Region</label>
                      <Input
                        type="text"
                        value={activeRequest.auth.region || ""}
                        onChange={(e) =>
                          updateActiveRequest((prev) => ({
                            ...prev,
                            auth: { ...prev.auth, region: e.target.value },
                          }))
                        }
                        placeholder="us-east-1"
                        className="h-8 bg-white dark:bg-zinc-950 text-xs"
                      />
                    </div>
                    <div>
                      <label className="block text-zinc-500 mb-1">AWS Service</label>
                      <Input
                        type="text"
                        value={activeRequest.auth.service || ""}
                        onChange={(e) =>
                          updateActiveRequest((prev) => ({
                            ...prev,
                            auth: { ...prev.auth, service: e.target.value },
                          }))
                        }
                        placeholder="s3 or execute-api"
                        className="h-8 bg-white dark:bg-zinc-950 text-xs"
                      />
                    </div>
                  </div>

                  <div>
                    <label className="block text-zinc-500 mb-1">Session Token (Optional)</label>
                    <Input
                      type="password"
                      value={activeRequest.auth.sessionToken || ""}
                      onChange={(e) =>
                        updateActiveRequest((prev) => ({
                          ...prev,
                          auth: { ...prev.auth, sessionToken: e.target.value },
                        }))
                      }
                      placeholder="Security STS token or {{AWS_SESSION_TOKEN}}"
                      className="h-8 bg-white dark:bg-zinc-950 font-mono text-xs"
                    />
                  </div>
                </div>
              )}

              {/* No Auth */}
              {activeRequest.auth.type === "none" && (
                <div className="text-zinc-500 border border-dashed border-zinc-200 dark:border-zinc-800 rounded-md p-4 text-center">
                  No authentication configured for this request.
                </div>
              )}
            </div>
          </TabsContent>

          <TabsContent value="settings">
            <div className="p-4 border border-zinc-200 dark:border-zinc-800 rounded-lg bg-zinc-50/50 dark:bg-zinc-900/30 divide-y divide-zinc-200/80 dark:divide-zinc-800/80 max-w-2xl text-xs space-y-4">
              {/* SSL/TLS Verification Toggle + Warning */}
              <div className="pt-2 first:pt-0 space-y-3">
                <label
                  htmlFor="setting-verify-ssl"
                  className="flex items-start gap-3 cursor-pointer group select-none"
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
                      Validate remote SSL/TLS certificates when sending HTTPS requests.
                    </span>
                  </div>
                </label>

                {!activeRequest.settings.verifySSL && (
                  <div className="p-3 bg-amber-500/10 border border-amber-500/30 rounded-md flex items-start gap-2.5 text-amber-700 dark:text-amber-400">
                    <ShieldAlert className="w-4 h-4 shrink-0 mt-0.5" />
                    <div>
                      <span className="font-semibold block">Warning: SSL Verification Disabled</span>
                      <span className="text-[11px]">
                        Requests will accept self-signed and invalid certificates. This makes network connections vulnerable to Man-in-the-Middle (MITM) attacks.
                      </span>
                    </div>
                  </div>
                )}
              </div>

              {/* Redirects */}
              <div className="pt-3 space-y-3">
                <label
                  htmlFor="setting-follow-redirects"
                  className="flex items-start gap-3 cursor-pointer group select-none"
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
                      Automatically follow 3xx redirect status codes returned by the server. Sensitive auth credentials are automatically stripped across origins.
                    </span>
                  </div>
                </label>

                {activeRequest.settings.followRedirects && (
                  <div className="pl-7 flex items-center gap-2">
                    <span className="text-zinc-500 text-[11px]">Max Redirect Hops:</span>
                    <Input
                      type="number"
                      min={1}
                      max={50}
                      value={activeRequest.settings.maxRedirects || 10}
                      onChange={(e) =>
                        updateActiveRequest((prev) => ({
                          ...prev,
                          settings: {
                            ...prev.settings,
                            maxRedirects: parseInt(e.target.value, 10) || 10,
                          },
                        }))
                      }
                      className="w-20 h-7 text-xs bg-white dark:bg-zinc-950"
                    />
                  </div>
                )}
              </div>

              {/* Cookie Jar Toggle & Management */}
              <div className="pt-3 flex items-start justify-between gap-4">
                <label
                  htmlFor="setting-enable-cookies"
                  className="flex items-start gap-3 cursor-pointer group select-none flex-1"
                >
                  <div className="pt-0.5">
                    <Checkbox
                      id="setting-enable-cookies"
                      checked={activeRequest.settings.enableCookies !== false}
                      onCheckedChange={(checked) =>
                        updateActiveRequest((prev) => ({
                          ...prev,
                          settings: {
                            ...prev.settings,
                            enableCookies: !!checked,
                          },
                        }))
                      }
                    />
                  </div>
                  <div className="flex flex-col gap-0.5">
                    <span className="text-xs font-medium text-zinc-800 dark:text-zinc-200 group-hover:text-blue-600 dark:group-hover:text-blue-400 transition-colors flex items-center gap-1.5">
                      <Cookie className="w-3.5 h-3.5 text-amber-500" />
                      Enable Workspace Cookie Jar
                    </span>
                    <span className="text-[11px] text-zinc-500 dark:text-zinc-400">
                      Automatically store Set-Cookie headers and send matching cookies for this domain. Never tracked in Git.
                    </span>
                  </div>
                </label>

                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setIsCookieManagerOpen(true)}
                  className="h-7 text-xs gap-1.5 shrink-0 cursor-pointer"
                >
                  <Cookie className="w-3.5 h-3.5 text-amber-500" />
                  Manage Cookies
                </Button>
              </div>

              {/* Timeouts */}
              <div className="pt-3 grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-zinc-700 dark:text-zinc-300 font-medium mb-1">
                    Request Timeout (ms)
                  </label>
                  <Input
                    type="number"
                    value={activeRequest.settings.timeoutMs}
                    onChange={(e) =>
                      updateActiveRequest((prev) => ({
                        ...prev,
                        settings: {
                          ...prev.settings,
                          timeoutMs: parseInt(e.target.value, 10) || 0,
                        },
                      }))
                    }
                    placeholder="30000"
                    className="h-8 bg-white dark:bg-zinc-950"
                  />
                  <span className="text-[10px] text-zinc-400">0 = use client default (30s)</span>
                </div>
                <div>
                  <label className="block text-zinc-700 dark:text-zinc-300 font-medium mb-1">
                    Connect Timeout (ms)
                  </label>
                  <Input
                    type="number"
                    value={activeRequest.settings.connectTimeoutMs || ""}
                    onChange={(e) =>
                      updateActiveRequest((prev) => ({
                        ...prev,
                        settings: {
                          ...prev.settings,
                          connectTimeoutMs: parseInt(e.target.value, 10) || 0,
                        },
                      }))
                    }
                    placeholder="10000"
                    className="h-8 bg-white dark:bg-zinc-950"
                  />
                  <span className="text-[10px] text-zinc-400">0 = use default (10s)</span>
                </div>
              </div>

              {/* User-Agent */}
              <div className="pt-3">
                <label className="block text-zinc-700 dark:text-zinc-300 font-medium mb-1">
                  Custom User-Agent
                </label>
                <Input
                  type="text"
                  value={activeRequest.settings.userAgent || ""}
                  onChange={(e) =>
                    updateActiveRequest((prev) => ({
                      ...prev,
                      settings: {
                        ...prev.settings,
                        userAgent: e.target.value,
                      },
                    }))
                  }
                  placeholder="PebblePost/1.0"
                  className="h-8 bg-white dark:bg-zinc-950"
                />
              </div>

              {/* Proxy */}
              <div className="pt-3">
                <label className="block text-zinc-700 dark:text-zinc-300 font-medium mb-1 flex items-center gap-1.5">
                  <Globe className="w-3.5 h-3.5 text-zinc-400" />
                  Proxy Configuration
                </label>
                <Input
                  type="text"
                  value={activeRequest.settings.proxyUrl || ""}
                  onChange={(e) =>
                    updateActiveRequest((prev) => ({
                      ...prev,
                      settings: {
                        ...prev.settings,
                        proxyUrl: e.target.value,
                      },
                    }))
                  }
                  placeholder="http://127.0.0.1:8080 or socks5://user:pass@127.0.0.1:1080"
                  className="h-8 bg-white dark:bg-zinc-950 font-mono text-xs"
                />
                <span className="text-[10px] text-zinc-400">
                  Supports HTTP, HTTPS, and SOCKS5 proxy URLs. Leave empty to use system environment proxies.
                </span>
              </div>

              {/* Client Certificate (mTLS) */}
              <div className="pt-3 space-y-2">
                <label className="block text-zinc-700 dark:text-zinc-300 font-medium flex items-center gap-1.5">
                  <Shield className="w-3.5 h-3.5 text-zinc-400" />
                  Client Certificate (mTLS)
                </label>
                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <span className="text-[11px] text-zinc-500 block mb-1">Certificate Path (.pem / .crt)</span>
                    <Input
                      type="text"
                      value={activeRequest.settings.clientCertPath || ""}
                      onChange={(e) =>
                        updateActiveRequest((prev) => ({
                          ...prev,
                          settings: {
                            ...prev.settings,
                            clientCertPath: e.target.value,
                          },
                        }))
                      }
                      placeholder="/path/to/client.crt or {{CERT_PATH}}"
                      className="h-8 bg-white dark:bg-zinc-950 font-mono text-[11px]"
                    />
                  </div>
                  <div>
                    <span className="text-[11px] text-zinc-500 block mb-1">Private Key Path (.key)</span>
                    <Input
                      type="text"
                      value={activeRequest.settings.clientKeyPath || ""}
                      onChange={(e) =>
                        updateActiveRequest((prev) => ({
                          ...prev,
                          settings: {
                            ...prev.settings,
                            clientKeyPath: e.target.value,
                          },
                        }))
                      }
                      placeholder="/path/to/client.key or {{KEY_PATH}}"
                      className="h-8 bg-white dark:bg-zinc-950 font-mono text-[11px]"
                    />
                  </div>
                </div>
              </div>
            </div>
          </TabsContent>

          <TabsContent value="docs" className="h-[calc(100vh-230px)] min-h-112.5 m-0 -m-3">
            <RequestDocsTab
              description={activeRequest.description}
              onChange={(newDesc) =>
                updateActiveRequest((prev) => ({
                  ...prev,
                  description: newDesc,
                }))
              }
            />
          </TabsContent>

          <TabsContent value="examples" className="h-[calc(100vh-230px)] min-h-112.5 m-0 -m-3">
            <RequestExamplesTab
              filePath={currentTab.filePath}
              examples={activeRequest.examples || []}
              onExamplesChange={(newExamples) =>
                updateActiveRequest((prev) => ({
                  ...prev,
                  examples: newExamples,
                }))
              }
            />
          </TabsContent>
        </div>
      </Tabs>
        );
      })()}

      <CookieManagerDialog
        isOpen={isCookieManagerOpen}
        onClose={() => setIsCookieManagerOpen(false)}
      />

      <ScriptTrustDialog
        open={isTrustDialogOpen}
        onOpenChange={setIsTrustDialogOpen}
        workspacePath={useWorkspaceStore.getState().workspacePath}
        hasPreRequest={Boolean(activeRequest?.scripts?.preRequest?.trim())}
        hasPostResponse={Boolean(activeRequest?.scripts?.postResponse?.trim())}
        onConfirmTrust={() => {
          const { workspacePath, setWorkspaceTrusted } = useWorkspaceStore.getState();
          setWorkspaceTrusted(true, workspacePath || undefined);
          setIsTrustDialogOpen(false);
          executeRequest(true);
        }}
        onRunWithoutScripts={() => {
          setIsTrustDialogOpen(false);
          executeRequest(false);
        }}
        onCancel={() => setIsTrustDialogOpen(false)}
      />

      <UndefinedVariablesDialog
        isOpen={undefinedDialogOpen}
        undefinedVars={pendingUndefinedVars}
        activeEnv={activeEnv}
        onCancel={() => setUndefinedDialogOpen(false)}
        onConfirmSend={handleConfirmSendAnyway}
        onOpenManageEnvironments={() => {
          setUndefinedDialogOpen(false);
          setIsEnvManagerOpen(true);
        }}
      />

      <ManageEnvironmentsDialog
        isOpen={isEnvManagerOpen}
        onClose={() => setIsEnvManagerOpen(false)}
      />
    </div>
  );
}
