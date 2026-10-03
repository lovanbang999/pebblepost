import { useState, useMemo } from "react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "../ui/dialog";
import { Button } from "../ui/button";
import { Badge } from "../ui/badge";
import { useHistoryStore } from "../../store/historyStore";
import { computeLineDiff, formatJsonForDiff } from "../../lib/diff";
import { getMethodTextColor, cn } from "../../lib/utils";
import { ArrowLeftRight, Columns, AlignJustify } from "lucide-react";

interface CompareHistoryDialogProps {
  open: boolean;
  onClose: () => void;
}

function getStatusBadgeClass(code: number): string {
  if (code >= 200 && code < 300) {
    return "bg-emerald-50 dark:bg-emerald-950/40 text-emerald-600 dark:text-emerald-400 border-emerald-300 dark:border-emerald-800";
  }
  if (code >= 300 && code < 400) {
    return "bg-sky-50 dark:bg-sky-950/40 text-sky-600 dark:text-sky-400 border-sky-300 dark:border-sky-800";
  }
  if (code >= 400 && code < 500) {
    return "bg-amber-50 dark:bg-amber-950/40 text-amber-600 dark:text-amber-400 border-amber-300 dark:border-amber-800";
  }
  return "bg-rose-50 dark:bg-rose-950/40 text-rose-600 dark:text-rose-400 border-rose-300 dark:border-rose-800";
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  return `${(bytes / 1024).toFixed(1)} KB`;
}

