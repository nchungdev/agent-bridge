import React, { useEffect, useRef, useState } from "react";
import { GitBranch, Globe, Plus, Terminal as TerminalIcon, X, ChevronsRight } from "lucide-react";
import { TerminalPanel } from "./TerminalPanel";
import { BrowserPanel } from "./BrowserPanel";
import { GitDiffPanel } from "./GitDiffPanel";

export type DockKind = "terminal" | "browser" | "changes";
export interface DockTab { id: string; kind: DockKind; n: number }

const KIND: Record<DockKind, { label: string; icon: React.ElementType; color: string }> = {
  terminal: { label: "Terminal", icon: TerminalIcon, color: "text-sky-400" },
  browser: { label: "Browser", icon: Globe, color: "text-amber-400" },
  changes: { label: "Changes", icon: GitBranch, color: "text-emerald-400" },
};

interface Props {
  tabs: DockTab[];
  activeId: string | null;
  visible: boolean;
  workDir: string;
  onSelect: (id: string) => void;
  onClose: (id: string) => void;
  onNew: (kind: DockKind) => void;
  onHide: () => void;
}

/** Tabbed right-hand dock: any number of terminals, browsers and change views; every tab stays alive in the background. */
export const RightDock: React.FC<Props> = ({ tabs, activeId, visible, workDir, onSelect, onClose, onNew, onHide }) => {
  const [menu, setMenu] = useState(false);
  const [titles, setTitles] = useState<Record<string, string>>({});
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!menu) return;
    const off = (e: MouseEvent) => { if (menuRef.current && !menuRef.current.contains(e.target as Node)) setMenu(false); };
    document.addEventListener("mousedown", off);
    return () => document.removeEventListener("mousedown", off);
  }, [menu]);

  const titleOf = (t: DockTab) => titles[t.id] || (t.kind === "changes" ? "Changes" : `${KIND[t.kind].label} ${t.n}`);

  return (
    <div className="flex h-full w-full flex-col">
      <div className="flex h-9 shrink-0 items-stretch border-b border-[#1d222b] bg-[#101319]">
        <div className="flex min-w-0 flex-1 items-stretch overflow-x-auto">
          {tabs.map((t) => {
            const { icon: Icon, color } = KIND[t.kind];
            const active = t.id === activeId;
            return (
              <div
                key={t.id}
                onClick={() => onSelect(t.id)}
                onAuxClick={(e) => { if (e.button === 1) onClose(t.id); }}
                className={`group flex max-w-[11rem] shrink-0 cursor-pointer items-center gap-1.5 border-r border-[#1d222b] pl-2.5 pr-1 text-[12px] ${active ? "bg-[#14171e] text-slate-100" : "text-slate-500 hover:bg-[#12161d] hover:text-slate-300"}`}
                title={titleOf(t)}
              >
                <Icon className={`h-3.5 w-3.5 shrink-0 ${color}`} />
                <span className="truncate">{titleOf(t)}</span>
                <button
                  type="button"
                  onClick={(e) => { e.stopPropagation(); onClose(t.id); }}
                  className={`rounded p-0.5 text-slate-500 hover:bg-[#242b39] hover:text-slate-100 cursor-pointer ${active ? "" : "opacity-0 group-hover:opacity-100"}`}
                  aria-label={`Close ${titleOf(t)}`}
                >
                  <X className="h-3 w-3" />
                </button>
              </div>
            );
          })}
        </div>
        <div className="relative flex shrink-0 items-center gap-0.5 px-1" ref={menuRef}>
          <button type="button" onClick={() => setMenu((m) => !m)} className="rounded p-1.5 text-slate-400 hover:bg-[#1f2533] hover:text-slate-100 cursor-pointer" title="New tab" aria-label="New tab">
            <Plus className="h-3.5 w-3.5" />
          </button>
          <button type="button" onClick={onHide} className="rounded p-1.5 text-slate-500 hover:bg-[#1f2533] hover:text-slate-100 cursor-pointer" title="Hide panel (tabs keep running)" aria-label="Hide panel">
            <ChevronsRight className="h-3.5 w-3.5" />
          </button>
          {menu && (
            <div className="absolute right-0 top-full z-50 mt-1 w-44 rounded-lg border border-[#2c3344] bg-[#171b23] py-1 shadow-2xl">
              {(Object.keys(KIND) as DockKind[]).map((k) => {
                const { icon: Icon, color, label } = KIND[k];
                return (
                  <button key={k} type="button" onClick={() => { onNew(k); setMenu(false); }} className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[12.5px] text-slate-200 hover:bg-[#222836] cursor-pointer">
                    <Icon className={`h-3.5 w-3.5 ${color}`} />New {label.toLowerCase()}
                  </button>
                );
              })}
            </div>
          )}
        </div>
      </div>

      <div className="relative min-h-0 flex-1">
        {tabs.map((t) => {
          const active = t.id === activeId;
          return (
            <div key={t.id} className={active ? "flex h-full flex-col" : "hidden"}>
              {t.kind === "terminal" && <TerminalPanel workDir={workDir} visible={visible && active} onClose={() => onClose(t.id)} />}
              {t.kind === "browser" && <BrowserPanel defaultUrl="" onClose={() => onClose(t.id)} onTitle={(title) => setTitles((m) => (m[t.id] === title ? m : { ...m, [t.id]: title }))} />}
              {t.kind === "changes" && <GitDiffPanel currentPath={workDir} onClose={() => onClose(t.id)} />}
            </div>
          );
        })}
      </div>
    </div>
  );
};
