import * as React from 'react'
import { FolderPlus, Folder } from 'lucide-react'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '../ui/dialog'
import { Input } from '../ui/input'
import { Button } from '../ui/button'
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from '../ui/select'
import type { FolderOption } from './NewRequestDialog'

export interface NewFolderDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  defaultParentFolder?: string
  folderOptions: FolderOption[]
  onSubmit: (folderName: string, parentPath: string) => Promise<void>
}

export function NewFolderDialog({
  open,
  onOpenChange,
  defaultParentFolder,
  folderOptions,
  onSubmit,
}: NewFolderDialogProps) {
  const [folderName, setFolderName] = React.useState('')
  const [parentFolder, setParentFolder] = React.useState(
    defaultParentFolder || folderOptions[0]?.path || ''
  )
  const [isSubmitting, setIsSubmitting] = React.useState(false)
  const [error, setError] = React.useState<string | null>(null)

  React.useEffect(() => {
    if (open) {
      setFolderName('')
      setParentFolder(defaultParentFolder || folderOptions[0]?.path || '')
      setError(null)
      setIsSubmitting(false)
    }
  }, [open, defaultParentFolder, folderOptions])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    const cleanName = folderName.trim().replace(/^\/+|\/+$/g, '')
    if (!cleanName) {
      setError('Please enter a folder name')
      return
    }

    try {
      setIsSubmitting(true)
      setError(null)
      await onSubmit(cleanName, parentFolder)
      onOpenChange(false)
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Failed to create folder')
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <form onSubmit={handleSubmit} className="space-y-4">
          <DialogHeader>
            <div className="flex items-center gap-2">
              <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-amber-500/10 text-amber-600 dark:text-amber-400">
                <FolderPlus className="h-4 w-4" />
              </div>
              <div>
                <DialogTitle>New Folder</DialogTitle>
                <DialogDescription>
                  Create a folder to group and organize your requests.
                </DialogDescription>
              </div>
            </div>
          </DialogHeader>

          <div className="space-y-3 py-1">
            {/* Folder Name */}
            <div className="space-y-1.5">
              <label
                htmlFor="folder-name"
                className="text-xs font-medium text-zinc-700 dark:text-zinc-300"
              >
                Folder Name
              </label>
              <Input
                id="folder-name"
                autoFocus
                value={folderName}
                onChange={(e) => setFolderName(e.target.value)}
                placeholder="e.g. users, auth, payments"
                className="font-mono text-xs h-8"
              />
            </div>

            {/* Parent Folder */}
            <div className="space-y-1.5">
              <label className="text-xs font-medium text-zinc-700 dark:text-zinc-300">
                Parent Folder
              </label>
              <Select
                value={parentFolder}
                onValueChange={(val) => typeof val === 'string' && setParentFolder(val)}
              >
                <SelectTrigger className="h-8">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent align="start">
                  {folderOptions.map((f) => (
                    <SelectItem key={f.path} value={f.path}>
                      <span className="flex items-center gap-1.5 text-xs truncate">
                        <Folder className="h-3 w-3 text-amber-500 shrink-0" />
                        <span className="truncate">{f.label}</span>
                      </span>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            {error && (
              <div className="text-xs text-red-600 dark:text-red-400 bg-red-50 dark:bg-red-950/40 border border-red-200 dark:border-red-900 rounded p-2">
                {error}
              </div>
            )}
          </div>

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => onOpenChange(false)}
              disabled={isSubmitting}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              size="sm"
              disabled={!folderName.trim() || isSubmitting}
            >
              {isSubmitting ? 'Creating...' : 'Create Folder'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