export function CompareHistoryDialog({
  open,
  onClose,
}: CompareHistoryDialogProps) {
  const { compareEntries, clearCompare } = useHistoryStore();
  const [viewMode, setViewMode] = useState<"unified" | "split">("unified");
  const [activeTab, setActiveTab] = useState<"body" | "headers">("body");

  const entryA = compareEntries?.[0];
  const entryB = compareEntries?.[1];

  const formattedBodyA = useMemo(() => {
    return entryA?.responseBody ? formatJsonForDiff(entryA.responseBody) : "";
  }, [entryA]);

  const formattedBodyB = useMemo(() => {
    return entryB?.responseBody ? formatJsonForDiff(entryB.responseBody) : "";
  }, [entryB]);

  const bodyDiff = useMemo(() => {
    return computeLineDiff(formattedBodyA, formattedBodyB);
  }, [formattedBodyA, formattedBodyB]);

  // Headers diff formatted as JSON
  const headersA = useMemo(() => {
    return entryA?.responseHeaders
      ? JSON.stringify(entryA.responseHeaders, null, 2)
      : "";
  }, [entryA]);

  const headersB = useMemo(() => {
    return entryB?.responseHeaders
      ? JSON.stringify(entryB.responseHeaders, null, 2)
      : "";
  }, [entryB]);

  const headersDiff = useMemo(() => {
    return computeLineDiff(headersA, headersB);
  }, [headersA, headersB]);

  const handleClose = () => {
    clearCompare();
    onClose();
  };

  if (!entryA || !entryB) return null;

  const durationDelta = entryB.durationMs - entryA.durationMs;
  const sizeDelta = entryB.sizeBytes - entryA.sizeBytes;

  return (
    <Dialog open={open} onOpenChange={(o) => !o && handleClose()}>
      <DialogContent className="max-w-5xl h-[85vh] flex flex-col p-0 gap-0 overflow-hidden bg-white dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 shadow-2xl">
        {/* Header */}
        <DialogHeader className="p-4 border-b border-zinc-200 dark:border-zinc-800 shrink-0">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <div className="p-1.5 rounded-md bg-zinc-100 dark:bg-zinc-800 text-zinc-700 dark:text-zinc-300">
                <ArrowLeftRight className="w-4 h-4" />
              </div>
              <div>
                <DialogTitle className="text-sm font-semibold text-zinc-900 dark:text-zinc-100">
                  Compare History Responses
                </DialogTitle>
                <DialogDescription className="text-xs text-zinc-500 dark:text-zinc-400">
                  Side-by-side inspection and JSON diff of recorded executions
                </DialogDescription>
              </div>
            </div>

            {/* View Mode Toggle */}
            <div className="flex items-center gap-1 bg-zinc-100 dark:bg-zinc-800/80 p-0.5 rounded-lg border border-zinc-200 dark:border-zinc-700/60">
              <button
                type="button"
                onClick={() => setViewMode("unified")}
                className={cn(
                  "flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium rounded-md transition-colors",
                  viewMode === "unified"
                    ? "bg-white dark:bg-zinc-900 text-zinc-900 dark:text-zinc-100 shadow-2xs"
                    : "text-zinc-500 hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-zinc-200",
                )}
              >
                <AlignJustify className="w-3.5 h-3.5" />
                Unified
              </button>
              <button
                type="button"
                onClick={() => setViewMode("split")}
                className={cn(
                  "flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium rounded-md transition-colors",
                  viewMode === "split"
                    ? "bg-white dark:bg-zinc-900 text-zinc-900 dark:text-zinc-100 shadow-2xs"
                    : "text-zinc-500 hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-zinc-200",
                )}
              >
                <Columns className="w-3.5 h-3.5" />
                Split
              </button>
            </div>
          </div>

          {/* Metric Comparison Badges Bar */}
          <div className="mt-3 grid grid-cols-2 gap-3 pt-3 border-t border-zinc-100 dark:border-zinc-800/60">
            {/* Entry A Card */}
            <div className="flex items-center justify-between p-2 rounded-md bg-zinc-50 dark:bg-zinc-900/60 border border-zinc-200/80 dark:border-zinc-800 text-xs">
              <div className="flex items-center gap-2 min-w-0">
                <span
                  className={cn(
                    "font-mono font-bold text-[11px]",
                    getMethodTextColor(entryA.method),
                  )}
                >
                  {entryA.method}
                </span>
                <span
                  className="font-medium truncate max-w-50"
                  title={entryA.url}
                >
                  {entryA.requestName || entryA.url}
                </span>
                <Badge
                  variant="outline"
                  className={cn(
                    "text-[10px] px-1.5 py-0",
                    getStatusBadgeClass(entryA.statusCode),
                  )}
                >
                  {entryA.statusCode}
                </Badge>
              </div>
              <div className="text-[11px] text-zinc-500 flex items-center gap-2 shrink-0">
                <span>{entryA.durationMs.toFixed(0)}ms</span>
                <span>•</span>
                <span>{formatBytes(entryA.sizeBytes)}</span>
              </div>
            </div>

            {/* Entry B Card */}
            <div className="flex items-center justify-between p-2 rounded-md bg-zinc-50 dark:bg-zinc-900/60 border border-zinc-200/80 dark:border-zinc-800 text-xs">
              <div className="flex items-center gap-2 min-w-0">
                <span
                  className={cn(
                    "font-mono font-bold text-[11px]",
                    getMethodTextColor(entryB.method),
                  )}
                >
                  {entryB.method}
                </span>
                <span
                  className="font-medium truncate max-w-50"
                  title={entryB.url}
                >
                  {entryB.requestName || entryB.url}
                </span>
                <Badge
                  variant="outline"
                  className={cn(
                    "text-[10px] px-1.5 py-0",
                    getStatusBadgeClass(entryB.statusCode),
                  )}
                >
                  {entryB.statusCode}
                </Badge>
              </div>
              <div className="text-[11px] text-zinc-500 flex items-center gap-2 shrink-0">
                <span
                  className={
                    durationDelta > 0 ? "text-amber-500" : "text-emerald-500"
                  }
                >
                  {entryB.durationMs.toFixed(0)}ms (
                  {durationDelta > 0
                    ? `+${durationDelta.toFixed(0)}ms`
                    : `${durationDelta.toFixed(0)}ms`}
                  )
                </span>
                <span>•</span>
                <span
                  className={
                    sizeDelta > 0 ? "text-amber-500" : "text-emerald-500"
                  }
                >
                  {formatBytes(entryB.sizeBytes)} (
                  {sizeDelta > 0
                    ? `+${formatBytes(sizeDelta)}`
                    : formatBytes(sizeDelta)}
                  )
                </span>
              </div>
            </div>
          </div>
        </DialogHeader>

        {/* Tab switcher: Response Body vs Response Headers */}
        <div className="px-4 py-2 border-b border-zinc-200 dark:border-zinc-800 flex items-center justify-between bg-zinc-50/50 dark:bg-zinc-900/20 shrink-0">
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => setActiveTab("body")}
              className={cn(
                "px-3 py-1 text-xs font-semibold rounded-md transition-colors",
                activeTab === "body"
                  ? "bg-zinc-200/80 dark:bg-zinc-800 text-zinc-900 dark:text-zinc-100"
                  : "text-zinc-500 hover:text-zinc-800 dark:hover:text-zinc-200",
              )}
            >
              Response Body Diff
            </button>
            <button
              type="button"
              onClick={() => setActiveTab("headers")}
              className={cn(
                "px-3 py-1 text-xs font-semibold rounded-md transition-colors",
                activeTab === "headers"
                  ? "bg-zinc-200/80 dark:bg-zinc-800 text-zinc-900 dark:text-zinc-100"
                  : "text-zinc-500 hover:text-zinc-800 dark:hover:text-zinc-200",
              )}
            >
              Headers Diff
            </button>
          </div>
          <div className="flex items-center gap-3 text-[11px] text-zinc-500">
            <span className="flex items-center gap-1">
              <span className="w-2.5 h-2.5 rounded-full bg-emerald-500 inline-block" />{" "}
              Added
            </span>
            <span className="flex items-center gap-1">
              <span className="w-2.5 h-2.5 rounded-full bg-rose-500 inline-block" />{" "}
              Removed
            </span>
          </div>
        </div>

        {/* Diff Content Area */}
        <div className="flex-1 overflow-auto p-3 font-mono text-xs leading-relaxed bg-zinc-950 text-zinc-200 select-text">
          {viewMode === "unified" ? (
            <div className="min-w-full">
              {(activeTab === "body" ? bodyDiff : headersDiff).map(
                (line, idx) => (
                  <div
                    key={idx}
                    className={cn(
                      "flex items-start px-2 py-0.5 rounded-xs transition-colors",
                      line.type === "added" &&
                        "bg-emerald-950/50 text-emerald-300 border-l-2 border-emerald-500",
                      line.type === "removed" &&
                        "bg-rose-950/50 text-rose-300 border-l-2 border-rose-500",
                      line.type === "unchanged" && "text-zinc-400",
                    )}
                  >
                    <span className="w-10 select-none text-right pr-2 text-zinc-600 shrink-0">
                      {line.oldLineNumber || ""}
                    </span>
                    <span className="w-10 select-none text-right pr-3 text-zinc-600 shrink-0">
                      {line.newLineNumber || ""}
                    </span>
                    <span className="w-4 select-none text-center shrink-0 font-bold">
                      {line.type === "added"
                        ? "+"
                        : line.type === "removed"
                          ? "-"
                          : " "}
                    </span>
                    <pre className="flex-1 font-mono whitespace-pre-wrap break-all m-0 p-0">
                      {line.text}
                    </pre>
                  </div>
                ),
              )}
            </div>
          ) : (
            /* Split View */
            <div className="grid grid-cols-2 gap-2 h-full">
              {/* Left Column (Entry A) */}
              <div className="border border-zinc-800 rounded p-2 overflow-auto bg-zinc-900/60">
                <div className="text-[10px] font-semibold text-zinc-400 pb-1 mb-2 border-b border-zinc-800">
                  Entry #{entryA.id} ({entryA.statusCode})
                </div>
                <pre className="whitespace-pre-wrap break-all text-zinc-300 text-[11px]">
                  {activeTab === "body"
                    ? formattedBodyA || "(Empty Body)"
                    : headersA || "(No Headers)"}
                </pre>
              </div>

              {/* Right Column (Entry B) */}
              <div className="border border-zinc-800 rounded p-2 overflow-auto bg-zinc-900/60">
                <div className="text-[10px] font-semibold text-zinc-400 pb-1 mb-2 border-b border-zinc-800">
                  Entry #{entryB.id} ({entryB.statusCode})
                </div>
                <pre className="whitespace-pre-wrap break-all text-zinc-300 text-[11px]">
                  {activeTab === "body"
                    ? formattedBodyB || "(Empty Body)"
                    : headersB || "(No Headers)"}
                </pre>
              </div>
            </div>
          )}
        </div>

        {/* Footer */}
        <DialogFooter className="p-3 border-t border-zinc-200 dark:border-zinc-800 flex items-center justify-between bg-zinc-50 dark:bg-zinc-900/40 shrink-0">
          <div className="text-xs text-zinc-500">
            Comparing{" "}
            <span className="font-mono font-medium">#{entryA.id}</span> with{" "}
            <span className="font-mono font-medium">#{entryB.id}</span>
          </div>
          <Button variant="outline" size="sm" onClick={handleClose}>
            Close
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
