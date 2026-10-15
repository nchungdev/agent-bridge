import React, { useState, useMemo } from "react";
import {
  Plus,
  Search,
  RefreshCw,
  BookOpen,
} from "lucide-react";
import type { BridgeSession, EngineStatus, KanbanStatus } from "../../types/bridge";
import { KanbanColumn } from "./KanbanColumn";
import { NewTaskModal } from "./NewTaskModal";
import { TaskDetailModal } from "./TaskDetailModal";
import { MemoryLedgerModal } from "../modals/MemoryLedgerModal";

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
  onCleanupTodo?: () => void;
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
  onCleanupTodo,
  onRefresh,
}) => {
  const [searchQuery, setSearchQuery] = useState("");
  const [agentFilter, setAgentFilter] = useState<string>("all");
  const [tagFilter, setTagFilter] = useState<string>("all");
  const [newTaskOpen, setNewTaskOpen] = useState(false);
  const [newTaskInitialStatus, setNewTaskInitialStatus] = useState<KanbanStatus>("todo");
  const [selectedTask, setSelectedTask] = useState<BridgeSession | null>(null);
  const [memoryModalOpen, setMemoryModalOpen] = useState(false);

  // Extract unique tags across tasks
  const availableTags = useMemo(() => {
    const set = new Set<string>();
    sessions.forEach((s) => {
      s.tags?.forEach((t) => set.add(t));
    });
    return Array.from(set).sort();
  }, [sessions]);

  // Filter tasks based on search, agent, and tag
  const filteredSessions = useMemo(() => {
    return sessions.filter((s) => {
      const matchesSearch =
        searchQuery === "" ||
        s.title.toLowerCase().includes(searchQuery.toLowerCase()) ||
        s.id.toLowerCase().includes(searchQuery.toLowerCase());

      const matchesAgent =
        agentFilter === "all" || (s.current_agent || "agy") === agentFilter;

      const matchesTag =
        tagFilter === "all" || (s.tags && s.tags.includes(tagFilter));

      return matchesSearch && matchesAgent && matchesTag;
    });
  }, [sessions, searchQuery, agentFilter, tagFilter]);

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
      {/* Kanban Board Controls Bar (Search, Filters, Stats & New Task) */}
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-slate-800/80 bg-[#0c1018] px-4 py-2 shrink-0">
        <div className="flex items-center gap-3 flex-wrap">
          {/* Search Bar */}
          <div className="relative group">
            <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-slate-400 group-focus-within:text-indigo-400 transition-colors pointer-events-none" />
            <input
              type="text"
              placeholder="Tìm kiếm task..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="h-8 rounded-lg border border-slate-700/80 bg-[#111520] pl-8 pr-3 text-xs text-slate-100 placeholder-slate-400 shadow-inner focus:border-indigo-500 focus:bg-[#131926] focus:outline-none focus:ring-1 focus:ring-indigo-500/30 transition-all w-44 sm:w-64"
            />
          </div>

          <span className="hidden sm:inline-block h-4 w-px bg-slate-800" />

          {/* Agent Filter */}
          <div className="flex items-center gap-1 text-xs">
            <span className="text-[11px] text-slate-500 font-medium mr-1 hidden lg:inline">Agent:</span>
            {["all", "agy", "claude", "codex"].map((ag) => (
              <button
                key={ag}
                onClick={() => setAgentFilter(ag)}
                className={`rounded-lg px-2.5 py-1 text-xs font-medium transition cursor-pointer ${
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

          {/* Tag Filter */}
          {availableTags.length > 0 && (
            <>
              <span className="hidden xl:inline-block h-4 w-px bg-slate-800" />
              <div className="flex items-center gap-1 text-xs">
                <span className="text-[11px] text-slate-500 font-medium mr-1 hidden 2xl:inline">Tag:</span>
                <button
                  onClick={() => setTagFilter("all")}
                  className={`rounded-lg px-2 py-1 text-xs font-mono transition cursor-pointer ${
                    tagFilter === "all"
                      ? "bg-slate-700/80 text-slate-200 border border-slate-600"
                      : "text-slate-400 hover:text-slate-200 hover:bg-slate-800"
                  }`}
                >
                  #all
                </button>
                {availableTags.map((tag) => (
                  <button
                    key={tag}
                    onClick={() => setTagFilter(tagFilter === tag ? "all" : tag)}
                    className={`rounded-lg px-2 py-1 text-xs font-mono transition cursor-pointer ${
                      tagFilter === tag
                        ? "bg-indigo-600/30 text-indigo-300 border border-indigo-500/40"
                        : "text-slate-400 hover:text-slate-200 hover:bg-slate-800"
                    }`}
                  >
                    #{tag}
                  </button>
                ))}
              </div>
            </>
          )}

          {/* Task count pill */}
          <span className="hidden md:inline-flex items-center rounded-full bg-slate-800/80 border border-slate-700/50 px-2.5 py-0.5 text-[11px] font-mono text-slate-400">
            {filteredSessions.length} tasks
          </span>
        </div>

        {/* Right Toolbar Actions */}
        <div className="flex items-center gap-2">
          {/* Project Memory Ledger Button */}
          <button
            onClick={() => setMemoryModalOpen(true)}
            title="Xem & sửa nhật ký dự án (.agent/memory.md)"
            className="flex items-center gap-1.5 rounded-xl border border-slate-700/80 bg-slate-800/70 hover:bg-slate-750 px-3 py-1.5 text-xs font-medium text-indigo-300 hover:text-indigo-200 shadow-sm transition cursor-pointer"
          >
            <BookOpen className="h-3.5 w-3.5 text-indigo-400" />
            <span className="hidden sm:inline">Nhật ký (Memory)</span>
          </button>

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
            className="flex items-center gap-1.5 rounded-lg bg-indigo-600 hover:bg-indigo-500 active:bg-indigo-700 px-3.5 py-1.5 text-xs font-semibold text-white shadow-sm shadow-indigo-600/30 transition cursor-pointer"
          >
            <Plus className="h-3.5 w-3.5 stroke-[2.5]" />
            <span>Giao Task mới</span>
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
              onCleanupTodo={col.status === "todo" ? onCleanupTodo : undefined}
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

      {/* Project Memory Ledger Modal */}
      <MemoryLedgerModal
        isOpen={memoryModalOpen}
        workspace={workspace}
        onClose={() => setMemoryModalOpen(false)}
      />
    </div>
  );
};
