import React, { useState } from "react";
import {
  ChevronDown,
  ChevronRight,
  Layers,
  Pencil,
  Trash2,
  Check,
  X,
  Plus,
} from "lucide-react";
import type { BridgeSession, EngineStatus, ToolBinding } from "../../types/bridge";
import { AGENT_META, AGENT_ORDER } from "../../constants/agentMeta";
import { AssignAgentDropdown } from "./AssignAgentDropdown";

interface Props {
  session: BridgeSession;
  isActive: boolean;
  isExpanded: boolean;
  installedEngines: EngineStatus[];
  sessionOpenAgents?: Set<string>;
  onToggleExpand: () => void;
  onOpenSession: (s: BridgeSession) => void;
  onDeleteSession: (id: string, e: React.MouseEvent) => void;
  onRenameSession: (id: string, newTitle: string) => Promise<void>;
  onOpenAgent: (agentId: string, taskId: string) => void;
}

const cleanDisplayName = (title?: string) => {
  if (!title) return "";
  const lines = title.split("\n").map((l) => l.trim()).filter(Boolean);
  if (lines.length === 0) return "";
  let first = lines[0];
  first = first.replace(/^[#\-*`]+\s*/, "").replace(/<[^>]+>/g, "").replace(/\[Image #[0-9]+\]/g, "").trim();
  return first;
};

export const TaskItem: React.FC<Props> = ({
  session,
  isActive,
  isExpanded,
  installedEngines,
  sessionOpenAgents,
  onToggleExpand,
  onOpenSession,
  onDeleteSession,
  onRenameSession,
  onOpenAgent,
}) => {
  const [editing, setEditing] = useState(false);
  const [editTitle, setEditTitle] = useState(session.title || "New Task");
  const [assignOpen, setAssignOpen] = useState(false);

  const boundAgents = Object.keys(session.bindings || {});
  const rawTools: ToolBinding[] =
    session.tools && session.tools.length > 0
      ? session.tools
      : boundAgents.map((ag) => ({
          agent: ag,
          native_session_id: session.bindings?.[ag] || "",
          updated_at: session.updated_at,
        }));

  // an agent opened in a tab but not bound yet (fresh session, no native id) still belongs to the task
  const openOnly: ToolBinding[] = [...(sessionOpenAgents || [])]
    .filter((ag) => ag !== "shell" && !rawTools.some((t) => t.agent === ag))
    .map((ag) => ({ agent: ag, native_session_id: "", updated_at: session.updated_at }));

  const toolsList = [...rawTools, ...openOnly].sort((a, b) => {
    const ordA = AGENT_ORDER[a.agent] ?? 99;
    const ordB = AGENT_ORDER[b.agent] ?? 99;
    if (ordA !== ordB) return ordA - ordB;
    return a.agent.localeCompare(b.agent);
  });

  const unassignedAgents = installedEngines.filter((e) => !boundAgents.includes(e.id) && !sessionOpenAgents?.has(e.id));

  const handleSaveRename = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!editTitle.trim()) return;
    await onRenameSession(session.id, editTitle.trim());
    setEditing(false);
  };

  return (
    <div className="space-y-0.5">
      {/* Level 2: Task */}
      {editing ? (
        <form onSubmit={handleSaveRename} className="flex h-7 items-center gap-1 px-1.5">
          <input
            autoFocus
            value={editTitle}
            onChange={(e) => setEditTitle(e.target.value)}
            className="h-5.5 w-full rounded bg-[#1e2433] px-1.5 font-sans text-xs text-slate-100 outline-none border border-indigo-500"
            onKeyDown={(e) => {
              if (e.key === "Escape") setEditing(false);
            }}
          />
          <button type="submit" className="flex h-5 w-5 shrink-0 items-center justify-center text-emerald-400 hover:text-emerald-300">
            <Check className="h-3 w-3" />
          </button>
          <button type="button" onClick={() => setEditing(false)} className="flex h-5 w-5 shrink-0 items-center justify-center text-slate-500 hover:text-slate-300">
            <X className="h-3 w-3" />
          </button>
        </form>
      ) : (
        <div
          onClick={() => onOpenSession(session)}
          className={`group relative flex h-7 cursor-pointer items-center justify-between rounded px-1.5 text-left transition-colors ${
            isActive
              ? "bg-white/[0.04] text-slate-100 font-medium"
              : "text-slate-400 hover:bg-white/[0.03] hover:text-slate-200"
          }`}
        >
          <div className="flex min-w-0 flex-1 items-center gap-1.5">
            <button
              onClick={(e) => {
                e.stopPropagation();
                onToggleExpand();
              }}
              className="flex h-4 w-4 shrink-0 cursor-pointer items-center justify-center text-slate-500 hover:text-slate-300"
              title={isExpanded ? "Thu gọn tools" : "Mở rộng tools"}
            >
              {isExpanded ? <ChevronDown className="h-3 w-3 shrink-0" /> : <ChevronRight className="h-3 w-3 shrink-0" />}
            </button>
            <Layers className={`h-3 w-3 shrink-0 ${isActive ? "text-indigo-400" : "text-slate-500"}`} />
            <span className="truncate text-xs leading-none">{session.title || "New Task"}</span>
          </div>

          <div className="flex shrink-0 items-center gap-0.5 opacity-0 pointer-events-none group-hover:opacity-100 group-hover:pointer-events-auto group-focus-within:opacity-100 group-focus-within:pointer-events-auto transition-opacity">
            <button
              onClick={(e) => {
                e.stopPropagation();
                setEditing(true);
                setEditTitle(session.title || "New Task");
              }}
              className="flex h-5 w-5 items-center justify-center rounded text-slate-500 hover:bg-white/[0.06] hover:text-indigo-300 transition-colors"
              title="Đổi tên task"
            >
              <Pencil className="h-2.5 w-2.5" />
            </button>
            <button
              onClick={(e) => onDeleteSession(session.id, e)}
              className="flex h-5 w-5 items-center justify-center rounded text-slate-500 hover:bg-white/[0.06] hover:text-rose-400 transition-colors"
              title="Xóa task"
            >
              <Trash2 className="h-2.5 w-2.5" />
            </button>
          </div>
        </div>
      )}

      {/* Level 3: Assigned Tools & Clean Assign Button */}
      {isExpanded && (
        <div className="ml-5 space-y-0.5 border-l border-white/[0.06] pl-2">
          {toolsList.map((tool) => {
            const meta = AGENT_META[tool.agent] || { dot: "bg-purple-400", label: tool.agent };
            const isOpenInTab = sessionOpenAgents?.has(tool.agent);
            const toolTitle = cleanDisplayName(tool.title);
            const toolDisplayName = toolTitle || meta.label;

            return (
              <div
                key={tool.agent}
                onClick={() => onOpenAgent(tool.agent, session.id)}
                className="group flex h-6 cursor-pointer items-center justify-between rounded px-1.5 text-[11px] text-slate-400 hover:bg-white/[0.04] hover:text-slate-200 transition-colors"
                title={`${meta.label}: ${toolDisplayName}`}
              >
                <div className="flex min-w-0 items-center gap-1.5">
                  <span className={`h-1.5 w-1.5 shrink-0 rounded-full ${meta.dot}`} />
                  <span className="truncate font-sans leading-none">{toolDisplayName}</span>
                </div>
                {isOpenInTab && (
                  <span className="text-[10px] text-emerald-400/80 font-mono leading-none">active</span>
                )}
              </div>
            );
          })}

          {/* Clean Assign Agent Button - No Horizontal Scroll! */}
          {unassignedAgents.length > 0 && (
            <div className="relative pt-0.5 pl-1">
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  setAssignOpen((prev) => !prev);
                }}
                className="inline-flex h-5 cursor-pointer items-center gap-1 rounded border border-[#232b3b] bg-[#131722] px-2 text-[10.5px] font-medium text-slate-300 hover:border-indigo-500/50 hover:bg-[#1a202e] hover:text-white transition-all shadow-sm"
                title="Gán thêm agent cho task này"
              >
                <Plus className="h-2.5 w-2.5 text-indigo-400" />
                <span className="leading-none">Assign</span>
              </button>

              {assignOpen && (
                <AssignAgentDropdown
                  availableAgents={unassignedAgents}
                  onAssign={(agentId) => {
                    setAssignOpen(false);
                    onOpenAgent(agentId, session.id);
                  }}
                  onClose={() => setAssignOpen(false)}
                />
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
};
