import { useState } from 'react'
import { Copy, Check } from 'lucide-react'

interface MarkdownViewProps {
  content?: string
  className?: string
  emptyMessage?: string
}

type BlockType = 'h1' | 'h2' | 'h3' | 'h4' | 'p' | 'ul' | 'ol' | 'code' | 'quote' | 'table'

interface Block {
  type: BlockType
  lines: string[]
  lang?: string
}

interface InlineMatch {
  type: 'code' | 'bold' | 'link'
  index: number
  length: number
  val?: string
  text?: string
  href?: string
}

export function MarkdownView({ content, className = '', emptyMessage = 'No description provided.' }: MarkdownViewProps) {
  const [copiedCodeIdx, setCopiedCodeIdx] = useState<number | null>(null)

  if (!content || !content.trim()) {
    return (
      <div className={`text-xs text-zinc-400 dark:text-zinc-500 italic p-3 ${className}`}>
        {emptyMessage}
      </div>
    )
  }

  // Parse lines into blocks
  const lines = content.split('\n')
  const blocks: Block[] = []

  let currentBlockType: BlockType | null = null
  let currentLines: string[] = []
  let codeLang = ''

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]

    // Code block toggle
    if (line.trim().startsWith('```')) {
      if (currentBlockType === 'code') {
        blocks.push({ type: 'code', lines: currentLines, lang: codeLang })
        currentBlockType = null
        currentLines = []
        codeLang = ''
        continue
      } else {
        if (currentLines.length > 0 && currentBlockType) {
          blocks.push({ type: currentBlockType, lines: currentLines })
          currentLines = []
        }
        currentBlockType = 'code'
        codeLang = line.trim().slice(3).trim()
        continue
      }
    }

    if (currentBlockType === 'code') {
      currentLines.push(line)
      continue
    }

    // Headings
    if (line.startsWith('# ') || line.startsWith('## ') || line.startsWith('### ') || line.startsWith('#### ')) {
      if (currentLines.length > 0 && currentBlockType) {
        blocks.push({ type: currentBlockType, lines: currentLines })
        currentLines = []
      }
      const type: BlockType = line.startsWith('#### ') ? 'h4' : line.startsWith('### ') ? 'h3' : line.startsWith('## ') ? 'h2' : 'h1'
      blocks.push({ type, lines: [line] })
      currentBlockType = null
      continue
    }

    // Blockquote
    if (line.startsWith('>')) {
      if (currentBlockType !== 'quote') {
        if (currentLines.length > 0 && currentBlockType) {
          blocks.push({ type: currentBlockType, lines: currentLines })
          currentLines = []
        }
        currentBlockType = 'quote'
      }
      currentLines.push(line.replace(/^>\s?/, ''))
      continue
    }

    // Table
    if (line.trim().startsWith('|')) {
      if (currentBlockType !== 'table') {
        if (currentLines.length > 0 && currentBlockType) {
          blocks.push({ type: currentBlockType, lines: currentLines })
          currentLines = []
        }
        currentBlockType = 'table'
      }
      currentLines.push(line)
      continue
    }

    // List item
    if (line.match(/^\s*[-*]\s+/)) {
      if (currentBlockType !== 'ul') {
        if (currentLines.length > 0 && currentBlockType) {
          blocks.push({ type: currentBlockType, lines: currentLines })
          currentLines = []
        }
        currentBlockType = 'ul'
      }
      currentLines.push(line)
      continue
    }

    if (line.match(/^\s*\d+\.\s+/)) {
      if (currentBlockType !== 'ol') {
        if (currentLines.length > 0 && currentBlockType) {
          blocks.push({ type: currentBlockType, lines: currentLines })
          currentLines = []
        }
        currentBlockType = 'ol'
      }
      currentLines.push(line)
      continue
    }

    // Blank line
    if (!line.trim()) {
      if (currentLines.length > 0 && currentBlockType) {
        blocks.push({ type: currentBlockType, lines: currentLines })
        currentLines = []
        currentBlockType = null
      }
      continue
    }

    // Regular paragraph line
    if (currentBlockType !== 'p') {
      if (currentLines.length > 0 && currentBlockType) {
        blocks.push({ type: currentBlockType, lines: currentLines })
        currentLines = []
      }
      currentBlockType = 'p'
    }
    currentLines.push(line)
  }

  if (currentLines.length > 0 && currentBlockType) {
    blocks.push({ type: currentBlockType, lines: currentLines, lang: codeLang })
  }

  // Inline formatting helper
  const renderInline = (text: string) => {
    // Regex for bold, italic, inline code, links
    const parts: React.ReactNode[] = []
    let remaining = text
    let keyIdx = 0

    while (remaining.length > 0) {
      // Inline code: `code`
      const codeMatch = remaining.match(/`([^`]+)`/)
      // Bold: **text**
      const boldMatch = remaining.match(/\*\*([^*]+)\*\*/)
      // Link: [text](url)
      const linkMatch = remaining.match(/\[([^\]]+)\]\(([^)]+)\)/)

      // Find which match comes first
      const rawMatches: (InlineMatch | null)[] = [
        codeMatch && codeMatch.index !== undefined ? { type: 'code', index: codeMatch.index, length: codeMatch[0].length, val: codeMatch[1] } : null,
        boldMatch && boldMatch.index !== undefined ? { type: 'bold', index: boldMatch.index, length: boldMatch[0].length, val: boldMatch[1] } : null,
        linkMatch && linkMatch.index !== undefined ? { type: 'link', index: linkMatch.index, length: linkMatch[0].length, text: linkMatch[1], href: linkMatch[2] } : null,
      ]
      const matches = rawMatches.filter((m): m is InlineMatch => m !== null).sort((a, b) => a.index - b.index)

      if (matches.length === 0) {
        parts.push(remaining)
        break
      }

      const first = matches[0]!
      if (first.index > 0) {
        parts.push(remaining.slice(0, first.index))
      }

      if (first.type === 'code') {
        parts.push(
          <code key={keyIdx++} className="px-1.5 py-0.5 rounded bg-zinc-100 dark:bg-zinc-800 text-pink-600 dark:text-pink-400 font-mono text-[11px] border border-zinc-200 dark:border-zinc-700">
            {first.val}
          </code>
        )
      } else if (first.type === 'bold') {
        parts.push(
          <strong key={keyIdx++} className="font-semibold text-zinc-900 dark:text-zinc-100">
            {first.val}
          </strong>
        )
      } else if (first.type === 'link') {
        const trimmedHref = first.href?.trim() ?? ''
        const isSafeProtocol = /^(https?:|mailto:|#)/i.test(trimmedHref)
        const safeHref = isSafeProtocol ? trimmedHref : '#'
        parts.push(
          <a
            key={keyIdx++}
            href={safeHref}
            target={safeHref.startsWith('#') ? undefined : '_blank'}
            rel={safeHref.startsWith('#') ? undefined : 'noopener noreferrer'}
            className="text-blue-600 dark:text-blue-400 hover:underline font-medium"
          >
            {first.text}
          </a>
        )
      }

      remaining = remaining.slice(first.index + first.length)
    }

    return parts
  }

  const handleCopyCode = (code: string, idx: number) => {
    navigator.clipboard.writeText(code)
    setCopiedCodeIdx(idx)
    setTimeout(() => setCopiedCodeIdx(null), 2000)
  }

  return (
    <div className={`space-y-3 text-xs text-zinc-700 dark:text-zinc-300 leading-relaxed ${className}`}>
      {blocks.map((block, bIdx) => {
        if (block.type === 'h1') {
          return (
            <h1 key={bIdx} className="text-xl font-bold text-zinc-900 dark:text-zinc-100 pb-1.5 border-b border-zinc-200 dark:border-zinc-800 pt-2">
              {renderInline(block.lines[0].replace(/^#\s+/, ''))}
            </h1>
          )
        }
        if (block.type === 'h2') {
          return (
            <h2 key={bIdx} className="text-base font-bold text-zinc-900 dark:text-zinc-100 pb-1 border-b border-zinc-200 dark:border-zinc-800 pt-1.5">
              {renderInline(block.lines[0].replace(/^##\s+/, ''))}
            </h2>
          )
        }
        if (block.type === 'h3') {
          return (
            <h3 key={bIdx} className="text-sm font-semibold text-zinc-900 dark:text-zinc-100 pt-1">
              {renderInline(block.lines[0].replace(/^###\s+/, ''))}
            </h3>
          )
        }
        if (block.type === 'h4') {
          return (
            <h4 key={bIdx} className="text-xs font-semibold text-zinc-900 dark:text-zinc-100 pt-0.5">
              {renderInline(block.lines[0].replace(/^####\s+/, ''))}
            </h4>
          )
        }
        if (block.type === 'quote') {
          return (
            <blockquote key={bIdx} className="border-l-2 border-blue-500 pl-3 py-1 text-zinc-600 dark:text-zinc-400 italic bg-blue-50/30 dark:bg-blue-950/20 rounded-r">
              {block.lines.map((l, lIdx) => (
                <p key={lIdx}>{renderInline(l)}</p>
              ))}
            </blockquote>
          )
        }
        if (block.type === 'ul') {
          return (
            <ul key={bIdx} className="list-disc pl-5 space-y-1">
              {block.lines.map((l, lIdx) => (
                <li key={lIdx}>
                  {renderInline(l.replace(/^\s*[-*]\s+/, ''))}
                </li>
              ))}
            </ul>
          )
        }
        if (block.type === 'ol') {
          return (
            <ol key={bIdx} className="list-decimal pl-5 space-y-1">
              {block.lines.map((l, lIdx) => (
                <li key={lIdx}>
                  {renderInline(l.replace(/^\s*\d+\.\s+/, ''))}
                </li>
              ))}
            </ol>
          )
        }
        if (block.type === 'code') {
          const codeString = block.lines.join('\n')
          return (
            <div key={bIdx} className="relative rounded-md border border-zinc-200 dark:border-zinc-800 bg-zinc-900 dark:bg-zinc-950 overflow-hidden my-2">
              <div className="flex items-center justify-between px-3 py-1 bg-zinc-800/80 border-b border-zinc-700/50 text-[10px] text-zinc-400 font-mono">
                <span>{block.lang || 'code'}</span>
                <button
                  type="button"
                  onClick={() => handleCopyCode(codeString, bIdx)}
                  className="flex items-center gap-1 hover:text-zinc-200 cursor-pointer"
                >
                  {copiedCodeIdx === bIdx ? (
                    <>
                      <Check className="w-3 h-3 text-emerald-400" />
                      <span className="text-emerald-400">Copied</span>
                    </>
                  ) : (
                    <>
                      <Copy className="w-3 h-3" />
                      <span>Copy</span>
                    </>
                  )}
                </button>
              </div>
              <pre className="p-3 font-mono text-[11px] text-zinc-200 overflow-x-auto whitespace-pre">
                {codeString}
              </pre>
            </div>
          )
        }
        if (block.type === 'table') {
          const rawRows = block.lines.filter((l) => !l.includes('---'))
          const rows = rawRows.map((r) =>
            r
              .split('|')
              .map((c) => c.trim())
              .filter((_, i, arr) => i > 0 && i < arr.length - 1)
          )
          const headers = rows[0] || []
          const dataRows = rows.slice(1)

          return (
            <div key={bIdx} className="border border-zinc-200 dark:border-zinc-800 rounded-md overflow-hidden my-2">
              <table className="w-full text-left text-[11px]">
                <thead className="bg-zinc-100 dark:bg-zinc-800/50 text-zinc-700 dark:text-zinc-300 font-semibold border-b border-zinc-200 dark:border-zinc-800">
                  <tr>
                    {headers.map((h, hIdx) => (
                      <th key={hIdx} className="px-2.5 py-1.5">{renderInline(h)}</th>
                    ))}
                  </tr>
                </thead>
                <tbody className="divide-y divide-zinc-200 dark:divide-zinc-800">
                  {dataRows.map((row, rIdx) => (
                    <tr key={rIdx} className="hover:bg-zinc-50 dark:hover:bg-zinc-900/50">
                      {row.map((cell, cIdx) => (
                        <td key={cIdx} className="px-2.5 py-1.5">{renderInline(cell)}</td>
                      ))}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )
        }
        return (
          <p key={bIdx} className="leading-relaxed">
            {renderInline(block.lines.join(' '))}
          </p>
        )
      })}
    </div>
  )
}
