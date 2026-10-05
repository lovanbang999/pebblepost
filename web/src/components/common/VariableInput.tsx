import React, { useState, useRef, useEffect, useMemo } from "react";
import { Lock, Sparkles, ExternalLink, AlertTriangle, CheckCircle2 } from "lucide-react";
import { useWorkspaceStore } from "../../store/workspaceStore";
import {
  getAvailableVariables,
  tokenizeVariables,
  VariableInfo,
  TokenPart,
} from "../../lib/variables";

import { Button } from "../ui/button";

interface VariableInputProps
  extends Omit<React.InputHTMLAttributes<HTMLInputElement>, "onChange"> {
  value: string;
  onChange: (value: string) => void;
  onOpenManageEnvironments?: () => void;
  placeholder?: string;
  className?: string;
}

export function VariableInput({
  value = "",
  onChange,
  onOpenManageEnvironments,
  placeholder,
  className = "",
  ...props
}: VariableInputProps) {
  const { environments, activeEnv } = useWorkspaceStore();
  const inputRef = useRef<HTMLInputElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const backdropRef = useRef<HTMLDivElement>(null);

  const handleScroll = () => {
    if (inputRef.current && backdropRef.current) {
      backdropRef.current.scrollLeft = inputRef.current.scrollLeft;
    }
  };

  const [isFocused, setIsFocused] = useState(false);

  const [hoveredVar, setHoveredVar] = useState<{
    key: string;
    info?: VariableInfo;
    rect: DOMRect;
  } | null>(null);

  const hoverTimeoutRef = useRef<NodeJS.Timeout | null>(null);

  const handleBadgeMouseEnter = (token: TokenPart, e: React.MouseEvent<HTMLElement>) => {
    if (hoverTimeoutRef.current) {
      clearTimeout(hoverTimeoutRef.current);
      hoverTimeoutRef.current = null;
    }
    const rect = e.currentTarget.getBoundingClientRect();
    setHoveredVar({
      key: token.varKey || "",
      info: token.varInfo,
      rect,
    });
  };

  const handleBadgeMouseLeave = () => {
    if (hoverTimeoutRef.current) {
      clearTimeout(hoverTimeoutRef.current);
    }
    hoverTimeoutRef.current = setTimeout(() => {
      setHoveredVar(null);
    }, 250);
  };

  const handleTooltipMouseEnter = () => {
    if (hoverTimeoutRef.current) {
      clearTimeout(hoverTimeoutRef.current);
      hoverTimeoutRef.current = null;
    }
  };

  const handleTooltipMouseLeave = () => {
    if (hoverTimeoutRef.current) {
      clearTimeout(hoverTimeoutRef.current);
    }
    hoverTimeoutRef.current = setTimeout(() => {
      setHoveredVar(null);
    }, 150);
  };

  useEffect(() => {
    return () => {
      if (hoverTimeoutRef.current) {
        clearTimeout(hoverTimeoutRef.current);
      }
    };
  }, []);

  // Autocomplete state

  const [autocompleteOpen, setAutocompleteOpen] = useState(false);
  const [autocompleteQuery, setAutocompleteQuery] = useState("");
  const [autocompleteIndex, setAutocompleteIndex] = useState(0);
  const [cursorPosition, setCursorPosition] = useState(0);

  // Build available variables map
  const availableMap = useMemo(() => {
    return getAvailableVariables(environments, activeEnv);
  }, [environments, activeEnv]);

  // Tokenize text into literals and {{VAR}}
  const tokens = useMemo(() => {
    return tokenizeVariables(value, availableMap);
  }, [value, availableMap]);

  // Check for autocomplete trigger upon cursor or text change
  useEffect(() => {
    if (!isFocused || !inputRef.current) {
      setAutocompleteOpen(false);
      return;
    }

    const pos = inputRef.current.selectionStart || 0;
    setCursorPosition(pos);
    const textBeforeCursor = value.slice(0, pos);

    // Look for unclosed "{{" before the cursor
    const lastOpen = textBeforeCursor.lastIndexOf("{{");
    const lastClose = textBeforeCursor.lastIndexOf("}}");

    if (lastOpen !== -1 && lastOpen > lastClose) {
      const query = textBeforeCursor.slice(lastOpen + 2).trim();
      setAutocompleteQuery(query);
      setAutocompleteIndex(0);
      setAutocompleteOpen(true);
    } else {
      setAutocompleteOpen(false);
    }
  }, [value, isFocused]);

  // Filtered autocomplete options
  const filteredOptions = useMemo(() => {
    const list = Array.from(availableMap.values());
    if (!autocompleteQuery) return list;
    const q = autocompleteQuery.toLowerCase();
    return list.filter(
      (v) =>
        v.key.toLowerCase().includes(q) ||
        (v.value && v.value.toLowerCase().includes(q))
    );
  }, [availableMap, autocompleteQuery]);

  // Insert selected variable from autocomplete
  const applyAutocomplete = (selectedKey: string) => {
    if (!inputRef.current) return;
    const pos = cursorPosition;
    const textBefore = value.slice(0, pos);
    const lastOpen = textBefore.lastIndexOf("{{");

    if (lastOpen === -1) return;

    // Check if there is already a closing "}}" immediately after cursor
    const textAfter = value.slice(pos);
    let afterReplacement = textAfter;
    if (afterReplacement.startsWith("}}")) {
      afterReplacement = afterReplacement.slice(2);
    }

    const nextValue = `${value.slice(0, lastOpen)}{{${selectedKey}}}${afterReplacement}`;
    onChange(nextValue);
    setAutocompleteOpen(false);

    // Restore focus and position cursor after "}}"
    const newCursor = lastOpen + selectedKey.length + 4;
    setTimeout(() => {
      if (inputRef.current) {
        inputRef.current.focus();
        inputRef.current.setSelectionRange(newCursor, newCursor);
      }
    }, 10);
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (autocompleteOpen && filteredOptions.length > 0) {
      if (e.key === "ArrowDown") {
        e.preventDefault();
        setAutocompleteIndex((prev) => (prev + 1) % filteredOptions.length);
        return;
      }
      if (e.key === "ArrowUp") {
        e.preventDefault();
        setAutocompleteIndex((prev) =>
          prev === 0 ? filteredOptions.length - 1 : prev - 1
        );
        return;
      }
      if (e.key === "Enter" || e.key === "Tab") {
        e.preventDefault();
        const selected = filteredOptions[autocompleteIndex];
        if (selected) {
          applyAutocomplete(selected.key);
        }
        return;
      }
      if (e.key === "Escape") {
        e.preventDefault();
        setAutocompleteOpen(false);
        return;
      }
    }

    props.onKeyDown?.(e);
  };

  // Check if input has any variable tokens
  const hasVariables = value.includes("{{");

  return (
    <div
      ref={containerRef}
      className="relative flex items-center w-full font-mono text-xs"
    >
      {/* Backdrop: Highlights {{VAR}} in green/red */}
      {hasVariables && (
        <div
          ref={backdropRef}
          aria-hidden="true"
          className="absolute inset-0 pointer-events-none flex items-center px-3 py-1.5 overflow-hidden whitespace-pre select-none font-mono text-xs leading-normal"
        >
          {tokens.map((token, idx) => {
            if (token.type === "text") {
              return (
                <span key={idx} className="text-zinc-900 dark:text-zinc-100">
                  {token.content}
                </span>
              );
            }

            const isDefined = token.isDefined;
            const isSecret = token.varInfo?.secret;

            return (
              <span
                key={idx}
                onMouseEnter={(e) => handleBadgeMouseEnter(token, e)}
                onMouseLeave={handleBadgeMouseLeave}
                className={`pointer-events-auto inline-flex items-center px-1 rounded transition-colors text-xs font-mono ${
                  isDefined
                    ? "bg-emerald-500/15 text-emerald-700 dark:text-emerald-300 border border-emerald-500/30 font-semibold"
                    : "bg-rose-500/15 text-rose-700 dark:text-rose-300 border border-rose-500/30 font-semibold"
                }`}
              >
                {isSecret && <Lock className="w-2.5 h-2.5 mr-0.5 inline opacity-70" />}
                {token.content}
              </span>
            );
          })}
        </div>
      )}

      {/* Actual Input */}
      <input
        ref={inputRef}
        type="text"
        value={value}
        onChange={(e) => {
          onChange(e.target.value);
          handleScroll();
        }}
        onScroll={handleScroll}
        onFocus={() => {
          setIsFocused(true);
          handleScroll();
        }}
        onBlur={() => {
          setIsFocused(false);
          // Small timeout so click on autocomplete option can trigger
          setTimeout(() => setAutocompleteOpen(false), 200);
        }}
        onKeyDown={handleKeyDown}
        placeholder={placeholder}
        className={`w-full h-8 px-3 py-1.5 rounded-md border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 font-mono text-xs outline-none focus:ring-1 focus:ring-blue-500/50 transition-colors ${
          hasVariables ? "text-transparent caret-zinc-900 dark:caret-zinc-100 selection:bg-blue-500/25 selection:text-transparent" : "text-zinc-900 dark:text-zinc-100"
        } ${className}`}
        {...props}
      />


      {/* Hover Tooltip for {{VAR}} */}
      {hoveredVar && (
        <div
          onMouseEnter={handleTooltipMouseEnter}
          onMouseLeave={handleTooltipMouseLeave}
          style={{
            position: "fixed",
            left: `${hoveredVar.rect.left}px`,
            top: `${hoveredVar.rect.bottom + 6}px`,
            zIndex: 9999,
          }}
          className="bg-zinc-900 text-zinc-100 dark:bg-zinc-950 dark:border dark:border-zinc-800 rounded-lg shadow-xl p-3 text-xs w-72 animate-in fade-in duration-100 pointer-events-auto before:absolute before:-top-3 before:left-0 before:right-0 before:h-3 before:content-['']"
        >
          <div className="flex items-center justify-between pb-1.5 border-b border-zinc-800">
            <span className="font-mono font-bold text-xs flex items-center gap-1.5">
              {hoveredVar.info ? (
                <CheckCircle2 className="w-3.5 h-3.5 text-emerald-400" />
              ) : (
                <AlertTriangle className="w-3.5 h-3.5 text-rose-400" />
              )}
              {`{{${hoveredVar.key}}}`}
            </span>
            <span
              className={`text-[10px] px-1.5 py-0.5 rounded uppercase font-semibold ${
                hoveredVar.info
                  ? "bg-emerald-500/20 text-emerald-300"
                  : "bg-rose-500/20 text-rose-300"
              }`}
            >
              {hoveredVar.info ? "Defined" : "Undefined"}
            </span>
          </div>

          <div className="pt-2 space-y-1.5">
            {hoveredVar.info ? (
              <>
                <div className="text-[11px] text-zinc-400">
                  <span className="text-zinc-500">Source:</span>{" "}
                  <span className="text-zinc-200">{hoveredVar.info.source}</span>
                </div>
                <div className="text-[11px]">
                  <span className="text-zinc-500">Value:</span>{" "}
                  <span className="font-mono text-zinc-200 bg-zinc-800 px-1 py-0.5 rounded break-all">
                    {hoveredVar.info.secret
                      ? "•••••••• (secret)"
                      : hoveredVar.info.value || "(empty)"}
                  </span>
                </div>
              </>
            ) : (
              <p className="text-[11px] text-rose-300">
                This variable is not defined in the active environment (
                <strong>{activeEnv || "none"}</strong>).
              </p>
            )}

            {onOpenManageEnvironments && (
              <div className="pt-2 border-t border-zinc-800/80 flex justify-end">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => {
                    if (hoverTimeoutRef.current) {
                      clearTimeout(hoverTimeoutRef.current);
                      hoverTimeoutRef.current = null;
                    }
                    setHoveredVar(null);
                    onOpenManageEnvironments();
                  }}
                  className="h-6 text-[11px] gap-1 px-2.5 bg-zinc-800 hover:bg-zinc-700 border-zinc-700 hover:border-zinc-600 text-zinc-200 hover:text-white transition-colors cursor-pointer"
                >
                  <ExternalLink className="w-3 h-3" />
                  Open Definition
                </Button>
              </div>
            )}

          </div>
        </div>
      )}


      {/* Autocomplete Menu Popover */}
      {autocompleteOpen && filteredOptions.length > 0 && (
        <div
          className="absolute left-0 top-full mt-1 w-80 max-h-56 overflow-y-auto bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 rounded-lg shadow-xl z-50 p-1 divide-y divide-zinc-100 dark:divide-zinc-800/60"
        >
          <div className="px-2 py-1 text-[10px] uppercase font-semibold text-zinc-400 tracking-wider flex items-center justify-between">
            <span>Variables ({filteredOptions.length})</span>
            <span className="text-[9px] text-zinc-500 font-normal">↑↓ to navigate, Enter to select</span>
          </div>
          <div className="py-0.5 space-y-0.5">
            {filteredOptions.map((opt, idx) => {
              const isSelected = idx === autocompleteIndex;
              return (
                <div
                  key={opt.key}
                  onMouseDown={(e) => {
                    e.preventDefault();
                    applyAutocomplete(opt.key);
                  }}
                  className={`px-2.5 py-1.5 rounded text-xs cursor-pointer flex items-center justify-between gap-2 transition-colors ${
                    isSelected
                      ? "bg-zinc-100 dark:bg-zinc-800 text-zinc-900 dark:text-zinc-100 font-medium"
                      : "text-zinc-600 dark:text-zinc-400 hover:bg-zinc-50 dark:hover:bg-zinc-800/50"
                  }`}
                >
                  <div className="flex items-center gap-1.5 truncate">
                    {opt.isDynamic ? (
                      <Sparkles className="w-3 h-3 text-amber-500 shrink-0" />
                    ) : opt.secret ? (
                      <Lock className="w-3 h-3 text-amber-500 shrink-0" />
                    ) : (
                      <span className="w-1.5 h-1.5 rounded-full bg-emerald-500 shrink-0" />
                    )}
                    <span className="font-mono text-zinc-900 dark:text-zinc-100 truncate">
                      {opt.key}
                    </span>
                  </div>
                  <div className="flex items-center gap-1.5 shrink-0">
                    <span className="text-[10px] text-zinc-400 max-w-25 truncate font-mono">
                      {opt.secret ? "••••" : opt.value}
                    </span>
                    <span className="text-[9px] px-1 py-0.2 rounded bg-zinc-200 dark:bg-zinc-800 text-zinc-500">
                      {opt.source.split(" ")[0]}
                    </span>
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}
