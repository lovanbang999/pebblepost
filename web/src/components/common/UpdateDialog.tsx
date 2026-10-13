import { useState, useEffect } from "react";
import {
  Download,
  CheckCircle2,
  AlertCircle,
  RefreshCw,
  ShieldCheck,
  Loader2,
  RotateCcw,
  ArrowUpCircle,
} from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "../ui/dialog";
import { Button } from "../ui/button";
import { Badge } from "../ui/badge";
import { Checkbox } from "../ui/checkbox";
import type { UpdateInfo, UpdateSettings } from "../../types";

// ── Wails runtime bridge ────────────────────────────────────────────────────
interface WailsGoApp {
  RestartApp?: () => Promise<void> | void;
  [key: string]: ((...args: unknown[]) => unknown) | undefined;
}
function getWailsGoApp(): WailsGoApp | undefined {
  if (typeof window === "undefined") return undefined;
  return (window as unknown as { go?: { main?: { App?: WailsGoApp } } }).go?.main
    ?.App;
}

type ApplyPhase = "idle" | "downloading" | "ready_to_restart" | "error";

interface UpdateDialogProps {
  isOpen: boolean;
  onClose: () => void;
  updateInfo: UpdateInfo | null;
  onRefresh: () => Promise<void>;
  isLoading: boolean;
}

