import {
  MatchDecorator,
  ViewPlugin,
  Decoration,
  EditorView,
  hoverTooltip,
  ViewUpdate,
} from "@codemirror/view";
import { autocompletion, CompletionContext } from "@codemirror/autocomplete";
import type { VariableInfo } from "./variables";

export const variableStyles = EditorView.baseTheme({
  ".cm-var-defined": {
    backgroundColor: "rgba(16, 185, 129, 0.15)",
    color: "#059669",
    border: "1px solid rgba(16, 185, 129, 0.3)",
    borderRadius: "3px",
    padding: "0 2px",
    fontWeight: "600",
  },
  ".dark .cm-var-defined": {
    backgroundColor: "rgba(16, 185, 129, 0.2)",
    color: "#34d399",
    border: "1px solid rgba(16, 185, 129, 0.4)",
  },
  ".cm-var-undefined": {
    backgroundColor: "rgba(244, 63, 94, 0.15)",
    color: "#e11d48",
    border: "1px solid rgba(244, 63, 94, 0.3)",
    borderRadius: "3px",
    padding: "0 2px",
    fontWeight: "600",
  },
  ".dark .cm-var-undefined": {
    backgroundColor: "rgba(244, 63, 94, 0.2)",
    color: "#fb7185",
    border: "1px solid rgba(244, 63, 94, 0.4)",
  },
});

export function createVariableHighlightExtension(
  getAvailableMap: () => Map<string, VariableInfo>
) {
  const decorator = new MatchDecorator({
    regexp: /\{\{\s*([a-zA-Z0-9_$.-]+)\s*\}\}/g,
    decoration: (match) => {
      const key = match[1].trim();
      const map = getAvailableMap();
      const isDefined = map.has(key);
      return Decoration.mark({
        class: isDefined ? "cm-var-defined" : "cm-var-undefined",
      });
    },
  });

  return ViewPlugin.fromClass(
    class {
      decorations;
      constructor(view: EditorView) {
        this.decorations = decorator.createDeco(view);
      }
      update(update: ViewUpdate) {
        if (update.docChanged || update.viewportChanged) {
          this.decorations = decorator.updateDeco(update, this.decorations);
        }
      }
    },
    {
      decorations: (v) => v.decorations,
    }
  );
}

export function createVariableHoverTooltip(
  getAvailableMap: () => Map<string, VariableInfo>,
  onOpenManageEnvironments?: () => void
) {
  return hoverTooltip((view, pos) => {
    const { from, text } = view.state.doc.lineAt(pos);
    const lineOffset = from;

    const regex = /\{\{\s*([a-zA-Z0-9_$.-]+)\s*\}\}/g;
    let match: RegExpExecArray | null;
    while ((match = regex.exec(text)) !== null) {
      const start = lineOffset + match.index;
      const end = start + match[0].length;
      if (pos >= start && pos <= end) {
        const key = match[1].trim();
        const map = getAvailableMap();
        const info = map.get(key);

        return {
          pos: start,
          end: end,
          above: true,
          create() {
            const dom = document.createElement("div");
            dom.className =
              "p-2.5 text-xs bg-zinc-900 text-zinc-100 rounded-lg shadow-xl border border-zinc-800 max-w-xs font-sans";

            const header = document.createElement("div");
            header.className =
              "flex items-center justify-between gap-3 pb-1 border-b border-zinc-800 font-mono text-[11px] font-bold";
            header.innerHTML = `<span>{{${key}}}</span><span class="${
              info ? "text-emerald-400" : "text-rose-400"
            }">${info ? "Defined" : "Undefined"}</span>`;
            dom.appendChild(header);

            const body = document.createElement("div");
            body.className = "pt-1.5 space-y-1 text-[11px]";

            if (info) {
              const src = document.createElement("div");
              src.className = "text-zinc-400";
              src.innerHTML = `Source: <span class="text-zinc-200">${info.source}</span>`;
              body.appendChild(src);

              const val = document.createElement("div");
              val.className = "text-zinc-400";
              const dispVal = info.secret ? "•••••••• (secret)" : info.value || "(empty)";
              val.innerHTML = `Value: <span class="font-mono text-zinc-200 bg-zinc-800 px-1 py-0.5 rounded break-all">${dispVal}</span>`;
              body.appendChild(val);
            } else {
              const undef = document.createElement("p");
              undef.className = "text-rose-400";
              undef.innerText = "Variable is not defined in active environment.";
              body.appendChild(undef);
            }

            if (onOpenManageEnvironments) {
              const btn = document.createElement("button");
              btn.className =
                "mt-2 w-full py-1 px-2 rounded text-[11px] font-medium bg-zinc-800 hover:bg-zinc-700 text-zinc-200 cursor-pointer";
              btn.innerText = "Open Definition";
              btn.onclick = () => onOpenManageEnvironments();
              body.appendChild(btn);
            }

            dom.appendChild(body);
            return { dom };
          },
        };
      }
    }
    return null;
  });
}

export function createVariableAutocomplete(
  getAvailableMap: () => Map<string, VariableInfo>
) {
  return autocompletion({
    override: [
      (context: CompletionContext) => {
        const word = context.matchBefore(/\{\{\s*[\w$.-]*/);
        if (!word) return null;

        const map = getAvailableMap();
        return {
          from: word.from,
          options: Array.from(map.values()).map((v) => ({
            label: `{{${v.key}}}`,
            detail: v.source,
            info: v.secret ? "••••••••" : v.value,
            type: v.isDynamic ? "keyword" : "variable",
          })),
        };
      },
    ],
  });
}
