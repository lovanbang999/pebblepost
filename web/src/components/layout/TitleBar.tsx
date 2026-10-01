import React from "react";
import { FolderOpen, Layers, Search, Sparkles, Send } from "lucide-react";
import { useWorkspaceStore } from "../../store/workspaceStore";
import { Button } from "../ui/button";
import { Badge } from "../ui/badge";
import { Separator } from "../ui/separator";
import { Tooltip } from "../ui/tooltip";

export const TitleBar: React.FC = () => {
  const {
    workspacePath,
    activeEnv,
    environments,
    setActiveEnv,
    setWorkspacePath,
  } = useWorkspaceStore();

  const handleOpenFolder = async () => {
    // Check if Wails runtime binding is available
    if (window.go?.main?.App?.SelectDirectory) {
      try {
        const path = await window.go.main.App.SelectDirectory();
        if (path) {
          setWorkspacePath(path);
        }
      } catch (err) {
        console.error("Failed to open directory dialog:", err);
      }
    } else {
      // Fallback in web browser mode
      const promptPath = prompt(
        "Enter workspace absolute path (or ./collections):",
        workspacePath || "./collections",
      );
      if (promptPath) {
        setWorkspacePath(promptPath);
      }
    }
  };

  return (
    <header className="wails-drag h-11 bg-zinc-950/90 backdrop-blur border-b border-zinc-800 flex items-center justify-between px-3 shrink-0 select-none">
      {/* Left: Brand + Workspace info */}
      <div className="flex items-center gap-2.5 wails-no-drag">
        <div className="flex items-center gap-2">
          <div className="w-6 h-6 rounded-md bg-linear-to-tr from-blue-600 to-indigo-500 flex items-center justify-center shadow-xs">
            <Send className="w-3.5 h-3.5 text-white" />
          </div>
          <span className="font-semibold text-xs text-zinc-100 tracking-wide flex items-center gap-1.5">
            PebblePost
            <Badge variant="default" className="text-[9px] py-0 px-1">
              v0.1
            </Badge>
          </span>
        </div>

        <Separator orientation="vertical" className="h-4 mx-1" />

        <Tooltip content="Open API Collection Folder (Local Filesystem)">
          <Button
            variant="outline"
            size="sm"
            onClick={handleOpenFolder}
            className="h-7 gap-1.5 px-2 bg-zinc-900/80 border-zinc-800 text-zinc-300 hover:text-white"
          >
            <FolderOpen className="w-3.5 h-3.5 text-blue-400" />
            <span className="max-w-44 truncate text-[11px]">
              {workspacePath
                ? workspacePath.split("/").pop() || workspacePath
                : "Open Workspace..."}
            </span>
          </Button>
        </Tooltip>
      </div>

      {/* Middle: Command search */}
      <div className="wails-no-drag flex items-center">
        <button className="flex items-center gap-2 px-3 py-1 text-xs text-zinc-400 bg-zinc-900/80 hover:bg-zinc-800/90 border border-zinc-800 rounded-full w-64 justify-between transition-colors shadow-inner cursor-pointer">
          <span className="flex items-center gap-1.5 text-zinc-400 text-[11px]">
            <Search className="w-3 h-3 text-zinc-500" />
            Quick search requests...
          </span>
          <kbd className="px-1.5 py-0.5 text-[9px] font-mono text-zinc-400 bg-zinc-950 border border-zinc-800 rounded shadow-xs">
            Ctrl+K
          </kbd>
        </button>
      </div>

      {/* Right: Environment Selector */}
      <div className="flex items-center gap-2 wails-no-drag">
        <div className="flex items-center gap-1.5 bg-zinc-900 border border-zinc-800 px-2 py-0.5 rounded-md h-7">
          <Layers className="w-3 h-3 text-emerald-400" />
          <span className="text-[11px] text-zinc-400">Env:</span>
          <select
            value={activeEnv}
            onChange={(e) => setActiveEnv(e.target.value)}
            className="bg-transparent text-xs text-zinc-200 focus:outline-none cursor-pointer pr-1 font-medium"
          >
            {environments.map((env) => (
              <option
                key={env.name}
                value={env.name}
                className="bg-zinc-900 text-zinc-100"
              >
                {env.name}
              </option>
            ))}
          </select>
        </div>

        <Tooltip content="AI Prompt Assistant & Generator">
          <Button
            variant="ghost"
            size="icon"
            className="h-7 w-7 text-zinc-400 hover:text-zinc-200 hover:bg-zinc-900"
          >
            <Sparkles className="w-3.5 h-3.5 text-indigo-400" />
          </Button>
        </Tooltip>
      </div>
    </header>
  );
};
