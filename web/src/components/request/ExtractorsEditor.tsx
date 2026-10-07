import {
  Plus,
  Trash2,
  Variable,
  HelpCircle,
  AlertTriangle,
  CheckCircle2,
} from 'lucide-react'
import type { ExtractorDefinition, ExecutionResult } from '../../types'
import { Button } from '../ui/button'
import { Badge } from '../ui/badge'
import { Checkbox } from '../ui/checkbox'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '../ui/table'
import { Tooltip } from '../ui/tooltip'

interface ExtractorsEditorProps {
  extractors: ExtractorDefinition[]
  lastResult?: ExecutionResult | null
  onChange: (extractors: ExtractorDefinition[]) => void
}

export function ExtractorsEditor({
  extractors = [],
  lastResult,
  onChange,
}: ExtractorsEditorProps) {
  const handleAddExtractor = () => {
    const newExt: ExtractorDefinition = {
      id: `ext_${Date.now()}_${Math.random().toString(36).slice(2, 6)}`,
      name: `var_${extractors.length + 1}`,
      type: 'jsonpath',
      path: '$.id',
      scope: 'runtime',
      enabled: true,
    }
    onChange([...extractors, newExt])
  }

  const handleUpdate = (
    index: number,
    field: keyof ExtractorDefinition,
    value: unknown
  ) => {
    const updated = extractors.map((ext, idx) => {
      if (idx !== index) return ext
      return { ...ext, [field]: value }
    })
    onChange(updated)
  }

  const handleDelete = (index: number) => {
    onChange(extractors.filter((_, idx) => idx !== index))
  }

  return (
    <div className="space-y-3">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <div className="flex items-center gap-1.5">
            <h4 className="text-xs font-semibold text-zinc-800 dark:text-zinc-200">
              Response Variable Extractors
            </h4>
            <Tooltip content="Extractors evaluate JSONPath against response bodies, setting variables before assertion scripts run.">
              <HelpCircle className="w-3.5 h-3.5 text-zinc-400 cursor-help" />
            </Tooltip>
          </div>
          <p className="text-[11px] text-zinc-500 dark:text-zinc-400">
            Chain requests declaratively without writing scripts.
          </p>
        </div>

        <Button
          size="sm"
          variant="outline"
          onClick={handleAddExtractor}
          className="h-7 text-xs gap-1 px-2 border-blue-500/30 text-blue-600 dark:text-blue-400 hover:bg-blue-50 dark:hover:bg-blue-950/40"
        >
          <Plus className="w-3.5 h-3.5" />
          Add Extractor
        </Button>
      </div>

      {/* Table or Empty State */}
      {extractors.length === 0 ? (
        <div className="rounded-lg border border-dashed border-zinc-300 dark:border-zinc-800 p-8 flex flex-col items-center justify-center text-center bg-zinc-50/50 dark:bg-zinc-900/20">
          <div className="w-10 h-10 rounded-full bg-blue-50 dark:bg-blue-950/40 border border-blue-200 dark:border-blue-800/60 flex items-center justify-center text-blue-600 dark:text-blue-400 mb-2">
            <Variable className="w-5 h-5" />
          </div>
          <p className="text-xs font-medium text-zinc-800 dark:text-zinc-200">
            No Extractors Configured
          </p>
          <p className="text-[11px] text-zinc-500 dark:text-zinc-400 max-w-sm mt-1">
            Extract variables from JSON responses and use them in subsequent requests via{' '}
            <code className="font-mono text-zinc-700 dark:text-zinc-300">{'{{VAR}}'}</code>.
          </p>
          <div className="flex items-center gap-2 mt-4">
            <Button
              size="sm"
              variant="outline"
              onClick={handleAddExtractor}
              className="h-7 text-xs gap-1 bg-white dark:bg-zinc-900"
            >
              <Plus className="w-3.5 h-3.5" /> Add First Extractor
            </Button>
          </div>
          <p className="text-[10px] text-zinc-400 dark:text-zinc-500 mt-3">
            Tip: You can also click any node in the response Tree viewer and choose &quot;Set as variable&quot;.
          </p>
        </div>
      ) : (
        <div className="border border-zinc-200 dark:border-zinc-800 rounded-md overflow-hidden bg-white dark:bg-zinc-950">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-8 text-center"></TableHead>
                <TableHead className="w-1/4">Variable Name</TableHead>
                <TableHead className="w-1/3">JSONPath Expression</TableHead>
                <TableHead className="w-28">Scope</TableHead>
                <TableHead>Last Execution Result</TableHead>
                <TableHead className="w-8"></TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {extractors.map((ext, idx) => {
                const extractedVal = lastResult?.extractedEnvVars?.[ext.name]
                const warningMsg = lastResult?.consoleLogs?.find(
                  (l) => l.source === 'extractor' && l.message.includes(`"${ext.name}"`)
                )?.message

                return (
                  <TableRow key={ext.id || idx}>
                    {/* Toggle Enabled */}
                    <TableCell className="text-center p-2">
                      <Checkbox
                        checked={ext.enabled}
                        onCheckedChange={(checked) =>
                          handleUpdate(idx, 'enabled', !!checked)
                        }
                      />
                    </TableCell>

                    {/* Variable Name */}
                    <TableCell className="p-1">
                      <input
                        type="text"
                        value={ext.name}
                        onChange={(e) =>
                          handleUpdate(
                            idx,
                            'name',
                            e.target.value.replace(/[^a-zA-Z0-9_]/g, '_')
                          )
                        }
                        placeholder="VAR_NAME"
                        className="w-full h-7 px-2 font-mono text-xs rounded bg-transparent border border-transparent hover:border-zinc-200 dark:hover:border-zinc-800 focus:border-blue-500 focus:bg-white dark:focus:bg-zinc-900 focus:outline-none"
                      />
                    </TableCell>

                    {/* JSONPath */}
                    <TableCell className="p-1">
                      <input
                        type="text"
                        value={ext.path}
                        onChange={(e) => handleUpdate(idx, 'path', e.target.value)}
                        placeholder="$.data.id"
                        className="w-full h-7 px-2 font-mono text-xs text-blue-600 dark:text-blue-400 rounded bg-transparent border border-transparent hover:border-zinc-200 dark:hover:border-zinc-800 focus:border-blue-500 focus:bg-white dark:focus:bg-zinc-900 focus:outline-none"
                      />
                    </TableCell>

                    {/* Scope Selector */}
                    <TableCell className="p-1">
                      <select
                        value={ext.scope || 'runtime'}
                        onChange={(e) => handleUpdate(idx, 'scope', e.target.value)}
                        className="w-full h-7 px-1.5 text-xs rounded border border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-900 text-zinc-800 dark:text-zinc-200 focus:outline-none focus:ring-1 focus:ring-blue-500"
                      >
                        <option value="runtime">Runtime</option>
                        <option value="environment">Environment</option>
                        <option value="folder">Folder</option>
                      </select>
                    </TableCell>

                    {/* Live Extracted Status */}
                    <TableCell className="p-2 text-xs">
                      {!ext.enabled ? (
                        <Badge variant="outline" className="text-[10px] text-zinc-400">
                          Disabled
                        </Badge>
                      ) : extractedVal !== undefined ? (
                        <div className="flex items-center gap-1.5 truncate max-w-xs">
                          <CheckCircle2 className="w-3.5 h-3.5 text-emerald-500 shrink-0" />
                          <span className="font-mono text-[11px] text-zinc-700 dark:text-zinc-300 truncate">
                            {extractedVal}
                          </span>
                        </div>
                      ) : warningMsg ? (
                        <div className="flex items-center gap-1.5 text-amber-600 dark:text-amber-400 text-[11px]">
                          <AlertTriangle className="w-3.5 h-3.5 shrink-0" />
                          <span className="truncate" title={warningMsg}>
                            Path not found
                          </span>
                        </div>
                      ) : (
                        <span className="text-zinc-400 italic text-[11px]">
                          Runs on response
                        </span>
                      )}
                    </TableCell>

                    {/* Delete */}
                    <TableCell className="p-1 text-center">
                      <button
                        type="button"
                        onClick={() => handleDelete(idx)}
                        className="p-1 text-zinc-400 hover:text-rose-500 transition-colors"
                        title="Delete extractor"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  )
}
