import { AlertTriangle, Send, X } from "lucide-react";
import { Button } from "../ui/button";

interface UndefinedVariablesDialogProps {
  isOpen: boolean;
  undefinedVars: string[];
  activeEnv?: string;
  onCancel: () => void;
  onConfirmSend: () => void;
  onOpenManageEnvironments?: () => void;
}

export function UndefinedVariablesDialog({
  isOpen,
  undefinedVars,
  activeEnv,
  onCancel,
  onConfirmSend,
  onOpenManageEnvironments,
}: UndefinedVariablesDialogProps) {
  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-xs p-4 animate-in fade-in duration-150">
      <div className="bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 rounded-xl shadow-2xl w-full max-w-md overflow-hidden flex flex-col">
        {/* Header */}
        <div className="px-5 py-4 border-b border-zinc-200 dark:border-zinc-800 flex items-start gap-3">
          <div className="w-9 h-9 rounded-lg bg-amber-500/10 text-amber-600 dark:text-amber-400 flex items-center justify-center shrink-0 mt-0.5">
            <AlertTriangle className="w-5 h-5" />
          </div>
          <div className="flex-1">
            <h3 className="text-sm font-semibold text-zinc-900 dark:text-zinc-100">
              Undefined Variables Detected
            </h3>
            <p className="text-xs text-zinc-500 dark:text-zinc-400 mt-0.5">
              The following variables are not defined in the active environment (
              <strong>{activeEnv || "none"}</strong>):
            </p>
          </div>
          <Button
            variant="ghost"
            size="icon"
            onClick={onCancel}
            className="h-7 w-7 text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-200"
          >
            <X className="w-4 h-4" />
          </Button>
        </div>

        {/* Missing Variable List */}
        <div className="p-5 max-h-56 overflow-y-auto space-y-1.5">
          {undefinedVars.map((v) => (
            <div
              key={v}
              className="flex items-center justify-between px-3 py-2 rounded-lg bg-rose-500/10 border border-rose-500/20 text-rose-700 dark:text-rose-300 font-mono text-xs"
            >
              <span>{`{{${v}}}`}</span>
              <span className="text-[10px] uppercase font-semibold text-rose-500">
                Undefined
              </span>
            </div>
          ))}
        </div>

        {/* Footer actions */}
        <div className="px-5 py-3.5 border-t border-zinc-200 dark:border-zinc-800 bg-zinc-50/50 dark:bg-zinc-950/20 flex items-center justify-between">
          {onOpenManageEnvironments ? (
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                onCancel();
                onOpenManageEnvironments();
              }}
              className="h-8 text-xs cursor-pointer text-zinc-600 dark:text-zinc-300"
            >
              Define Variables
            </Button>
          ) : (
            <div />
          )}

          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={onCancel}
              className="h-8 text-xs cursor-pointer"
            >
              Cancel
            </Button>
            <Button
              variant="default"
              size="sm"
              onClick={onConfirmSend}
              className="h-8 text-xs gap-1.5 font-semibold cursor-pointer bg-amber-600 hover:bg-amber-700 text-white"
            >
              <Send className="w-3 h-3" />
              Send Anyway
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}
