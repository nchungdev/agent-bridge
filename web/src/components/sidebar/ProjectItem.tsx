import React, { useState } from "react";
import {
  ChevronDown,
  ChevronRight,
  Folder,
  FolderOpen,
  Pencil,
  Trash2,
  Check,
  X,
  Plus,
  Shield,
} from "lucide-react";
import type { BridgeSession, EngineStatus, Workspace } from "../../types/bridge";
import { isProtectedWorkspacePath } from "../../constants/agentMeta";
import { TaskItem } from "./TaskItem";

interface Props {
  workspace: Workspace;
  isActive: boolean;
  isExpanded: boolean;
  sessions: BridgeSession[];
  activeSessionId: string | null;
  expandedTasks: Record<string, boolean>;
  sessionOpenAgents: Map<string, Set<string>>;
  installedEngines: EngineStatus[];
  onSelectWorkspace: (path: string) => void;
  onToggleExpand: () => void;
  onRenameWorkspace: (id: string, newName: string) => Promise<void>;
  onDeleteWorkspace: (id: string, name: string, e: React.MouseEvent) => void;
  onCreateSession: () => void;
  onToggleTaskExpand: (taskId: string) => void;
  onOpenSession: (s: BridgeSession) => void;
  onDeleteSession: (id: string, e: React.MouseEvent) => void;
  onRenameSession: (id: string, newTitle: string) => Promise<void>;
  onOpenAgent: (agentId: string, taskId: string) => void;
}

export const ProjectItem: React.FC<Props> = ({
  workspace,
  isActive,
  isExpanded,
  sessions,
  activeSessionId,
  expandedTasks,
  sessionOpenAgents,
  installedEngines,
  onSelectWorkspace,
  onToggleExpand,
  onRenameWorkspace,
  onDeleteWorkspace,
  onCreateSession,
  onToggleTaskExpand,
  onOpenSession,
  onDeleteSession,
  onRenameSession,
  onOpenAgent,
}) => {
  const [editing, setEditing] = useState(false);
  const [editName, setEditName] = useState(workspace.name);

  const handleSaveRename = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!editName.trim()) return;
    await onRenameWorkspace(workspace.id, editName.trim());
    setEditing(false);
  };

  return (
    <div>
      {/* Level 1: Project (Folder/Workspace) */}
      {editing ? (
        <form onSubmit={handleSaveRename} className="flex items-center gap-1 py-1 px-1">
          <input
            autoFocus
            value={editName}
            onChange={(e) => setEditName(e.target.value)}
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
        <div className="group relative flex w-full items-center justify-between rounded-md px-1.5 py-1.5 text-left text-xs transition-colors hover:bg-white/[0.04]">
          <div
            onClick={() => {
              if (isActive) onToggleExpand();
              else onSelectWorkspace(workspace.path);
            }}
            className="flex min-w-0 flex-1 cursor-pointer items-center gap-1.5"
            title={`Folder: ${workspace.path}`}
          >
            {isExpanded ? (
              <ChevronDown className="h-3 w-3 shrink-0 text-slate-500" />
            ) : (
              <ChevronRight className="h-3 w-3 shrink-0 text-slate-500" />
            )}
            {isExpanded ? (
              <FolderOpen className="h-3.5 w-3.5 shrink-0 text-amber-400" />
            ) : (
              <Folder className="h-3.5 w-3.5 shrink-0 text-slate-500" />
            )}
            <span className={`truncate ${isActive ? "font-semibold text-slate-100" : "text-slate-400"}`}>
              {workspace.name}
            </span>
          </div>

          <div className="hidden shrink-0 items-center gap-1 group-hover:flex">
            <button
              onClick={(e) => {
                e.stopPropagation();
                setEditing(true);
                setEditName(workspace.name);
              }}
              className="p-1 text-slate-500 hover:text-indigo-300 rounded hover:bg-white/[0.06] transition-colors"
              title="Đổi tên project"
            >
              <Pencil className="h-2.5 w-2.5" />
            </button>
            {isProtectedWorkspacePath(workspace.path) ? (
              <span
                className="p-1 text-slate-500 cursor-not-allowed select-none"
                title="Thư mục hệ thống / tài khoản được bảo vệ"
              >
                <Shield className="h-2.5 w-2.5 text-slate-500" />
              </span>
            ) : (
              <button
                onClick={(e) => onDeleteWorkspace(workspace.id, workspace.name, e)}
                className="p-1 text-slate-500 hover:text-rose-400 rounded hover:bg-white/[0.06] transition-colors"
                title="Gỡ project khỏi danh sách"
              >
                <Trash2 className="h-2.5 w-2.5" />
              </button>
            )}
          </div>
        </div>
      )}

      {/* Project tasks */}
      {isExpanded && (
        <div className="ml-2 space-y-0.5 pl-1.5 pt-0.5">
          {sessions.length === 0 ? (
            <div className="px-2 py-3 text-center">
              <p className="text-[11px] text-slate-500">Chưa có task nào.</p>
              <button
                onClick={onCreateSession}
                className="mt-2 inline-flex items-center gap-1 rounded bg-white/[0.06] px-2.5 py-1 text-[11px] text-indigo-300 hover:bg-indigo-600 hover:text-white transition-colors"
              >
                <Plus className="h-3 w-3" /> New Task
              </button>
            </div>
          ) : (
            sessions.map((s) => (
              <TaskItem
                key={s.id}
                session={s}
                isActive={activeSessionId === s.id}
                isExpanded={expandedTasks[s.id] ?? (activeSessionId === s.id)}
                installedEngines={installedEngines}
                sessionOpenAgents={sessionOpenAgents.get(s.id)}
                onToggleExpand={() => onToggleTaskExpand(s.id)}
                onOpenSession={onOpenSession}
                onDeleteSession={onDeleteSession}
                onRenameSession={onRenameSession}
                onOpenAgent={onOpenAgent}
              />
            ))
          )}
        </div>
      )}
    </div>
  );
};