export function UpdateDialog({
  isOpen,
  onClose,
  updateInfo,
  onRefresh,
  isLoading,
}: UpdateDialogProps) {
  const [settings, setSettings] = useState<UpdateSettings>({
    checkOnStartup: true,
    channel: "stable",
  });
  const [isSavingSettings, setIsSavingSettings] = useState(false);
  const [applyPhase, setApplyPhase] = useState<ApplyPhase>("idle");
  const [applyError, setApplyError] = useState<string | null>(null);

  // Reset apply phase when dialog closes
  useEffect(() => {
    if (!isOpen) {
      setApplyPhase("idle");
      setApplyError(null);
    }
  }, [isOpen]);

  // Load persisted update settings when dialog opens
  useEffect(() => {
    if (!isOpen) return;
    fetch("/api/update/settings")
      .then((res) => res.json())
      .then((data) => {
        if (data && typeof data.checkOnStartup === "boolean") {
          setSettings(data);
        }
      })
      .catch((err) => console.error("Failed to load update settings:", err));
  }, [isOpen]);

  const handleToggleAutoCheck = async (checked: boolean) => {
    const updated = { ...settings, checkOnStartup: checked };
    setSettings(updated);
    setIsSavingSettings(true);
    try {
      await fetch("/api/update/settings", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(updated),
      });
    } catch (err) {
      console.error("Failed to save update settings:", err);
    } finally {
      setIsSavingSettings(false);
    }
  };

  // ── Update Now handler ───────────────────────────────────────────────────
  const handleUpdateNow = async () => {
    if (!updateInfo) return;
    setApplyPhase("downloading");
    setApplyError(null);
    try {
      const res = await fetch("/api/update/apply", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(updateInfo),
      });
      const data = await res.json();
      if (!res.ok) {
        throw new Error(data?.error || `Server returned ${res.status}`);
      }
      setApplyPhase("ready_to_restart");
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      setApplyError(msg);
      setApplyPhase("error");
    }
  };

  // ── Restart handler ──────────────────────────────────────────────────────
  const handleRestart = () => {
    const goApp = getWailsGoApp();
    if (goApp?.RestartApp) {
      goApp.RestartApp();
    } else {
      // Fallback: prompt manual restart for browser / server context
      alert("Update applied! Please close and reopen the application.");
    }
  };

  // ── Derived UI state ─────────────────────────────────────────────────────
  const isUpdating = applyPhase === "downloading";
  const isReadyToRestart = applyPhase === "ready_to_restart";
  const hasApplyError = applyPhase === "error";

  return (
    <Dialog open={isOpen} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-w-lg bg-white dark:bg-zinc-900 border-zinc-200 dark:border-zinc-800 text-zinc-900 dark:text-zinc-100">
        <DialogHeader>
          <div className="flex items-center gap-2.5">
            <div className="w-8 h-8 rounded-lg bg-blue-500/10 dark:bg-blue-400/10 flex items-center justify-center text-blue-600 dark:text-blue-400">
              <Download className="w-4 h-4" />
            </div>
            <div>
              <DialogTitle className="text-base font-semibold">
                Software Updates
              </DialogTitle>
              <DialogDescription className="text-xs text-zinc-500 dark:text-zinc-400">
                PebblePost release channel and automatic update
              </DialogDescription>
            </div>
          </div>
        </DialogHeader>

        <div className="space-y-4 py-2 text-sm">
          {/* Version status card */}
          <div className="rounded-lg border border-zinc-200 dark:border-zinc-800 p-3.5 bg-zinc-50/70 dark:bg-zinc-950/50 space-y-3">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <span className="text-xs text-zinc-500 dark:text-zinc-400">
                  Installed Version:
                </span>
                <Badge variant="outline" className="font-mono text-xs">
                  v{updateInfo?.currentVersion || "—"}
                </Badge>
              </div>

              {updateInfo?.hasUpdate ? (
                <Badge className="bg-emerald-600 hover:bg-emerald-700 text-white gap-1 text-[11px]">
                  <AlertCircle className="w-3 h-3" />
                  v{updateInfo.latestVersion} Available
                </Badge>
              ) : (
                <Badge
                  variant="secondary"
                  className="text-emerald-600 dark:text-emerald-400 bg-emerald-50 dark:bg-emerald-950/40 border border-emerald-200 dark:border-emerald-800 gap-1 text-[11px]"
                >
                  <CheckCircle2 className="w-3 h-3" />
                  Up to Date
                </Badge>
              )}
            </div>

            {/* Update available — release notes + action buttons */}
            {updateInfo?.hasUpdate && (
              <div className="border-t border-zinc-200 dark:border-zinc-800 pt-2.5 space-y-3">
                {updateInfo.releaseNotes && (
                  <div>
                    <p className="text-[11px] font-medium text-zinc-500 dark:text-zinc-400 mb-1">
                      What's new in v{updateInfo.latestVersion}:
                    </p>
                    <div className="max-h-28 overflow-y-auto rounded bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 p-2.5 font-mono text-[11px] whitespace-pre-wrap text-zinc-600 dark:text-zinc-300">
                      {updateInfo.releaseNotes}
                    </div>
                  </div>
                )}

                {/* Signature indicator */}
                {updateInfo.signatureUrl && (
                  <div className="flex items-center gap-1.5 text-[11px] text-zinc-500 dark:text-zinc-400">
                    <ShieldCheck className="w-3.5 h-3.5 text-blue-500" />
                    <span>Ed25519 / Minisign signature available</span>
                  </div>
                )}

                {/* Apply error */}
                {hasApplyError && applyError && (
                  <div className="rounded bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-800 px-3 py-2 text-[11px] text-red-700 dark:text-red-400">
                    <strong>Update failed:</strong> {applyError}
                  </div>
                )}

                {/* Ready-to-restart success state */}
                {isReadyToRestart && (
                  <div className="rounded bg-emerald-50 dark:bg-emerald-950/30 border border-emerald-200 dark:border-emerald-800 px-3 py-2 text-[11px] text-emerald-700 dark:text-emerald-400 flex items-center gap-2">
                    <CheckCircle2 className="w-3.5 h-3.5 shrink-0" />
                    <span>
                      Update downloaded and applied! Restart to run the new
                      version.
                    </span>
                  </div>
                )}

                {/* Action button row */}
                <div className="flex items-center gap-2 pt-0.5">
                  {!isReadyToRestart && (
                    <Button
                      size="sm"
                      className="h-7 text-xs gap-1.5 bg-blue-600 hover:bg-blue-700 disabled:opacity-60 text-white"
                      onClick={handleUpdateNow}
                      disabled={isUpdating}
                    >
                      {isUpdating ? (
                        <>
                          <Loader2 className="w-3 h-3 animate-spin" />
                          Downloading…
                        </>
                      ) : (
                        <>
                          <ArrowUpCircle className="w-3 h-3" />
                          {hasApplyError ? "Retry Update" : "Update Now"}
                        </>
                      )}
                    </Button>
                  )}

                  {isReadyToRestart && (
                    <Button
                      size="sm"
                      className="h-7 text-xs gap-1.5 bg-emerald-600 hover:bg-emerald-700 text-white"
                      onClick={handleRestart}
                    >
                      <RotateCcw className="w-3 h-3" />
                      Restart PebblePost
                    </Button>
                  )}
                </div>
              </div>
            )}
          </div>

          {/* Preferences */}
          <div className="rounded-lg border border-zinc-200 dark:border-zinc-800 p-3.5 bg-zinc-50/70 dark:bg-zinc-950/50 space-y-2.5">
            <div className="text-xs font-medium text-zinc-900 dark:text-zinc-200">
              Update Preferences
            </div>
            <div className="flex items-center gap-2">
              <Checkbox
                id="pp-auto-check-updates"
                checked={settings.checkOnStartup}
                onCheckedChange={(val) => handleToggleAutoCheck(Boolean(val))}
                disabled={isSavingSettings}
              />
              <label
                htmlFor="pp-auto-check-updates"
                className="text-xs text-zinc-700 dark:text-zinc-300 cursor-pointer select-none"
              >
                Automatically check for new releases on startup
              </label>
            </div>
            <div className="text-[11px] text-zinc-500 dark:text-zinc-400 pl-6">
              When enabled, PebblePost silently queries GitHub Releases on every
              launch and shows a notification badge when an update is available.
            </div>
          </div>
        </div>

        <DialogFooter className="flex items-center justify-between sm:justify-between">
          <Button
            variant="outline"
            size="sm"
            onClick={onRefresh}
            disabled={isLoading || isUpdating}
            className="h-8 gap-1.5 text-xs text-zinc-700 dark:text-zinc-300"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${isLoading ? "animate-spin" : ""}`} />
            <span>Check Now</span>
          </Button>

          <Button
            variant="default"
            size="sm"
            onClick={onClose}
            disabled={isUpdating}
            className="h-8 text-xs"
          >
            Done
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
