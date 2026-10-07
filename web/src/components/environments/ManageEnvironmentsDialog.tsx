import { useState, useEffect } from "react";
import {
  Layers,
  Plus,
  Trash2,
  Copy,
  Eye,
  EyeOff,
  Save,
  Check,
  X,
  Lock,
  Globe,
  AlertCircle,
  KeyRound,
  ShieldCheck,
} from "lucide-react";
import { useWorkspaceStore } from "../../store/workspaceStore";
import type { EnvironmentDefinition, EnvironmentVariable } from "../../types";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { Badge } from "../ui/badge";
import { Checkbox } from "../ui/checkbox";
import { Tooltip } from "../ui/tooltip";

interface ManageEnvironmentsDialogProps {
  isOpen: boolean;
  onClose: () => void;
  initialEnvName?: string;
}

export function ManageEnvironmentsDialog({
  isOpen,
  onClose,
  initialEnvName,
}: ManageEnvironmentsDialogProps) {
  const { workspacePath, environments, activeEnv, setActiveEnv, setEnvironments } =
    useWorkspaceStore();

  const [selectedEnvName, setSelectedEnvName] = useState<string>("");
  const [editingEnv, setEditingEnv] = useState<EnvironmentDefinition | null>(null);
  const [revealedSecrets, setRevealedSecrets] = useState<Record<number, boolean>>({});
  const [isSaving, setIsSaving] = useState(false);
  const [saveSuccess, setSaveSuccess] = useState(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [backendConfig, setBackendConfig] = useState<{ default?: string; perEnv?: Record<string, string> } | null>(null);
  const [isMigrating, setIsMigrating] = useState(false);
  const [migrationMessage, setMigrationMessage] = useState<string | null>(null);

  // Initialize selected environment
  useEffect(() => {
    if (!isOpen) return;
    const target =
      initialEnvName ||
      activeEnv ||
      (environments.length > 0 ? environments[0].name : "");

    const found = environments.find((e) => e.name === target) || environments[0];
    if (found) {
      setSelectedEnvName(found.name);
      setEditingEnv(JSON.parse(JSON.stringify(found)));
    } else {
      setSelectedEnvName("");
      setEditingEnv(null);
    }
    setRevealedSecrets({});
    setSaveSuccess(false);
    setErrorMessage(null);
    setMigrationMessage(null);
  }, [isOpen, initialEnvName, environments, activeEnv]);

  // Load workspace secret backend configuration
  useEffect(() => {
    if (!isOpen) return;
    fetch("/api/workspace/secret-backend")
      .then((res) => (res.ok ? res.json() : null))
      .then((data) => {
        if (data) setBackendConfig(data);
      })
      .catch(() => {});
  }, [isOpen]);

  const handleMigrate = async (targetBackend: "keychain" | "file") => {
    if (!editingEnv) return;
    const targetLabel = targetBackend === "keychain" ? "OS Keychain" : "Plaintext File (*.secret.env.json)";
    if (
      !window.confirm(
        `Are you sure you want to migrate all secrets for environment "${editingEnv.name}" to ${targetLabel}?\n\nA rollback backup file will be created automatically.`
      )
    ) {
      return;
    }

    setIsMigrating(true);
    setErrorMessage(null);
    setMigrationMessage(null);

    try {
      const res = await fetch("/api/workspace/secret-backend/migrate", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          envName: editingEnv.name,
          to: targetBackend,
        }),
      });
      const data = await res.json();
      if (!res.ok) {
        throw new Error(data.error || "Secret migration failed");
      }
      setMigrationMessage(
        data.backupPath
          ? `Secrets migrated to ${targetLabel}! Backup saved to: ${data.backupPath}`
          : `Secrets successfully migrated to ${targetLabel}!`
      );

      // Refresh backend configuration
      const cfgRes = await fetch("/api/workspace/secret-backend");
      if (cfgRes.ok) {
        setBackendConfig(await cfgRes.json());
      }
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : "Failed to migrate secrets";
      setErrorMessage(msg);
    } finally {
      setIsMigrating(false);
    }
  };

  // When switching environment in left list
  const handleSelectEnv = (envName: string) => {
    const found = environments.find((e) => e.name === envName);
    if (found) {
      setSelectedEnvName(found.name);
      setEditingEnv(JSON.parse(JSON.stringify(found)));
      setRevealedSecrets({});
      setSaveSuccess(false);
      setErrorMessage(null);
    }
  };

  const handleAddNewEnv = () => {
    let baseName = "new-env";
    let counter = 1;
    while (environments.some((e) => e.name === baseName)) {
      baseName = `new-env-${counter++}`;
    }

    const newEnv: EnvironmentDefinition = {
      name: baseName,
      variables: [
        {
          key: "BASE_URL",
          value: "https://api.example.com",
          enabled: true,
          secret: false,
        },
      ],
    };

    setSelectedEnvName(newEnv.name);
    setEditingEnv(newEnv);
    setRevealedSecrets({});
    setSaveSuccess(false);
  };

  const handleDuplicateEnv = (sourceEnv: EnvironmentDefinition) => {
    let copyName = `${sourceEnv.name}-copy`;
    let counter = 1;
    while (environments.some((e) => e.name === copyName)) {
      copyName = `${sourceEnv.name}-copy-${counter++}`;
    }

    const cloned: EnvironmentDefinition = {
      name: copyName,
      variables: JSON.parse(JSON.stringify(sourceEnv.variables || [])),
    };

    setSelectedEnvName(cloned.name);
    setEditingEnv(cloned);
    setRevealedSecrets({});
    setSaveSuccess(false);
  };

  const handleDeleteEnv = async (envName: string) => {
    if (!workspacePath) return;
    if (
      !window.confirm(
        `Are you sure you want to delete environment '${envName}'? This will delete both public and secret files.`
      )
    ) {
      return;
    }

    try {
      const url = new URL("/api/environments", window.location.origin);
      url.searchParams.set("path", workspacePath);
      url.searchParams.set("name", envName);
      const res = await fetch(url.toString(), { method: "DELETE" });

      if (res.ok) {
        const remaining = environments.filter((e) => e.name !== envName);
        setEnvironments(remaining);

        if (activeEnv === envName && remaining.length > 0) {
          setActiveEnv(remaining[0].name);
        }

        if (remaining.length > 0) {
          handleSelectEnv(remaining[0].name);
        } else {
          setEditingEnv(null);
          setSelectedEnvName("");
        }
      } else {
        const err = await res.json().catch(() => ({ error: "Failed to delete" }));
        setErrorMessage(err.error || "Failed to delete environment");
      }
    } catch (err) {
      console.error("Delete environment error:", err);
      setErrorMessage("Network error while deleting environment");
    }
  };

  const handleAddVariable = () => {
    if (!editingEnv) return;
    const newVars: EnvironmentVariable[] = [
      ...(editingEnv.variables || []),
      { key: "", value: "", enabled: true, secret: false },
    ];
    setEditingEnv({ ...editingEnv, variables: newVars });
  };

  const handleUpdateVariable = (
    index: number,
    field: keyof EnvironmentVariable,
    val: unknown
  ) => {
    if (!editingEnv) return;
    const nextVars = [...(editingEnv.variables || [])];
    nextVars[index] = { ...nextVars[index], [field]: val };
    setEditingEnv({ ...editingEnv, variables: nextVars });
    setSaveSuccess(false);
  };

  const handleDeleteVariable = (index: number) => {
    if (!editingEnv) return;
    const nextVars = (editingEnv.variables || []).filter((_, i) => i !== index);
    setEditingEnv({ ...editingEnv, variables: nextVars });
    setSaveSuccess(false);
  };

  const toggleRevealSecret = (index: number) => {
    setRevealedSecrets((prev) => ({ ...prev, [index]: !prev[index] }));
  };

  const handleSaveCurrentEnv = async () => {
    if (!editingEnv || !workspacePath) return;
    const cleanName = editingEnv.name.trim();

    if (!cleanName) {
      setErrorMessage("Environment name cannot be empty");
      return;
    }

    setIsSaving(true);
    setErrorMessage(null);

    try {
      const payload = {
        workspacePath,
        environment: {
          name: cleanName,
          variables: editingEnv.variables || [],
        },
      };

      const res = await fetch("/api/environments/save", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });

      if (res.ok) {
        // Refresh environments from backend to get verified merged state
        const listRes = await fetch(
          `/api/environments?path=${encodeURIComponent(workspacePath)}`
        );
        if (listRes.ok) {
          const freshEnvs: EnvironmentDefinition[] = await listRes.json();
          setEnvironments(freshEnvs);

          // If renamed, update activeEnv
          if (selectedEnvName !== cleanName && activeEnv === selectedEnvName) {
            setActiveEnv(cleanName);
          }
          setSelectedEnvName(cleanName);
          const found = freshEnvs.find((e) => e.name === cleanName);
          if (found) {
            setEditingEnv(JSON.parse(JSON.stringify(found)));
          }
        }

        setSaveSuccess(true);
        setTimeout(() => setSaveSuccess(false), 2500);
      } else {
        const err = await res.json().catch(() => ({ error: "Failed to save" }));
        setErrorMessage(err.error || "Failed to save environment");
      }
    } catch (err) {
      console.error("Save environment error:", err);
      setErrorMessage("Network error while saving environment");
    } finally {
      setIsSaving(false);
    }
  };

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-xs p-4 animate-in fade-in duration-150">
      <div className="bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 rounded-xl shadow-2xl w-full max-w-4xl h-160 flex flex-col overflow-hidden">
        {/* Header */}
        <div className="flex items-center justify-between px-5 py-3.5 border-b border-zinc-200 dark:border-zinc-800 shrink-0">
          <div className="flex items-center gap-2.5">
            <div className="w-8 h-8 rounded-lg bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 flex items-center justify-center">
              <Layers className="w-4 h-4" />
            </div>
            <div>
              <h2 className="text-sm font-semibold text-zinc-900 dark:text-zinc-100 flex items-center gap-2">
                Manage Environments
                {activeEnv && (
                  <Badge variant="outline" className="text-[10px] py-0 font-normal">
                    Active: {activeEnv}
                  </Badge>
                )}
              </h2>
              <p className="text-xs text-zinc-500 dark:text-zinc-400">
                Configure workspace variables and secrets for environment interpolation
              </p>
            </div>
          </div>
          <Button
            variant="ghost"
            size="icon"
            onClick={onClose}
            className="h-8 w-8 text-zinc-500 hover:text-zinc-800 dark:hover:text-zinc-200"
          >
            <X className="w-4 h-4" />
          </Button>
        </div>

        {/* Content Body: Two columns */}
        <div className="flex-1 flex overflow-hidden">
          {/* Left column: Environment List */}
          <div className="w-64 border-r border-zinc-200 dark:border-zinc-800 flex flex-col bg-zinc-50/50 dark:bg-zinc-950/30 shrink-0">
            <div className="p-3 border-b border-zinc-200 dark:border-zinc-800 flex items-center justify-between">
              <span className="text-xs font-semibold text-zinc-500 uppercase tracking-wider">
                Environments
              </span>
              <Button
                variant="outline"
                size="sm"
                onClick={handleAddNewEnv}
                className="h-7 text-xs px-2 gap-1 font-medium cursor-pointer"
              >
                <Plus className="w-3.5 h-3.5" />
                New
              </Button>
            </div>

            <div className="flex-1 overflow-y-auto p-2 space-y-1">
              {environments.map((env) => {
                const isSelected = env.name === selectedEnvName;
                const isActive = env.name === activeEnv;
                const varCount = env.variables?.length || 0;
                const secretCount = env.variables?.filter((v) => v.secret).length || 0;

                return (
                  <div
                    key={env.name}
                    onClick={() => handleSelectEnv(env.name)}
                    className={`group flex items-center justify-between px-3 py-2 rounded-lg text-xs cursor-pointer transition-colors ${
                      isSelected
                        ? "bg-zinc-200/80 dark:bg-zinc-800 font-medium text-zinc-900 dark:text-zinc-100"
                        : "text-zinc-600 dark:text-zinc-400 hover:bg-zinc-100 dark:hover:bg-zinc-800/50"
                    }`}
                  >
                    <div className="flex items-center gap-2 truncate">
                      <Globe className="w-3.5 h-3.5 text-zinc-400 shrink-0" />
                      <span className="truncate">{env.name}</span>
                      {isActive && (
                        <span className="w-1.5 h-1.5 rounded-full bg-emerald-500 shrink-0" title="Active Environment" />
                      )}
                    </div>
                    <div className="flex items-center gap-1 shrink-0">
                      <span className="text-[10px] text-zinc-400">
                        {varCount}
                        {secretCount > 0 && ` (${secretCount} 🔒)`}
                      </span>
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={(e) => {
                          e.stopPropagation();
                          handleDuplicateEnv(env);
                        }}
                        className="h-5 w-5 opacity-0 group-hover:opacity-100 text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-200"
                        title="Duplicate Environment"
                      >
                        <Copy className="w-3 h-3" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={(e) => {
                          e.stopPropagation();
                          handleDeleteEnv(env.name);
                        }}
                        className="h-5 w-5 opacity-0 group-hover:opacity-100 text-zinc-400 hover:text-rose-600 dark:hover:text-rose-400"
                        title="Delete Environment"
                      >
                        <Trash2 className="w-3 h-3" />
                      </Button>
                    </div>
                  </div>
                );
              })}

              {environments.length === 0 && (
                <div className="text-center py-8 px-2 text-xs text-zinc-400">
                  No environments found. Click <strong>+ New</strong> to create one.
                </div>
              )}
            </div>
          </div>

          {/* Right column: Editor & Variable Table */}
          <div className="flex-1 flex flex-col overflow-hidden bg-white dark:bg-zinc-900">
            {editingEnv ? (
              <div className="flex-1 flex flex-col overflow-hidden">
                {/* Top Action Bar */}
                <div className="p-4 border-b border-zinc-200 dark:border-zinc-800 flex items-center justify-between gap-4 shrink-0">
                  <div className="flex items-center gap-3 flex-1 max-w-sm">
                    <label className="text-xs font-semibold text-zinc-500 shrink-0">
                      Name:
                    </label>
                    <Input
                      type="text"
                      value={editingEnv.name}
                      onChange={(e) => {
                        setEditingEnv({ ...editingEnv, name: e.target.value });
                        setSaveSuccess(false);
                      }}
                      className="h-8 text-xs font-mono"
                      placeholder="e.g. production, staging, dev"
                    />
                  </div>

                  <div className="flex items-center gap-2">
                    {activeEnv !== editingEnv.name && (
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => setActiveEnv(editingEnv.name)}
                        className="h-8 text-xs font-medium cursor-pointer"
                      >
                        Set as Active
                      </Button>
                    )}
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => handleDuplicateEnv(editingEnv)}
                      className="h-8 text-xs gap-1 font-medium cursor-pointer"
                    >
                      <Copy className="w-3.5 h-3.5" />
                      Duplicate
                    </Button>
                    <Button
                      variant={saveSuccess ? "success" : "default"}
                      size="sm"
                      onClick={handleSaveCurrentEnv}
                      disabled={isSaving}
                      className="h-8 text-xs gap-1.5 font-medium cursor-pointer"
                    >
                      {saveSuccess ? (
                        <>
                          <Check className="w-3.5 h-3.5" />
                          Saved
                        </>
                      ) : (
                        <>
                          <Save className="w-3.5 h-3.5" />
                          {isSaving ? "Saving..." : "Save Changes"}
                        </>
                      )}
                    </Button>
                  </div>
                </div>

                {/* Error Banner */}
                {errorMessage && (
                  <div className="px-4 py-2 bg-rose-500/10 border-b border-rose-500/20 text-rose-600 dark:text-rose-400 text-xs flex items-center gap-2">
                    <AlertCircle className="w-4 h-4 shrink-0" />
                    <span>{errorMessage}</span>
                  </div>
                )}

                {/* Variable Table Header & Action */}
                <div className="px-4 py-2.5 bg-zinc-50 dark:bg-zinc-950/40 border-b border-zinc-200 dark:border-zinc-800 flex items-center justify-between text-xs text-zinc-500">
                  <div className="font-semibold text-zinc-700 dark:text-zinc-300">
                    Variables ({editingEnv.variables?.length || 0})
                  </div>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={handleAddVariable}
                    className="h-7 text-xs gap-1 text-emerald-600 dark:text-emerald-400 font-medium hover:bg-emerald-500/10 cursor-pointer"
                  >
                    <Plus className="w-3.5 h-3.5" />
                    Add Variable
                  </Button>
                </div>

                {/* Variables List / Table */}
                <div className="flex-1 overflow-y-auto p-4 space-y-2">
                  {(editingEnv.variables || []).length > 0 ? (
                    <div className="border border-zinc-200 dark:border-zinc-800 rounded-lg overflow-hidden">
                      <table className="w-full text-xs text-left">
                        <thead className="bg-zinc-100 dark:bg-zinc-800/60 text-zinc-500 border-b border-zinc-200 dark:border-zinc-800">
                          <tr>
                            <th className="w-10 px-3 py-2 text-center">✓</th>
                            <th className="w-1/3 px-3 py-2 font-medium">Variable Key</th>
                            <th className="px-3 py-2 font-medium">Value</th>
                            <th className="w-20 px-3 py-2 text-center font-medium">Secret</th>
                            <th className="w-10 px-2 py-2"></th>
                          </tr>
                        </thead>
                        <tbody className="divide-y divide-zinc-200 dark:divide-zinc-800">
                          {editingEnv.variables.map((kv, idx) => {
                            const isSecret = Boolean(kv.secret);
                            const isRevealed = Boolean(revealedSecrets[idx]);

                            return (
                              <tr
                                key={idx}
                                className="hover:bg-zinc-50 dark:hover:bg-zinc-800/40 group"
                              >
                                {/* Enabled Checkbox */}
                                <td className="px-3 py-1.5 text-center">
                                  <Checkbox
                                    checked={kv.enabled}
                                    onCheckedChange={(checked) =>
                                      handleUpdateVariable(idx, "enabled", Boolean(checked))
                                    }
                                  />
                                </td>

                                {/* Key Input */}
                                <td className="px-2 py-1.5">
                                  <Input
                                    type="text"
                                    value={kv.key}
                                    onChange={(e) =>
                                      handleUpdateVariable(idx, "key", e.target.value)
                                    }
                                    placeholder="KEY_NAME"
                                    className="h-7 text-xs font-mono"
                                  />
                                </td>

                                {/* Value Input with Reveal/Mask button */}
                                <td className="px-2 py-1.5">
                                  <div className="relative flex items-center">
                                    <Input
                                      type={isSecret && !isRevealed ? "password" : "text"}
                                      value={kv.value}
                                      onChange={(e) =>
                                        handleUpdateVariable(idx, "value", e.target.value)
                                      }
                                      placeholder={isSecret ? "•••••••• (secret value)" : "value"}
                                      className={`h-7 text-xs font-mono pr-8 ${
                                        isSecret
                                          ? "border-amber-500/30 focus-visible:ring-amber-500/30"
                                          : ""
                                      }`}
                                    />
                                    {isSecret && (
                                      <Button
                                        type="button"
                                        variant="ghost"
                                        size="icon"
                                        onClick={() => toggleRevealSecret(idx)}
                                        className="absolute right-1 h-5 w-5 text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-200"
                                        title={isRevealed ? "Mask Value" : "Reveal Value"}
                                      >
                                        {isRevealed ? (
                                          <EyeOff className="w-3 h-3" />
                                        ) : (
                                          <Eye className="w-3 h-3" />
                                        )}
                                      </Button>
                                    )}
                                  </div>
                                </td>

                                {/* Secret Checkbox */}
                                <td className="px-2 py-1.5 text-center">
                                  <Tooltip
                                    content={
                                      isSecret
                                        ? "Secret variable: saved to *.secret.env.json and hidden"
                                        : "Public variable: saved to *.env.json"
                                    }
                                  >
                                    <label className="inline-flex items-center gap-1 cursor-pointer select-none">
                                      <Checkbox
                                        checked={isSecret}
                                        onCheckedChange={(checked) =>
                                          handleUpdateVariable(idx, "secret", Boolean(checked))
                                        }
                                      />
                                      {isSecret && (
                                        <Lock className="w-3 h-3 text-amber-500 shrink-0" />
                                      )}
                                    </label>
                                  </Tooltip>
                                </td>

                                {/* Delete Row Action */}
                                <td className="px-2 py-1.5 text-right">
                                  <Button
                                    variant="ghost"
                                    size="icon"
                                    onClick={() => handleDeleteVariable(idx)}
                                    className="h-6 w-6 text-zinc-400 hover:text-rose-600 dark:hover:text-rose-400 opacity-60 group-hover:opacity-100"
                                    title="Delete variable"
                                  >
                                    <Trash2 className="w-3 h-3" />
                                  </Button>
                                </td>
                              </tr>
                            );
                          })}
                        </tbody>
                      </table>
                    </div>
                  ) : (
                    <div className="border border-dashed border-zinc-200 dark:border-zinc-800 rounded-lg p-8 text-center text-xs text-zinc-400 flex flex-col items-center justify-center gap-2">
                      <p>No variables configured for this environment.</p>
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={handleAddVariable}
                        className="h-7 text-xs gap-1 font-medium cursor-pointer"
                      >
                        <Plus className="w-3.5 h-3.5" />
                        Add First Variable
                      </Button>
                    </div>
                  )}

                  {/* Migration Success / Info Banner */}
                  {migrationMessage && (
                    <div className="mt-4 p-3 bg-emerald-50 dark:bg-emerald-950/30 rounded-lg border border-emerald-200 dark:border-emerald-800 text-[11px] text-emerald-700 dark:text-emerald-300 flex items-start gap-2">
                      <ShieldCheck className="w-4 h-4 text-emerald-500 shrink-0 mt-0.5" />
                      <div>{migrationMessage}</div>
                    </div>
                  )}

                  {/* Informational & Secret Backend Footer */}
                  <div className="mt-4 p-3 bg-zinc-50 dark:bg-zinc-950/30 rounded-lg border border-zinc-200 dark:border-zinc-800 text-[11px] text-zinc-500 dark:text-zinc-400 flex flex-col gap-2">
                    <div className="flex items-start justify-between gap-2">
                      <div className="flex items-start gap-2">
                        <Lock className="w-3.5 h-3.5 text-amber-500 shrink-0 mt-0.5" />
                        <div>
                          <strong className="text-zinc-700 dark:text-zinc-300">
                            Secret Storage:
                          </strong>{" "}
                          Current backend for <code>{editingEnv.name}</code>:{" "}
                          <Badge variant="outline" className="text-[10px] uppercase font-mono tracking-wider ml-1 py-0">
                            {backendConfig?.perEnv?.[editingEnv.name] || backendConfig?.default || "file"}
                          </Badge>
                        </div>
                      </div>

                      {/* Migration Actions */}
                      <div className="flex items-center gap-1.5 shrink-0">
                        {(backendConfig?.perEnv?.[editingEnv.name] || backendConfig?.default) === "keychain" ? (
                          <Button
                            type="button"
                            variant="outline"
                            size="sm"
                            disabled={isMigrating}
                            onClick={() => handleMigrate("file")}
                            className="h-6 text-[10px] px-2 gap-1 cursor-pointer"
                          >
                            <KeyRound className="w-3 h-3 text-amber-500" />
                            Migrate to File
                          </Button>
                        ) : (
                          <Button
                            type="button"
                            variant="outline"
                            size="sm"
                            disabled={isMigrating}
                            onClick={() => handleMigrate("keychain")}
                            className="h-6 text-[10px] px-2 gap-1 cursor-pointer"
                          >
                            <ShieldCheck className="w-3 h-3 text-indigo-500" />
                            Migrate to OS Keychain
                          </Button>
                        )}
                      </div>
                    </div>

                    <div className="text-[10px] text-zinc-400 border-t border-zinc-200 dark:border-zinc-800/60 pt-2">
                      Non-secret variables are stored in <code className="text-emerald-600 dark:text-emerald-400">{editingEnv.name}.env.json</code>.
                      Secrets are stored in <code className="text-amber-600 dark:text-amber-400">{editingEnv.name}.secret.env.json</code> (with file backend) or encrypted in OS Keychain.
                    </div>
                  </div>
                </div>
              </div>
            ) : (
              <div className="flex-1 flex flex-col items-center justify-center text-zinc-400 text-xs gap-2">
                <Layers className="w-8 h-8 opacity-40" />
                <p>Select an environment from the sidebar or create a new one.</p>
              </div>
            )}
          </div>
        </div>

        {/* Footer */}
        <div className="px-5 py-3 border-t border-zinc-200 dark:border-zinc-800 flex items-center justify-between shrink-0 bg-zinc-50/50 dark:bg-zinc-950/20 text-xs">
          <span className="text-zinc-400">
            Reference variables in requests with{" "}
            <code className="px-1 py-0.5 bg-zinc-200 dark:bg-zinc-800 rounded font-mono text-[11px]">
              {"{{VARIABLE_NAME}}"}
            </code>
          </span>
          <Button variant="outline" size="sm" onClick={onClose} className="h-8 px-4 cursor-pointer">
            Close
          </Button>
        </div>
      </div>
    </div>
  );
}
