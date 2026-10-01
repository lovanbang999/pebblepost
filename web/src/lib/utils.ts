import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

export function formatBytes(bytes: number, decimals = 2) {
  if (bytes === 0) return '0 Bytes'
  const k = 1024
  const dm = decimals < 0 ? 0 : decimals
  const sizes = ['Bytes', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i]
}

export function formatDuration(ms: number) {
  if (ms < 1000) return `${Math.round(ms)} ms`
  return `${(ms / 1000).toFixed(2)} s`
}

export function getMethodColor(method: string) {
  switch (method.toUpperCase()) {
    case 'GET':
      return 'text-emerald-400 bg-emerald-950/60 border-emerald-800/60'
    case 'POST':
      return 'text-blue-400 bg-blue-950/60 border-blue-800/60'
    case 'PUT':
      return 'text-amber-400 bg-amber-950/60 border-amber-800/60'
    case 'PATCH':
      return 'text-violet-400 bg-violet-950/60 border-violet-800/60'
    case 'DELETE':
      return 'text-rose-400 bg-rose-950/60 border-rose-800/60'
    default:
      return 'text-zinc-400 bg-zinc-900 border-zinc-700'
  }
}
