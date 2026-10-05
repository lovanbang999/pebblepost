import { useEffect } from 'react'
import { Calendar, User } from 'lucide-react'
import { useGitStore } from '../../store/gitStore'

interface GitLogTabProps {
  workspacePath: string
}

export function GitLogTab({ workspacePath }: GitLogTabProps) {
  const { log, fetchLog } = useGitStore()

  useEffect(() => {
    if (workspacePath) {
      void fetchLog(workspacePath, 30)
    }
  }, [workspacePath, fetchLog])

  const safeLog = Array.isArray(log) ? log : []
  return (
    <div className="flex-1 flex flex-col min-h-0 bg-white dark:bg-zinc-950 p-3">
      <div className="text-[11px] font-semibold uppercase tracking-wider text-zinc-500 mb-2 px-1">
        Recent Commits ({safeLog.length})
      </div>

      <div className="flex-1 overflow-y-auto space-y-2 pr-1">
        {safeLog.length === 0 ? (
          <div className="text-center py-12 text-zinc-400 text-xs">
            No commits found in this repository
          </div>
        ) : (
          safeLog.map((commit) => (
            <div
              key={commit.hash}
              className="p-2.5 rounded-lg border border-zinc-200 dark:border-zinc-800 bg-zinc-50/50 dark:bg-zinc-900/40 text-xs hover:border-zinc-300 dark:hover:border-zinc-700 transition-colors"
            >
              <div className="flex items-start justify-between gap-2 mb-1.5">
                <div className="font-semibold text-zinc-800 dark:text-zinc-200 text-xs leading-snug">
                  {commit.subject}
                </div>
                <span className="font-mono text-[10px] px-1.5 py-0.5 rounded bg-zinc-200 dark:bg-zinc-800 text-zinc-600 dark:text-zinc-400 shrink-0">
                  {commit.short}
                </span>
              </div>

              <div className="flex items-center gap-3 text-[11px] text-zinc-500 font-sans">
                <span className="flex items-center gap-1">
                  <User className="w-3 h-3 text-zinc-400" />
                  {commit.author}
                </span>
                <span className="flex items-center gap-1">
                  <Calendar className="w-3 h-3 text-zinc-400" />
                  {commit.date}
                </span>
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  )
}
