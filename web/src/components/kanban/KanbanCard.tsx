import React, { useState } from "react";
import {
  CheckSquare,
  Square,
  Plus,
  Trash2,
  ExternalLink,
  Terminal,
  ChevronDown,
  ChevronRight,
  MoreVertical,
  Bot,
  CheckCircle2,
  Clock,
  ArrowRight,
  Play
} from "lucide-react";
import type { BridgeSession, EngineStatus, KanbanStatus } from "../../types/bridge";

interface KanbanCardProps {
  task: BridgeSession;
  installedEngines: EngineStatus[];
  onOpenTerminal: (task: BridgeSession) => void;
  onOpenRemote: (task: BridgeSession) => void;
  onUpdateStatus: (taskId: string, status: KanbanStatus) => void;
  onAssignAgent: (taskId: string, agent: string) => void;
  onToggleSubtask: (taskId: string, subtaskId: string, done: boolean) => void;
  onAddSubtask: (taskId: string, title: string) => void;
  onDeleteSubtask: (taskId: string, subtaskId: string) => void;
  onDeleteTask: (taskId: string) => void;
  onViewDetail?: (task: BridgeSession) => void;
}

const AGENT_COLORS: Record<string, { bg: string; text: string; border: string; label: string }> = {
  agy: { bg: "bg-purple-950/40", text: "text-purple-300", border: "border-purple-800/40", label: "Antigravity" },
  claude: { bg: "bg-amber-950/40", text: "text-amber-300", border: "border-amber-800/40", label: "Claude Code" },
  codex: { bg: "bg-emerald-950/40", text: "text-emerald-300", border: "border-emerald-800/40", label: "Codex CLI" },
};

