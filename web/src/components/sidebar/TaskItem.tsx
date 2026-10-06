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

  const toolsList = [...rawTools].sort((a, b) => {
    const ordA = AGENT_ORDER[a.agent] ?? 99;
    const ordB = AGENT_ORDER[b.agent] ?? 99;
    if (ordA !== ordB) return ordA - ordB;
    return a.agent.localeCompare(b.agent);
  });

  const unassignedAgents = installedEngines.filter((e) => !boundAgents.includes(e.id));

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
        <form onSubmit={handleSaveRename} className="flex items-center gap-1 py-1 px-1">
          <input
            autoFocus
            value={editTitle}
            onChange={(e) => setEditTitle(e.target.value)}
            className="h-6 w-full rounded bg-[#1e2433] px-1.5 font-sans text-xs text-slate-100 outline-none border border-indigo-500"
            onKeyDown={(e) => {
              if (e.key === "Escape") setEditing(false);
            }}
          />
          <button type="submit" className="p-0.5 text-emerald-400 hover:text-emerald-300">
            <Check className="h-3 w-3" />
          </button>
          <button type="button" onClick={() => setEditing(false)} className="p-0.5 text-slate-500 hover:text-slate-300">
            <X className="h-3 w-3" />
          </button>
        </form>
      ) : (
        <div
          onClick={() => onOpenSession(session)}
          className={`group relative flex cursor-pointer items-center justify-between rounded px-1.5 py-1 text-left transition-colors ${
            isActive
              ? "text-slate-100 font-medium hover:bg-white/[0.03]"
              : "text-slate-400 hover:bg-white/[0.03] hover:text-slate-200"
          }`}
        >
          <div className="flex min-w-0 items-center gap-1.5">
            <button
              onClick={(e) => {
                e.stopPropagation();
                onToggleExpand();
              }}
              className="cursor-pointer p-0.5 text-slate-500 hover:text-slate-300"
              title={isExpanded ? "Thu gọn tools" : "Mở rộng tools"}
            >
              {isExpanded ? <ChevronDown className="h-3 w-3 shrink-0" /> : <ChevronRight className="h-3 w-3 shrink-0" />}
            </button>
            <Layers className={`h-3 w-3 shrink-0 ${isActive ? "text-indigo-400" : "text-slate-500"}`} />
            <span className="truncate text-xs">{session.title || "New Task"}</span>
          </div>

          <div className="hidden shrink-0 items-center gap-1 group-hover:flex">
            <button
              onClick={(e) => {
                e.stopPropagation();
                setEditing(true);
                setEditTitle(session.title || "New Task");
              }}
              className="p-1 text-slate-500 hover:text-indigo-300 rounded hover:bg-white/[0.06] transition-colors"
              title="Đổi tên task"
            >
              <Pencil className="h-2.5 w-2.5" />
            </button>
            <button
              onClick={(e) => onDeleteSession(session.id, e)}
              className="p-1 text-slate-500 hover:text-rose-400 rounded hover:bg-white/[0.06] transition-colors"
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

            return (
              <div
                key={tool.agent}
                onClick={() => onOpenAgent(tool.agent, session.id)}
                className="group flex cursor-pointer items-center justify-between rounded px-1.5 py-0.5 text-[11px] text-slate-400 hover:bg-white/[0.04] hover:text-slate-200 transition-colors"
                title={`Open ${meta.label} for this task`}
              >
                <div className="flex min-w-0 items-center gap-1.5">
                  <span className={`h-1.5 w-1.5 shrink-0 rounded-full ${meta.dot}`} />
                  <span className="truncate font-sans">{meta.label}</span>
                </div>
                {isOpenInTab && (
                  <span className="text-[10px] text-emerald-400/80 font-mono">active</span>
                )}
              </div>
            );
          })}

          {/* Clean Assign Agent Button - No Horizontal Scroll! */}
          {unassignedAgents.length > 0 && (
            <div className="relative pt-1 pl-1">
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  setAssignOpen((prev) => !prev);
                }}
                className="inline-flex cursor-pointer items-center gap-1 rounded border border-[#232b3b] bg-[#131722] px-2 py-0.5 text-[10.5px] font-medium text-slate-300 hover:border-indigo-500/50 hover:bg-[#1a202e] hover:text-white transition-all shadow-sm"
                title="Gán thêm agent cho task này"
              >
                <Plus className="h-2.5 w-2.5 text-indigo-400" />
                <span>Assign</span>
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
