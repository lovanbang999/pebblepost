import { ArrowLeft, FileCode, Check, Minus, Plus } from "lucide-react";
import { useGitStore } from "../../store/gitStore";
import { Button } from "../ui/button";

interface GitDiffViewProps {
  onBack: () => void;
}

export function GitDiffView({ onBack }: GitDiffViewProps) {
  const { selectedFile, diffEntries, isDiffLoading, error } = useGitStore();

  if (isDiffLoading) {
    return (
      <div className="flex-1 flex flex-col items-center justify-center p-8 text-zinc-400 text-sm">
        <div className="w-6 h-6 border-2 border-primary border-t-transparent rounded-full animate-spin mb-3" />
        Loading diff for {selectedFile}...
      </div>
    );
  }

  return (
    <div className="flex-1 flex flex-col min-h-0 bg-white dark:bg-zinc-950">
      {/* Header bar */}
      <div className="flex items-center gap-2 p-3 border-b border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-900/60">
        <Button
          variant="ghost"
          size="sm"
          onClick={onBack}
          className="h-7 px-2 text-xs flex items-center gap-1 text-zinc-600 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-100"
        >
          <ArrowLeft className="w-3.5 h-3.5" />
          <span>Files</span>
        </Button>
        <span className="text-xs font-mono font-medium text-zinc-700 dark:text-zinc-300 truncate flex-1 min-w-0" title={selectedFile || ''}>
          {selectedFile}
        </span>
      </div>

      {/* Error alert */}
      {error && (
        <div className="p-3 text-xs text-rose-600 bg-rose-50 dark:bg-rose-950/20 border-b border-rose-200 dark:border-rose-800">
          {error}
        </div>
      )}

      {/* Diff content list */}
      <div className="flex-1 overflow-y-auto p-4 space-y-4">
        {diffEntries.length === 0 ? (
          <div className="text-center py-12 text-zinc-400 text-xs">
            <Check className="w-8 h-8 mx-auto mb-2 text-emerald-500 opacity-60" />
            No differences found between HEAD and working copy.
          </div>
        ) : (
          diffEntries.map((entry, idx) => (
            <div
              key={`${entry.field}-${idx}`}
              className="border border-zinc-200 dark:border-zinc-800 rounded-lg overflow-hidden bg-zinc-50/50 dark:bg-zinc-900/40 shadow-sm"
            >
              {/* Field title header */}
              <div className="flex items-center justify-between px-3 py-1.5 bg-zinc-100 dark:bg-zinc-800/80 border-b border-zinc-200 dark:border-zinc-800 text-xs">
                <span className="font-semibold uppercase tracking-wider text-[11px] text-zinc-700 dark:text-zinc-300 flex items-center gap-1.5">
                  <FileCode className="w-3.5 h-3.5 text-zinc-500" />
                  {entry.field}
                </span>
                <span className="text-[10px] uppercase font-mono px-1.5 py-0.5 rounded bg-zinc-200 dark:bg-zinc-700 text-zinc-600 dark:text-zinc-400">
                  {entry.type}
                </span>
              </div>

              {/* Side-by-side or comparative view */}
              <div className="divide-y divide-zinc-200 dark:divide-zinc-800 text-xs font-mono">
                {/* Before (HEAD) */}
                <div className="p-2.5 bg-rose-50/60 dark:bg-rose-950/20 text-rose-900 dark:text-rose-200">
                  <div className="flex items-center gap-1 text-[10px] font-sans font-semibold text-rose-600 dark:text-rose-400 mb-1">
                    <Minus className="w-3 h-3" />
                    <span>HEAD</span>
                  </div>
                  {entry.before ? (
                    <pre className="whitespace-pre-wrap break-all text-[11px] leading-relaxed">
                      {entry.before}
                    </pre>
                  ) : (
                    <span className="italic text-zinc-400 font-sans text-[11px]">
                      (empty / newly created)
                    </span>
                  )}
                </div>

                {/* After (Working copy) */}
                <div className="p-2.5 bg-emerald-50/60 dark:bg-emerald-950/20 text-emerald-900 dark:text-emerald-200">
                  <div className="flex items-center gap-1 text-[10px] font-sans font-semibold text-emerald-600 dark:text-emerald-400 mb-1">
                    <Plus className="w-3 h-3" />
                    <span>Working Copy</span>
                  </div>
                  {entry.after ? (
                    <pre className="whitespace-pre-wrap break-all text-[11px] leading-relaxed">
                      {entry.after}
                    </pre>
                  ) : (
                    <span className="italic text-zinc-400 font-sans text-[11px]">
                      (deleted)
                    </span>
                  )}
                </div>
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  );
}
