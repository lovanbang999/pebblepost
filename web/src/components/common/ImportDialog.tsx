import { useState } from "react";
import {
  Upload,
  FileJson,
  Terminal,
  Layers,
  AlertCircle,
  AlertTriangle,
  CheckCircle2,
  Loader2,
  FolderTree,
  Variable,
  FileCode,
  Flame,
} from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogTrigger,
} from "../ui/dialog";
import { Button } from "../ui/button";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "../ui/tabs";
import { useWorkspaceStore } from "../../store/workspaceStore";
import type { RequestDefinition } from "../../types";
import { Tooltip } from "../ui/tooltip";

type ImportSource = "auto" | "bruno" | "insomnia" | "har" | "postman" | "openapi" | "curl";

interface SkippedItem {
  name: string;
  path?: string;
  reason: string;
}

interface ImportReport {
  sourceFormat: string;
  collectionName: string;
  totalRequests: number;
  totalFolders: number;
  totalVariables: number;
  warnings?: string[];
  skipped?: SkippedItem[];
}

interface ImportResponse {
  requests: RequestDefinition[];
  count: number;
  report?: ImportReport;
}

interface Props {
  onImported?: (requests: RequestDefinition[]) => void;
}

const TABS: {
  key: ImportSource;
  label: string;
  icon: React.ReactNode;
  placeholder: string;
  acceptFiles?: string;
}[] = [
  {
    key: "auto",
    label: "Auto-Detect",
    icon: <Upload className="w-3.5 h-3.5" />,
    placeholder: "Paste any Postman, Bruno, Insomnia, HAR, OpenAPI JSON, or cURL command...",
    acceptFiles: ".json,.har,.bru,.txt",
  },
  {
    key: "postman",
    label: "Postman v2.1",
    icon: <FileJson className="w-3.5 h-3.5 text-orange-400" />,
    placeholder: "Paste your Postman Collection v2.1 JSON here...",
    acceptFiles: ".json",
  },
  {
    key: "bruno",
    label: "Bruno (.bru)",
    icon: <Flame className="w-3.5 h-3.5 text-amber-500" />,
    placeholder: "Paste your Bruno (.bru) file text here...",
    acceptFiles: ".bru,.txt",
  },
  {
    key: "insomnia",
    label: "Insomnia v4",
    icon: <Layers className="w-3.5 h-3.5 text-purple-400" />,
    placeholder: "Paste your Insomnia v4 export JSON here...",
    acceptFiles: ".json",
  },
  {
    key: "har",
    label: "HAR 1.2",
    icon: <FileCode className="w-3.5 h-3.5 text-cyan-400" />,
    placeholder: "Paste your HTTP Archive (.har) JSON here...",
    acceptFiles: ".har,.json",
  },
  {
    key: "openapi",
    label: "OpenAPI 3.0",
    icon: <Layers className="w-3.5 h-3.5 text-emerald-400" />,
    placeholder: "Paste your OpenAPI 3.0 JSON specification here...",
    acceptFiles: ".json,.yaml,.yml",
  },
  {
    key: "curl",
    label: "cURL",
    icon: <Terminal className="w-3.5 h-3.5 text-sky-400" />,
    placeholder: `curl -X POST 'https://api.example.com/users' \\\n  -H 'Authorization: Bearer {{TOKEN}}' \\\n  -H 'Content-Type: application/json' \\\n  --data-raw '{"name": "Alice"}'`,
  },
];

