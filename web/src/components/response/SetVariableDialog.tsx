import React, { useState, useEffect } from 'react'
import { Sparkles, Copy, Check, Variable, ShieldAlert } from 'lucide-react'
import { Button } from '../ui/button'

interface SetVariableDialogProps {
  open: boolean
  onClose: () => void
  jsonPath: string
  initialValue: string
  suggestedName: string
  onSave: (name: string, scope: 'runtime' | 'environment' | 'folder', path: string) => void
}

export function SetVariableDialog({
  open,
  onClose,
  jsonPath,
  initialValue,
  suggestedName,
  onSave,
}: SetVariableDialogProps) {
  const [varName, setVarName] = useState(suggestedName)
  const [scope, setScope] = useState<'runtime' | 'environment' | 'folder'>('runtime')
  const [copied, setCopied] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (open) {
      setVarName(suggestedName)
      setScope('runtime')
      setError('')
      setCopied(false)
    }
  }, [open, suggestedName])

  if (!open) return null

  const handleCopyPath = () => {
    navigator.clipboard.writeText(jsonPath)
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    const clean = varName.trim().replace(/[^a-zA-Z0-9_]/g, '_')
    if (!clean) {
      setError('Variable name cannot be empty')
      return
    }
    onSave(clean, scope, jsonPath)
    onClose()
  }

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="set-variable-dialog-title"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-xs p-4 animate-in fade-in duration-150"
    >
      <div className="bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 rounded-xl shadow-2xl w-full max-w-md overflow-hidden text-zinc-900 dark:text-zinc-100 animate-in zoom-in-95 duration-150">
        {/* Header */}
        <div className="px-5 py-4 border-b border-zinc-100 dark:border-zinc-800 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <div className="w-8 h-8 rounded-lg bg-blue-50 dark:bg-blue-950/40 border border-blue-200 dark:border-blue-800/60 flex items-center justify-center text-blue-600 dark:text-blue-400">
              <Variable className="w-4 h-4" />
            </div>
            <div>
              <h3 id="set-variable-dialog-title" className="text-sm font-semibold">Set as Variable</h3>
              <p className="text-[11px] text-zinc-500 dark:text-zinc-400">Extract response value for request chaining</p>
            </div>
          </div>
          <button
            onClick={onClose}
            aria-label="Close dialog"
            className="text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-200 text-sm p-1 rounded-md"
          >
            ✕
          </button>
        </div>

        {/* Form Body */}
        <form onSubmit={handleSubmit} className="p-5 space-y-4">
          {/* Target JSONPath */}
          <div>
            <label className="text-xs font-medium text-zinc-700 dark:text-zinc-300 mb-1 flex items-center justify-between">
              <span>Target JSONPath</span>
              <button
                type="button"
                onClick={handleCopyPath}
                className="text-[10px] text-blue-600 dark:text-blue-400 hover:underline flex items-center gap-1"
              >
                {copied ? <Check className="w-3 h-3 text-emerald-500" /> : <Copy className="w-3 h-3" />}
                {copied ? 'Copied' : 'Copy'}
              </button>
            </label>
            <div className="px-3 py-2 bg-zinc-50 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 rounded-lg font-mono text-xs text-blue-600 dark:text-blue-400 truncate select-all">
              {jsonPath}
            </div>
          </div>

          {/* Current Extracted Value Preview */}
          <div>
            <label className="text-xs font-medium text-zinc-700 dark:text-zinc-300 mb-1 block">
              Value Preview
            </label>
            <div className="px-3 py-2 bg-zinc-50 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 rounded-lg font-mono text-xs text-zinc-600 dark:text-zinc-300 max-h-24 overflow-y-auto break-all">
              {initialValue || <span className="text-zinc-400 italic">(empty value)</span>}
            </div>
          </div>

          {/* Variable Name Input */}
          <div>
            <label htmlFor="varNameInput" className="text-xs font-medium text-zinc-700 dark:text-zinc-300 mb-1 block">
              Variable Name
            </label>
            <div className="relative">
              <input
                id="varNameInput"
                type="text"
                autoFocus
                value={varName}
                onChange={(e) => {
                  setVarName(e.target.value)
                  setError('')
                }}
                placeholder="e.g. authToken or userId"
                className="w-full text-xs font-mono px-3 py-2 rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 focus:outline-none focus:ring-2 focus:ring-blue-500/30 focus:border-blue-500"
              />
            </div>
            {error && (
              <p className="text-[11px] text-rose-500 mt-1 flex items-center gap-1">
                <ShieldAlert className="w-3 h-3" /> {error}
              </p>
            )}
            <p className="text-[10px] text-zinc-400 dark:text-zinc-500 mt-1">
              Referenced in subsequent requests via <span className="font-mono text-zinc-600 dark:text-zinc-300">{`{{${varName.trim() || 'VAR'}}}`}</span>
            </p>
          </div>

          {/* Scope Selector */}
          <div>
            <label className="text-xs font-medium text-zinc-700 dark:text-zinc-300 mb-1.5 block">
              Target Scope
            </label>
            <div className="grid grid-cols-3 gap-2">
              <button
                type="button"
                onClick={() => setScope('runtime')}
                className={`p-2.5 rounded-lg border text-left transition-all ${
                  scope === 'runtime'
                    ? 'border-blue-500 bg-blue-50/50 dark:bg-blue-950/30 text-blue-900 dark:text-blue-100 ring-1 ring-blue-500'
                    : 'border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 text-zinc-600 dark:text-zinc-400 hover:border-zinc-300'
                }`}
              >
                <div className="text-xs font-semibold flex items-center gap-1">
                  <Sparkles className="w-3 h-3 text-blue-500" /> Runtime
                </div>
                <div className="text-[10px] text-zinc-500 dark:text-zinc-400 mt-0.5 leading-tight">
                  Session & chained runner
                </div>
              </button>

              <button
                type="button"
                onClick={() => setScope('environment')}
                className={`p-2.5 rounded-lg border text-left transition-all ${
                  scope === 'environment'
                    ? 'border-blue-500 bg-blue-50/50 dark:bg-blue-950/30 text-blue-900 dark:text-blue-100 ring-1 ring-blue-500'
                    : 'border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 text-zinc-600 dark:text-zinc-400 hover:border-zinc-300'
                }`}
              >
                <div className="text-xs font-semibold flex items-center gap-1">
                  <Variable className="w-3 h-3 text-emerald-500" /> Environment
                </div>
                <div className="text-[10px] text-zinc-500 dark:text-zinc-400 mt-0.5 leading-tight">
                  Active environment
                </div>
              </button>

              <button
                type="button"
                onClick={() => setScope('folder')}
                className={`p-2.5 rounded-lg border text-left transition-all ${
                  scope === 'folder'
                    ? 'border-blue-500 bg-blue-50/50 dark:bg-blue-950/30 text-blue-900 dark:text-blue-100 ring-1 ring-blue-500'
                    : 'border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 text-zinc-600 dark:text-zinc-400 hover:border-zinc-300'
                }`}
              >
                <div className="text-xs font-semibold flex items-center gap-1">
                  <Variable className="w-3 h-3 text-amber-500" /> Folder
                </div>
                <div className="text-[10px] text-zinc-500 dark:text-zinc-400 mt-0.5 leading-tight">
                  Folder variables
                </div>
              </button>
            </div>
          </div>

          {/* Action buttons */}
          <div className="pt-2 flex items-center justify-end gap-2">
            <Button type="button" variant="outline" size="sm" onClick={onClose} className="h-8 text-xs">
              Cancel
            </Button>
            <Button type="submit" size="sm" className="h-8 text-xs gap-1.5 bg-blue-600 hover:bg-blue-700 text-white">
              <Sparkles className="w-3.5 h-3.5" /> Save Extractor
            </Button>
          </div>
        </form>
      </div>
    </div>
  )
}
