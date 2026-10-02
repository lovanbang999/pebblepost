import { useState } from 'react'
import { Upload, FileJson, Terminal, Layers, AlertCircle, CheckCircle2, Loader2 } from 'lucide-react'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogTrigger,
} from '../ui/dialog'
import { Button } from '../ui/button'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '../ui/tabs'
import { useWorkspaceStore } from '../../store/workspaceStore'
import type { RequestDefinition } from '../../types'

type ImportSource = 'curl' | 'postman' | 'openapi'

interface ImportedPreview {
  requests: RequestDefinition[]
  count: number
}

interface Props {
  onImported?: (requests: RequestDefinition[]) => void
}

const TABS: { key: ImportSource; label: string; icon: React.ReactNode; placeholder: string }[] = [
  {
    key: 'curl',
    label: 'cURL',
    icon: <Terminal className="w-3.5 h-3.5" />,
    placeholder: `curl -X POST 'https://api.example.com/users' \\
  -H 'Authorization: Bearer {{TOKEN}}' \\
  -H 'Content-Type: application/json' \\
  --data-raw '{"name": "Alice"}'`,
  },
  {
    key: 'postman',
    label: 'Postman',
    icon: <FileJson className="w-3.5 h-3.5" />,
    placeholder: `Paste your Postman Collection v2.1 JSON here...`,
  },
  {
    key: 'openapi',
    label: 'OpenAPI',
    icon: <Layers className="w-3.5 h-3.5" />,
    placeholder: `Paste your OpenAPI 3.0 JSON spec here...`,
  },
]

