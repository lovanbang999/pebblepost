import { Component, useEffect, type ErrorInfo, type ReactNode } from "react";
import { X, GitBranch, Files, History } from "lucide-react";
import { useGitStore } from "../../store/gitStore";
import { GitFilesTab } from "./GitFilesTab";
import { BranchesTab } from "./BranchesTab";
import { GitLogTab } from "./GitLogTab";
import { GitDiffView } from "./GitDiffView";
import { GitPushPullBar } from "./GitPushPullBar";

interface ErrorBoundaryProps {
  children: ReactNode;
  fallbackTitle?: string;
  onReset?: () => void;
}

interface ErrorBoundaryState {
  hasError: boolean;
  error: Error | null;
}

class GitErrorBoundary extends Component<
  ErrorBoundaryProps,
  ErrorBoundaryState
> {
  constructor(props: ErrorBoundaryProps) {
    super(props);
    this.state = { hasError: false, error: null };
  }

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("GitPanel ErrorBoundary caught error:", error, info);
  }

  render() {
    if (this.state.hasError) {
      return (
        <div className="flex-1 flex flex-col items-center justify-center p-6 text-center">
          <div className="w-10 h-10 rounded-full bg-rose-100 dark:bg-rose-950/40 text-rose-600 dark:text-rose-400 flex items-center justify-center mb-3">
            <X className="w-5 h-5" />
          </div>
          <div className="font-semibold text-xs text-zinc-900 dark:text-zinc-100 mb-1">
            {this.props.fallbackTitle || "Failed to load Git section"}
          </div>
          <p className="text-[11px] text-zinc-500 mb-4 max-w-xs wrap-break-word">
            {this.state.error?.message || "An unexpected error occurred."}
          </p>
          <button
            type="button"
            onClick={() => {
              this.setState({ hasError: false, error: null });
              this.props.onReset?.();
            }}
            className="px-3 py-1 text-xs rounded bg-zinc-200 dark:bg-zinc-800 text-zinc-700 dark:text-zinc-300 hover:bg-zinc-300 dark:hover:bg-zinc-700 transition-colors cursor-pointer"
          >
            Retry
          </button>
        </div>
      );
    }
    return this.props.children;
  }
}

interface GitPanelProps {
  workspacePath: string | null;
}

export function GitPanel({ workspacePath }: GitPanelProps) {
  const {
    isPanelOpen,
    closePanel,
    activeTab,
    setActiveTab,
    selectedFile,
    closeDiff,
  } = useGitStore();

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        if (selectedFile) {
          closeDiff();
        } else {
          closePanel();
        }
      }
    };
    if (isPanelOpen) {
      window.addEventListener("keydown", handleKeyDown);
      return () => window.removeEventListener("keydown", handleKeyDown);
    }
  }, [isPanelOpen, selectedFile, closeDiff, closePanel]);

  if (!isPanelOpen || !workspacePath) {
    return null;
  }

  return (
    <>
      {/* Backdrop overlay */}
      <div
        className="fixed inset-0 bg-black/20 dark:bg-black/50 backdrop-blur-[1px] z-40 transition-opacity animate-in fade-in duration-150"
        onClick={closePanel}
      />

      <div className="fixed inset-y-0 right-0 z-50 w-105 max-w-[90vw] bg-white dark:bg-zinc-950 border-l border-zinc-200 dark:border-zinc-800 shadow-2xl flex flex-col transition-transform animate-in slide-in-from-right duration-200">
        {/* Top Header */}
        <div className="flex items-center justify-between px-4 py-3 border-b border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-900/60">
          <div className="flex items-center gap-2">
            <GitBranch className="w-4 h-4 text-primary" />
            <span className="font-semibold text-sm text-zinc-900 dark:text-zinc-100">
              Git Source Control
            </span>
          </div>

          <button
            type="button"
            onClick={closePanel}
            className="p-1 rounded-md text-zinc-400 hover:text-zinc-600 dark:hover:text-zinc-200 hover:bg-zinc-200/50 dark:hover:bg-zinc-800 transition-colors cursor-pointer"
            title="Close (Esc)"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* Navigation Tab Bar (hidden when diff view is active) */}
        {!selectedFile && (
          <div className="flex border-b border-zinc-200 dark:border-zinc-800 bg-zinc-100/50 dark:bg-zinc-900/40 px-2 pt-1 gap-1 text-xs">
            <button
              type="button"
              onClick={() => setActiveTab("files")}
              className={`flex items-center gap-1.5 px-3 py-1.5 font-medium rounded-t-md transition-colors border-b-2 -mb-px cursor-pointer ${
                activeTab === "files"
                  ? "border-primary text-primary bg-white dark:bg-zinc-950"
                  : "border-transparent text-zinc-500 hover:text-zinc-800 dark:hover:text-zinc-300"
              }`}
            >
              <Files className="w-3.5 h-3.5" />
              <span>Files</span>
            </button>

            <button
              type="button"
              onClick={() => setActiveTab("branches")}
              className={`flex items-center gap-1.5 px-3 py-1.5 font-medium rounded-t-md transition-colors border-b-2 -mb-px cursor-pointer ${
                activeTab === "branches"
                  ? "border-primary text-primary bg-white dark:bg-zinc-950"
                  : "border-transparent text-zinc-500 hover:text-zinc-800 dark:hover:text-zinc-300"
              }`}
            >
              <GitBranch className="w-3.5 h-3.5" />
              <span>Branches</span>
            </button>

            <button
              type="button"
              onClick={() => setActiveTab("log")}
              className={`flex items-center gap-1.5 px-3 py-1.5 font-medium rounded-t-md transition-colors border-b-2 -mb-px cursor-pointer ${
                activeTab === "log"
                  ? "border-primary text-primary bg-white dark:bg-zinc-950"
                  : "border-transparent text-zinc-500 hover:text-zinc-800 dark:hover:text-zinc-300"
              }`}
            >
              <History className="w-3.5 h-3.5" />
              <span>History</span>
            </button>
          </div>
        )}

        {/* Main Content Area with Error Boundary */}
        <div className="flex-1 flex flex-col min-h-0">
          <GitErrorBoundary
            key={selectedFile ? "diff" : activeTab}
            onReset={() => {
              if (selectedFile) closeDiff();
              else setActiveTab("files");
            }}
          >
            {selectedFile ? (
              <GitDiffView onBack={closeDiff} />
            ) : (
              <>
                {activeTab === "files" && (
                  <GitFilesTab workspacePath={workspacePath} />
                )}
                {activeTab === "branches" && (
                  <BranchesTab workspacePath={workspacePath} />
                )}
                {activeTab === "log" && (
                  <GitLogTab workspacePath={workspacePath} />
                )}
              </>
            )}
          </GitErrorBoundary>
        </div>

        {/* Push/Pull bottom bar */}
        <GitPushPullBar workspacePath={workspacePath} />
      </div>
    </>
  );
}
