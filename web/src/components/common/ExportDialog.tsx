import { useState } from "react";
import {
  Download,
  Copy,
  Check,
  FileJson,
  Layers,
  Loader2,
  AlertCircle,
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
import { Tabs, TabsList, TabsTrigger } from "../ui/tabs";
import { useWorkspaceStore } from "../../store/workspaceStore";
import { Tooltip } from "../ui/tooltip";

type ExportFormat = "postman" | "openapi";

interface ExportResponse {
  format: ExportFormat;
  filename: string;
  content: string;
  count: number;
}

interface ExportDialogProps {
  trigger?: React.ReactElement;
}

export function ExportDialog({ trigger }: ExportDialogProps = {}) {
  const [open, setOpen] = useState(false);
  const [format, setFormat] = useState<ExportFormat>("postman");
  const [loading, setLoading] = useState(false);
  const [copied, setCopied] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [exportData, setExportData] = useState<ExportResponse | null>(null);

  const { workspacePath, activeFilePath } = useWorkspaceStore();

  const handleGenerate = async (targetFormat: ExportFormat = format) => {
    setLoading(true);
    setError(null);
    setCopied(false);

    try {
      // Determine folder path to export (prefer active folder or collections directory)
      let folderPath = "collections";
      if (activeFilePath && activeFilePath.startsWith(workspacePath || "")) {
        const rel = activeFilePath.slice((workspacePath || "").length).replace(/^\//, "");
        const parts = rel.split("/");
        if (parts.length > 1) {
          folderPath = parts.slice(0, 2).join("/"); // e.g. collections/my-api
        }
      }

      const res = await fetch("/api/impexp/export", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          format: targetFormat,
          folderPath: `${workspacePath || "."}/${folderPath}`,
        }),
      });

      const data = await res.json();
      if (!res.ok) {
        throw new Error(data.error || "Export failed");
      }

      setExportData(data as ExportResponse);
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : "Failed to generate export";
      setError(message);
    } finally {
      setLoading(false);
    }
  };

  const handleOpenChange = (isOpen: boolean) => {
    setOpen(isOpen);
    if (isOpen) {
      handleGenerate(format);
    } else {
      setError(null);
      setCopied(false);
    }
  };

  const handleFormatChange = (newFormat: string) => {
    const fmt = newFormat as ExportFormat;
    setFormat(fmt);
    handleGenerate(fmt);
  };

  const handleCopy = async () => {
    if (!exportData?.content) return;
    await navigator.clipboard.writeText(exportData.content);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const handleDownload = () => {
    if (!exportData?.content) return;
    const blob = new Blob([exportData.content], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = exportData.filename || `export_${format}.json`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      {trigger ? (
        <DialogTrigger render={trigger} />
      ) : (
        <Tooltip content="Export Collection">
          <DialogTrigger
            id="export-trigger"
            render={
              <Button
                variant="ghost"
                size="icon"
                className="h-6 w-6 text-zinc-500 hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-zinc-200 hover:bg-zinc-100 dark:hover:bg-zinc-900"
              />
            }
          >
            <Download className="w-3.5 h-3.5" />
          </DialogTrigger>
        </Tooltip>
      )}

      <DialogContent className="max-w-2xl w-full max-h-[85vh] overflow-hidden flex flex-col">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 text-sm font-semibold">
            <Download className="w-4 h-4 text-emerald-400" />
            Export Collection
          </DialogTitle>
        </DialogHeader>

        <div className="space-y-3 flex-1 overflow-auto py-2">
          {/* Format selection tabs */}
          <Tabs value={format} onValueChange={handleFormatChange}>
            <TabsList className="grid grid-cols-2 w-full">
              <TabsTrigger value="postman" className="text-xs gap-1.5">
                <FileJson className="w-3.5 h-3.5 text-orange-400" />
                Postman v2.1
              </TabsTrigger>
              <TabsTrigger value="openapi" className="text-xs gap-1.5">
                <Layers className="w-3.5 h-3.5 text-emerald-400" />
                OpenAPI 3.0
              </TabsTrigger>
            </TabsList>
          </Tabs>

          {/* Description */}
          <p className="text-xs text-zinc-400">
            {format === "postman"
              ? "Export your requests, nested folders, inherited auth/headers/scripts, and variables into a Postman Collection v2.1.0 JSON file."
              : "Infers paths, HTTP methods, parameters, and JSON schemas from saved requests into an OpenAPI 3.0 specification."}
          </p>

          {/* Error notice */}
          {error && (
            <div className="flex items-start gap-2 rounded-lg border border-rose-800/50 bg-rose-950/30 p-3 text-xs text-rose-300">
              <AlertCircle className="w-3.5 h-3.5 mt-0.5 shrink-0" />
              {error}
            </div>
          )}

          {/* Preview Box */}
          <div className="relative">
            {loading ? (
              <div className="h-64 flex flex-col items-center justify-center gap-2 rounded-lg border border-zinc-800 bg-zinc-950 text-xs text-zinc-400">
                <Loader2 className="w-5 h-5 animate-spin text-zinc-300" />
                <span>Generating export...</span>
              </div>
            ) : exportData ? (
              <div className="space-y-1">
                <div className="flex items-center justify-between text-[11px] text-zinc-400 px-1">
                  <span>
                    {exportData.count} request{exportData.count !== 1 ? "s" : ""} exported ({exportData.filename})
                  </span>
                  <span className="font-mono text-zinc-500">
                    {(exportData.content.length / 1024).toFixed(1)} KB
                  </span>
                </div>
                <pre className="h-64 overflow-auto rounded-lg border border-zinc-800 bg-zinc-950 p-3 text-[11px] font-mono text-zinc-300 select-text">
                  {exportData.content}
                </pre>
              </div>
            ) : null}
          </div>
        </div>

        <DialogFooter className="gap-2 shrink-0 pt-3 border-t border-zinc-200 dark:border-zinc-800">
          <Button
            variant="outline"
            size="sm"
            onClick={handleCopy}
            disabled={!exportData || loading}
            className="gap-1.5 text-xs"
          >
            {copied ? (
              <>
                <Check className="w-3.5 h-3.5 text-emerald-400" />
                Copied!
              </>
            ) : (
              <>
                <Copy className="w-3.5 h-3.5" />
                Copy to Clipboard
              </>
            )}
          </Button>

          <Button
            variant="default"
            size="sm"
            onClick={handleDownload}
            disabled={!exportData || loading}
            className="gap-1.5 text-xs bg-emerald-600 hover:bg-emerald-500 text-white"
          >
            <Download className="w-3.5 h-3.5" />
            Download {format === "postman" ? "Postman JSON" : "OpenAPI Spec"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
