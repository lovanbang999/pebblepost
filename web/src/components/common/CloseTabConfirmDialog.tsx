import { AlertCircle } from 'lucide-react'
import { useTabStore } from '../../store/tabStore'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '../ui/dialog'
import { Button } from '../ui/button'

export function CloseTabConfirmDialog() {
  const { pendingCloseTab, confirmCloseTab } = useTabStore()

  if (!pendingCloseTab) {
    return null
  }

  return (
    <Dialog open={true} onOpenChange={(open) => !open && confirmCloseTab('cancel')}>
      <DialogContent className="max-w-md p-6">
        <DialogHeader>
          <div className="flex items-center gap-2.5 text-amber-500 mb-1">
            <AlertCircle className="w-5 h-5 shrink-0" />
            <DialogTitle className="text-base font-semibold text-zinc-900 dark:text-zinc-100">
              Unsaved Changes
            </DialogTitle>
          </div>
          <DialogDescription className="text-xs text-zinc-600 dark:text-zinc-400 leading-relaxed">
            Do you want to save the changes you made to{' '}
            <strong className="text-zinc-800 dark:text-zinc-200">
              "{pendingCloseTab.title}"
            </strong>{' '}
            before closing?
          </DialogDescription>
        </DialogHeader>

        <p className="text-[11px] text-zinc-500 dark:text-zinc-400">
          Your changes will be permanently lost if you close this tab without saving.
        </p>

        <DialogFooter className="flex items-center justify-end gap-2 pt-3">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => confirmCloseTab('cancel')}
            className="text-xs font-medium"
          >
            Cancel
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => confirmCloseTab('discard')}
            className="text-xs font-medium text-rose-600 dark:text-rose-400 hover:text-rose-700 hover:bg-rose-50 dark:hover:bg-rose-950/30"
          >
            Don't Save
          </Button>
          <Button
            type="button"
            variant="default"
            size="sm"
            onClick={() => confirmCloseTab('save')}
            className="text-xs font-semibold"
          >
            Save
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
