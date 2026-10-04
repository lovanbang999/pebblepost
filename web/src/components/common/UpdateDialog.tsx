import { useState, useEffect } from "react";
import { Download, CheckCircle2, AlertCircle, RefreshCw, ShieldCheck, ExternalLink } from "lucide-react";
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

  useEffect(() => {
    if (isOpen) {
      fetch("/api/update/settings")
        .then((res) => res.json())
        .then((data) => {
          if (data && typeof data.checkOnStartup === "boolean") {
            setSettings(data);
          }
        })
        .catch((err) => console.error("Failed to load update settings:", err));
    }
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
                PebblePost release channel and cryptographic update verification
              </DialogDescription>
            </div>
          </div>
        </DialogHeader>

        <div className="space-y-4 py-2 text-sm">
          {/* Version status card */}
          <div className="rounded-lg border border-zinc-200 dark:border-zinc-800 p-3.5 bg-zinc-50/70 dark:bg-zinc-950/50 space-y-3">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <span className="text-xs text-zinc-500 dark:text-zinc-400">Installed Version:</span>
                <Badge variant="outline" className="font-mono text-xs">
                  v{updateInfo?.currentVersion || "0.2.0"}
                </Badge>
              </div>

              {updateInfo?.hasUpdate ? (
                <Badge className="bg-emerald-600 hover:bg-emerald-700 text-white gap-1 text-[11px]">
                  <AlertCircle className="w-3 h-3" />
                  v{updateInfo.latestVersion} Available
                </Badge>
              ) : (
                <Badge variant="secondary" className="text-emerald-600 dark:text-emerald-400 bg-emerald-50 dark:bg-emerald-950/40 border border-emerald-200 dark:border-emerald-800 gap-1 text-[11px]">
                  <CheckCircle2 className="w-3 h-3" />
                  Up to Date
                </Badge>
              )}
            </div>

            {updateInfo?.hasUpdate && (
              <div className="border-t border-zinc-200 dark:border-zinc-800 pt-2.5 space-y-2">
                <div className="text-xs text-zinc-700 dark:text-zinc-300">
                  A newer release of PebblePost is available on GitHub Releases.
                </div>
                {updateInfo.releaseNotes && (
                  <div className="max-h-32 overflow-y-auto rounded bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 p-2.5 font-mono text-[11px] whitespace-pre-wrap text-zinc-600 dark:text-zinc-300">
                    {updateInfo.releaseNotes}
                  </div>
                )}
                <div className="flex items-center justify-between pt-1">
                  <div className="flex items-center gap-1.5 text-[11px] text-zinc-500 dark:text-zinc-400">
                    <ShieldCheck className="w-3.5 h-3.5 text-blue-500" />
                    <span>Minisign / Ed25519 signature available</span>
                  </div>
                  {updateInfo.releaseUrl && (
                    <Button
                      size="sm"
                      className="h-7 text-xs gap-1.5 bg-blue-600 hover:bg-blue-700 text-white"
                      onClick={() => window.open(updateInfo.releaseUrl, "_blank")}
                    >
                      <span>View Release</span>
                      <ExternalLink className="w-3 h-3" />
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
                id="auto-check-updates"
                checked={settings.checkOnStartup}
                onCheckedChange={(val) => handleToggleAutoCheck(Boolean(val))}
                disabled={isSavingSettings}
              />
              <label
                htmlFor="auto-check-updates"
                className="text-xs text-zinc-700 dark:text-zinc-300 cursor-pointer select-none"
              >
                Automatically check for new releases on startup
              </label>
            </div>
            <div className="text-[11px] text-zinc-500 dark:text-zinc-400 pl-6">
              When enabled, PebblePost queries the GitHub Releases API to notify you of critical security patches and updates.
            </div>
          </div>
        </div>

        <DialogFooter className="flex items-center justify-between sm:justify-between">
          <Button
            variant="outline"
            size="sm"
            onClick={onRefresh}
            disabled={isLoading}
            className="h-8 gap-1.5 text-xs text-zinc-700 dark:text-zinc-300"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${isLoading ? "animate-spin" : ""}`} />
            <span>Check Now</span>
          </Button>

          <Button
            variant="default"
            size="sm"
            onClick={onClose}
            className="h-8 text-xs"
          >
            Done
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