export const KanbanCard: React.FC<KanbanCardProps> = ({
  task,
  installedEngines: _installedEngines,
  onOpenTerminal,
  onOpenRemote,
  onUpdateStatus,
  onAssignAgent,
  onToggleSubtask,
  onAddSubtask,
  onDeleteSubtask,
  onDeleteTask,
  onViewDetail,
}) => {
  const [expandedSubtasks, setExpandedSubtasks] = useState(false);
  const [newSubtaskTitle, setNewSubtaskTitle] = useState("");
  const [isAddingSubtask, setIsAddingSubtask] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);

  const subtasks = task.subtasks || [];
  const completedSubtasks = subtasks.filter((st) => st.done).length;
  const progressPercent = subtasks.length > 0 ? Math.round((completedSubtasks / subtasks.length) * 100) : 0;

  const currentAgent = task.current_agent || "agy";
  const agentMeta = AGENT_COLORS[currentAgent] || {
    bg: "bg-slate-800/60",
    text: "text-slate-300",
    border: "border-slate-700",
    label: currentAgent,
  };

  const handleDragStart = (e: React.DragEvent) => {
    e.dataTransfer.setData("text/plain", task.id);
    e.dataTransfer.effectAllowed = "move";
  };

  const handleAddSubtaskSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (newSubtaskTitle.trim()) {
      onAddSubtask(task.id, newSubtaskTitle.trim());
      setNewSubtaskTitle("");
      setIsAddingSubtask(false);
    }
  };

  return (
    <div
      draggable
      onDragStart={handleDragStart}
      className="group relative flex flex-col rounded-xl border border-slate-800/80 bg-[#121620] p-3.5 shadow-sm transition-all hover:border-slate-700 hover:bg-[#151a26] hover:shadow-md cursor-grab active:cursor-grabbing"
    >
      {/* Top row: Status/Agent Indicator + Actions */}
      <div className="flex items-center justify-between gap-2 mb-2">
        {/* Left: Agent/Assignee Indicator */}
        <div className="relative">
          {task.kanban_status === "todo" ? (
            /* In TO DO: Show Assignee badge with editable trigger */
            <button
              onClick={(e) => {
                e.stopPropagation();
                const nextAgents = ["codex", "claude", "agy"];
                const curIdx = nextAgents.indexOf(currentAgent);
                const next = nextAgents[(curIdx + 1) % nextAgents.length];
                onAssignAgent(task.id, next);
              }}
              title="Click để đổi Agent phụ trách"
              className="inline-flex items-center gap-1.5 rounded-md px-2 py-0.5 text-[11px] font-medium border border-slate-700/80 bg-slate-800/50 text-slate-300 hover:border-slate-600 hover:text-slate-100 transition cursor-pointer"
            >
              <Bot className="h-3 w-3 text-slate-400" />
              <span>Giao: <span className={agentMeta.text}>{agentMeta.label}</span></span>
            </button>
          ) : task.kanban_status === "in_progress" ? (
            /* In PROGRESS: Show Active pulsing status */
            <div className={`inline-flex items-center gap-1.5 rounded-md px-2 py-0.5 text-[11px] font-medium border ${agentMeta.bg} ${agentMeta.text} ${agentMeta.border}`}>
              <span className="relative flex h-2 w-2">
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
                <span className="relative inline-flex rounded-full h-2 w-2 bg-emerald-500"></span>
              </span>
              <span>{agentMeta.label}</span>
            </div>
          ) : task.kanban_status === "review" ? (
            /* In REVIEW */
            <div className="inline-flex items-center gap-1.5 rounded-md px-2 py-0.5 text-[11px] font-medium border bg-amber-950/40 text-amber-300 border-amber-800/40">
              <Clock className="h-3 w-3 text-amber-400" />
              <span>Chờ nghiệm thu</span>
            </div>
          ) : (
            /* In DONE */
            <div className="inline-flex items-center gap-1.5 rounded-md px-2 py-0.5 text-[11px] font-medium border bg-emerald-950/40 text-emerald-300 border-emerald-800/40">
              <CheckCircle2 className="h-3 w-3 text-emerald-400" />
              <span>Hoàn thành</span>
            </div>
          )}
        </div>

        {/* Quick Actions */}
        <div className="flex items-center gap-1">
          {/* If in TO DO: Show prominent [▶ Bắt đầu] button */}
          {task.kanban_status === "todo" && (
            <button
              onClick={(e) => {
                e.stopPropagation();
                onUpdateStatus(task.id, "in_progress");
                onOpenTerminal(task);
              }}
              title="Khởi chạy Agent thực thi task này"
              className="flex items-center gap-1 rounded-md bg-indigo-600 hover:bg-indigo-500 active:bg-indigo-700 text-white px-2 py-0.5 text-[11px] font-semibold shadow-sm transition cursor-pointer"
            >
              <Play className="h-2.5 w-2.5 fill-current" />
              <span>Bắt đầu</span>
            </button>
          )}

          {/* If IN PROGRESS or REVIEW: Show Remote & Console */}
          {(task.kanban_status === "in_progress" || task.kanban_status === "review") && (
            <>
              {/* Direct Remote Launcher */}
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  onOpenRemote(task);
                }}
                title={`Mở giao diện Web/Remote của ${agentMeta.label}`}
                className="flex items-center gap-1 rounded-md bg-indigo-950/50 hover:bg-indigo-900/60 text-indigo-300 border border-indigo-800/40 px-2 py-0.5 text-[11px] font-medium transition cursor-pointer"
              >
                <ExternalLink className="h-3 w-3" />
                <span>Remote</span>
              </button>

              {/* Inspect Terminal */}
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  onOpenTerminal(task);
                }}
                title="Xem Terminal / CLI trực tiếp"
                className="rounded-md p-1 text-slate-400 hover:bg-slate-800 hover:text-slate-200 transition cursor-pointer"
              >
                <Terminal className="h-3.5 w-3.5" />
              </button>
            </>
          )}

          {/* Quick Menu */}
          <div className="relative">
            <button
              onClick={(e) => {
                e.stopPropagation();
                setMenuOpen(!menuOpen);
              }}
              className="rounded-md p-1 text-slate-400 hover:bg-slate-800 hover:text-slate-200 transition cursor-pointer"
            >
              <MoreVertical className="h-3.5 w-3.5" />
            </button>

            {menuOpen && (
              <>
                <div
                  className="fixed inset-0 z-40"
                  onClick={(e) => {
                    e.stopPropagation();
                    setMenuOpen(false);
                  }}
                />
                <div className="absolute right-0 top-6 z-50 w-44 rounded-lg border border-slate-700 bg-[#161b26] p-1 shadow-xl text-xs text-slate-200">
                  <div className="px-2 py-1 text-[10px] uppercase font-semibold text-slate-400 border-b border-slate-800 mb-1">
                    Chuyển trạng thái
                  </div>
                  {(["todo", "in_progress", "review", "done"] as KanbanStatus[]).map((st) => (
                    <button
                      key={st}
                      onClick={(e) => {
                        e.stopPropagation();
                        onUpdateStatus(task.id, st);
                        setMenuOpen(false);
                      }}
                      className={`flex w-full items-center gap-2 rounded px-2 py-1.5 text-left hover:bg-slate-800 transition ${
                        task.kanban_status === st ? "text-indigo-400 font-semibold" : ""
                      }`}
                    >
                      <ArrowRight className="h-3 w-3" />
                      {st === "todo" && "To Do"}
                      {st === "in_progress" && "In Progress"}
                      {st === "review" && "Review / Handoff"}
                      {st === "done" && "Done"}
                    </button>
                  ))}
                  <div className="border-t border-slate-800 my-1" />
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      onDeleteTask(task.id);
                      setMenuOpen(false);
                    }}
                    className="flex w-full items-center gap-2 rounded px-2 py-1.5 text-rose-400 hover:bg-rose-950/30 transition"
                  >
                    <Trash2 className="h-3 w-3" />
                    <span>Xoá Task</span>
                  </button>
                </div>
              </>
            )}
          </div>
        </div>
      </div>

      {/* Task Title */}
      <h3
        onClick={() => onViewDetail?.(task)}
        className="font-medium text-sm text-slate-100 hover:text-indigo-300 transition line-clamp-2 cursor-pointer leading-snug mb-2"
      >
        {task.title}
      </h3>

      {/* Result summary snippet if present */}
      {task.result_summary && (
        <div className="mb-2 rounded-md bg-emerald-950/30 border border-emerald-800/30 px-2.5 py-1.5 text-xs text-emerald-200 flex items-start gap-1.5">
          <CheckCircle2 className="h-3.5 w-3.5 text-emerald-400 shrink-0 mt-0.5" />
          <span className="line-clamp-2 font-mono text-[11px]">{task.result_summary}</span>
        </div>
      )}

      {/* Subtasks Section */}
      <div className="mt-1 pt-2 border-t border-slate-800/60">
        <div className="flex items-center justify-between text-xs text-slate-400 mb-1">
          <button
            onClick={(e) => {
              e.stopPropagation();
              setExpandedSubtasks(!expandedSubtasks);
            }}
            className="flex items-center gap-1 hover:text-slate-200 transition cursor-pointer"
          >
            {expandedSubtasks ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
            <span className="font-medium text-[11px]">Subtasks</span>
            <span className="text-[10px] text-slate-500">
              ({completedSubtasks}/{subtasks.length})
            </span>
          </button>

          <button
            onClick={(e) => {
              e.stopPropagation();
              setIsAddingSubtask(!isAddingSubtask);
              if (!expandedSubtasks) setExpandedSubtasks(true);
            }}
            title="Thêm Subtask"
            className="rounded p-0.5 text-slate-400 hover:text-indigo-400 hover:bg-slate-800 transition cursor-pointer"
          >
            <Plus className="h-3.5 w-3.5" />
          </button>
        </div>

        {/* Progress bar */}
        {subtasks.length > 0 && (
          <div className="h-1.5 w-full rounded-full bg-slate-800 overflow-hidden mb-2">
            <div
              className={`h-full transition-all duration-300 ${
                progressPercent === 100 ? "bg-emerald-500" : "bg-indigo-500"
              }`}
              style={{ width: `${progressPercent}%` }}
            />
          </div>
        )}

        {/* Subtasks List */}
        {expandedSubtasks && (
          <div className="space-y-1.5 mt-2">
            {subtasks.map((st) => (
              <div
                key={st.id}
                className="group/st flex items-center justify-between gap-2 rounded px-1.5 py-1 text-xs hover:bg-slate-800/40 transition"
              >
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    onToggleSubtask(task.id, st.id, !st.done);
                  }}
                  className="flex items-center gap-2 flex-1 text-left cursor-pointer"
                >
                  {st.done ? (
                    <CheckSquare className="h-3.5 w-3.5 text-emerald-400 shrink-0" />
                  ) : (
                    <Square className="h-3.5 w-3.5 text-slate-500 hover:text-slate-300 shrink-0" />
                  )}
                  <span
                    className={`text-[12px] leading-tight ${
                      st.done ? "text-slate-500 line-through" : "text-slate-300"
                    }`}
                  >
                    {st.title}
                  </span>
                </button>

                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    onDeleteSubtask(task.id, st.id);
                  }}
                  className="opacity-0 group-hover/st:opacity-100 p-0.5 text-slate-500 hover:text-rose-400 transition cursor-pointer"
                >
                  <Trash2 className="h-3 w-3" />
                </button>
              </div>
            ))}

            {/* Inline Add Subtask Input */}
            {isAddingSubtask && (
              <form onSubmit={handleAddSubtaskSubmit} className="flex items-center gap-1.5 pt-1">
                <input
                  type="text"
                  autoFocus
                  placeholder="Nhập tên subtask..."
                  value={newSubtaskTitle}
                  onChange={(e) => setNewSubtaskTitle(e.target.value)}
                  className="flex-1 rounded border border-slate-700 bg-slate-900 px-2 py-1 text-xs text-slate-200 placeholder-slate-500 focus:border-indigo-500 focus:outline-none"
                />
                <button
                  type="submit"
                  className="rounded bg-indigo-600 px-2 py-1 text-[11px] font-medium text-white hover:bg-indigo-500 cursor-pointer"
                >
                  Thêm
                </button>
              </form>
            )}
          </div>
        )}
      </div>

      {/* Card Footer: Timestamp and Native Bound Tools */}
      <div className="mt-3 flex items-center justify-between text-[10px] text-slate-500">
        <span className="flex items-center gap-1">
          <Clock className="h-3 w-3" />
          {task.updated_at ? new Date(task.updated_at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }) : "Vừa xong"}
        </span>

        {task.tools && task.tools.length > 0 && (
          <div className="flex items-center gap-1">
            {task.tools.map((tb) => (
              <span
                key={tb.agent}
                title={`Native session: ${tb.native_session_id}`}
                className="font-mono text-[9px] px-1 py-0.2 rounded bg-slate-800 text-slate-400 border border-slate-700"
              >
                {tb.agent}
              </span>
            ))}
          </div>
        )}
      </div>
    </div>
  );
};
