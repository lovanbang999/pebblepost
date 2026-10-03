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
      return 'text-emerald-700 dark:text-emerald-400 bg-emerald-50 dark:bg-emerald-950/60 border-emerald-300 dark:border-emerald-800/60'
    case 'POST':
      return 'text-blue-700 dark:text-blue-400 bg-blue-50 dark:bg-blue-950/60 border-blue-300 dark:border-blue-800/60'
    case 'PUT':
      return 'text-amber-700 dark:text-amber-400 bg-amber-50 dark:bg-amber-950/60 border-amber-300 dark:border-amber-800/60'
    case 'PATCH':
      return 'text-violet-700 dark:text-violet-400 bg-violet-50 dark:bg-violet-950/60 border-violet-300 dark:border-violet-800/60'
    case 'DELETE':
      return 'text-rose-700 dark:text-rose-400 bg-rose-50 dark:bg-rose-950/60 border-rose-300 dark:border-rose-800/60'
    case 'GRPC':
      return 'text-indigo-700 dark:text-indigo-400 bg-indigo-50 dark:bg-indigo-950/60 border-indigo-300 dark:border-indigo-800/60'
    default:
      return 'text-zinc-700 dark:text-zinc-400 bg-zinc-100 dark:bg-zinc-900 border-zinc-300 dark:border-zinc-700'
  }
}

export function getMethodTextColor(method: string) {
  switch (method.toUpperCase()) {
    case 'GET':
      return 'text-emerald-600 dark:text-emerald-400'
    case 'POST':
      return 'text-blue-600 dark:text-blue-400'
    case 'PUT':
      return 'text-amber-600 dark:text-amber-400'
    case 'PATCH':
      return 'text-violet-600 dark:text-violet-400'
    case 'DELETE':
      return 'text-rose-600 dark:text-rose-400'
    case 'HEAD':
    case 'OPTIONS':
      return 'text-teal-600 dark:text-teal-400'
    case 'GRPC':
      return 'text-indigo-600 dark:text-indigo-400'
    default:
      return 'text-zinc-600 dark:text-zinc-400'
  }
}

