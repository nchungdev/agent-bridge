import React, { useState, useMemo } from "react";
import {
  Plus,
  Search,
  Layers,
  RefreshCw,
} from "lucide-react";
import type { BridgeSession, EngineStatus, KanbanStatus } from "../../types/bridge";
import { KanbanColumn } from "./KanbanColumn";
import { NewTaskModal } from "./NewTaskModal";
import { TaskDetailModal } from "./TaskDetailModal";

interface KanbanBoardProps {
  sessions: BridgeSession[];
  workspace: string;
  installedEngines: EngineStatus[];
  onOpenTerminal: (task: BridgeSession) => void;
  onOpenRemote: (task: BridgeSession) => void;
  onUpdateStatus: (taskId: string, status: KanbanStatus) => void;
  onAssignAgent: (taskId: string, agent: string) => void;
  onToggleSubtask: (taskId: string, subtaskId: string, done: boolean) => void;
  onAddSubtask: (taskId: string, title: string) => void;
  onDeleteSubtask: (taskId: string, subtaskId: string) => void;
  onDecompose: (taskId: string, titles: string[]) => void;
  onUpdateResult: (taskId: string, summary: string, status?: string) => void;
  onCreateTask: (
    title: string,
    agent: string,
    status: KanbanStatus,
    subtasks: string[]
  ) => Promise<void>;
  onDeleteTask: (taskId: string) => void;
  onRefresh?: () => void;
}

const COLUMNS: { status: KanbanStatus; title: string }[] = [
  { status: "todo", title: "To Do (Cần làm)" },
  { status: "in_progress", title: "In Progress (Đang chạy)" },
  { status: "review", title: "Review & Handoff" },
  { status: "done", title: "Done (Hoàn thành)" },
];

