import { useState } from 'react'
import { AlertTriangle, GitCompare, ArrowDownToLine, FileEdit } from 'lucide-react'
import { useWorkspaceStore } from '../../store/workspaceStore'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '../ui/dialog'
import { Button } from '../ui/button'

export function ConflictDialog() {
  const { diskConflict, resolveConflict, dismissConflict } = useWorkspaceStore()
  const [showDiff, setShowDiff] = useState(false)

  if (!diskConflict) {
    return null
  }

  const fileName = diskConflict.filePath.split('/').pop() || diskConflict.filePath
  const localJSON = JSON.stringify(diskConflict.localRequest, null, 2)
  const diskJSON = JSON.stringify(diskConflict.diskRequest, null, 2)

  return (
    <Dialog open={true} onOpenChange={(open) => !open && dismissConflict()}>
      <DialogContent className={showDiff ? "max-w-4xl max-h-[85vh] flex flex-col p-6" : "max-w-md p-6"}>
        <DialogHeader>
          <div className="flex items-center gap-2.5 text-amber-500 mb-1">
            <AlertTriangle className="w-5 h-5 shrink-0" />
            <DialogTitle className="text-base font-semibold text-zinc-900 dark:text-zinc-100">
              File Conflict: Modified on Disk
            </DialogTitle>
          </div>
          <DialogDescription className="text-xs text-zinc-600 dark:text-zinc-400 leading-relaxed">
            The request file <code className="font-mono text-zinc-800 dark:text-zinc-200 bg-zinc-100 dark:bg-zinc-800 px-1 py-0.5 rounded">{fileName}</code> was modified by an external process or Git while you have unsaved changes in PebblePost.
          </DialogDescription>
        </DialogHeader>

        {showDiff ? (
          <div className="flex-1 flex flex-col min-h-0 my-3 gap-2">
            <div className="grid grid-cols-2 gap-3 text-xs font-semibold text-zinc-500">
              <span className="flex items-center gap-1.5 text-blue-500 dark:text-blue-400">
                <FileEdit className="w-3.5 h-3.5" /> Your Unsaved Changes (Local)
              </span>
              <span className="flex items-center gap-1.5 text-amber-500 dark:text-amber-400">
                <ArrowDownToLine className="w-3.5 h-3.5" /> New Content on Disk (External)
              </span>
            </div>
            <div className="grid grid-cols-2 gap-3 flex-1 min-h-[300px] overflow-hidden">
              <pre className="p-3 text-xs font-mono bg-zinc-100 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 rounded-lg overflow-auto select-text text-zinc-800 dark:text-zinc-200 leading-relaxed">
                {localJSON}
              </pre>
              <pre className="p-3 text-xs font-mono bg-zinc-100 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 rounded-lg overflow-auto select-text text-zinc-800 dark:text-zinc-200 leading-relaxed">
                {diskJSON}
              </pre>
            </div>
          </div>
        ) : (
          <div className="my-2 p-3 bg-amber-500/10 border border-amber-500/20 rounded-lg text-xs text-amber-700 dark:text-amber-300">
            Choose whether to overwrite the disk with your editor changes, or discard your unsaved changes and reload from disk.
          </div>
        )}

        <DialogFooter className="flex items-center justify-between sm:justify-between w-full pt-2">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => setShowDiff((prev) => !prev)}
            className="text-xs text-zinc-500 hover:text-zinc-800 dark:hover:text-zinc-200 gap-1.5"
          >
            <GitCompare className="w-3.5 h-3.5" />
            {showDiff ? "Hide Diff" : "View Diff"}
          </Button>

          <div className="flex items-center gap-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => resolveConflict('mine')}
              className="text-xs font-medium"
            >
              Keep Mine
            </Button>
            <Button
              type="button"
              variant="default"
              size="sm"
              onClick={() => resolveConflict('disk')}
              className="text-xs font-medium bg-amber-600 hover:bg-amber-500 text-white"
            >
              Reload from Disk
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
