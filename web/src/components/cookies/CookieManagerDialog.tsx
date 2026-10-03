import { useState, useEffect } from "react";
import {
  Cookie,
  Trash2,
  Plus,
  RefreshCw,
  X,
  Globe,
  Shield,
  Key,
} from "lucide-react";
import { useWorkspaceStore } from "../../store/workspaceStore";
import type { CookieItem } from "../../types";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { Badge } from "../ui/badge";
import { Checkbox } from "../ui/checkbox";

interface CookieManagerDialogProps {
  isOpen: boolean;
  onClose: () => void;
}

export function CookieManagerDialog({
  isOpen,
  onClose,
}: CookieManagerDialogProps) {
  const { workspacePath } = useWorkspaceStore();
  const [cookiesByDomain, setCookiesByDomain] = useState<
    Record<string, CookieItem[]>
  >({});
  const [isLoading, setIsLoading] = useState(false);
  const [selectedDomain, setSelectedDomain] = useState<string | null>(null);

  // New Cookie Form State
  const [isAdding, setIsAdding] = useState(false);
  const [newDomain, setNewDomain] = useState("");
  const [newName, setNewName] = useState("");
  const [newValue, setNewValue] = useState("");
  const [newPath, setNewPath] = useState("/");
  const [newSecure, setNewSecure] = useState(false);
  const [newHttpOnly, setNewHttpOnly] = useState(false);

  const fetchCookies = async () => {
    if (!workspacePath) return;
    setIsLoading(true);
    try {
      const url = new URL("/api/cookies", window.location.origin);
      url.searchParams.set("workspacePath", workspacePath);
      const res = await fetch(url.toString());
      if (res.ok) {
        const data = await res.json();
        setCookiesByDomain(data || {});
        const domains = Object.keys(data || {});
        if (domains.length > 0 && (!selectedDomain || !data[selectedDomain])) {
          setSelectedDomain(domains[0]);
        }
      }
    } catch (err) {
      console.error("Failed to fetch cookies:", err);
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    if (isOpen) {
      fetchCookies();
    }
  }, [isOpen, workspacePath]);

  const handleAddCookie = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!workspacePath || !newDomain.trim() || !newName.trim()) return;

    try {
      const res = await fetch("/api/cookies", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          workspacePath,
          cookie: {
            domain: newDomain.trim(),
            name: newName.trim(),
            value: newValue,
            path: newPath.trim() || "/",
            secure: newSecure,
            httpOnly: newHttpOnly,
          },
        }),
      });

      if (res.ok) {
        setIsAdding(false);
        setNewName("");
        setNewValue("");
        setSelectedDomain(newDomain.trim().toLowerCase());
        await fetchCookies();
      }
    } catch (err) {
      console.error("Failed to add cookie:", err);
    }
  };

  const handleDeleteCookie = async (
    domain: string,
    path: string,
    name: string,
  ) => {
    if (!workspacePath) return;
    try {
      const url = new URL("/api/cookies", window.location.origin);
      url.searchParams.set("workspacePath", workspacePath);
      url.searchParams.set("domain", domain);
      url.searchParams.set("path", path);
      url.searchParams.set("name", name);

      const res = await fetch(url.toString(), { method: "DELETE" });
      if (res.ok) {
        await fetchCookies();
      }
    } catch (err) {
      console.error("Failed to delete cookie:", err);
    }
  };

  const handleClearDomain = async (domain: string) => {
    if (!workspacePath) return;
    try {
      const url = new URL("/api/cookies", window.location.origin);
      url.searchParams.set("workspacePath", workspacePath);
      url.searchParams.set("domain", domain);

      const res = await fetch(url.toString(), { method: "DELETE" });
      if (res.ok) {
        await fetchCookies();
      }
    } catch (err) {
      console.error("Failed to clear domain cookies:", err);
    }
  };

  const handleClearAll = async () => {
    if (
      !workspacePath ||
      !confirm("Are you sure you want to clear ALL workspace cookies?")
    )
      return;
    try {
      const url = new URL("/api/cookies", window.location.origin);
      url.searchParams.set("workspacePath", workspacePath);

      const res = await fetch(url.toString(), { method: "DELETE" });
      if (res.ok) {
        setCookiesByDomain({});
        setSelectedDomain(null);
      }
    } catch (err) {
      console.error("Failed to clear all cookies:", err);
    }
  };

  if (!isOpen) return null;

  const domains = Object.keys(cookiesByDomain).sort();
  const currentCookies = selectedDomain
    ? cookiesByDomain[selectedDomain] || []
    : [];

  return (
    <div className="fixed inset-0 z-50 bg-black/50 backdrop-blur-xs flex items-center justify-center p-4">
      <div className="bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 rounded-xl shadow-2xl w-full max-w-4xl h-162.5 flex flex-col overflow-hidden animate-in fade-in-50 zoom-in-95">
        {/* Header */}
        <div className="p-4 border-b border-zinc-200 dark:border-zinc-800 flex items-center justify-between bg-zinc-50/50 dark:bg-zinc-900/50">
          <div className="flex items-center gap-3">
            <div className="w-8 h-8 rounded-lg bg-amber-500/10 dark:bg-amber-500/20 text-amber-600 dark:text-amber-400 flex items-center justify-center border border-amber-500/20">
              <Cookie className="w-4 h-4" />
            </div>
            <div>
              <h2 className="text-sm font-semibold text-zinc-900 dark:text-zinc-100 flex items-center gap-2">
                Manage Workspace Cookies
                <Badge
                  variant="outline"
                  className="text-[10px] font-normal text-zinc-500"
                >
                  .pebble/cookies.json (never tracked in git)
                </Badge>
              </h2>
              <p className="text-[11px] text-zinc-500">
                View, add, and delete stored session and persistent cookies by
                domain.
              </p>
            </div>
          </div>

          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={fetchCookies}
              disabled={isLoading}
              className="h-8 text-xs gap-1.5 cursor-pointer"
            >
              <RefreshCw
                className={`w-3.5 h-3.5 ${isLoading ? "animate-spin" : ""}`}
              />
              Refresh
            </Button>
            <Button
              size="sm"
              onClick={() => {
                setIsAdding(true);
                if (selectedDomain && !newDomain) setNewDomain(selectedDomain);
              }}
              className="h-8 text-xs gap-1.5 bg-blue-600 hover:bg-blue-700 text-white cursor-pointer"
            >
              <Plus className="w-3.5 h-3.5" />
              Add Cookie
            </Button>
            <button
              onClick={onClose}
              className="p-1.5 text-zinc-400 hover:text-zinc-600 dark:hover:text-zinc-200 rounded-md hover:bg-zinc-100 dark:hover:bg-zinc-800 transition-colors cursor-pointer"
            >
              <X className="w-4 h-4" />
            </button>
          </div>
        </div>

        {/* Content Body: Split Left (Domains) / Right (Cookies) */}
        <div className="flex-1 flex overflow-hidden">
          {/* Domain Sidebar */}
          <div className="w-64 border-r border-zinc-200 dark:border-zinc-800 flex flex-col bg-zinc-50/30 dark:bg-zinc-950/20">
            <div className="p-2 border-b border-zinc-200 dark:border-zinc-800 flex items-center justify-between text-xs text-zinc-500 font-medium">
              <span>DOMAINS ({domains.length})</span>
              {domains.length > 0 && (
                <button
                  onClick={handleClearAll}
                  className="text-[10px] text-red-500 hover:underline cursor-pointer"
                >
                  Clear All
                </button>
              )}
            </div>

            <div className="flex-1 overflow-y-auto p-1.5 space-y-0.5">
              {domains.length === 0 ? (
                <div className="p-4 text-center text-xs text-zinc-400">
                  No cookies stored in workspace.
                </div>
              ) : (
                domains.map((dom) => {
                  const count = cookiesByDomain[dom]?.length || 0;
                  const isSelected = selectedDomain === dom;
                  return (
                    <button
                      key={dom}
                      onClick={() => setSelectedDomain(dom)}
                      className={`w-full text-left px-2.5 py-2 rounded-md text-xs flex items-center justify-between transition-colors cursor-pointer ${
                        isSelected
                          ? "bg-amber-500/10 text-amber-600 dark:text-amber-400 font-medium border border-amber-500/20"
                          : "text-zinc-700 dark:text-zinc-300 hover:bg-zinc-100 dark:hover:bg-zinc-800"
                      }`}
                    >
                      <div className="flex items-center gap-2 truncate">
                        <Globe className="w-3.5 h-3.5 shrink-0 text-zinc-400" />
                        <span className="truncate">{dom}</span>
                      </div>
                      <span className="text-[10px] font-mono text-zinc-400 shrink-0">
                        {count}
                      </span>
                    </button>
                  );
                })
              )}
            </div>
          </div>

          {/* Cookies Details / Add Form */}
          <div className="flex-1 flex flex-col bg-white dark:bg-zinc-950 overflow-hidden">
            {isAdding ? (
              <form
                onSubmit={handleAddCookie}
                className="p-6 space-y-4 max-w-lg"
              >
                <div className="flex items-center justify-between border-b border-zinc-200 dark:border-zinc-800 pb-2">
                  <h3 className="text-xs font-semibold text-zinc-800 dark:text-zinc-200 flex items-center gap-1.5">
                    <Plus className="w-4 h-4 text-blue-500" />
                    Add New Cookie
                  </h3>
                  <button
                    type="button"
                    onClick={() => setIsAdding(false)}
                    className="text-xs text-zinc-400 hover:text-zinc-600 dark:hover:text-zinc-200 cursor-pointer"
                  >
                    Cancel
                  </button>
                </div>

                <div className="grid grid-cols-2 gap-3 text-xs">
                  <div>
                    <label className="block text-zinc-500 mb-1">Domain *</label>
                    <Input
                      required
                      value={newDomain}
                      onChange={(e) => setNewDomain(e.target.value)}
                      placeholder="api.example.com"
                      className="h-8 text-xs"
                    />
                  </div>
                  <div>
                    <label className="block text-zinc-500 mb-1">Path</label>
                    <Input
                      value={newPath}
                      onChange={(e) => setNewPath(e.target.value)}
                      placeholder="/"
                      className="h-8 text-xs"
                    />
                  </div>
                </div>

                <div className="text-xs space-y-1">
                  <label className="block text-zinc-500">Cookie Name *</label>
                  <Input
                    required
                    value={newName}
                    onChange={(e) => setNewName(e.target.value)}
                    placeholder="session_id"
                    className="h-8 text-xs"
                  />
                </div>

                <div className="text-xs space-y-1">
                  <label className="block text-zinc-500">Cookie Value *</label>
                  <Input
                    required
                    value={newValue}
                    onChange={(e) => setNewValue(e.target.value)}
                    placeholder="eyJhbGciOi..."
                    className="h-8 text-xs font-mono"
                  />
                </div>

                <div className="flex items-center gap-6 pt-2">
                  <label className="flex items-center gap-2 text-xs text-zinc-600 dark:text-zinc-400 cursor-pointer">
                    <Checkbox
                      checked={newSecure}
                      onCheckedChange={(c) => setNewSecure(!!c)}
                    />
                    Secure (HTTPS only)
                  </label>
                  <label className="flex items-center gap-2 text-xs text-zinc-600 dark:text-zinc-400 cursor-pointer">
                    <Checkbox
                      checked={newHttpOnly}
                      onCheckedChange={(c) => setNewHttpOnly(!!c)}
                    />
                    HttpOnly
                  </label>
                </div>

                <div className="pt-4 flex gap-2">
                  <Button
                    type="submit"
                    size="sm"
                    className="h-8 text-xs bg-blue-600 hover:bg-blue-700 text-white cursor-pointer"
                  >
                    Save Cookie
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => setIsAdding(false)}
                    className="h-8 text-xs cursor-pointer"
                  >
                    Cancel
                  </Button>
                </div>
              </form>
            ) : selectedDomain ? (
              <div className="flex-1 flex flex-col overflow-hidden">
                {/* Domain Header Action */}
                <div className="p-3 border-b border-zinc-200 dark:border-zinc-800 flex items-center justify-between bg-zinc-50/30 dark:bg-zinc-900/30">
                  <div className="flex items-center gap-2">
                    <Globe className="w-4 h-4 text-amber-500" />
                    <span className="text-xs font-semibold text-zinc-800 dark:text-zinc-200">
                      {selectedDomain}
                    </span>
                    <Badge variant="outline" className="text-[10px]">
                      {currentCookies.length} cookie
                      {currentCookies.length === 1 ? "" : "s"}
                    </Badge>
                  </div>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => handleClearDomain(selectedDomain)}
                    className="h-7 text-[11px] text-red-500 hover:text-red-600 gap-1 cursor-pointer"
                  >
                    <Trash2 className="w-3 h-3" />
                    Clear Domain
                  </Button>
                </div>

                {/* Cookies List */}
                <div className="flex-1 overflow-y-auto p-4 space-y-3">
                  {currentCookies.map((c) => (
                    <div
                      key={`${c.domain}-${c.path}-${c.name}`}
                      className="border border-zinc-200 dark:border-zinc-800 rounded-lg p-3 bg-zinc-50/50 dark:bg-zinc-900/40 text-xs space-y-2 group hover:border-zinc-300 dark:hover:border-zinc-700 transition-colors"
                    >
                      <div className="flex items-center justify-between">
                        <div className="flex items-center gap-2">
                          <span className="font-semibold text-zinc-900 dark:text-zinc-100 font-mono">
                            {c.name}
                          </span>
                          <span className="text-zinc-400 font-mono text-[10px]">
                            Path: {c.path}
                          </span>
                        </div>
                        <div className="flex items-center gap-2">
                          {c.secure && (
                            <Badge
                              variant="outline"
                              className="text-[9px] bg-green-500/10 text-green-600 dark:text-green-400 border-green-500/20"
                            >
                              <Shield className="w-2.5 h-2.5 mr-0.5 inline" />
                              Secure
                            </Badge>
                          )}
                          {c.httpOnly && (
                            <Badge
                              variant="outline"
                              className="text-[9px] bg-blue-500/10 text-blue-600 dark:text-blue-400 border-blue-500/20"
                            >
                              <Key className="w-2.5 h-2.5 mr-0.5 inline" />
                              HttpOnly
                            </Badge>
                          )}
                          <button
                            onClick={() =>
                              handleDeleteCookie(c.domain, c.path, c.name)
                            }
                            className="p-1 text-zinc-400 hover:text-red-500 rounded transition-colors cursor-pointer"
                            title="Delete Cookie"
                          >
                            <Trash2 className="w-3.5 h-3.5" />
                          </button>
                        </div>
                      </div>

                      <div className="bg-white dark:bg-zinc-950 p-2 rounded border border-zinc-100 dark:border-zinc-900 overflow-x-auto">
                        <code className="font-mono text-[11px] text-zinc-700 dark:text-zinc-300 break-all select-all">
                          {c.value}
                        </code>
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            ) : (
              <div className="flex-1 flex flex-col items-center justify-center p-8 text-center text-zinc-400 text-xs">
                <Cookie className="w-8 h-8 text-zinc-300 dark:text-zinc-700 mb-2" />
                <p>Select a domain from the left to view and manage cookies.</p>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
