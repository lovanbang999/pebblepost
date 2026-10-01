import React from "react";
import { FolderOpen, Layers, Search, Sparkles, Send } from "lucide-react";
import { useWorkspaceStore } from "../../store/workspaceStore";

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
    <header className="wails-drag h-11 bg-zinc-950/80 backdrop-blur border-b border-zinc-800/80 flex items-center justify-between px-3 shrink-0 select-none">
      {/* Left: Brand + Workspace info */}
      <div className="flex items-center gap-3 wails-no-drag">
        <div className="flex items-center gap-2">
          <div className="w-6 h-6 rounded-md bg-linear-to-tr from-blue-600 to-indigo-500 flex items-center justify-center shadow-sm">
            <Send className="w-3.5 h-3.5 text-white" />
          </div>
          <span className="font-semibold text-xs text-zinc-100 tracking-wide flex items-center gap-1.5">
            PebblePost
            <span className="text-[10px] uppercase font-mono px-1 py-0.2 bg-blue-500/10 text-blue-400 border border-blue-500/20 rounded">
              v0.1
            </span>
          </span>
        </div>

        <div className="h-4 w-px bg-zinc-800" />

        <button
          onClick={handleOpenFolder}
          className="flex items-center gap-1.5 px-2 py-1 text-xs text-zinc-300 hover:text-white bg-zinc-900 hover:bg-zinc-800 border border-zinc-800 rounded-md transition-colors"
          title="Open API Workspace Folder"
        >
          <FolderOpen className="w-3.5 h-3.5 text-blue-400" />
          <span className="max-w-50 truncate text-[11px]">
            {workspacePath
              ? workspacePath.split("/").pop() || workspacePath
              : "Open Workspace..."}
          </span>
        </button>
      </div>

      {/* Middle: Command search */}
      <div className="wails-no-drag flex items-center">
        <div className="relative flex items-center">
          <button className="flex items-center gap-2 px-3 py-1 text-xs text-zinc-400 bg-zinc-900/90 hover:bg-zinc-800 border border-zinc-800/80 rounded-full w-64 justify-between transition-colors shadow-inner">
            <span className="flex items-center gap-1.5 text-zinc-400 text-[11px]">
              <Search className="w-3 h-3 text-zinc-500" />
              Quick search requests...
            </span>
            <kbd className="px-1.5 py-0.5 text-[9px] font-mono text-zinc-400 bg-zinc-950 border border-zinc-800 rounded shadow-sm">
              Ctrl+K
            </kbd>
          </button>
        </div>
      </div>

      {/* Right: Environment Selector */}
      <div className="flex items-center gap-2 wails-no-drag">
        <div className="flex items-center gap-1.5 bg-zinc-900 border border-zinc-800 px-2 py-0.5 rounded-md">
          <Layers className="w-3 h-3 text-emerald-400" />
          <span className="text-[11px] text-zinc-400">Env:</span>
          <select
            value={activeEnv}
            onChange={(e) => setActiveEnv(e.target.value)}
            className="bg-transparent text-xs text-zinc-200 focus:outline-none cursor-pointer pr-1"
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

        <button
          className="p-1 text-zinc-400 hover:text-zinc-200 hover:bg-zinc-900 rounded transition-colors"
          title="AI Assistant / Code Generation"
        >
          <Sparkles className="w-4 h-4 text-indigo-400" />
        </button>
      </div>
    </header>
  );
};
