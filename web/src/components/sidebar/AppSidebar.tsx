import React from "react";
import {
  ChevronRight,
  FolderPlus,
  PanelLeftClose,
  PanelLeftOpen,
  Plus,
  Settings,
  Terminal as TerminalIcon,
  Zap,
} from "lucide-react";
import type { BridgeSession, EngineStatus, UpdateStatus, Workspace } from "../../types/bridge";
import { ProjectItem } from "./ProjectItem";

interface Props {
  collapsed: boolean;
  isMobile: boolean;
  mobileScreen?: "tasks" | "terminal";
  activeSessionInfo?: BridgeSession | null;
  onGoToTerminal?: () => void;
  workspaces: Workspace[];
  currentWorkspace: string;
  treeCollapsed: boolean;
  sessions: BridgeSession[];
  activeSessionId: string | null;
  expandedTasks: Record<string, boolean>;
  sessionOpenAgents: Map<string, Set<string>>;
  installedEngines: EngineStatus[];
  updateStatus: UpdateStatus | null;
  onSetCollapsed: (v: boolean) => void;
  onSelectWorkspace: (path: string) => void;
  onToggleTree: () => void;
  onOpenNewProjectModal: () => void;
  onCreateSession: () => void;
  onRenameWorkspace: (id: string, name: string) => Promise<void>;
  onDeleteWorkspace: (id: string, name: string, e: React.MouseEvent) => void;
  onToggleTaskExpand: (taskId: string) => void;
  onOpenSession: (s: BridgeSession) => void;
  onDeleteSession: (id: string, e: React.MouseEvent) => void;
  onRenameSession: (id: string, newTitle: string) => Promise<void>;
  onOpenAgent: (agentId: string, taskId: string) => void;
  onOpenSettings: (tab?: "general" | "agents" | "appearance" | "updates") => void;
}

