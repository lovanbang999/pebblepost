import { useState, useEffect } from "react";
import { FolderOpen, Layers, Search, Sun, Moon, Folder, Cookie, SlidersHorizontal, ArrowUpCircle } from "lucide-react";
import { useWorkspaceStore } from "../../store/workspaceStore";
import { Button } from "../ui/button";
import { Badge } from "../ui/badge";
import { Separator } from "../ui/separator";
import { Tooltip } from "../ui/tooltip";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "../ui/select";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "../ui/dialog";
import { Input } from "../ui/input";
import { UpdateDialog } from "../common/UpdateDialog";
import type { UpdateInfo } from "../../types";

interface TitleBarProps {
  onOpenQuickSearch?: () => void
  onOpenCookieManager?: () => void
  onOpenManageEnvironments?: () => void
}

interface WailsApp {
  SelectDirectory?: () => Promise<string>;
  [key: string]: ((...args: unknown[]) => unknown) | undefined;
}

interface WailsWindow {
  go?: {
    main?: {
      App?: WailsApp;
    };
  };
}

function getWailsApp(): WailsApp | undefined {
  if (typeof window === "undefined") return undefined;
  return (window as unknown as WailsWindow).go?.main?.App;
}


function wailsCall(method: string) {
  const app = getWailsApp();
  if (app && typeof app[method] === "function") {
    app[method]?.();
  }
}