export function ImportDialog({ onImported }: Props) {
  const [open, setOpen] = useState(false);
  const [activeTab, setActiveTab] = useState<ImportSource>("auto");
  const [content, setContent] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [preview, setPreview] = useState<ImportResponse | null>(null);

  const { workspacePath, loadWorkspace } = useWorkspaceStore();

  const handleParse = async () => {
    if (!content.trim()) {
      setError("Please paste or upload content first.");
      return;
    }

    setLoading(true);
    setError(null);
    setPreview(null);

    try {
      const source = activeTab === "auto" ? "" : activeTab;
      const res = await fetch("/api/import", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ source, content }),
      });

      const data = await res.json();

      if (!res.ok) {
        setError(data.error || "Failed to parse import file.");
        return;
      }

      setPreview(data as ImportResponse);
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : "Network error while parsing import.";
      setError(message);
    } finally {
      setLoading(false);
    }
  };

  const handleSaveAll = async () => {
    if (!preview || !content.trim()) return;

    setLoading(true);
    setError(null);

    try {
      const source = activeTab === "auto" ? "" : activeTab;
      const res = await fetch("/api/import", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          source,
          content,
          save: true,
          workspace: workspacePath || ".",
        }),
      });

      const data = await res.json();
      if (!res.ok) {
        throw new Error(data.error || "Failed to save requests to workspace");
      }

      await loadWorkspace();
      onImported?.(preview.requests);

      setOpen(false);
      setContent("");
      setPreview(null);
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : "Failed to save requests";
      setError(message);
    } finally {
      setLoading(false);
    }
  };

  const handleTabChange = (tab: string) => {
    setActiveTab(tab as ImportSource);
    setContent("");
    setError(null);
    setPreview(null);
  };

  const handleFileUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    if (file.size > 50 * 1024 * 1024) {
      setError("File exceeds 50 MB limit.");
      return;
    }

    const reader = new FileReader();
    reader.onload = (ev) => {
      setContent(ev.target?.result as string);
      setError(null);
      setPreview(null);
    };
    reader.readAsText(file);
  };


  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <Tooltip content="Import Request / Collection">
        <DialogTrigger
          id="import-trigger"
          render={
            <Button
              variant="ghost"
              size="icon"
              className="h-6 w-6 text-zinc-500 hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-zinc-200 hover:bg-zinc-100 dark:hover:bg-zinc-900"
            />
          }
        >
          <Upload className="w-3.5 h-3.5" />
        </DialogTrigger>
      </Tooltip>

      <DialogContent className="max-w-2xl w-full max-h-[88vh] overflow-hidden flex flex-col">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 text-sm font-semibold">
            <Upload className="w-4 h-4 text-sky-400" />
            Import Collection or Request
          </DialogTitle>
        </DialogHeader>

        <div className="space-y-3 flex-1 overflow-auto py-1">
          {/* Format Tabs */}
          <Tabs value={activeTab} onValueChange={handleTabChange} className="mt-1">
            <TabsList className="w-full grid grid-cols-4 sm:grid-cols-7 gap-1 h-auto p-1 bg-zinc-100 dark:bg-zinc-900">
              {TABS.map((t) => (
                <TabsTrigger
                  key={t.key}
                  value={t.key}
                  className="text-[11px] gap-1 px-1.5 py-1"
                >
                  {t.icon}
                  <span className="truncate">{t.label}</span>
                </TabsTrigger>
              ))}
            </TabsList>

            {TABS.map((t) => (
              <TabsContent key={t.key} value={t.key} className="mt-3 space-y-2">
                {t.acceptFiles && (
                  <div className="flex items-center justify-between text-xs text-zinc-400">
                    <div className="flex items-center gap-2">
                      <label
                        htmlFor={`import-file-${t.key}`}
                        className="cursor-pointer flex items-center gap-1.5 font-medium text-sky-400 hover:text-sky-300 transition-colors"
                      >
                        <Upload className="w-3.5 h-3.5" />
                        Choose file ({t.acceptFiles})
                      </label>
                      <input
                        id={`import-file-${t.key}`}
                        type="file"
                        accept={t.acceptFiles}
                        className="hidden"
                        onChange={handleFileUpload}
                      />
                      <span className="text-zinc-600">or paste content below</span>
                    </div>
                    {content.length > 0 && (
                      <span className="font-mono text-[10px] text-zinc-500">
                        {(content.length / 1024).toFixed(1)} KB
                      </span>
                    )}
                  </div>
                )}

                <textarea
                  id={`import-textarea-${t.key}`}
                  value={content}
                  onChange={(e) => {
                    setContent(e.target.value);
                    setError(null);
                    setPreview(null);
                  }}
                  rows={9}
                  placeholder={t.placeholder}
                  spellCheck={false}
                  className="w-full rounded-lg border border-zinc-800 bg-zinc-950 p-3 text-xs font-mono text-zinc-200 resize-y focus:outline-none focus:ring-1 focus:ring-sky-500 placeholder:text-zinc-600"
                />
              </TabsContent>
            ))}
          </Tabs>

          {/* Error banner */}
          {error && (
            <div className="flex items-start gap-2 rounded-lg border border-rose-800/50 bg-rose-950/30 p-3 text-xs text-rose-300">
              <AlertCircle className="w-3.5 h-3.5 mt-0.5 shrink-0" />
              <span>{error}</span>
            </div>
          )}

          {/* Import Report & Preview */}
          {preview && (
            <div className="rounded-lg border border-emerald-800/50 bg-emerald-950/20 p-3 space-y-3">
              {/* Header metrics */}
              <div className="flex flex-wrap items-center justify-between gap-2 border-b border-emerald-800/40 pb-2">
                <div className="flex items-center gap-2 text-xs font-semibold text-emerald-400">
                  <CheckCircle2 className="w-4 h-4 shrink-0" />
                  <span>
                    Successfully parsed {preview.count} request{preview.count !== 1 ? "s" : ""}
                  </span>
                  {preview.report?.sourceFormat && (
                    <span className="ml-1 rounded px-1.5 py-0.5 text-[10px] font-mono uppercase bg-emerald-900/50 text-emerald-300 border border-emerald-700/50">
                      {preview.report.sourceFormat}
                    </span>
                  )}
                </div>

                <div className="flex items-center gap-3 text-[11px] text-zinc-400">
                  {preview.report?.totalFolders ? (
                    <span className="flex items-center gap-1">
                      <FolderTree className="w-3 h-3 text-amber-400" />
                      {preview.report.totalFolders} folders
                    </span>
                  ) : null}
                  {preview.report?.totalVariables ? (
                    <span className="flex items-center gap-1">
                      <Variable className="w-3 h-3 text-cyan-400" />
                      {preview.report.totalVariables} variables
                    </span>
                  ) : null}
                </div>
              </div>

              {/* Warnings */}
              {preview.report?.warnings && preview.report.warnings.length > 0 && (
                <div className="rounded border border-amber-800/50 bg-amber-950/30 p-2 text-xs text-amber-300 space-y-1">
                  <div className="flex items-center gap-1.5 font-semibold text-[11px]">
                    <AlertTriangle className="w-3.5 h-3.5 shrink-0" />
                    Conversion Notes & Warnings ({preview.report.warnings.length}):
                  </div>
                  <ul className="list-disc list-inside text-[11px] space-y-0.5 text-amber-200/90 pl-1">
                    {preview.report.warnings.map((w, idx) => (
                      <li key={idx} className="truncate">{w}</li>
                    ))}
                  </ul>
                </div>
              )}

              {/* Skipped items */}
              {preview.report?.skipped && preview.report.skipped.length > 0 && (
                <div className="rounded border border-zinc-800 bg-zinc-900/80 p-2 text-xs text-zinc-300 space-y-1">
                  <div className="flex items-center gap-1.5 font-semibold text-[11px] text-zinc-400">
                    <AlertCircle className="w-3.5 h-3.5 text-zinc-500" />
                    Skipped Items ({preview.report.skipped.length}):
                  </div>
                  <div className="text-[11px] space-y-1 text-zinc-400">
                    {preview.report.skipped.map((sk, idx) => (
                      <div key={idx} className="flex items-start gap-1">
                        <span className="font-mono text-zinc-300">{sk.name}:</span>
                        <span>{sk.reason}</span>
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {/* Requests list preview */}
              <div className="space-y-1 max-h-36 overflow-y-auto pr-1">
                {preview.requests.map((r, i) => (
                  <div
                    key={i}
                    className="flex items-center gap-2 text-xs text-zinc-300 bg-zinc-900/40 rounded px-2 py-1"
                  >
                    <span className="font-mono font-bold text-sky-400 w-12 shrink-0 text-[11px]">
                      {r.method}
                    </span>
                    <span className="truncate text-zinc-300 flex-1">{r.name}</span>
                    <span className="truncate text-zinc-500 text-[10px] font-mono hidden sm:inline max-w-50">
                      {r.url}
                    </span>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>

        <DialogFooter className="gap-2 shrink-0 pt-3 border-t border-zinc-200 dark:border-zinc-800">
          <Button
            id="import-parse-btn"
            variant="outline"
            size="sm"
            onClick={handleParse}
            disabled={loading}
            className="gap-1.5 text-xs"
          >
            {loading && !preview ? (
              <Loader2 className="w-3.5 h-3.5 animate-spin" />
            ) : null}
            Parse & Inspect
          </Button>

          {preview && (
            <Button
              id="import-save-btn"
              variant="default"
              size="sm"
              onClick={handleSaveAll}
              disabled={loading}
              className="gap-1.5 text-xs bg-sky-600 hover:bg-sky-500 text-white"
            >
              {loading ? (
                <Loader2 className="w-3.5 h-3.5 animate-spin" />
              ) : (
                <Upload className="w-3.5 h-3.5" />
              )}
              Save {preview.count} Request{preview.count !== 1 ? "s" : ""} to Workspace
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
