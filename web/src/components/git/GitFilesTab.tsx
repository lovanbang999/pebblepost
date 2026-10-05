import { useState, useMemo } from 'react'
import {
  Plus,
  Minus,
  Lock,
  AlertCircle,
} from 'lucide-react'
import { useGitStore } from '../../store/gitStore'
import { Button } from '../ui/button'
import { Tooltip } from '../ui/tooltip'
import type { GitFileStatus } from '../../types'

interface GitFilesTabProps {
  workspacePath: string
}

export function GitFilesTab({ workspacePath }: GitFilesTabProps) {
  const {
    status,
    stage,
    unstage,
    commit,
    fetchDiff,
    isLoading,
    error,
    clearError,
  } = useGitStore()

  const [selectedPaths, setSelectedPaths] = useState<Set<string>>(new Set())
  const [commitMessage, setCommitMessage] = useState('')

  const isSecretProtected = (path: string) =>
    path.endsWith('.secret.env.json') || path === '.secret.env.json'

  // Partition files
  const { stagedFiles, unstagedFiles, untrackedFiles } = useMemo(() => {
    const staged: GitFileStatus[] = []
    const unstaged: GitFileStatus[] = []
    const untracked: GitFileStatus[] = []

    if (status?.files) {
      for (const f of status.files) {
        if (f.code === '?') {
          untracked.push(f)
        } else {
          if (f.staged) staged.push(f)
          // File could have both staged and unstaged parts or be pure unstaged
          if (!f.staged || f.code.length > 1) unstaged.push(f)
        }
      }
    }

    return { stagedFiles: staged, unstagedFiles: unstaged, untrackedFiles: untracked }
  }, [status?.files])

  const toggleSelect = (path: string) => {
    if (isSecretProtected(path)) return
    setSelectedPaths((prev) => {
      const next = new Set(prev)
      if (next.has(path)) next.delete(path)
      else next.add(path)
      return next
    })
  }

  const handleStageSelected = async () => {
    const toStage = Array.from(selectedPaths).filter((p) => !isSecretProtected(p))
    if (toStage.length === 0) return
    try {
      await stage(toStage, workspacePath)
      setSelectedPaths(new Set())
    } catch {
      // error handled in store
    }
  }

  const handleStageAll = async () => {
    const all = [...unstagedFiles, ...untrackedFiles]
      .map((f) => f.path)
      .filter((p) => !isSecretProtected(p))
    if (all.length === 0) return
    try {
      await stage(all, workspacePath)
      setSelectedPaths(new Set())
    } catch {
      // error handled in store
    }
  }

  const handleUnstageSelected = async () => {
    const toUnstage = Array.from(selectedPaths)
    if (toUnstage.length === 0) return
    try {
      await unstage(toUnstage, workspacePath)
      setSelectedPaths(new Set())
    } catch {
      // error handled in store
    }
  }

  const handleUnstageAll = async () => {
    const all = stagedFiles.map((f) => f.path)
    if (all.length === 0) return
    try {
      await unstage(all, workspacePath)
      setSelectedPaths(new Set())
    } catch {
      // error handled in store
    }
  }

  const handleCommit = async () => {
    if (!commitMessage.trim() || stagedFiles.length === 0) return
    try {
      await commit(commitMessage.trim(), workspacePath)
      setCommitMessage('')
    } catch {
      // error handled in store
    }
  }

  const renderBadge = (code: string) => {
    switch (code) {
      case 'M':
      case 'MM':
        return (
          <span className="px-1.5 py-0.2 rounded text-[10px] font-bold font-mono bg-amber-500/10 text-amber-600 dark:text-amber-400 border border-amber-500/20">
            M
          </span>
        )
      case 'A':
        return (
          <span className="px-1.5 py-0.2 rounded text-[10px] font-bold font-mono bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20">
            A
          </span>
        )
      case 'D':
        return (
          <span className="px-1.5 py-0.2 rounded text-[10px] font-bold font-mono bg-rose-500/10 text-rose-600 dark:text-rose-400 border border-rose-500/20">
            D
          </span>
        )
      default:
        return (
          <span className="px-1.5 py-0.2 rounded text-[10px] font-bold font-mono bg-zinc-500/10 text-zinc-500 dark:text-zinc-400 border border-zinc-500/20">
            ?
          </span>
        )
    }
  }

  const renderFileRow = (file: GitFileStatus, isStagedSection: boolean) => {
    const protectedFile = isSecretProtected(file.path)
    const isChecked = selectedPaths.has(file.path)

    return (
      <div
        key={`${file.path}-${isStagedSection ? 'staged' : 'unstaged'}`}
        className="flex items-center justify-between p-2 rounded-md hover:bg-zinc-100 dark:hover:bg-zinc-800/60 text-xs transition-colors group"
      >
        <div className="flex items-center gap-2 min-w-0 flex-1">
          {protectedFile ? (
            <Tooltip content="Protected: secret files must never be committed">
              <span className="p-1 text-amber-500 dark:text-amber-400 cursor-not-allowed">
                <Lock className="w-3.5 h-3.5" />
              </span>
            </Tooltip>
          ) : (
            <input
              type="checkbox"
              checked={isChecked}
              onChange={() => toggleSelect(file.path)}
              className="rounded border-zinc-300 dark:border-zinc-700 text-primary focus:ring-primary w-3.5 h-3.5 cursor-pointer"
            />
          )}

          {renderBadge(file.code)}

          <button
            type="button"
            onClick={() => void fetchDiff(file.path, workspacePath)}
            className="truncate text-left font-mono text-zinc-800 dark:text-zinc-200 hover:text-primary dark:hover:text-primary transition-colors flex-1"
            title={`Click to view diff for ${file.path}`}
          >
            {file.path}
          </button>
        </div>

        {/* Quick single-file stage / unstage button */}
        {!protectedFile && (
          <div className="opacity-0 group-hover:opacity-100 transition-opacity">
            {isStagedSection ? (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => void unstage([file.path], workspacePath)}
                className="h-6 w-6 p-0 text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-200"
                title="Unstage this file"
              >
                <Minus className="w-3 h-3" />
              </Button>
            ) : (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => void stage([file.path], workspacePath)}
                className="h-6 w-6 p-0 text-zinc-400 hover:text-emerald-600 dark:hover:text-emerald-400"
                title="Stage this file"
              >
                <Plus className="w-3 h-3" />
              </Button>
            )}
          </div>
        )}
      </div>
    )
  }

  return (
    <div className="flex-1 flex flex-col min-h-0 bg-white dark:bg-zinc-950">
      {/* Error alert banner */}
      {error && (
        <div className="p-3 bg-rose-50 dark:bg-rose-950/30 border-b border-rose-200 dark:border-rose-800 text-rose-800 dark:text-rose-300 text-xs flex items-start justify-between gap-2">
          <div className="flex items-start gap-1.5">
            <AlertCircle className="w-4 h-4 text-rose-600 dark:text-rose-400 shrink-0 mt-0.5" />
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

      {/* Files list */}
      <div className="flex-1 overflow-y-auto p-3 space-y-4">
        {/* Staged Section */}
        <div>
          <div className="flex items-center justify-between mb-1.5 px-1">
            <span className="text-[11px] font-semibold uppercase tracking-wider text-zinc-500">
              Staged Changes ({stagedFiles.length})
            </span>
            {stagedFiles.length > 0 && (
              <div className="flex items-center gap-1.5">
                {selectedPaths.size > 0 && (
                  <button
                    type="button"
                    onClick={handleUnstageSelected}
                    className="text-[11px] text-zinc-500 hover:text-zinc-800 dark:hover:text-zinc-200"
                  >
                    Unstage Selected
                  </button>
                )}
                <button
                  type="button"
                  onClick={handleUnstageAll}
                  className="text-[11px] text-primary hover:underline"
                >
                  Unstage All
                </button>
              </div>
            )}
          </div>
          {stagedFiles.length === 0 ? (
            <div className="p-3 text-center text-xs text-zinc-400 border border-dashed border-zinc-200 dark:border-zinc-800 rounded-md">
              No staged changes
            </div>
          ) : (
            <div className="space-y-0.5">
              {stagedFiles.map((f) => renderFileRow(f, true))}
            </div>
          )}
        </div>

        {/* Unstaged & Untracked Section */}
        <div>
          <div className="flex items-center justify-between mb-1.5 px-1">
            <span className="text-[11px] font-semibold uppercase tracking-wider text-zinc-500">
              Changes ({unstagedFiles.length + untrackedFiles.length})
            </span>
            {unstagedFiles.length + untrackedFiles.length > 0 && (
              <div className="flex items-center gap-1.5">
                {selectedPaths.size > 0 && (
                  <button
                    type="button"
                    onClick={handleStageSelected}
                    className="text-[11px] text-zinc-500 hover:text-zinc-800 dark:hover:text-zinc-200"
                  >
                    Stage Selected
                  </button>
                )}
                <button
                  type="button"
                  onClick={handleStageAll}
                  className="text-[11px] text-primary hover:underline"
                >
                  Stage All
                </button>
              </div>
            )}
          </div>
          {unstagedFiles.length === 0 && untrackedFiles.length === 0 ? (
            <div className="p-3 text-center text-xs text-zinc-400 border border-dashed border-zinc-200 dark:border-zinc-800 rounded-md">
              Working tree clean
            </div>
          ) : (
            <div className="space-y-0.5">
              {unstagedFiles.map((f) => renderFileRow(f, false))}
              {untrackedFiles.map((f) => renderFileRow(f, false))}
            </div>
          )}
        </div>
      </div>

      {/* Commit Box */}
      <div className="border-t border-zinc-200 dark:border-zinc-800 p-3 bg-zinc-50 dark:bg-zinc-900/40">
        <textarea
          value={commitMessage}
          onChange={(e) => setCommitMessage(e.target.value)}
          onKeyDown={(e) => {
            if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
              e.preventDefault()
              void handleCommit()
            }
          }}
          placeholder={
            stagedFiles.length === 0
              ? 'Stage changes above to commit...'
              : 'Commit message (Ctrl+Enter to commit)'
          }
          disabled={stagedFiles.length === 0 || isLoading}
          rows={3}
          className="w-full text-xs font-mono p-2 rounded-md border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-950 focus:outline-none focus:ring-1 focus:ring-primary disabled:opacity-50 resize-none"
        />

        <div className="mt-2 flex items-center justify-between">
          <span className="text-[11px] text-zinc-500">
            {stagedFiles.length} file{stagedFiles.length === 1 ? '' : 's'} staged
          </span>
          <Button
            size="sm"
            onClick={handleCommit}
            disabled={
              stagedFiles.length === 0 || !commitMessage.trim() || isLoading
            }
            className="h-8 px-4 text-xs font-medium"
          >
            {isLoading ? 'Committing...' : 'Commit'}
          </Button>
        </div>
      </div>
    </div>
  )
}
