import { useState, useMemo } from 'react'
import { Code2, Copy, Check } from 'lucide-react'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from '../ui/dialog'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '../ui/tabs'
import { CODE_GENERATORS } from '../../lib/codegen'
import type { RequestDefinition } from '../../types'

interface Props {
  request: RequestDefinition
}

const LANG_COLORS: Record<string, string> = {
  curl:   'text-amber-400',
  go:     'text-sky-400',
  node:   'text-emerald-400',
  python:  'text-violet-400',
  csharp: 'text-rose-400',
}

export function CodeGeneratorDialog({ request }: Props) {
  const [open, setOpen] = useState(false)
  const [activeTab, setActiveTab] = useState<string>('curl')
  const [copied, setCopied] = useState(false)

  const code = useMemo(() => {
    const gen = CODE_GENERATORS[activeTab]
    if (!gen) return ''
    try {
      return gen.fn(request)
    } catch {
      return '// Error generating code'
    }
  }, [activeTab, request])

  const handleCopy = async () => {
    await navigator.clipboard.writeText(code)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger
        id="codegen-trigger"
        title="Generate Code Snippet"
        render={
          <button className="inline-flex items-center justify-center gap-1.5 rounded-md h-7 px-2 text-xs font-medium text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800/60 transition-colors cursor-pointer" />
        }
      >
        <Code2 className="w-3.5 h-3.5" />
        <span className="hidden sm:inline">Code</span>
      </DialogTrigger>

      {/*
        No overflow-auto on DialogContent — header + tabs must stay visible.
        Only the pre block scrolls with a fixed height.
      */}
      <DialogContent className="max-w-2xl w-full">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 text-sm">
            <Code2 className="w-4 h-4 text-violet-400" />
            Code Generator
            <span className="ml-1 text-xs font-normal text-zinc-400 truncate">
              — {request.method} {request.url || '(no URL)'}
            </span>
          </DialogTitle>
        </DialogHeader>

        <Tabs value={activeTab} onValueChange={setActiveTab} className="mt-2">
          {/* Language tabs — always visible */}
          <TabsList className="w-full grid grid-cols-5">
            {Object.entries(CODE_GENERATORS).map(([key, gen]) => (
              <TabsTrigger key={key} value={key} className="text-xs gap-1">
                <span className={`font-semibold ${LANG_COLORS[key] ?? ''}`}>
                  {gen.label}
                </span>
              </TabsTrigger>
            ))}
          </TabsList>

          {Object.keys(CODE_GENERATORS).map((key) => (
            <TabsContent key={key} value={key} className="mt-3">
              <div className="rounded-lg border border-zinc-800 bg-zinc-950 overflow-hidden">
                {/* Toolbar row */}
                <div className="flex items-center justify-between px-3 py-1.5 border-b border-zinc-800 bg-zinc-900">
                  <span className={`text-xs font-mono font-semibold ${LANG_COLORS[key] ?? 'text-zinc-400'}`}>
                    {CODE_GENERATORS[key]?.label}
                  </span>
                  <button
                    id={`codegen-copy-${key}`}
                    onClick={handleCopy}
                    className="inline-flex items-center gap-1 h-6 px-2 text-xs rounded text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800 transition-colors cursor-pointer"
                  >
                    {copied ? (
                      <><Check className="w-3 h-3 text-emerald-400" /> Copied!</>
                    ) : (
                      <><Copy className="w-3 h-3" /> Copy</>
                    )}
                  </button>
                </div>

                {/*
                  Fixed height code area — h-56 (224px) keeps dialog within
                  Wails window. overflow-y-auto for internal scrolling only.
                */}
                <pre className="h-56 text-xs font-mono text-zinc-200 p-4 overflow-auto whitespace-pre leading-relaxed">
                  <code>{code}</code>
                </pre>
              </div>
            </TabsContent>
          ))}
        </Tabs>
      </DialogContent>
    </Dialog>
  )
}