export function TitleBar({
  onOpenQuickSearch,
  onOpenCookieManager,
  onOpenManageEnvironments,
}: TitleBarProps) {
  const [isMaximised, setIsMaximised] = useState(true);
  const [isOpenWorkspaceModalOpen, setIsOpenWorkspaceModalOpen] = useState(false);
  const [inputWorkspacePath, setInputWorkspacePath] = useState("");
  const [updateInfo, setUpdateInfo] = useState<UpdateInfo | null>(null);
  const [isUpdateDialogOpen, setIsUpdateDialogOpen] = useState(false);
  const [isCheckingUpdate, setIsCheckingUpdate] = useState(false);
  const {
    theme,
    toggleTheme,
    workspacePath,
    activeEnv,
    environments,
    setActiveEnv,
    setWorkspacePath,
  } = useWorkspaceStore();

  const fetchUpdateCheck = async (force: boolean = false) => {
    setIsCheckingUpdate(true);
    try {
      const res = await fetch(`/api/update/check${force ? "?force=true" : ""}`);
      if (res.ok) {
        const data = await res.json();
        setUpdateInfo(data);
      }
    } catch (err) {
      console.error("Failed to check for updates:", err);
    } finally {
      setIsCheckingUpdate(false);
    }
  };

  useEffect(() => {
    fetchUpdateCheck(false);
  }, []);

  const handleOpenFolder = async () => {
    const app = getWailsApp();
    if (app?.SelectDirectory) {
      try {
        const path = await app.SelectDirectory();
        if (path) setWorkspacePath(path);
      } catch (err) {
        console.error("Failed to open directory dialog:", err);
      }
    } else {
      setInputWorkspacePath(workspacePath || "./collections");
      setIsOpenWorkspaceModalOpen(true);
    }
  };

  return (
    <header className="wails-drag relative z-20 h-11 bg-white/95 dark:bg-zinc-950/90 backdrop-blur border-b border-zinc-200 dark:border-zinc-800 flex items-center justify-between pl-3 pr-0 shrink-0 select-none transition-colors duration-150">
      {/* Left: Brand + Workspace info */}
      <div className="flex items-center gap-2.5 wails-no-drag">
        <div className="flex items-center gap-2">
          <img
            src="/favicon.svg"
            alt="PebblePost"
            className="w-5.5 h-5.5 select-none shrink-0 drop-shadow-sm hover:scale-105 transition-transform"
          />
          <span className="font-semibold text-xs text-zinc-900 dark:text-zinc-100 tracking-tight flex items-center gap-1.5">
            PebblePost
            <button
              onClick={() => setIsUpdateDialogOpen(true)}
              className="hover:opacity-80 transition-opacity cursor-pointer flex items-center gap-1"
              title="Click to check updates and releases"
            >
              <Badge variant="outline" className="text-[9px] font-mono lowercase tracking-normal py-0 px-1 text-zinc-500 dark:text-zinc-400 border-zinc-200 dark:border-zinc-800">
                v0.2.0
              </Badge>
              {updateInfo?.hasUpdate && (
                <Badge className="bg-emerald-600 hover:bg-emerald-700 text-white text-[9px] font-medium py-0 px-1.5 gap-1 shadow-xs animate-pulse">
                  <ArrowUpCircle className="w-2.5 h-2.5" />
                  v{updateInfo.latestVersion}
                </Badge>
              )}
            </button>
          </span>
        </div>

        <Separator orientation="vertical" className="h-4 mx-1 bg-zinc-200 dark:bg-zinc-800" />

        <Tooltip content="Open API Collection Folder (Local Filesystem)" side="bottom" sideOffset={6}>
          <Button
            variant="outline"
            size="sm"
            onClick={handleOpenFolder}
            className="h-7 gap-1.5 px-2 bg-zinc-100 hover:bg-zinc-200/80 dark:bg-zinc-900/80 dark:hover:bg-zinc-800 border-zinc-200 dark:border-zinc-800 text-zinc-700 dark:text-zinc-300 hover:text-zinc-900 dark:hover:text-white"
          >
            <FolderOpen className="w-3.5 h-3.5 text-blue-500 dark:text-blue-400" />
            <span className="max-w-44 truncate text-[11px]">
              {workspacePath && workspacePath !== "."
                ? workspacePath.split("/").pop() || workspacePath
                : "Open Workspace..."}
            </span>
          </Button>
        </Tooltip>
      </div>

      {/* Middle: Command search */}
      <div className="wails-no-drag flex items-center">
        <button
          onClick={onOpenQuickSearch}
          className="flex items-center gap-2 px-3 py-1 text-xs text-zinc-500 dark:text-zinc-400 bg-zinc-100 hover:bg-zinc-200/80 dark:bg-zinc-900/80 dark:hover:bg-zinc-800/90 border border-zinc-200 dark:border-zinc-800 rounded-full w-64 justify-between transition-colors shadow-inner cursor-pointer"
        >
          <span className="flex items-center gap-1.5 text-[11px]">
            <Search className="w-3 h-3 text-zinc-400 dark:text-zinc-500" />
            Quick search requests...
          </span>
          <kbd className="px-1.5 py-0.5 text-[9px] font-mono text-zinc-500 dark:text-zinc-400 bg-white dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800 rounded shadow-xs">
            Ctrl+K
          </kbd>
        </button>
      </div>

      {/* Right: Environment + Theme Toggle + Window Controls */}
      <div className="flex items-center h-full gap-2 wails-no-drag">
        {/* Environment Selector */}
        <Select value={activeEnv} onValueChange={(val) => typeof val === 'string' && setActiveEnv(val)}>
          <SelectTrigger className="h-7 gap-1.5 px-2 bg-zinc-100 dark:bg-zinc-900 border-zinc-200 dark:border-zinc-800 text-xs font-medium w-auto focus:ring-1 focus:ring-blue-500/50">
            <Layers className="w-3 h-3 text-emerald-500 dark:text-emerald-400 shrink-0" />
            <span className="text-[11px] text-zinc-500 dark:text-zinc-400">Env:</span>
            <SelectValue placeholder="Select env" className="font-medium text-xs text-zinc-800 dark:text-zinc-200" />
          </SelectTrigger>
          <SelectContent align="end">
            {environments.map((env) => (
              <SelectItem key={env.name} value={env.name}>
                {env.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        {/* Manage Environments */}
        <Tooltip content="Manage Environments & Variables" side="bottom" sideOffset={6}>
          <Button
            id="manage-environments-btn"
            variant="ghost"
            size="icon"
            onClick={onOpenManageEnvironments}
            className="h-7 w-7 text-zinc-500 hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-zinc-200 hover:bg-zinc-200/70 dark:hover:bg-zinc-900 transition-colors cursor-pointer"
          >
            <SlidersHorizontal className="w-3.5 h-3.5" />
          </Button>
        </Tooltip>

        {/* Manage Cookies */}
        <Tooltip content="Workspace Cookies (.pebble/cookies.json)" side="bottom" sideOffset={6}>
          <Button
            id="manage-cookies-btn"
            variant="ghost"
            size="sm"
            onClick={onOpenCookieManager}
            className="h-7 px-2 text-xs gap-1.5 text-zinc-500 hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-zinc-200 hover:bg-zinc-200/70 dark:hover:bg-zinc-900 transition-colors cursor-pointer"
          >
            <Cookie className="w-3.5 h-3.5 text-amber-500" />
            <span className="hidden sm:inline">Cookies</span>
          </Button>
        </Tooltip>

        {/* Theme Toggle (Sun / Moon) */}
        <Tooltip content={theme === 'dark' ? 'Switch to Light Theme' : 'Switch to Dark Theme'} side="bottom" sideOffset={6}>
          <Button
            id="theme-toggle-btn"
            variant="ghost"
            size="icon"
            onClick={toggleTheme}
            className="h-7 w-7 text-zinc-500 hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-zinc-200 hover:bg-zinc-200/70 dark:hover:bg-zinc-900 transition-colors cursor-pointer"
          >
            {theme === 'dark' ? (
              <Sun className="w-3.5 h-3.5 text-amber-400 hover:rotate-45 transition-transform" />
            ) : (
              <Moon className="w-3.5 h-3.5 text-indigo-600 hover:-rotate-12 transition-transform" />
            )}
          </Button>
        </Tooltip>

        {/* Window Controls — Frameless Desktop Titlebar Controls */}
        <div className="flex items-stretch h-full ml-1">
          <button
            id="win-minimise"
            onClick={() => wailsCall("WindowMinimise")}
            title="Minimise"
            className="w-11 h-full flex items-center justify-center text-zinc-500 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-100 hover:bg-zinc-200/80 dark:hover:bg-zinc-800/80 transition-colors cursor-pointer"
          >
            <svg width="11" height="1" viewBox="0 0 11 1">
              <rect width="11" height="1" fill="currentColor" />
            </svg>
          </button>

          <button
            id="win-maximise"
            onClick={() => {
              wailsCall("WindowToggleMaximise");
              setIsMaximised((prev) => !prev);
            }}
            title={isMaximised ? "Restore" : "Maximise"}
            className="w-11 h-full flex items-center justify-center text-zinc-500 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-100 hover:bg-zinc-200/80 dark:hover:bg-zinc-800/80 transition-colors cursor-pointer"
          >
            {isMaximised ? (
              <svg width="10" height="10" viewBox="0 0 10 10" fill="none">
                <path d="M2.5 1.5H8.5V7.5" stroke="currentColor" strokeWidth="1" />
                <rect x="1.5" y="2.5" width="6" height="6" stroke="currentColor" strokeWidth="1" fill="none" />
              </svg>
            ) : (
              <svg width="10" height="10" viewBox="0 0 10 10" fill="none">
                <rect x="0.5" y="0.5" width="9" height="9" stroke="currentColor" strokeWidth="1" />
              </svg>
            )}
          </button>

          <button
            id="win-close"
            onClick={() => wailsCall("WindowClose")}
            title="Close"
            className="w-12 h-full flex items-center justify-center text-zinc-500 dark:text-zinc-400 hover:text-white hover:bg-rose-600 transition-colors cursor-pointer"
          >
            <svg width="10" height="10" viewBox="0 0 10 10" fill="none">
              <path d="M1 1L9 9M9 1L1 9" stroke="currentColor" strokeWidth="1.2" strokeLinecap="square" />
            </svg>
          </button>
        </div>
      </div>

      {/* Open Workspace Dialog for Browser fallback */}
      <Dialog open={isOpenWorkspaceModalOpen} onOpenChange={setIsOpenWorkspaceModalOpen}>
        <DialogContent className="sm:max-w-md">
          <form
            onSubmit={(e) => {
              e.preventDefault();
              const clean = inputWorkspacePath.trim();
              if (clean) {
                setWorkspacePath(clean);
                setIsOpenWorkspaceModalOpen(false);
              }
            }}
            className="space-y-4"
          >
            <DialogHeader>
              <div className="flex items-center gap-2">
                <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-blue-500/10 text-blue-600 dark:text-blue-400">
                  <Folder className="h-4 w-4" />
                </div>
                <div>
                  <DialogTitle>Open Workspace</DialogTitle>
                  <DialogDescription>
                    Enter the path to your PebblePost workspace folder.
                  </DialogDescription>
                </div>
              </div>
            </DialogHeader>

            <div className="space-y-1.5 py-1">
              <label
                htmlFor="workspace-path"
                className="text-xs font-medium text-zinc-700 dark:text-zinc-300"
              >
                Workspace Directory Path
              </label>
              <Input
                id="workspace-path"
                autoFocus
                value={inputWorkspacePath}
                onChange={(e) => setInputWorkspacePath(e.target.value)}
                placeholder="/path/to/workspace or ./collections"
                className="font-mono text-xs h-8"
              />
            </div>

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => setIsOpenWorkspaceModalOpen(false)}
              >
                Cancel
              </Button>
              <Button type="submit" size="sm" disabled={!inputWorkspacePath.trim()}>
                Open Workspace
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Software Update Modal & Preferences */}
      <UpdateDialog
        isOpen={isUpdateDialogOpen}
        onClose={() => setIsUpdateDialogOpen(false)}
        updateInfo={updateInfo}
        onRefresh={() => fetchUpdateCheck(true)}
        isLoading={isCheckingUpdate}
      />
    </header>
  );
}