export const AppSidebar: React.FC<Props> = ({
  collapsed,
  isMobile,
  mobileScreen = "tasks",
  activeSessionInfo,
  onGoToTerminal,
  workspaces,
  currentWorkspace,
  treeCollapsed,
  sessions,
  activeSessionId,
  expandedTasks,
  sessionOpenAgents,
  installedEngines,
  updateStatus,
  onSetCollapsed,
  onSelectWorkspace,
  onToggleTree,
  onOpenNewProjectModal,
  onCreateSession,
  onRenameWorkspace,
  onDeleteWorkspace,
  onToggleTaskExpand,
  onOpenSession,
  onDeleteSession,
  onRenameSession,
  onOpenAgent,
  onOpenSettings,
}) => {
  // Mobile: dedicated full-screen Tasks View (Screen 1)
  if (isMobile) {
    if (mobileScreen === "terminal") return null;
    return (
      <div className="flex h-dvh w-screen flex-col bg-[#0b0e14] text-slate-300 select-none overflow-hidden pt-[env(safe-area-inset-top)] pb-[env(safe-area-inset-bottom)]">
        {/* Mobile Header: Brand + Quick Actions */}
        <div className="flex h-13 shrink-0 items-center justify-between border-b border-[#1c212a] bg-[#101319] px-4">
          <div className="flex items-center gap-2.5">
            <Zap className="h-4 w-4 fill-amber-400 text-amber-400" />
            <span className="text-sm font-semibold tracking-wide text-slate-100">Agent Bridge</span>
          </div>

          <div className="flex items-center gap-2">
            {activeSessionId && onGoToTerminal && (
              <button
                onClick={onGoToTerminal}
                className="flex items-center gap-1.5 rounded-lg bg-indigo-600/20 hover:bg-indigo-600/30 border border-indigo-500/30 px-2.5 py-1.5 text-xs font-medium text-indigo-300 active:scale-95 transition-all shadow-sm max-w-[160px]"
                title={`Vào Terminal: ${activeSessionInfo?.title || "Task"}`}
              >
                <TerminalIcon className="h-3.5 w-3.5 text-indigo-400 shrink-0" />
                <span className="truncate">{activeSessionInfo?.title || "Terminal"}</span>
                <ChevronRight className="h-3.5 w-3.5 opacity-70 shrink-0" />
              </button>
            )}
            <button
              onClick={() => onOpenSettings(updateStatus?.has_update ? "updates" : "general")}
              className="relative cursor-pointer rounded-lg p-2 text-slate-400 hover:bg-white/[0.06] hover:text-slate-200"
              title="Settings"
            >
              <Settings className="h-4 w-4 text-violet-400" />
              {updateStatus?.has_update && (
                <span className="absolute -top-0.5 -right-0.5 flex h-2 w-2">
                  <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-amber-400 opacity-75"></span>
                  <span className="relative inline-flex rounded-full h-2 w-2 bg-amber-500"></span>
                </span>
              )}
            </button>
          </div>
        </div>

        {/* Section Actions: + Project, + Task */}
        <div className="flex items-center justify-between border-b border-[#1c212a]/60 bg-[#0e1117] px-4 py-2.5 text-xs font-semibold uppercase tracking-wider text-slate-400 shrink-0">
          <span>Projects &amp; Tasks</span>
          <div className="flex items-center gap-2">
            <button
              onClick={onOpenNewProjectModal}
              className="flex cursor-pointer items-center gap-1 rounded-md bg-[#18202d] border border-[#263147] px-2.5 py-1 text-xs font-medium text-emerald-400 hover:bg-emerald-950/40 hover:text-emerald-300 transition-colors"
              title="Add or import project"
            >
              <FolderPlus className="h-3.5 w-3.5" />
              <span>Project</span>
            </button>
            <button
              onClick={onCreateSession}
              className="flex cursor-pointer items-center gap-1 rounded-md bg-indigo-600/20 border border-indigo-500/30 px-2.5 py-1 text-xs font-medium text-indigo-300 hover:bg-indigo-600/30 hover:text-indigo-200 transition-colors"
              title="Create new task for active project"
            >
              <Plus className="h-3.5 w-3.5" />
              <span>Task</span>
            </button>
          </div>
        </div>

        {/* Scrollable list of Projects and Tasks */}
        <div className="min-h-0 flex-1 space-y-1 overflow-y-auto overflow-x-hidden p-3 pb-8">
          {workspaces.map((ws) => (
            <ProjectItem
              key={ws.id}
              workspace={ws}
              isActive={ws.path === currentWorkspace}
              isExpanded={ws.path === currentWorkspace && !treeCollapsed}
              sessions={ws.path === currentWorkspace ? sessions : []}
              activeSessionId={activeSessionId}
              expandedTasks={expandedTasks}
              sessionOpenAgents={sessionOpenAgents}
              installedEngines={installedEngines}
              onSelectWorkspace={onSelectWorkspace}
              onToggleExpand={onToggleTree}
              onRenameWorkspace={onRenameWorkspace}
              onDeleteWorkspace={onDeleteWorkspace}
              onCreateSession={onCreateSession}
              onToggleTaskExpand={onToggleTaskExpand}
              onOpenSession={onOpenSession}
              onDeleteSession={onDeleteSession}
              onRenameSession={onRenameSession}
              onOpenAgent={onOpenAgent}
            />
          ))}
        </div>
      </div>
    );
  }

  // Desktop: Collapsed sidebar icon bar
  if (collapsed) {
    return (
      <div className="flex w-11 shrink-0 flex-col items-center justify-between border-r border-[#1d222b] bg-[#12151c] py-3 select-none">
        <button
          onClick={() => onSetCollapsed(false)}
          className="cursor-pointer rounded-lg p-2 text-slate-400 hover:bg-[#1a1e28] hover:text-slate-200"
          title="Mở rộng menu"
        >
          <PanelLeftOpen className="h-4 w-4" />
        </button>
        <button
          onClick={() => onOpenSettings(updateStatus?.has_update ? "updates" : "general")}
          className="relative cursor-pointer rounded-lg p-2 text-slate-400 hover:bg-[#1a1e28] hover:text-slate-200"
          title={updateStatus?.has_update ? "Settings (Có bản cập nhật mới)" : "Settings"}
        >
          <Settings className="h-4 w-4 text-violet-400" />
          {updateStatus?.has_update && (
            <span className="absolute -top-0.5 -right-0.5 flex h-2.5 w-2.5">
              <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-amber-400 opacity-75"></span>
              <span className="relative inline-flex rounded-full h-2.5 w-2.5 bg-amber-500 border border-[#12151c]"></span>
            </span>
          )}
        </button>
      </div>
    );
  }

  // Desktop: Expanded standard sidebar
  return (
    <aside className="h-full w-72 shrink-0 flex flex-col border-r border-[#1d222b] bg-[#12151c] text-slate-300 select-none overflow-x-hidden">
        {/* Brand header */}
        <div className="flex items-center justify-between border-b border-[#1c212a] px-3.5 pt-3 pb-2 shrink-0">
          <div className="flex items-center gap-2">
            <Zap className="h-3.5 w-3.5 fill-amber-400 text-amber-400" />
            <span className="text-xs font-semibold tracking-wide text-slate-200">Agent Bridge</span>
          </div>
          <button
            onClick={() => onSetCollapsed(true)}
            className="cursor-pointer p-1 text-slate-500 hover:text-slate-300 rounded"
            title="Thu gọn"
          >
            <PanelLeftClose className="h-3.5 w-3.5" />
          </button>
        </div>

        {/* Section actions */}
        <div className="flex items-center justify-between px-3.5 pt-3 pb-1 text-[10px] font-semibold uppercase tracking-wider text-slate-500 shrink-0">
          <span>Projects &amp; Tasks</span>
          <div className="flex items-center gap-1">
            <button
              onClick={onOpenNewProjectModal}
              className="flex cursor-pointer items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium text-emerald-400 hover:bg-emerald-950/40 hover:text-emerald-300 transition-colors"
              title="Add or import project"
            >
              <FolderPlus className="h-3 w-3" />
              <span>Project</span>
            </button>
            <button
              onClick={onCreateSession}
              className="flex cursor-pointer items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium text-indigo-400 hover:bg-indigo-950/40 hover:text-indigo-300 transition-colors"
              title="Create new task for active project"
            >
              <Plus className="h-3 w-3" />
              <span>Task</span>
            </button>
          </div>
        </div>

        {/* Scrollable projects & tasks list - Strict overflow-x-hidden */}
        <div className="min-h-0 flex-1 space-y-0.5 overflow-y-auto overflow-x-hidden px-2 pb-2">
          {workspaces.map((ws) => (
            <ProjectItem
              key={ws.id}
              workspace={ws}
              isActive={ws.path === currentWorkspace}
              isExpanded={ws.path === currentWorkspace && !treeCollapsed}
              sessions={ws.path === currentWorkspace ? sessions : []}
              activeSessionId={activeSessionId}
              expandedTasks={expandedTasks}
              sessionOpenAgents={sessionOpenAgents}
              installedEngines={installedEngines}
              onSelectWorkspace={onSelectWorkspace}
              onToggleExpand={onToggleTree}
              onRenameWorkspace={onRenameWorkspace}
              onDeleteWorkspace={onDeleteWorkspace}
              onCreateSession={onCreateSession}
              onToggleTaskExpand={onToggleTaskExpand}
              onOpenSession={onOpenSession}
              onDeleteSession={onDeleteSession}
              onRenameSession={onRenameSession}
              onOpenAgent={onOpenAgent}
            />
          ))}
        </div>

        {/* Settings pinned at bottom */}
        <div className="shrink-0 border-t border-[#1d222b] bg-[#101217] p-2.5">
          <button
            onClick={() => onOpenSettings(updateStatus?.has_update ? "updates" : "general")}
            className="group flex w-full cursor-pointer items-center justify-between rounded-lg px-2.5 py-1.5 text-xs text-slate-400 hover:bg-white/[0.04] hover:text-slate-200 transition-colors"
          >
            <div className="flex items-center gap-2">
              <Settings className="h-3.5 w-3.5 text-violet-400 group-hover:rotate-45 transition-transform duration-200" />
              <span>Settings</span>
            </div>
            {updateStatus?.has_update && (
              <span className="flex items-center gap-1.5 rounded-full bg-amber-500/15 border border-amber-500/30 px-2 py-0.5 text-[10px] font-medium text-amber-300 shadow-sm animate-pulse">
                <span className="flex h-1.5 w-1.5 relative">
                  <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-amber-400 opacity-75"></span>
                  <span className="relative inline-flex rounded-full h-1.5 w-1.5 bg-amber-400"></span>
                </span>
                <span>{updateStatus.latest_version ? `${updateStatus.latest_version}` : "Update"}</span>
              </span>
            )}
          </button>
        </div>
      </aside>
  );
};
