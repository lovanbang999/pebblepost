import React, { useState, useMemo, useRef, useEffect } from "react";
import {
  Play,
  Square,
  Upload,
  FileSpreadsheet,
  Download,
  CheckCircle2,
  XCircle,
  Clock,
  Check,
  ChevronDown,
  ChevronRight,
  Search,
  Layers,
  FileCode,
  Eye,
  Trash2,
} from "lucide-react";
import { useWorkspaceStore } from "../../store/workspaceStore";
import { type RequestTab } from "../../store/tabStore";
import type { TreeNode, RequestRunResult, RunSummary } from "../../types";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { Badge } from "../ui/badge";
import { Checkbox } from "../ui/checkbox";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../ui/select";
import { cn, getMethodTextColor } from "../../lib/utils";

interface RunnerPanelProps {
  currentTab: RequestTab;
}

interface FlatRequestItem {
  id: string;
  name: string;
  path: string;
  relPath: string;
  method: string;
}

export function RunnerPanel({ currentTab }: RunnerPanelProps) {
  const { tree, environments, activeEnv, setActiveEnv, workspacePath } =
    useWorkspaceStore();

  const targetFolder = currentTab.runnerConfig?.folderPath || "";
  const folderTitle =
    currentTab.runnerConfig?.folderName ||
    (targetFolder ? targetFolder.split("/").pop() : "All Collections");

  // 1. Flatten all request nodes under targetFolder
  const availableRequests = useMemo(() => {
    const list: FlatRequestItem[] = [];
    const walk = (node: TreeNode) => {
      if (node.isDir) {
        if (node.children) {
          node.children.forEach(walk);
        }
      } else if (node.path.endsWith(".pebble.json")) {
        if (
          !targetFolder ||
          node.path.startsWith(targetFolder) ||
          node.relPath.startsWith(targetFolder)
        ) {
          list.push({
            id: node.id || node.path,
            name: node.displayName || node.name.replace(/\.pebble\.json$/, ""),
            path: node.path,
            relPath: node.relPath,
            method: node.method || "GET",
          });
        }
      }
    };
    tree.forEach(walk);
    return list;
  }, [tree, targetFolder]);

  // 2. Selection state
  const [selectedPaths, setSelectedPaths] = useState<Record<string, boolean>>(
    {},
  );

  // Initialize selection when availableRequests loads
  useEffect(() => {
    const initial: Record<string, boolean> = {};
    availableRequests.forEach((req) => {
      initial[req.path] = true;
    });
    setSelectedPaths(initial);
  }, [availableRequests]);

  // 3. Execution configuration state
  const [iterations, setIterations] = useState<number>(1);
  const [delayMs, setDelayMs] = useState<number>(0);
  const [bail, setBail] = useState<boolean>(false);
  const [searchFilter, setSearchFilter] = useState<string>("");

  // 4. Data-driven file state
  const [dataFile, setDataFile] = useState<string | null>(null);
  const [dataRows, setDataRows] = useState<Record<string, unknown>[] | null>(
    null,
  );
  const [dataColumns, setDataColumns] = useState<string[]>([]);
  const [isDataPreviewOpen, setIsDataPreviewOpen] = useState(false);
  const [isParsingData, setIsParsingData] = useState(false);
  const fileInputRef = useRef<HTMLInputElement | null>(null);

  // 5. Execution & results state
  const [isRunning, setIsRunning] = useState(false);
  const [activeRunId, setActiveRunId] = useState<string | null>(null);
  const [liveResults, setLiveResults] = useState<RequestRunResult[]>([]);
  const [summary, setSummary] = useState<RunSummary | null>(null);
  const [currentProgress, setCurrentProgress] = useState<{
    current: number;
    total: number;
  }>({ current: 0, total: 0 });
  const [expandedResults, setExpandedResults] = useState<
    Record<string, boolean>
  >({});
  const [selectedIterationFilter, setSelectedIterationFilter] = useState<
    number | "all"
  >("all");
  const abortControllerRef = useRef<AbortController | null>(null);

  const selectedCount = useMemo(() => {
    return availableRequests.filter((r) => selectedPaths[r.path]).length;
  }, [availableRequests, selectedPaths]);

  const filteredRequests = useMemo(() => {
    if (!searchFilter.trim()) return availableRequests;
    const q = searchFilter.toLowerCase();
    return availableRequests.filter(
      (r) =>
        r.name.toLowerCase().includes(q) ||
        r.relPath.toLowerCase().includes(q) ||
        r.method.toLowerCase().includes(q),
    );
  }, [availableRequests, searchFilter]);

  const handleSelectAll = (checked: boolean) => {
    const updated: Record<string, boolean> = {};
    availableRequests.forEach((r) => {
      updated[r.path] = checked;
    });
    setSelectedPaths(updated);
  };

  const handleToggleRequest = (path: string) => {
    setSelectedPaths((prev) => ({
      ...prev,
      [path]: !prev[path],
    }));
  };

  // Handle data file selection
  const handleDataFileChange = async (
    e: React.ChangeEvent<HTMLInputElement>,
  ) => {
    const file = e.target.files?.[0];
    if (!file) return;

    setIsParsingData(true);
    try {
      const content = await file.text();
      const res = await fetch("/api/runner/parse-data", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          filename: file.name,
          content,
        }),
      });

      if (!res.ok) {
        const err = await res.json();
        alert(`Failed to parse ${file.name}: ${err.error || "Unknown error"}`);
        return;
      }

      const parsed = await res.json();
      setDataFile(file.name);
      setDataRows(parsed.rows);
      setDataColumns(parsed.columns || []);
      if (parsed.totalRows > 0) {
        setIterations(parsed.totalRows);
      }
    } catch (err: unknown) {
      alert(
        `Error reading file: ${err instanceof Error ? err.message : String(err)}`,
      );
    } finally {
      setIsParsingData(false);
      if (fileInputRef.current) {
        fileInputRef.current.value = "";
      }
    }
  };

  const handleClearData = () => {
    setDataFile(null);
    setDataRows(null);
    setDataColumns([]);
    setIterations(1);
  };

  // Handle Run Execution via SSE
  const handleStartRun = async () => {
    const chosenPaths = availableRequests
      .filter((r) => selectedPaths[r.path])
      .map((r) => r.path);
    if (chosenPaths.length === 0) {
      alert("Please select at least one request to run.");
      return;
    }

    const runId = `run_${Date.now()}`;
    setActiveRunId(runId);
    setIsRunning(true);
    setLiveResults([]);
    setSummary(null);
    const totalExpected = chosenPaths.length * iterations;
    setCurrentProgress({ current: 0, total: totalExpected });

    const abortController = new AbortController();
    abortControllerRef.current = abortController;

    try {
      const response = await fetch("/api/runner/run", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Accept: "text/event-stream",
        },
        body: JSON.stringify({
          runId,
          workspacePath: workspacePath || ".",
          targetPath: targetFolder || ".",
          requestPaths: chosenPaths,
          environmentName: activeEnv,
          iterations,
          delayMs,
          bail,
          dataRows: dataRows || undefined,
          stream: true,
        }),
        signal: abortController.signal,
      });

      if (!response.ok || !response.body) {
        const text = await response.text();
        throw new Error(`Runner error: ${text || response.statusText}`);
      }

      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = "";

      while (true) {
        const { value, done } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split("\n\n");
        buffer = lines.pop() || "";

        for (const block of lines) {
          if (!block.trim()) continue;
          let eventType = "message";
          let eventData = "";

          const blockLines = block.split("\n");
          for (const line of blockLines) {
            if (line.startsWith("event: ")) {
              eventType = line.substring(7).trim();
            } else if (line.startsWith("data: ")) {
              eventData = line.substring(6).trim();
            }
          }

          if (!eventData) continue;

          try {
            const parsed = JSON.parse(eventData);
            if (eventType === "request_result") {
              const res = parsed as RequestRunResult;
              setLiveResults((prev) => [...prev, res]);
              setCurrentProgress((prev) => ({
                ...prev,
                current: prev.current + 1,
              }));
            } else if (eventType === "summary" || eventType === "complete") {
              setSummary(parsed as RunSummary);
            }
          } catch {
            // ignore non-json
          }
        }
      }
    } catch (err: unknown) {
      if ((err as Error)?.name !== "AbortError") {
        console.error("Run failed:", err);
      }
    } finally {
      setIsRunning(false);
      setActiveRunId(null);
      abortControllerRef.current = null;
    }
  };

  // Handle Stop
  const handleStopRun = async () => {
    if (activeRunId) {
      try {
        await fetch("/api/runner/stop", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ runId: activeRunId }),
        });
      } catch {
        // ignore
      }
    }
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
    }
    setIsRunning(false);
  };

  // Export JSON Report
  const handleExportJSON = () => {
    if (!summary) return;
    const blob = new Blob([JSON.stringify(summary, null, 2)], {
      type: "application/json",
    });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `pebblepost-run-${Date.now()}.json`;
    a.click();
    URL.revokeObjectURL(url);
  };

  // Export HTML Report
  const handleExportHTML = () => {
    if (!summary) return;
    const rowsHtml = summary.results
      .map(
        (r) => `
      <tr class="${r.passed ? "pass" : "fail"}">
        <td><span class="badge ${r.passed ? "pass" : "fail"}">${r.passed ? "PASS" : "FAIL"}</span></td>
        ${summary.totalIterations > 1 ? `<td>#${r.iteration || 1}</td>` : ""}
        <td><code>${r.request?.method || "REQ"}</code></td>
        <td>${r.relPath}</td>
        <td>${r.result?.statusCode || "-"}</td>
        <td>${Math.round(r.duration / 1000000)}ms</td>
        <td>${r.error || ""}</td>
      </tr>`,
      )
      .join("");

    const htmlContent = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<title>PebblePost Run Report</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; background: #09090b; color: #f4f4f5; padding: 2rem; }
  h1 { font-size: 1.5rem; margin-bottom: 0.5rem; }
  .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(140px, 1fr)); gap: 1rem; margin: 1.5rem 0; }
  .card { background: #18181b; border: 1px solid #27272a; border-radius: 0.5rem; padding: 1rem; }
  .card-val { font-size: 1.5rem; font-weight: 700; margin-top: 0.25rem; }
  .green { color: #22c55e; } .red { color: #ef4444; } .blue { color: #3b82f6; }
  table { width: 100%; border-collapse: collapse; background: #18181b; border-radius: 0.5rem; overflow: hidden; margin-top: 1rem; }
  th, td { padding: 0.75rem 1rem; text-align: left; border-bottom: 1px solid #27272a; font-size: 0.875rem; }
  .badge { display: inline-block; padding: 0.15rem 0.5rem; border-radius: 9999px; font-weight: 700; font-size: 0.75rem; }
  .badge.pass { background: #14532d; color: #4ade80; } .badge.fail { background: #450a0a; color: #f87171; }
</style>
</head>
<body>
  <h1>PebblePost Test Run Report</h1>
  <p style="color:#a1a1aa">Target: ${summary.target} | Environment: ${summary.environment || "none"}</p>
  <div class="grid">
    <div class="card"><div>Status</div><div class="card-val ${summary.success ? "green" : "red"}">${summary.success ? "PASSED" : "FAILED"}</div></div>
    <div class="card"><div>Pass Rate</div><div class="card-val ${summary.passRate === 100 ? "green" : "red"}">${summary.passRate.toFixed(1)}%</div></div>
    <div class="card"><div>Requests</div><div class="card-val blue">${summary.passedRequests} / ${summary.totalRequests}</div></div>
    <div class="card"><div>Assertions</div><div class="card-val blue">${summary.passedTests} / ${summary.totalTests}</div></div>
    <div class="card"><div>Avg Latency</div><div class="card-val">${summary.avgDurationMs.toFixed(1)}ms</div></div>
    <div class="card"><div>P95 Latency</div><div class="card-val">${summary.p95DurationMs.toFixed(1)}ms</div></div>
  </div>
  <table>
    <thead><tr><th>Status</th>${summary.totalIterations > 1 ? "<th>Iter</th>" : ""}<th>Method</th><th>Path</th><th>HTTP</th><th>Duration</th><th>Error</th></tr></thead>
    <tbody>${rowsHtml}</tbody>
  </table>
</body>
</html>`;

    const blob = new Blob([htmlContent], { type: "text/html" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `pebblepost-report-${Date.now()}.html`;
    a.click();
    URL.revokeObjectURL(url);
  };

  // Filtered displayed results
  const displayedResults = useMemo(() => {
    if (selectedIterationFilter === "all") return liveResults;
    return liveResults.filter(
      (r) => (r.iteration || 1) === selectedIterationFilter,
    );
  }, [liveResults, selectedIterationFilter]);

  // Compute live pass rate & latency
  const liveStats = useMemo(() => {
    const passed = liveResults.filter((r) => r.passed && !r.skipped).length;
    const failed = liveResults.filter((r) => !r.passed && !r.skipped).length;
    const totalExec = passed + failed;
    const rate = totalExec > 0 ? (passed / totalExec) * 100 : 100;
    const durations = liveResults
      .filter((r) => !r.skipped)
      .map((r) => r.duration / 1000000);
    const avg =
      durations.length > 0
        ? durations.reduce((a, b) => a + b, 0) / durations.length
        : 0;
    let p95 = 0;
    if (durations.length > 0) {
      const sorted = [...durations].sort((a, b) => a - b);
      const idx = Math.max(
        0,
        Math.min(sorted.length - 1, Math.round(sorted.length * 0.95) - 1),
      );
      p95 = sorted[idx];
    }
    return { passed, failed, totalExec, rate, avg, p95 };
  }, [liveResults]);

  const progressPercent =
    currentProgress.total > 0
      ? Math.min(
          100,
          Math.round((currentProgress.current / currentProgress.total) * 100),
        )
      : 0;

  return (
    <div className="flex-1 flex flex-col h-full bg-zinc-50 dark:bg-zinc-950 text-zinc-900 dark:text-zinc-100 overflow-hidden">
      {/* Top Banner / Hero Controls */}
      <div className="border-b border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900/60 p-4 shrink-0 flex flex-wrap items-center justify-between gap-4">
        <div className="flex items-center gap-3">
          <div className="w-10 h-10 rounded-xl bg-emerald-500/10 dark:bg-emerald-500/20 text-emerald-600 dark:text-emerald-400 flex items-center justify-center border border-emerald-500/20">
            <Layers className="w-5 h-5" />
          </div>
          <div>
            <div className="flex items-center gap-2">
              <h1 className="text-base font-bold text-zinc-900 dark:text-zinc-100 tracking-tight">
                {folderTitle}
              </h1>
              <Badge
                variant="outline"
                className="text-[10px] font-mono border-zinc-200 dark:border-zinc-800"
              >
                Runner
              </Badge>
              {targetFolder && (
                <span className="text-xs text-zinc-400 dark:text-zinc-500 font-mono">
                  {targetFolder}
                </span>
              )}
            </div>
            <p className="text-xs text-zinc-500 dark:text-zinc-400 mt-0.5">
              Execute collection requests sequentially, run multi-iteration
              data-driven tests, and export reports.
            </p>
          </div>
        </div>

        {/* Action Buttons */}
        <div className="flex items-center gap-2">
          {summary && (
            <>
              <Button
                variant="outline"
                size="sm"
                onClick={handleExportJSON}
                className="text-xs gap-1.5 h-8 border-zinc-200 dark:border-zinc-800"
              >
                <FileCode className="w-3.5 h-3.5 text-blue-500" />
                <span>Export JSON</span>
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={handleExportHTML}
                className="text-xs gap-1.5 h-8 border-zinc-200 dark:border-zinc-800"
              >
                <Download className="w-3.5 h-3.5 text-emerald-500" />
                <span>Export HTML</span>
              </Button>
            </>
          )}

          {isRunning ? (
            <Button
              variant="destructive"
              size="sm"
              onClick={handleStopRun}
              className="text-xs gap-1.5 h-8 px-4 font-semibold shadow-xs"
            >
              <Square className="w-3.5 h-3.5 fill-current" />
              <span>Stop Execution</span>
            </Button>
          ) : (
            <Button
              size="sm"
              onClick={handleStartRun}
              disabled={selectedCount === 0}
              className="text-xs gap-1.5 h-8 px-4 font-semibold bg-emerald-600 hover:bg-emerald-700 text-white shadow-xs"
            >
              <Play className="w-3.5 h-3.5 fill-current" />
              <span>
                Run {selectedCount} Request{selectedCount === 1 ? "" : "s"}
              </span>
            </Button>
          )}
        </div>
      </div>

      {/* Main Dual-Column Content */}
      <div className="flex-1 flex overflow-hidden">
        {/* Left Column: Configuration & Request Checklist */}
        <div className="w-95 border-r border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900/30 flex flex-col overflow-hidden shrink-0">
          {/* Settings Section */}
          <div className="p-3.5 border-b border-zinc-200 dark:border-zinc-800 space-y-3 bg-zinc-50/50 dark:bg-zinc-900/50 shrink-0">
            <div className="grid grid-cols-2 gap-2.5">
              {/* Environment */}
              <div>
                <label className="text-[11px] font-semibold text-zinc-600 dark:text-zinc-400 block mb-1">
                  Environment
                </label>
                <Select
                  value={activeEnv}
                  onValueChange={(val) => setActiveEnv(val ?? "")}
                >
                  <SelectTrigger className="h-8 text-xs bg-white dark:bg-zinc-950">
                    <SelectValue placeholder="No Environment" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="">(None)</SelectItem>
                    {environments.map((env) => (
                      <SelectItem key={env.name} value={env.name}>
                        {env.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              {/* Iterations */}
              <div>
                <label className="text-[11px] font-semibold text-zinc-600 dark:text-zinc-400 block mb-1">
                  Iterations
                </label>
                <Input
                  type="number"
                  min={1}
                  value={iterations}
                  onChange={(e) =>
                    setIterations(Math.max(1, parseInt(e.target.value) || 1))
                  }
                  className="h-8 text-xs bg-white dark:bg-zinc-950"
                  disabled={isRunning}
                />
              </div>
            </div>

            <div className="grid grid-cols-2 gap-2.5 items-center">
              {/* Delay between requests */}
              <div>
                <label className="text-[11px] font-semibold text-zinc-600 dark:text-zinc-400 block mb-1">
                  Delay (ms)
                </label>
                <Input
                  type="number"
                  min={0}
                  step={50}
                  value={delayMs}
                  onChange={(e) =>
                    setDelayMs(Math.max(0, parseInt(e.target.value) || 0))
                  }
                  placeholder="0 ms"
                  className="h-8 text-xs bg-white dark:bg-zinc-950"
                  disabled={isRunning}
                />
              </div>

              {/* Bail option */}
              <div className="pt-4">
                <label className="flex items-center gap-2 cursor-pointer select-none text-xs text-zinc-700 dark:text-zinc-300">
                  <Checkbox
                    checked={bail}
                    onCheckedChange={(c) => setBail(Boolean(c))}
                    disabled={isRunning}
                  />
                  <span>Stop on first error</span>
                </label>
              </div>
            </div>

            {/* Data-Driven File Upload Section */}
            <div className="pt-1 border-t border-zinc-200/80 dark:border-zinc-800/80">
              <div className="flex items-center justify-between mb-1.5">
                <span className="text-[11px] font-semibold text-zinc-600 dark:text-zinc-400 flex items-center gap-1.5">
                  <FileSpreadsheet className="w-3.5 h-3.5 text-emerald-500" />
                  <span>Data File (CSV / JSON)</span>
                </span>
                {dataFile && (
                  <button
                    onClick={handleClearData}
                    className="text-[10px] text-zinc-400 hover:text-rose-500 flex items-center gap-1 cursor-pointer"
                  >
                    <Trash2 className="w-3 h-3" />
                    <span>Clear</span>
                  </button>
                )}
              </div>

              {dataFile ? (
                <div className="p-2.5 rounded-lg bg-emerald-500/5 border border-emerald-500/20 text-xs">
                  <div className="flex items-center justify-between">
                    <span className="font-semibold text-emerald-700 dark:text-emerald-400 truncate max-w-50">
                      {dataFile}
                    </span>
                    <Badge
                      variant="outline"
                      className="text-[10px] text-emerald-600 border-emerald-500/30"
                    >
                      {dataRows?.length || 0} Rows
                    </Badge>
                  </div>
                  {dataColumns.length > 0 && (
                    <div className="mt-1.5 flex flex-wrap gap-1">
                      {dataColumns.map((col) => (
                        <span
                          key={col}
                          className="px-1.5 py-0.5 rounded text-[10px] bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 font-mono"
                        >
                          {`{{${col}}}`}
                        </span>
                      ))}
                    </div>
                  )}
                  <div className="mt-2 flex justify-end">
                    <button
                      onClick={() => setIsDataPreviewOpen((prev) => !prev)}
                      className="text-[11px] text-emerald-600 dark:text-emerald-400 hover:underline flex items-center gap-1 cursor-pointer"
                    >
                      <Eye className="w-3 h-3" />
                      <span>
                        {isDataPreviewOpen ? "Hide Preview" : "View Preview"}
                      </span>
                    </button>
                  </div>
                </div>
              ) : (
                <div>
                  <input
                    ref={fileInputRef}
                    type="file"
                    accept=".csv,.tsv,.json"
                    onChange={handleDataFileChange}
                    className="hidden"
                    id="runner-data-file"
                  />
                  <label
                    htmlFor="runner-data-file"
                    className="flex flex-col items-center justify-center p-3 rounded-lg border border-dashed border-zinc-300 dark:border-zinc-700 hover:border-emerald-500 dark:hover:border-emerald-500/70 hover:bg-emerald-50/50 dark:hover:bg-emerald-950/20 cursor-pointer transition-all duration-150 text-center"
                  >
                    <Upload className="w-4 h-4 text-zinc-400 mb-1" />
                    <span className="text-xs font-medium text-zinc-700 dark:text-zinc-300">
                      {isParsingData ? "Parsing Data..." : "Import CSV or JSON"}
                    </span>
                    <span className="text-[10px] text-zinc-400 mt-0.5">
                      Columns mapped to {"{{data.var}}"} and {"{{var}}"}
                    </span>
                  </label>
                </div>
              )}

              {/* Data Preview Table */}
              {isDataPreviewOpen && dataRows && dataRows.length > 0 && (
                <div className="mt-2 max-h-40 overflow-auto border border-zinc-200 dark:border-zinc-800 rounded bg-white dark:bg-zinc-950 text-[10px]">
                  <table className="w-full border-collapse">
                    <thead>
                      <tr className="bg-zinc-100 dark:bg-zinc-900 border-b border-zinc-200 dark:border-zinc-800">
                        <th className="p-1 text-left font-mono text-zinc-500">
                          #
                        </th>
                        {dataColumns.map((col) => (
                          <th
                            key={col}
                            className="p-1 text-left font-mono font-semibold text-zinc-700 dark:text-zinc-300"
                          >
                            {col}
                          </th>
                        ))}
                      </tr>
                    </thead>
                    <tbody>
                      {dataRows.slice(0, 5).map((row, idx) => (
                        <tr
                          key={idx}
                          className="border-b border-zinc-100 dark:border-zinc-900"
                        >
                          <td className="p-1 font-mono text-zinc-400">
                            {idx + 1}
                          </td>
                          {dataColumns.map((col) => (
                            <td
                              key={col}
                              className="p-1 font-mono truncate max-w-30"
                            >
                              {String(row[col] ?? "")}
                            </td>
                          ))}
                        </tr>
                      ))}
                    </tbody>
                  </table>
                  {dataRows.length > 5 && (
                    <div className="p-1 text-center text-[10px] text-zinc-400 bg-zinc-50 dark:bg-zinc-900">
                      Showing first 5 of {dataRows.length} rows
                    </div>
                  )}
                </div>
              )}
            </div>
          </div>

          {/* Request Checklist Header */}
          <div className="p-3 border-b border-zinc-200 dark:border-zinc-800 flex items-center justify-between gap-2 shrink-0">
            <div className="flex items-center gap-2">
              <Checkbox
                checked={
                  selectedCount === availableRequests.length &&
                  availableRequests.length > 0
                }
                onCheckedChange={(checked) => handleSelectAll(Boolean(checked))}
                disabled={availableRequests.length === 0 || isRunning}
              />
              <span className="text-xs font-semibold text-zinc-700 dark:text-zinc-300">
                {selectedCount} of {availableRequests.length} Selected
              </span>
            </div>
            <div className="flex items-center gap-1">
              <Button
                variant="ghost"
                size="sm"
                onClick={() => handleSelectAll(true)}
                className="text-[10px] h-6 px-1.5"
                disabled={isRunning}
              >
                All
              </Button>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => handleSelectAll(false)}
                className="text-[10px] h-6 px-1.5"
                disabled={isRunning}
              >
                None
              </Button>
            </div>
          </div>

          {/* Search Filter input */}
          <div className="px-3 py-2 border-b border-zinc-200 dark:border-zinc-800 shrink-0">
            <div className="relative">
              <Search className="w-3.5 h-3.5 absolute left-2.5 top-2.5 text-zinc-400" />
              <Input
                type="text"
                placeholder="Filter requests..."
                value={searchFilter}
                onChange={(e) => setSearchFilter(e.target.value)}
                className="pl-8 h-8 text-xs bg-white dark:bg-zinc-950"
              />
            </div>
          </div>

          {/* Request Checklist Items */}
          <div className="flex-1 overflow-y-auto p-2 space-y-1">
            {filteredRequests.length === 0 ? (
              <div className="p-6 text-center text-xs text-zinc-400 dark:text-zinc-500">
                No requests found matching criteria.
              </div>
            ) : (
              filteredRequests.map((req, idx) => {
                const isChecked = Boolean(selectedPaths[req.path]);
                return (
                  <div
                    key={req.path}
                    onClick={() => !isRunning && handleToggleRequest(req.path)}
                    className={cn(
                      "flex items-center gap-2.5 p-2 rounded-lg cursor-pointer transition-colors text-xs select-none",
                      isChecked
                        ? "bg-zinc-100/80 dark:bg-zinc-800/50 text-zinc-900 dark:text-zinc-100"
                        : "hover:bg-zinc-50 dark:hover:bg-zinc-900/40 text-zinc-500 dark:text-zinc-400",
                    )}
                  >
                    <Checkbox
                      checked={isChecked}
                      disabled={isRunning}
                      onCheckedChange={() => handleToggleRequest(req.path)}
                    />
                    <span className="font-mono text-[10px] text-zinc-400 w-5 shrink-0">
                      {idx + 1}.
                    </span>
                    <span
                      className={cn(
                        "text-[10px] font-mono font-bold tracking-tight uppercase shrink-0 w-12",
                        getMethodTextColor(req.method),
                      )}
                    >
                      {req.method}
                    </span>
                    <div className="flex-1 min-w-0">
                      <div className="font-medium truncate">{req.name}</div>
                      <div className="text-[10px] text-zinc-400 truncate font-mono">
                        {req.relPath}
                      </div>
                    </div>
                  </div>
                );
              })
            )}
          </div>
        </div>

        {/* Right Column: Live Scorecard, Progress & Results */}
        <div className="flex-1 flex flex-col overflow-hidden bg-white dark:bg-zinc-950">
          {/* Progress Bar & Status Header */}
          <div className="p-4 border-b border-zinc-200 dark:border-zinc-800 shrink-0 bg-zinc-50/40 dark:bg-zinc-900/20">
            {/* Live Progress Bar */}
            <div className="space-y-1.5 mb-3">
              <div className="flex items-center justify-between text-xs">
                <span className="font-semibold text-zinc-700 dark:text-zinc-300 flex items-center gap-2">
                  {isRunning ? (
                    <span className="flex items-center gap-1.5 text-blue-600 dark:text-blue-400">
                      <span className="w-2 h-2 rounded-full bg-blue-500 animate-ping" />
                      Running Iterations...
                    </span>
                  ) : summary ? (
                    summary.success ? (
                      <span className="flex items-center gap-1.5 text-emerald-600 dark:text-emerald-400">
                        <CheckCircle2 className="w-3.5 h-3.5" /> Run Completed
                        Successfully
                      </span>
                    ) : (
                      <span className="flex items-center gap-1.5 text-rose-600 dark:text-rose-400">
                        <XCircle className="w-3.5 h-3.5" /> Run Finished with
                        Failures
                      </span>
                    )
                  ) : (
                    <span>Ready to Run</span>
                  )}
                </span>
                <span className="text-zinc-500 font-mono text-[11px]">
                  {currentProgress.current} / {currentProgress.total} requests (
                  {progressPercent}%)
                </span>
              </div>
              <div className="h-2 w-full bg-zinc-200 dark:bg-zinc-800 rounded-full overflow-hidden">
                <div
                  className={cn(
                    "h-full transition-all duration-300 ease-out",
                    isRunning
                      ? "bg-blue-500"
                      : summary?.success
                        ? "bg-emerald-500"
                        : "bg-rose-500",
                  )}
                  style={{ width: `${progressPercent}%` }}
                />
              </div>
            </div>

            {/* Scorecard Hero Cards */}
            <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-6 gap-2.5">
              {/* Card 1: Pass Rate */}
              <div className="p-2.5 rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900">
                <div className="text-[10px] uppercase font-bold tracking-wider text-zinc-400">
                  Pass Rate
                </div>
                <div
                  className={cn(
                    "text-xl font-bold mt-0.5",
                    liveStats.rate === 100
                      ? "text-emerald-500"
                      : "text-rose-500",
                  )}
                >
                  {liveStats.totalExec > 0
                    ? `${liveStats.rate.toFixed(1)}%`
                    : "--"}
                </div>
              </div>

              {/* Card 2: Passed */}
              <div className="p-2.5 rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900">
                <div className="text-[10px] uppercase font-bold tracking-wider text-zinc-400">
                  Passed
                </div>
                <div className="text-xl font-bold text-emerald-500 mt-0.5">
                  {summary ? summary.passedRequests : liveStats.passed}
                </div>
              </div>

              {/* Card 3: Failed */}
              <div className="p-2.5 rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900">
                <div className="text-[10px] uppercase font-bold tracking-wider text-zinc-400">
                  Failed
                </div>
                <div className="text-xl font-bold text-rose-500 mt-0.5">
                  {summary ? summary.failedRequests : liveStats.failed}
                </div>
              </div>

              {/* Card 4: Iterations */}
              <div className="p-2.5 rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900">
                <div className="text-[10px] uppercase font-bold tracking-wider text-zinc-400">
                  Iterations
                </div>
                <div className="text-xl font-bold text-purple-500 mt-0.5">
                  {iterations}
                </div>
              </div>

              {/* Card 5: Avg Duration */}
              <div className="p-2.5 rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900">
                <div className="text-[10px] uppercase font-bold tracking-wider text-zinc-400">
                  Avg Latency
                </div>
                <div className="text-xl font-bold text-zinc-700 dark:text-zinc-200 mt-0.5 font-mono">
                  {summary
                    ? `${summary.avgDurationMs.toFixed(1)}ms`
                    : liveStats.totalExec > 0
                      ? `${liveStats.avg.toFixed(1)}ms`
                      : "--"}
                </div>
              </div>

              {/* Card 6: P95 Duration */}
              <div className="p-2.5 rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900">
                <div className="text-[10px] uppercase font-bold tracking-wider text-zinc-400">
                  P95 Latency
                </div>
                <div className="text-xl font-bold text-amber-500 mt-0.5 font-mono">
                  {summary
                    ? `${summary.p95DurationMs.toFixed(1)}ms`
                    : liveStats.totalExec > 0
                      ? `${liveStats.p95.toFixed(1)}ms`
                      : "--"}
                </div>
              </div>
            </div>
          </div>

          {/* Iteration Filter Tabs */}
          {iterations > 1 && liveResults.length > 0 && (
            <div className="px-4 py-2 border-b border-zinc-200 dark:border-zinc-800 flex items-center gap-1.5 overflow-x-auto no-scrollbar shrink-0 bg-zinc-50/30 dark:bg-zinc-900/30">
              <span className="text-[11px] font-semibold text-zinc-500 mr-1">
                View:
              </span>
              <button
                onClick={() => setSelectedIterationFilter("all")}
                className={cn(
                  "px-2.5 py-1 rounded text-xs font-medium cursor-pointer transition-colors",
                  selectedIterationFilter === "all"
                    ? "bg-zinc-900 dark:bg-zinc-100 text-white dark:text-zinc-900"
                    : "text-zinc-600 dark:text-zinc-400 hover:bg-zinc-200 dark:hover:bg-zinc-800",
                )}
              >
                All Runs ({liveResults.length})
              </button>
              {Array.from({ length: iterations }, (_, i) => i + 1).map(
                (iterNum) => {
                  const count = liveResults.filter(
                    (r) => (r.iteration || 1) === iterNum,
                  ).length;
                  const hasFail = liveResults.some(
                    (r) => (r.iteration || 1) === iterNum && !r.passed,
                  );
                  return (
                    <button
                      key={iterNum}
                      onClick={() => setSelectedIterationFilter(iterNum)}
                      className={cn(
                        "px-2.5 py-1 rounded text-xs font-medium cursor-pointer transition-colors flex items-center gap-1.5",
                        selectedIterationFilter === iterNum
                          ? "bg-purple-600 text-white"
                          : "text-zinc-600 dark:text-zinc-400 hover:bg-zinc-200 dark:hover:bg-zinc-800",
                      )}
                    >
                      <span>Iteration {iterNum}</span>
                      <span
                        className={cn(
                          "w-1.5 h-1.5 rounded-full",
                          hasFail ? "bg-rose-400" : "bg-emerald-400",
                        )}
                      />
                      <span className="text-[10px] opacity-75 font-mono">
                        ({count})
                      </span>
                    </button>
                  );
                },
              )}
            </div>
          )}

          {/* Results List */}
          <div className="flex-1 overflow-y-auto p-4 space-y-2">
            {liveResults.length === 0 ? (
              <div className="h-full flex flex-col items-center justify-center text-center p-8 text-zinc-400">
                <Play className="w-10 h-10 text-zinc-300 dark:text-zinc-700 stroke-[1.5] mb-2" />
                <span className="text-sm font-semibold text-zinc-700 dark:text-zinc-300">
                  Ready to Run
                </span>
                <span className="text-xs text-zinc-500 max-w-sm mt-1">
                  Click "Run {selectedCount} Requests" to start execution.
                  Results, status codes, and test assertions will stream here in
                  real time.
                </span>
              </div>
            ) : (
              displayedResults.map((result, idx) => {
                const isExpanded = Boolean(
                  expandedResults[
                    `${result.iteration}_${result.filePath}_${idx}`
                  ],
                );
                const toggleExpanded = () => {
                  setExpandedResults((prev) => ({
                    ...prev,
                    [`${result.iteration}_${result.filePath}_${idx}`]:
                      !isExpanded,
                  }));
                };

                return (
                  <div
                    key={`${result.iteration}_${result.filePath}_${idx}`}
                    className={cn(
                      "rounded-lg border text-xs overflow-hidden transition-all duration-150",
                      result.passed
                        ? "border-zinc-200 dark:border-zinc-800/80 bg-white dark:bg-zinc-900/40"
                        : "border-rose-200 dark:border-rose-950/60 bg-rose-50/20 dark:bg-rose-950/10",
                    )}
                  >
                    {/* Header Row */}
                    <div
                      onClick={toggleExpanded}
                      className="p-2.5 flex items-center justify-between gap-3 cursor-pointer select-none hover:bg-zinc-50/50 dark:hover:bg-zinc-800/30"
                    >
                      <div className="flex items-center gap-2.5 min-w-0">
                        {/* Status Icon */}
                        {result.passed ? (
                          <CheckCircle2 className="w-4 h-4 text-emerald-500 shrink-0" />
                        ) : (
                          <XCircle className="w-4 h-4 text-rose-500 shrink-0" />
                        )}

                        {/* Iteration Badge (if multiple iterations) */}
                        {iterations > 1 && (
                          <Badge
                            variant="outline"
                            className="text-[10px] font-mono border-purple-500/30 text-purple-600 dark:text-purple-400"
                          >
                            #{result.iteration || 1}
                          </Badge>
                        )}

                        {/* Method */}
                        <span
                          className={cn(
                            "text-[10px] font-mono font-bold tracking-tight uppercase shrink-0",
                            getMethodTextColor(result.request?.method || "GET"),
                          )}
                        >
                          {result.request?.method || "GET"}
                        </span>

                        {/* Request Title / RelPath */}
                        <span className="font-medium text-zinc-900 dark:text-zinc-100 truncate">
                          {result.request?.name || result.relPath}
                        </span>

                        <span className="text-[10px] text-zinc-400 font-mono truncate hidden md:inline">
                          {result.relPath}
                        </span>
                      </div>

                      {/* Right Meta: Status Code & Duration */}
                      <div className="flex items-center gap-3 shrink-0">
                        {result.result?.statusCode ? (
                          <Badge
                            variant="outline"
                            className={cn(
                              "text-[10px] font-mono font-semibold",
                              result.result.statusCode >= 200 &&
                                result.result.statusCode < 300
                                ? "text-emerald-600 border-emerald-500/30"
                                : "text-rose-600 border-rose-500/30",
                            )}
                          >
                            {result.result.statusCode}{" "}
                            {result.result.statusText}
                          </Badge>
                        ) : null}

                        <span className="text-[10px] text-zinc-500 font-mono flex items-center gap-1">
                          <Clock className="w-3 h-3" />
                          {Math.round(result.duration / 1000000)}ms
                        </span>

                        {isExpanded ? (
                          <ChevronDown className="w-3.5 h-3.5 text-zinc-400" />
                        ) : (
                          <ChevronRight className="w-3.5 h-3.5 text-zinc-400" />
                        )}
                      </div>
                    </div>

                    {/* Expandable Assertions & Error Details */}
                    {isExpanded && (
                      <div className="px-3 pb-3 pt-1 border-t border-zinc-100 dark:border-zinc-800/60 bg-zinc-50/50 dark:bg-zinc-950/40 space-y-2">
                        {/* Error Banner */}
                        {result.error && (
                          <div className="p-2 rounded bg-rose-500/10 border border-rose-500/20 text-rose-600 dark:text-rose-400 text-xs font-mono">
                            {result.error}
                          </div>
                        )}

                        {/* Assertions */}
                        {result.result?.tests &&
                        result.result.tests.length > 0 ? (
                          <div className="space-y-1">
                            <span className="text-[10px] uppercase font-bold tracking-wider text-zinc-400">
                              Assertions (
                              {
                                result.result.tests.filter((t) => t.passed)
                                  .length
                              }
                              /{result.result.tests.length})
                            </span>
                            {result.result.tests.map((t, tIdx) => (
                              <div
                                key={tIdx}
                                className={cn(
                                  "flex items-start gap-2 p-1.5 rounded text-xs",
                                  t.passed
                                    ? "text-emerald-700 dark:text-emerald-400 bg-emerald-500/5"
                                    : "text-rose-700 dark:text-rose-400 bg-rose-500/5",
                                )}
                              >
                                {t.passed ? (
                                  <Check className="w-3.5 h-3.5 shrink-0 text-emerald-500 mt-0.5" />
                                ) : (
                                  <XCircle className="w-3.5 h-3.5 shrink-0 text-rose-500 mt-0.5" />
                                )}
                                <div>
                                  <div className="font-medium">{t.name}</div>
                                  {t.message && (
                                    <div className="text-[11px] opacity-80 font-mono mt-0.5">
                                      {t.message}
                                    </div>
                                  )}
                                </div>
                              </div>
                            ))}
                          </div>
                        ) : (
                          !result.error && (
                            <span className="text-zinc-400 text-xs italic">
                              No test assertions defined on this request.
                            </span>
                          )
                        )}
                      </div>
                    )}
                  </div>
                );
              })
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
