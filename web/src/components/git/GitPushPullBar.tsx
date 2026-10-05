import { ArrowDown, ArrowUp, RefreshCw, X, AlertCircle, CheckCircle2 } from 'lucide-react'
import { useGitStore } from '../../store/gitStore'
import { Button } from '../ui/button'

interface GitPushPullBarProps {
  workspacePath: string
}

export function GitPushPullBar({ workspacePath }: GitPushPullBarProps) {
  const {
    pull,
    push,
    isPushPullRunning,
    pushPullOutput,
    clearPushPullOutput,
  } = useGitStore()

  const handlePull = async () => {
    try {
      await pull(workspacePath)
    } catch {
      // output state handled in store
    }
  }

  const handlePush = async () => {
    try {
      await push(workspacePath)
    } catch {
      // output state handled in store
    }
  }

  return (
    <div className="border-t border-zinc-200 dark:border-zinc-800 p-3 bg-zinc-50 dark:bg-zinc-900/60">
      {/* Output / Error banner if present */}
      {pushPullOutput && (
        <div
          className={`mb-3 p-2.5 rounded-lg border text-xs font-mono transition-all ${
            pushPullOutput.success
              ? 'bg-emerald-50 dark:bg-emerald-950/30 border-emerald-200 dark:border-emerald-800 text-emerald-800 dark:text-emerald-300'
              : 'bg-rose-50 dark:bg-rose-950/30 border-rose-200 dark:border-rose-800 text-rose-800 dark:text-rose-300'
          }`}
        >
          <div className="flex items-start justify-between gap-2 mb-1">
            <span className="flex items-center gap-1.5 font-sans font-semibold">
              {pushPullOutput.success ? (
                <>
                  <CheckCircle2 className="w-3.5 h-3.5 text-emerald-600 dark:text-emerald-400" />
                  Operation successful
                </>
              ) : (
                <>
                  <AlertCircle className="w-3.5 h-3.5 text-rose-600 dark:text-rose-400" />
                  Operation failed
                </>
              )}
            </span>
            <button
              type="button"
              onClick={clearPushPullOutput}
              className="text-zinc-400 hover:text-zinc-600 dark:hover:text-zinc-200 p-0.5 rounded"
            >
              <X className="w-3.5 h-3.5" />
            </button>
          </div>
          {pushPullOutput.error && (
            <div className="font-sans text-xs text-rose-700 dark:text-rose-400 mb-1">
              {pushPullOutput.error}
            </div>
          )}
          {pushPullOutput.output && (
            <pre className="mt-1 max-h-24 overflow-y-auto whitespace-pre-wrap text-[11px] opacity-80 bg-black/5 dark:bg-black/20 p-1.5 rounded">
              {pushPullOutput.output}
            </pre>
          )}
        </div>
      )}

      {/* Buttons */}
      <div className="flex items-center gap-2">
        <Button
          variant="outline"
          size="sm"
          onClick={handlePull}
          disabled={isPushPullRunning}
          className="flex-1 flex items-center justify-center gap-1.5 text-xs h-8"
        >
          {isPushPullRunning ? (
            <RefreshCw className="w-3.5 h-3.5 animate-spin text-zinc-500" />
          ) : (
            <ArrowDown className="w-3.5 h-3.5 text-blue-500" />
          )}
          <span>Pull</span>
        </Button>

        <Button
          variant="outline"
          size="sm"
          onClick={handlePush}
          disabled={isPushPullRunning}
          className="flex-1 flex items-center justify-center gap-1.5 text-xs h-8"
        >
          {isPushPullRunning ? (
            <RefreshCw className="w-3.5 h-3.5 animate-spin text-zinc-500" />
          ) : (
            <ArrowUp className="w-3.5 h-3.5 text-emerald-500" />
          )}
          <span>Push</span>
        </Button>
      </div>
    </div>
  )
}
