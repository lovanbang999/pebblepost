import { useState, useEffect } from 'react'
import { Bookmark, Loader2, ShieldCheck, Check } from 'lucide-react'
import { Button } from '../ui/button'
import { Input } from '../ui/input'
import { Badge } from '../ui/badge'
import type { ExecutionResult, ExampleResponse, KeyValue } from '../../types'

interface SaveExampleDialogProps {
  isOpen: boolean
  onClose: () => void
  filePath: string
  result: ExecutionResult
  onSaved: (example: ExampleResponse) => void
}

export function SaveExampleDialog({
  isOpen,
  onClose,
  filePath,
  result,
  onSaved,
}: SaveExampleDialogProps) {
  const defaultName = `${result.statusCode} ${result.statusText || 'Response'}`.trim()
  const [name, setName] = useState(defaultName)
  const [isSaving, setIsSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (isOpen) {
      setName(`${result.statusCode} ${result.statusText || 'Response'}`.trim())
      setError(null)
      setIsSaving(false)
    }
  }, [isOpen, result])

  if (!isOpen) return null

  // Convert headers from Record<string, string[]> to KeyValue[]
  const headerKeyValues: KeyValue[] = Object.entries(result.headers || {}).flatMap(([k, vals]) =>
    vals.map((v) => ({ key: k, value: v, enabled: true }))
  )

  const contentType =
    Object.entries(result.headers || {}).find(
      ([k]) => k.toLowerCase() === 'content-type'
    )?.[1]?.[0] || 'application/json'

  const size = result.bodySizeBytes ?? result.size ?? 0
  const durationMs = Math.round(result.timing?.totalDurationMs || 0)

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return

    setIsSaving(true)
    setError(null)

    try {
      const res = await fetch('/api/request/example/save', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          filePath,
          name: name.trim(),
          statusCode: result.statusCode,
          statusText: result.statusText,
          headers: headerKeyValues,
          body: result.body,
          contentType,
          durationMs,
          size,
        }),
      })

      if (!res.ok) {
        const errText = await res.text()
        throw new Error(errText || `Failed to save example (${res.status})`)
      }

      const data = await res.json()
      if (data.example) {
        onSaved(data.example)
      }
      onClose()
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Failed to save example')
    } finally {
      setIsSaving(false)
    }
  }

  const isSuccess = result.statusCode >= 200 && result.statusCode < 300

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-xs p-4">
      <div className="bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 rounded-xl shadow-2xl max-w-md w-full overflow-hidden animate-in fade-in zoom-in-95 duration-150">
        {/* Header */}
        <div className="p-4 border-b border-zinc-200 dark:border-zinc-800 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <div className="w-8 h-8 rounded-lg bg-blue-500/10 dark:bg-blue-500/20 text-blue-600 dark:text-blue-400 flex items-center justify-center border border-blue-500/20">
              <Bookmark className="w-4 h-4" />
            </div>
            <div>
              <h3 className="font-semibold text-sm text-zinc-900 dark:text-zinc-100">
                Save as Example
              </h3>
              <p className="text-xs text-zinc-500">
                Store snapshot next to request file
              </p>
            </div>
          </div>
          <Badge
            variant={isSuccess ? 'success' : 'destructive'}
            className="text-xs font-mono font-bold"
          >
            {result.statusCode} {result.statusText}
          </Badge>
        </div>

        {/* Content */}
        <form onSubmit={handleSave} className="p-4 space-y-4">
          <div>
            <label className="block text-xs font-medium text-zinc-700 dark:text-zinc-300 mb-1.5">
              Example Name
            </label>
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g. 200 OK - Successful response"
              autoFocus
              className="text-xs"
              required
            />
          </div>

          {/* Details summary */}
          <div className="p-2.5 rounded-lg bg-zinc-50 dark:bg-zinc-800/50 border border-zinc-200/80 dark:border-zinc-800 text-[11px] space-y-1 font-mono text-zinc-600 dark:text-zinc-400">
            <div className="flex justify-between">
              <span>Content-Type:</span>
              <span className="text-zinc-800 dark:text-zinc-200 truncate max-w-50">{contentType}</span>
            </div>
            <div className="flex justify-between">
              <span>Response Size:</span>
              <span className="text-zinc-800 dark:text-zinc-200">{size.toLocaleString()} bytes</span>
            </div>
            <div className="flex justify-between">
              <span>Duration:</span>
              <span className="text-zinc-800 dark:text-zinc-200">{durationMs} ms</span>
            </div>
          </div>

          {/* Security note */}
          <div className="flex items-start gap-2 p-2.5 rounded-lg bg-emerald-50 dark:bg-emerald-950/20 border border-emerald-200/60 dark:border-emerald-900/40 text-[11px] text-emerald-800 dark:text-emerald-300">
            <ShieldCheck className="w-4 h-4 text-emerald-600 dark:text-emerald-400 shrink-0 mt-0.5" />
            <span>
              Authentication headers, cookies, and active environment secrets are automatically masked before saving. Payload capped at 512 KB.
            </span>
          </div>

          {error && (
            <div className="p-2.5 rounded-lg bg-red-50 dark:bg-red-950/20 border border-red-200 dark:border-red-900/40 text-xs text-red-600 dark:text-red-400">
              {error}
            </div>
          )}

          {/* Footer Actions */}
          <div className="flex items-center justify-end gap-2 pt-2 border-t border-zinc-100 dark:border-zinc-800">
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={onClose}
              disabled={isSaving}
              className="text-xs h-8"
            >
              Cancel
            </Button>
            <Button
              type="submit"
              size="sm"
              disabled={isSaving || !name.trim()}
              className="text-xs h-8 gap-1.5 bg-blue-600 hover:bg-blue-700 text-white"
            >
              {isSaving ? (
                <>
                  <Loader2 className="w-3.5 h-3.5 animate-spin" />
                  Saving...
                </>
              ) : (
                <>
                  <Check className="w-3.5 h-3.5" />
                  Save Example
                </>
              )}
            </Button>
          </div>
        </form>
      </div>
    </div>
  )
}
