import React, { useState } from "react";
import { Sparkles, Terminal, Code2, Edit2, Check } from "lucide-react";

export interface ToolSwitcherProps {
  activeAgent: string;
  onSelectTool: (agentId: "agy" | "claude" | "codex") => void;
  aliases?: Record<string, string>;
  onUpdateAlias?: (agentId: string, alias: string) => void;
}

export const ToolSwitcher: React.FC<ToolSwitcherProps> = ({
  activeAgent,
  onSelectTool,
  aliases = {},
  onUpdateAlias,
}) => {
  const [editingId, setEditingId] = useState<string | null>(null);
  const [tempAlias, setTempAlias] = useState("");

  const defaultTools = [
    {
      id: "agy" as const,
      defaultName: "Antigravity",
      icon: Sparkles,
      color: "text-sky-400",
      activeBg: "bg-[#1c2433] text-sky-300 border-sky-500/30",
      statusDot: "bg-emerald-400",
    },
    {
      id: "claude" as const,
      defaultName: "Claude Code",
      icon: Terminal,
      color: "text-purple-400",
      activeBg: "bg-[#251e30] text-purple-300 border-purple-500/30",
      statusDot: "bg-purple-400",
    },
    {
      id: "codex" as const,
      defaultName: "OpenAI Codex",
      icon: Code2,
      color: "text-emerald-400",
      activeBg: "bg-[#192724] text-emerald-300 border-emerald-500/30",
      statusDot: "bg-emerald-400",
    },
  ];

  const handleStartEdit = (e: React.MouseEvent, id: string, currentName: string) => {
    e.stopPropagation();
    setEditingId(id);
    setTempAlias(currentName);
  };

  const handleSaveEdit = (e: React.MouseEvent | React.KeyboardEvent, id: string) => {
    e.stopPropagation();
    if (onUpdateAlias) {
      onUpdateAlias(id, tempAlias.trim());
    }
    setEditingId(null);
  };

  return (
    <div className="flex items-center p-0.5 rounded-lg bg-[#141821] border border-[#232a38] text-xs">
      {defaultTools.map((t) => {
        const isActive = activeAgent === t.id;
        const Icon = t.icon;
        const displayName = aliases[t.id] || t.defaultName;
        const isEditing = editingId === t.id;

        return (
          <div
            key={t.id}
            onClick={() => onSelectTool(t.id)}
            className={`group flex items-center gap-1.5 px-2.5 py-1 rounded-md font-medium transition-all cursor-pointer select-none ${
              isActive
                ? `${t.activeBg} border shadow-sm`
                : "text-slate-400 hover:text-slate-200 hover:bg-[#1a1f2c]"
            }`}
          >
            <Icon className={`w-3.5 h-3.5 ${isActive ? t.color : "text-slate-500"}`} />

            {isEditing ? (
              <div
                className="flex items-center gap-1"
                onClick={(e) => e.stopPropagation()}
              >
                <input
                  type="text"
                  value={tempAlias}
                  onChange={(e) => setTempAlias(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") handleSaveEdit(e, t.id);
                    if (e.key === "Escape") setEditingId(null);
                  }}
                  autoFocus
                  className="bg-[#0e1117] border border-sky-500/50 rounded px-1 py-0.2 text-xs text-white outline-none w-24"
                />
                <button
                  type="button"
                  onClick={(e) => handleSaveEdit(e, t.id)}
                  className="p-0.5 text-emerald-400 hover:text-emerald-300"
                >
                  <Check className="w-3 h-3" />
                </button>
              </div>
            ) : (
              <span className="truncate max-w-[120px]">{displayName}</span>
            )}

            {!isEditing && (
              <button
                type="button"
                onClick={(e) => handleStartEdit(e, t.id, displayName)}
                className="opacity-0 group-hover:opacity-100 p-0.5 text-slate-500 hover:text-slate-300 transition-opacity"
                title="Rename alias"
              >
                <Edit2 className="w-2.5 h-2.5" />
              </button>
            )}

            <span className={`w-1.5 h-1.5 rounded-full ${t.statusDot} ml-0.5 shrink-0`} />
          </div>
        );
      })}
    </div>
  );
};
