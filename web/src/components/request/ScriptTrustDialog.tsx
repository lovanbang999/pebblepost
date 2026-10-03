import { ShieldAlert, ShieldCheck } from 'lucide-react'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '../ui/dialog'
import { Button } from '../ui/button'

interface ScriptTrustDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  workspacePath?: string | null
  hasPreRequest?: boolean
  hasPostResponse?: boolean
  onConfirmTrust: () => void
  onRunWithoutScripts: () => void
  onCancel: () => void
}

export function ScriptTrustDialog({
  open,
  onOpenChange,
  workspacePath,
  hasPreRequest,
  hasPostResponse,
  onConfirmTrust,
  onRunWithoutScripts,
  onCancel,
}: ScriptTrustDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg p-6 overflow-hidden">
        <DialogHeader>
          <div className="flex items-center gap-2.5 text-amber-500 mb-1">
            <ShieldAlert className="w-5 h-5 shrink-0" />
            <DialogTitle className="text-base font-semibold text-zinc-900 dark:text-zinc-100">
              Trust Workspace Scripts?
            </DialogTitle>
          </div>
          <DialogDescription className="text-xs text-zinc-600 dark:text-zinc-400 leading-relaxed">
            This request contains JavaScript pre-request or test scripts. Executing scripts from untrusted workspaces allows code execution on your system.
          </DialogDescription>
        </DialogHeader>

        <div className="bg-zinc-50 dark:bg-zinc-900/60 border border-zinc-200 dark:border-zinc-800 rounded-lg p-3 space-y-2.5 min-w-0">
          <div className="flex items-center justify-between text-xs">
            <span className="text-zinc-500 dark:text-zinc-400 font-medium">Detected Scripts:</span>
            <div className="flex items-center gap-1.5 flex-wrap">
              {hasPreRequest && (
                <span className="inline-flex items-center text-[10px] font-medium px-2 py-0.5 rounded bg-zinc-200/80 dark:bg-zinc-800 text-zinc-700 dark:text-zinc-300 border border-zinc-300/60 dark:border-zinc-700/60">
                  Pre-request
                </span>
              )}
              {hasPostResponse && (
                <span className="inline-flex items-center text-[10px] font-medium px-2 py-0.5 rounded bg-zinc-200/80 dark:bg-zinc-800 text-zinc-700 dark:text-zinc-300 border border-zinc-300/60 dark:border-zinc-700/60">
                  Post-response
                </span>
              )}
            </div>
          </div>
          {workspacePath && (
            <div className="space-y-1 min-w-0">
              <span className="text-[11px] font-medium text-zinc-500 dark:text-zinc-400">Workspace</span>
              <div className="bg-white dark:bg-zinc-950 p-2 rounded border border-zinc-200/80 dark:border-zinc-800/80 min-w-0">
                <code className="block font-mono text-[11px] text-zinc-700 dark:text-zinc-300 break-all select-all leading-relaxed">
                  {workspacePath}
                </code>
              </div>
            </div>
          )}
        </div>

        <p className="text-xs text-zinc-500 dark:text-zinc-400">
          Do you trust this workspace to execute scripts?
        </p>

        <DialogFooter className="flex flex-col-reverse sm:flex-row sm:items-center sm:justify-end gap-2 sm:gap-2 sm:space-x-0 pt-3">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={onCancel}
            className="text-xs font-medium"
          >
            Cancel
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={onRunWithoutScripts}
            className="text-xs font-medium text-zinc-700 dark:text-zinc-300"
          >
            Run without Scripts
          </Button>
          <Button
            type="button"
            size="sm"
            onClick={onConfirmTrust}
            className="text-xs font-medium gap-1.5 bg-blue-600 hover:bg-blue-700 text-white cursor-pointer"
          >
            <ShieldCheck className="w-3.5 h-3.5" />
            Trust &amp; Execute
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