export function ImportDialog({ onImported }: Props) {
  const [open, setOpen] = useState(false)
  const [activeTab, setActiveTab] = useState<ImportSource>('curl')
  const [content, setContent] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [preview, setPreview] = useState<ImportedPreview | null>(null)

  const { workspacePath, activeFilePath, loadWorkspace } = useWorkspaceStore()

  const handleParse = async () => {
    if (!content.trim()) {
      setError('Please paste your content above.')
      return
    }

    setLoading(true)
    setError(null)
    setPreview(null)

    try {
      const res = await fetch('/api/import', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ source: activeTab, content }),
      })

      const data = await res.json()

      if (!res.ok) {
        setError(data.error || 'Import failed.')
        return
      }

      setPreview(data as ImportedPreview)
    } catch (err: any) {
      setError(err.message || 'Network error')
    } finally {
      setLoading(false)
    }
  }

  const handleSaveAll = async () => {
    if (!preview || !workspacePath) return

    setLoading(true)
    setError(null)

    try {
      // Determine save directory: alongside active request or workspace root collections/
      const baseDir = activeFilePath
        ? activeFilePath.replace(/\/[^/]+$/, '')
        : `${workspacePath}/collections/imported`

      for (const req of preview.requests) {
        const safeName = req.name
          .toLowerCase()
          .replace(/[^a-z0-9]+/g, '-')
          .replace(/(^-|-$)/g, '') || 'request'

        const filePath = `${baseDir}/${safeName}.pebble.json`

        await fetch('/api/request/save', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ path: filePath, request: req }),
        })
      }

      // Refresh workspace tree
      await loadWorkspace()

      // Notify parent
      onImported?.(preview.requests)

      setOpen(false)
      setContent('')
      setPreview(null)
    } catch (err: any) {
      setError(err.message || 'Failed to save requests')
    } finally {
      setLoading(false)
    }
  }

  const handleTabChange = (tab: string) => {
    setActiveTab(tab as ImportSource)
    setContent('')
    setError(null)
    setPreview(null)
  }

  const handleFileUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    const reader = new FileReader()
    reader.onload = (ev) => {
      setContent(ev.target?.result as string)
      setError(null)
      setPreview(null)
    }
    reader.readAsText(file)
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button
          id="import-trigger"
          variant="ghost"
          size="sm"
          className="h-7 gap-1.5 text-xs text-zinc-400 hover:text-zinc-200 px-2"
          title="Import Request"
        >
          <Upload className="w-3.5 h-3.5" />
          <span className="hidden sm:inline">Import</span>
        </Button>
      </DialogTrigger>

      <DialogContent className="max-w-2xl w-full">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 text-sm">
            <Upload className="w-4 h-4 text-sky-400" />
            Import Requests
          </DialogTitle>
        </DialogHeader>

        <Tabs value={activeTab} onValueChange={handleTabChange} className="mt-1">
          <TabsList className="w-full grid grid-cols-3">
            {TABS.map((t) => (
              <TabsTrigger key={t.key} value={t.key} className="text-xs gap-1.5">
                {t.icon}
                {t.label}
              </TabsTrigger>
            ))}
          </TabsList>

          {TABS.map((t) => (
            <TabsContent key={t.key} value={t.key} className="mt-3 space-y-3">
              {/* File upload shortcut for JSON tabs */}
              {(t.key === 'postman' || t.key === 'openapi') && (
                <div className="flex items-center gap-2">
                  <label
                    htmlFor={`import-file-${t.key}`}
                    className="cursor-pointer flex items-center gap-1.5 text-xs text-sky-400 hover:text-sky-300 transition-colors"
                  >
                    <FileJson className="w-3.5 h-3.5" />
                    Upload .json file
                  </label>
                  <input
                    id={`import-file-${t.key}`}
                    type="file"
                    accept=".json"
                    className="hidden"
                    onChange={handleFileUpload}
                  />
                  <span className="text-zinc-600 text-xs">or paste below</span>
                </div>
              )}

              <textarea
                id={`import-textarea-${t.key}`}
                value={content}
                onChange={(e) => {
                  setContent(e.target.value)
                  setError(null)
                  setPreview(null)
                }}
                rows={10}
                placeholder={t.placeholder}
                spellCheck={false}
                className="w-full rounded-lg border border-zinc-800 bg-zinc-950 p-3 text-xs font-mono text-zinc-200 resize-y focus:outline-none focus:ring-1 focus:ring-sky-500 placeholder:text-zinc-600"
              />
            </TabsContent>
          ))}
        </Tabs>

        {/* Error banner */}
        {error && (
          <div className="flex items-start gap-2 rounded-lg border border-rose-800/50 bg-rose-950/30 p-3 text-xs text-rose-300">
            <AlertCircle className="w-3.5 h-3.5 mt-0.5 shrink-0" />
            {error}
          </div>
        )}

        {/* Preview / success state */}
        {preview && (
          <div className="rounded-lg border border-emerald-800/50 bg-emerald-950/20 p-3 space-y-2">
            <div className="flex items-center gap-2 text-xs font-semibold text-emerald-400">
              <CheckCircle2 className="w-3.5 h-3.5" />
              Parsed {preview.count} request{preview.count !== 1 ? 's' : ''} successfully
            </div>
            <div className="space-y-1 max-h-40 overflow-y-auto">
              {preview.requests.map((r, i) => (
                <div key={i} className="flex items-center gap-2 text-xs text-zinc-300">
                  <span className="font-mono font-bold text-sky-400 w-14 shrink-0">{r.method}</span>
                  <span className="truncate text-zinc-400">{r.name}</span>
                </div>
              ))}
            </div>
          </div>
        )}

        <DialogFooter className="gap-2">
          <Button
            id="import-parse-btn"
            variant="outline"
            size="sm"
            onClick={handleParse}
            disabled={loading}
            className="gap-1.5"
          >
            {loading ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : null}
            Parse
          </Button>

          {preview && (
            <Button
              id="import-save-btn"
              variant="default"
              size="sm"
              onClick={handleSaveAll}
              disabled={loading || !workspacePath}
              className="gap-1.5"
            >
              {loading ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Upload className="w-3.5 h-3.5" />}
              Save {preview.count} Request{preview.count !== 1 ? 's' : ''} to Workspace
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