export const KanbanBoard: React.FC<KanbanBoardProps> = ({
  sessions,
  workspace,
  installedEngines,
  onOpenTerminal,
  onOpenRemote,
  onUpdateStatus,
  onAssignAgent,
  onToggleSubtask,
  onAddSubtask,
  onDeleteSubtask,
  onDecompose,
  onUpdateResult,
  onCreateTask,
  onDeleteTask,
  onRefresh,
}) => {
  const [searchQuery, setSearchQuery] = useState("");
  const [agentFilter, setAgentFilter] = useState<string>("all");
  const [newTaskOpen, setNewTaskOpen] = useState(false);
  const [newTaskInitialStatus, setNewTaskInitialStatus] = useState<KanbanStatus>("todo");
  const [selectedTask, setSelectedTask] = useState<BridgeSession | null>(null);

  // Filter tasks based on search and agent
  const filteredSessions = useMemo(() => {
    return sessions.filter((s) => {
      const matchesSearch =
        searchQuery === "" ||
        s.title.toLowerCase().includes(searchQuery.toLowerCase()) ||
        s.id.toLowerCase().includes(searchQuery.toLowerCase());

      const matchesAgent =
        agentFilter === "all" || (s.current_agent || "agy") === agentFilter;

      return matchesSearch && matchesAgent;
    });
  }, [sessions, searchQuery, agentFilter]);

  // Group into columns
  const tasksByColumn = useMemo(() => {
    const map: Record<KanbanStatus, BridgeSession[]> = {
      todo: [],
      in_progress: [],
      review: [],
      done: [],
    };

    filteredSessions.forEach((s) => {
      const st = s.kanban_status || "todo";
      if (map[st]) {
        map[st].push(s);
      } else {
        map.todo.push(s);
      }
    });

    return map;
  }, [filteredSessions]);

  const handleOpenAddTaskModal = (status: KanbanStatus = "todo") => {
    setNewTaskInitialStatus(status);
    setNewTaskOpen(true);
  };

  return (
    <div className="flex h-full w-full flex-col bg-[#0b0e14] text-slate-100 overflow-hidden">
      {/* Kanban Toolbar */}
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-slate-800/80 bg-[#0e121b] px-4 py-3 shrink-0">
        <div className="flex items-center gap-3">
          <div className="flex items-center gap-2">
            <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-indigo-600/20 text-indigo-400 border border-indigo-500/30">
              <Layers className="h-4 w-4" />
            </div>
            <div className="flex flex-col">
              <h1 className="font-semibold text-sm text-slate-100">Autonomous Mission Control</h1>
              {workspace && (
                <span className="text-[10px] text-slate-500 font-mono truncate max-w-[200px]" title={workspace}>
                  {workspace}
                </span>
              )}
            </div>
          </div>

          <span className="hidden sm:inline-block h-4 w-px bg-slate-800" />

          {/* Search Bar */}
          <div className="relative">
            <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-slate-500" />
            <input
              type="text"
              placeholder="Tìm kiếm task..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="h-8 rounded-lg border border-slate-800 bg-slate-900/90 pl-8 pr-3 text-xs text-slate-200 placeholder-slate-500 focus:border-indigo-500 focus:outline-none w-36 sm:w-52"
            />
          </div>

          {/* Agent Filter */}
          <div className="hidden md:flex items-center gap-1 text-xs">
            {["all", "agy", "claude", "codex"].map((ag) => (
              <button
                key={ag}
                onClick={() => setAgentFilter(ag)}
                className={`rounded-lg px-2.5 py-1 font-medium transition cursor-pointer ${
                  agentFilter === ag
                    ? "bg-indigo-600/30 text-indigo-300 border border-indigo-500/40"
                    : "text-slate-400 hover:text-slate-200 hover:bg-slate-800"
                }`}
              >
                {ag === "all" && "Tất cả"}
                {ag === "agy" && "Antigravity"}
                {ag === "claude" && "Claude"}
                {ag === "codex" && "Codex"}
              </button>
            ))}
          </div>
        </div>

        {/* Right Toolbar Actions */}
        <div className="flex items-center gap-2">
          {onRefresh && (
            <button
              onClick={onRefresh}
              title="Làm mới dữ liệu"
              className="rounded-lg p-1.5 text-slate-400 hover:bg-slate-800 hover:text-slate-200 transition cursor-pointer"
            >
              <RefreshCw className="h-4 w-4" />
            </button>
          )}

          <button
            onClick={() => handleOpenAddTaskModal("todo")}
            className="flex items-center gap-1.5 rounded-xl bg-indigo-600 px-3.5 py-1.5 text-xs font-semibold text-white shadow-lg shadow-indigo-600/30 hover:bg-indigo-500 transition cursor-pointer"
          >
            <Plus className="h-3.5 w-3.5" />
            <span>+ Giao Task mới</span>
          </button>
        </div>
      </div>

      {/* Kanban Board Columns Area */}
      <div className="flex-1 overflow-x-auto overflow-y-hidden p-4">
        <div className="flex h-full gap-4 min-w-[1160px] pb-2">
          {COLUMNS.map((col) => (
            <KanbanColumn
              key={col.status}
              status={col.status}
              title={col.title}
              tasks={tasksByColumn[col.status]}
              installedEngines={installedEngines}
              onOpenTerminal={onOpenTerminal}
              onOpenRemote={onOpenRemote}
              onUpdateStatus={onUpdateStatus}
              onAssignAgent={onAssignAgent}
              onToggleSubtask={onToggleSubtask}
              onAddSubtask={onAddSubtask}
              onDeleteSubtask={onDeleteSubtask}
              onDeleteTask={onDeleteTask}
              onAddTaskToColumn={handleOpenAddTaskModal}
              onViewDetail={(t) => setSelectedTask(t)}
            />
          ))}
        </div>
      </div>

      {/* New Task Modal */}
      <NewTaskModal
        isOpen={newTaskOpen}
        initialStatus={newTaskInitialStatus}
        installedEngines={installedEngines}
        onClose={() => setNewTaskOpen(false)}
        onCreateTask={onCreateTask}
      />

      {/* Task Detail Modal */}
      <TaskDetailModal
        task={selectedTask}
        isOpen={!!selectedTask}
        installedEngines={installedEngines}
        onClose={() => setSelectedTask(null)}
        onOpenTerminal={onOpenTerminal}
        onOpenRemote={onOpenRemote}
        onUpdateStatus={onUpdateStatus}
        onAssignAgent={onAssignAgent}
        onToggleSubtask={onToggleSubtask}
        onAddSubtask={onAddSubtask}
        onDeleteSubtask={onDeleteSubtask}
        onDecompose={onDecompose}
        onUpdateResult={onUpdateResult}
        onDeleteTask={onDeleteTask}
      />
    </div>
  );
};
