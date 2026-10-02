import * as React from 'react'
import { FilePlus2, Folder } from 'lucide-react'
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
import type { RequestDefinition } from '../../types'
import { getMethodColor, cn } from '../../lib/utils'

const METHODS: RequestDefinition['method'][] = [
  'GET',
  'POST',
  'PUT',
  'PATCH',
  'DELETE',
  'HEAD',
  'OPTIONS',
]

export interface FolderOption {
  path: string
  label: string
}

export interface NewRequestDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  defaultFolder?: string
  folderOptions: FolderOption[]
  onSubmit: (
    name: string,
    method: RequestDefinition['method'],
    folderPath: string
  ) => Promise<void>
}

export function NewRequestDialog({
  open,
  onOpenChange,
  defaultFolder,
  folderOptions,
  onSubmit,
}: NewRequestDialogProps) {
  const [name, setName] = React.useState('')
  const [method, setMethod] = React.useState<RequestDefinition['method']>('GET')
  const [folder, setFolder] = React.useState(defaultFolder || folderOptions[0]?.path || '')
  const [isSubmitting, setIsSubmitting] = React.useState(false)
  const [error, setError] = React.useState<string | null>(null)

  React.useEffect(() => {
    if (open) {
      setName('')
      setMethod('GET')
      setFolder(defaultFolder || folderOptions[0]?.path || '')
      setError(null)
      setIsSubmitting(false)
    }
  }, [open, defaultFolder, folderOptions])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    const cleanName = name.trim()
    if (!cleanName) {
      setError('Please enter a request name')
      return
    }

    try {
      setIsSubmitting(true)
      setError(null)
      await onSubmit(cleanName, method, folder)
      onOpenChange(false)
    } catch (err: any) {
      setError(err?.message || 'Failed to create request')
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
              <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-blue-500/10 text-blue-600 dark:text-blue-400">
                <FilePlus2 className="h-4 w-4" />
              </div>
              <div>
                <DialogTitle>New Request</DialogTitle>
                <DialogDescription>
                  Create an HTTP request (.pebble.json) in your collection.
                </DialogDescription>
              </div>
            </div>
          </DialogHeader>

          <div className="space-y-3 py-1">
            {/* Request Name */}
            <div className="space-y-1.5">
              <label
                htmlFor="request-name"
                className="text-xs font-medium text-zinc-700 dark:text-zinc-300"
              >
                Request Name
              </label>
              <Input
                id="request-name"
                autoFocus
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="e.g. get-user-profile"
                className="font-mono text-xs h-8"
              />
              <p className="text-[11px] text-zinc-400 dark:text-zinc-500">
                Will be saved as {name.trim() ? `${name.trim()}.pebble.json` : '[name].pebble.json'}
              </p>
            </div>

            {/* Method & Folder in Grid */}
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <label className="text-xs font-medium text-zinc-700 dark:text-zinc-300">
                  HTTP Method
                </label>
                <Select
                  value={method}
                  onValueChange={(val) =>
                    typeof val === 'string' && setMethod(val as RequestDefinition['method'])
                  }
                >
                  <SelectTrigger className="h-8">
                    <SelectValue>
                      <span
                        className={cn(
                          'text-[10px] font-mono font-bold px-1.5 py-0.5 rounded border uppercase',
                          getMethodColor(method)
                        )}
                      >
                        {method}
                      </span>
                    </SelectValue>
                  </SelectTrigger>
                  <SelectContent align="start">
                    {METHODS.map((m) => (
                      <SelectItem key={m} value={m}>
                        <span
                          className={cn(
                            'text-[10px] font-mono font-bold px-1.5 py-0.5 rounded border uppercase mr-2',
                            getMethodColor(m)
                          )}
                        >
                          {m}
                        </span>
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              <div className="space-y-1.5">
                <label className="text-xs font-medium text-zinc-700 dark:text-zinc-300">
                  Save In Folder
                </label>
                <Select
                  value={folder}
                  onValueChange={(val) => typeof val === 'string' && setFolder(val)}
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
              disabled={!name.trim() || isSubmitting}
            >
              {isSubmitting ? 'Creating...' : 'Create Request'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
