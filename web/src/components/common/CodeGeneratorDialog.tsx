import { useState, useMemo } from 'react'
import { Code2, Copy, Check } from 'lucide-react'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from '../ui/dialog'
import { Button } from '../ui/button'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '../ui/tabs'
import { CODE_GENERATORS } from '../../lib/codegen'
import type { RequestDefinition } from '../../types'

interface Props {
  request: RequestDefinition
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
      <DialogTrigger asChild>
        <Button
          id="codegen-trigger"
          variant="ghost"
          size="sm"
          className="h-7 gap-1.5 text-xs text-zinc-400 hover:text-zinc-200 px-2"
          title="Generate Code Snippet"
        >
          <Code2 className="w-3.5 h-3.5" />
          <span className="hidden sm:inline">Code</span>
        </Button>
      </DialogTrigger>

      <DialogContent className="max-w-2xl w-full">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 text-sm">
            <Code2 className="w-4 h-4 text-violet-400" />
            Code Generator
            <span className="ml-1 text-xs font-normal text-zinc-400">
              — {request.method} {request.url || '(no URL)'}
            </span>
          </DialogTitle>
        </DialogHeader>

        <Tabs value={activeTab} onValueChange={setActiveTab} className="mt-2">
          <TabsList className="w-full grid grid-cols-5">
            {Object.entries(CODE_GENERATORS).map(([key, gen]) => (
              <TabsTrigger key={key} value={key} className="text-xs">
                {gen.label}
              </TabsTrigger>
            ))}
          </TabsList>

          {Object.keys(CODE_GENERATORS).map((key) => (
            <TabsContent key={key} value={key}>
              <div className="relative mt-3 rounded-lg border border-zinc-800 bg-zinc-950 overflow-hidden">
                {/* Copy button */}
                <div className="absolute top-2 right-2 z-10">
                  <Button
                    id={`codegen-copy-${key}`}
                    variant="ghost"
                    size="sm"
                    onClick={handleCopy}
                    className="h-7 gap-1 text-xs text-zinc-400 hover:text-zinc-200 bg-zinc-900 hover:bg-zinc-800 border border-zinc-700"
                  >
                    {copied ? (
                      <><Check className="w-3 h-3 text-emerald-400" /> Copied!</>
                    ) : (
                      <><Copy className="w-3 h-3" /> Copy</>
                    )}
                  </Button>
                </div>

                {/* Code block */}
                <pre className="text-xs font-mono text-zinc-200 p-4 pr-20 overflow-x-auto whitespace-pre leading-relaxed max-h-96">
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
