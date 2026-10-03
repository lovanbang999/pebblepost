/**
 * HexViewer – renders the first N bytes of a binary body as a formatted hex dump.
 */
import { useMemo } from 'react'

interface HexViewerProps {
  data: string // raw body (may be binary, first 1 KB will be shown)
  maxBytes?: number
}

function toHexRow(offset: number, row: Uint8Array): string {
  const hex = Array.from(row)
    .map((b) => b.toString(16).padStart(2, '0'))
    .join(' ')
    .padEnd(47, ' ')

  const ascii = Array.from(row)
    .map((b) => (b >= 0x20 && b < 0x7f ? String.fromCharCode(b) : '.'))
    .join('')

  return `${offset.toString(16).padStart(8, '0')}  ${hex}  |${ascii}|`
}

export function HexViewer({ data, maxBytes = 1024 }: HexViewerProps) {
  const lines = useMemo(() => {
    const encoder = new TextEncoder()
    const bytes = encoder.encode(data).slice(0, maxBytes)
    const rows: string[] = []
    for (let i = 0; i < bytes.length; i += 16) {
      rows.push(toHexRow(i, bytes.slice(i, i + 16)))
    }
    return rows
  }, [data, maxBytes])

  return (
    <div className="font-mono text-[11px] leading-relaxed text-zinc-300 bg-zinc-950 p-3 rounded-md overflow-x-auto border border-zinc-800">
      <div className="text-zinc-500 mb-2 text-[10px] select-none">
        Offset    00 01 02 03 04 05 06 07 08 09 0a 0b 0c 0d 0e 0f  |ASCII|
      </div>
      {lines.map((line, i) => (
        <div key={i} className="whitespace-pre hover:bg-zinc-900 px-1 rounded">
          {line}
        </div>
      ))}
      {data.length > maxBytes && (
        <div className="text-zinc-500 mt-2 text-[10px]">
          … showing first {maxBytes} bytes of {data.length} total
        </div>
      )}
    </div>
  )
}
