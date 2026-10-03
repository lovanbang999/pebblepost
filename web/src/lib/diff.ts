export interface DiffLine {
  type: 'added' | 'removed' | 'unchanged'
  text: string
  oldLineNumber?: number
  newLineNumber?: number
}

/**
 * Computes a line-by-line diff between two strings using Longest Common Subsequence (LCS).
 */
export function computeLineDiff(oldText: string, newText: string): DiffLine[] {
  const oldLines = oldText ? oldText.split('\n') : []
  const newLines = newText ? newText.split('\n') : []

  const n = oldLines.length
  const m = newLines.length

  // Fast path for identical content
  if (oldText === newText) {
    return oldLines.map((text, i) => ({
      type: 'unchanged',
      text,
      oldLineNumber: i + 1,
      newLineNumber: i + 1,
    }))
  }

  // Cap diff size for performance if lines are huge (> 2000 lines)
  if (n > 2000 || m > 2000) {
    const diff: DiffLine[] = []
    oldLines.forEach((text, i) => diff.push({ type: 'removed', text, oldLineNumber: i + 1 }))
    newLines.forEach((text, i) => diff.push({ type: 'added', text, newLineNumber: i + 1 }))
    return diff
  }

  // DP table for LCS
  const dp: number[][] = Array.from({ length: n + 1 }, () => new Array(m + 1).fill(0))

  for (let i = 1; i <= n; i++) {
    for (let j = 1; j <= m; j++) {
      if (oldLines[i - 1] === newLines[j - 1]) {
        dp[i][j] = dp[i - 1][j - 1] + 1
      } else {
        dp[i][j] = Math.max(dp[i - 1][j], dp[i][j - 1])
      }
    }
  }

  // Backtrack to reconstruct diff
  const result: DiffLine[] = []
  let i = n
  let j = m

  while (i > 0 || j > 0) {
    if (i > 0 && j > 0 && oldLines[i - 1] === newLines[j - 1]) {
      result.unshift({
        type: 'unchanged',
        text: oldLines[i - 1],
        oldLineNumber: i,
        newLineNumber: j,
      })
      i--
      j--
    } else if (j > 0 && (i === 0 || dp[i][j - 1] >= dp[i - 1][j])) {
      result.unshift({
        type: 'added',
        text: newLines[j - 1],
        newLineNumber: j,
      })
      j--
    } else if (i > 0) {
      result.unshift({
        type: 'removed',
        text: oldLines[i - 1],
        oldLineNumber: i,
      })
      i--
    }
  }

  return result
}

/**
 * Format a JSON string with 2 spaces indentation for clean diffing.
 */
export function formatJsonForDiff(raw: string): string {
  if (!raw || !raw.trim()) return ''
  try {
    const parsed = JSON.parse(raw)
    return JSON.stringify(parsed, null, 2)
  } catch {
    return raw
  }
}
