import { useState } from 'react'
import { FileText, Eye, Edit3, Columns, Sparkles } from 'lucide-react'
import { Button } from '../ui/button'
import { Badge } from '../ui/badge'
import { MarkdownView } from '../common/MarkdownView'

interface RequestDocsTabProps {
  description: string | undefined
  onChange: (description: string) => void
}

export function RequestDocsTab({ description = '', onChange }: RequestDocsTabProps) {
  const [viewMode, setViewMode] = useState<'split' | 'edit' | 'preview'>('split')

  return (
    <div className="flex-1 flex flex-col h-full overflow-hidden bg-white dark:bg-zinc-950">
      {/* Top Header / Mode Switcher */}
      <div className="p-3 border-b border-zinc-200 dark:border-zinc-800 flex items-center justify-between bg-zinc-50/50 dark:bg-zinc-900/30">
        <div className="flex items-center gap-2">
          <FileText className="w-4 h-4 text-blue-500" />
          <span className="font-semibold text-xs text-zinc-800 dark:text-zinc-200">
            Request Description & Documentation
          </span>
          <Badge variant="outline" className="text-[10px] font-mono text-zinc-500">
            Markdown
          </Badge>
        </div>

        <div className="flex items-center gap-1 bg-zinc-100 dark:bg-zinc-800/80 p-0.5 rounded-lg border border-zinc-200/80 dark:border-zinc-700/60">
          <Button
            type="button"
            size="sm"
            variant="ghost"
            onClick={() => setViewMode('edit')}
            className={`h-6 px-2 text-[11px] gap-1 cursor-pointer ${
              viewMode === 'edit'
                ? 'bg-white dark:bg-zinc-900 text-blue-600 dark:text-blue-400 shadow-xs'
                : 'text-zinc-600 dark:text-zinc-400'
            }`}
          >
            <Edit3 className="w-3 h-3" />
            Edit
          </Button>
          <Button
            type="button"
            size="sm"
            variant="ghost"
            onClick={() => setViewMode('split')}
            className={`h-6 px-2 text-[11px] gap-1 cursor-pointer ${
              viewMode === 'split'
                ? 'bg-white dark:bg-zinc-900 text-blue-600 dark:text-blue-400 shadow-xs'
                : 'text-zinc-600 dark:text-zinc-400'
            }`}
          >
            <Columns className="w-3 h-3" />
            Split
          </Button>
          <Button
            type="button"
            size="sm"
            variant="ghost"
            onClick={() => setViewMode('preview')}
            className={`h-6 px-2 text-[11px] gap-1 cursor-pointer ${
              viewMode === 'preview'
                ? 'bg-white dark:bg-zinc-900 text-blue-600 dark:text-blue-400 shadow-xs'
                : 'text-zinc-600 dark:text-zinc-400'
            }`}
          >
            <Eye className="w-3 h-3" />
            Preview
          </Button>
        </div>
      </div>

      {/* Info notice */}
      <div className="px-3 py-1.5 bg-blue-50/40 dark:bg-blue-950/20 border-b border-blue-200/50 dark:border-blue-900/30 text-[11px] text-blue-800 dark:text-blue-300 flex items-center justify-between">
        <div className="flex items-center gap-1.5">
          <Sparkles className="w-3.5 h-3.5 text-blue-500 shrink-0" />
          <span>
            This description is automatically included when generating API documentation (Markdown/HTML) and OpenAPI 3.0 specifications.
          </span>
        </div>
      </div>

      {/* Editor / Preview Content */}
      <div className="flex-1 flex overflow-hidden">
        {/* Editor Pane */}
        {(viewMode === 'edit' || viewMode === 'split') && (
          <div
            className={`flex flex-col h-full overflow-hidden ${
              viewMode === 'split' ? 'w-1/2 border-r border-zinc-200 dark:border-zinc-800' : 'w-full'
            }`}
          >
            <textarea
              value={description}
              onChange={(e) => onChange(e.target.value)}
              placeholder="Write detailed endpoint documentation in Markdown...&#10;&#10;### Overview&#10;Explain what this endpoint does and key business logic.&#10;&#10;### Parameters & Headers&#10;Describe query parameters or special authorization requirements.&#10;&#10;### Error Responses&#10;Document common failure modes and status codes."
              className="flex-1 w-full p-4 bg-white dark:bg-zinc-950 text-xs font-mono text-zinc-900 dark:text-zinc-100 placeholder:text-zinc-400 dark:placeholder:text-zinc-600 resize-none outline-none focus:ring-0 leading-relaxed"
            />
          </div>
        )}

        {/* Preview Pane */}
        {(viewMode === 'preview' || viewMode === 'split') && (
          <div
            className={`flex-1 h-full overflow-y-auto p-4 bg-zinc-50/50 dark:bg-zinc-900/20 ${
              viewMode === 'split' ? 'w-1/2' : 'w-full'
            }`}
          >
            <MarkdownView
              content={description}
              emptyMessage="No description written yet. Type in the editor to see live formatted Markdown."
            />
          </div>
        )}
      </div>
    </div>
  )
}
