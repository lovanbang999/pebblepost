import React, { useState, useEffect } from "react";
import { GitBranch, Check, Plus, AlertCircle, Sparkles } from "lucide-react";
import { useGitStore } from "../../store/gitStore";
import { Button } from "../ui/button";

interface BranchesTabProps {
  workspacePath: string;
}

export function BranchesTab({ workspacePath }: BranchesTabProps) {
  const {
    branches,
    fetchBranches,
    checkout,
    createBranch,
    isLoading,
    lastCheckoutResult,
    clearCheckoutResult,
    error,
    clearError,
  } = useGitStore();

  const [newBranchName, setNewBranchName] = useState("");
  const [isCreating, setIsCreating] = useState(false);

  useEffect(() => {
    if (workspacePath) {
      void fetchBranches(workspacePath);
    }
  }, [workspacePath, fetchBranches]);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newBranchName.trim()) return;
    try {
      await createBranch(newBranchName.trim(), workspacePath);
      setNewBranchName("");
      setIsCreating(false);
    } catch {
      // error handled in store
    }
  };

  const handleCheckout = async (branchName: string) => {
    try {
      await checkout(branchName, workspacePath);
    } catch {
      // error handled in store
    }
  };

  const safeBranches = Array.isArray(branches) ? branches : [];

  return (
    <div className="flex-1 flex flex-col min-h-0 bg-white dark:bg-zinc-950 p-3">
      {/* Toast / Checkout info banner */}
      {lastCheckoutResult && (
        <div
          className={`mb-3 p-2.5 rounded-lg border text-xs flex items-start justify-between gap-2 ${
            lastCheckoutResult.stashed
              ? "bg-amber-50 dark:bg-amber-950/30 border-amber-200 dark:border-amber-800 text-amber-800 dark:text-amber-300"
              : "bg-emerald-50 dark:bg-emerald-950/30 border-emerald-200 dark:border-emerald-800 text-emerald-800 dark:text-emerald-300"
          }`}
        >
          <div className="flex items-start gap-1.5">
            <Sparkles className="w-3.5 h-3.5 shrink-0 mt-0.5 text-amber-500" />
            <span>{lastCheckoutResult.message}</span>
          </div>
          <button
            type="button"
            onClick={clearCheckoutResult}
            className="text-zinc-400 hover:text-zinc-600 dark:hover:text-zinc-200 font-bold"
          >
            ×
          </button>
        </div>
      )}

      {/* Error banner */}
      {error && (
        <div className="mb-3 p-2.5 rounded-lg bg-rose-50 dark:bg-rose-950/30 border border-rose-200 dark:border-rose-800 text-rose-800 dark:text-rose-300 text-xs flex items-start justify-between gap-2">
          <div className="flex items-start gap-1.5">
            <AlertCircle className="w-3.5 h-3.5 shrink-0 mt-0.5 text-rose-600 dark:text-rose-400" />
            <span>{error}</span>
          </div>
          <button
            type="button"
            onClick={clearError}
            className="text-zinc-400 hover:text-zinc-600 dark:hover:text-zinc-200 font-bold"
          >
            ×
          </button>
        </div>
      )}

      {/* Top action: New branch */}
      <div className="mb-4">
        {!isCreating ? (
          <Button
            variant="outline"
            size="sm"
            onClick={() => setIsCreating(true)}
            className="w-full h-8 text-xs flex items-center justify-center gap-1.5 border-dashed"
          >
            <Plus className="w-3.5 h-3.5" />
            <span>Create new branch</span>
          </Button>
        ) : (
          <form
            onSubmit={handleCreate}
            className="space-y-2 p-2.5 rounded-lg border border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-900/40"
          >
            <div className="text-[11px] font-semibold text-zinc-500 uppercase tracking-wider">
              New Branch Name
            </div>
            <input
              type="text"
              value={newBranchName}
              onChange={(e) => setNewBranchName(e.target.value)}
              placeholder="e.g. feat/add-websocket"
              autoFocus
              className="w-full text-xs font-mono p-1.5 rounded border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-950 focus:outline-none focus:ring-1 focus:ring-primary"
            />
            <div className="flex items-center justify-end gap-2">
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={() => {
                  setIsCreating(false);
                  setNewBranchName("");
                }}
                className="h-7 text-xs px-2"
              >
                Cancel
              </Button>
              <Button
                type="submit"
                size="sm"
                disabled={!newBranchName.trim() || isLoading}
                className="h-7 text-xs px-3"
              >
                Create & Switch
              </Button>
            </div>
          </form>
        )}
      </div>

      {/* Branch list */}
      <div className="flex-1 overflow-y-auto space-y-1">
        <div className="text-[11px] font-semibold uppercase tracking-wider text-zinc-500 mb-2 px-1">
          Local Branches ({safeBranches.length})
        </div>

        {safeBranches.length === 0 ? (
          <div className="text-center py-8 text-zinc-400 text-xs">
            No branches found
          </div>
        ) : (
          safeBranches.map((b) => (
            <div
              key={b.name}
              className={`flex items-center justify-between p-2 rounded-lg text-xs transition-colors ${
                b.current
                  ? "bg-primary/10 border border-primary/20 text-primary font-medium"
                  : "hover:bg-zinc-100 dark:hover:bg-zinc-800/60 text-zinc-700 dark:text-zinc-300"
              }`}
            >
              <div className="flex items-center gap-2 min-w-0 flex-1">
                <GitBranch
                  className={`w-3.5 h-3.5 shrink-0 ${b.current ? "text-primary" : "text-zinc-400"}`}
                />
                <span className="font-mono truncate">{b.name}</span>
                {b.remote && (
                  <span className="text-[10px] text-zinc-400 truncate max-w-30">
                    [{b.remote}]
                  </span>
                )}
              </div>

              {b.current ? (
                <span className="flex items-center gap-1 text-[11px] text-primary font-semibold">
                  <Check className="w-3.5 h-3.5" />
                  Current
                </span>
              ) : (
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => handleCheckout(b.name)}
                  disabled={isLoading}
                  className="h-6 text-[11px] px-2 text-zinc-500 hover:text-zinc-900 dark:hover:text-zinc-100"
                >
                  Switch
                </Button>
              )}
            </div>
          ))
        )}
      </div>
    </div>
  );
}
