import { useEffect } from "react";
import { GitBranch, ArrowUp, ArrowDown, Sparkles } from "lucide-react";
import { useGitStore } from "../../store/gitStore";
import { Tooltip } from "../ui/tooltip";

interface GitBadgeProps {
  workspacePath: string | null;
}

export function GitBadge({ workspacePath }: GitBadgeProps) {
  const { status, refresh, openPanel, initRepo, isLoading } = useGitStore();

  useEffect(() => {
    if (workspacePath) {
      void refresh(workspacePath);
    }
  }, [workspacePath, refresh]);

  if (!workspacePath) {
    return null;
  }

  if (status && !status.isRepo) {
    return (
      <div className="w-full flex items-center justify-between">
        <button
          type="button"
          onClick={() => void initRepo(workspacePath)}
          disabled={isLoading}
          className="flex items-center gap-2 text-xs text-zinc-600 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-100 transition-colors cursor-pointer group"
          title="Initialize Git repository in this workspace"
        >
          <GitBranch className="w-4 h-4 text-zinc-400 group-hover:text-amber-500 transition-colors shrink-0" />
          <span className="font-medium text-xs">Initialize Git</span>
        </button>

        <button
          type="button"
          onClick={() => void initRepo(workspacePath)}
          disabled={isLoading}
          className="flex items-center gap-1 px-2 py-0.5 rounded text-[11px] font-mono font-medium bg-amber-500/10 text-amber-600 dark:text-amber-400 hover:bg-amber-500/20 border border-amber-500/20 transition-all cursor-pointer"
          title="Click to run git init"
        >
          <Sparkles className="w-3 h-3" />
          <span>git init</span>
        </button>
      </div>
    );
  }

  const branchName = status?.branch || "git";
  const ahead = status?.ahead ?? 0;
  const behind = status?.behind ?? 0;
  const isDirty = status?.dirty ?? false;

  return (
    <div className="w-full flex items-center justify-between min-w-0">
      <Tooltip
        content={`Git Repository: ${branchName} (${isDirty ? "uncommitted changes" : "clean tree"})`}
      >
        <button
          type="button"
          onClick={() => openPanel("files")}
          className="flex items-center gap-2 text-xs text-zinc-700 dark:text-zinc-300 hover:text-zinc-950 dark:hover:text-zinc-50 transition-colors cursor-pointer group min-w-0 flex-1 truncate"
        >
          <GitBranch className="w-4 h-4 text-zinc-400 group-hover:text-primary transition-colors shrink-0" />
          <span className="font-mono font-semibold text-zinc-800 dark:text-zinc-200 truncate max-w-30">
            {branchName}
          </span>

          {isDirty && (
            <span
              className="w-2 h-2 rounded-full bg-amber-500 ring-2 ring-amber-500/20 shrink-0"
              title="Uncommitted changes"
            />
          )}

          {(ahead > 0 || behind > 0) && (
            <span className="flex items-center gap-1.5 text-[11px] font-sans text-zinc-500 shrink-0 ml-1">
              {ahead > 0 && (
                <span className="flex items-center text-emerald-600 dark:text-emerald-400 font-medium">
                  <ArrowUp className="w-3 h-3" />
                  {ahead}
                </span>
              )}
              {behind > 0 && (
                <span className="flex items-center text-amber-600 dark:text-amber-400 font-medium">
                  <ArrowDown className="w-3 h-3" />
                  {behind}
                </span>
              )}
            </span>
          )}
        </button>
      </Tooltip>

      <button
        type="button"
        onClick={() => openPanel("files")}
        className="text-[11px] font-medium text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-200 px-1.5 py-0.5 rounded hover:bg-zinc-200/50 dark:hover:bg-zinc-800 transition-colors cursor-pointer shrink-0"
        title="Open Git Source Control panel"
      >
        Source Control
      </button>
    </div>
  );
}
