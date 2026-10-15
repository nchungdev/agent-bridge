import React, { useState } from "react";
import { Plus, ListTodo, PlayCircle, Eye, CheckCircle2, Trash2 } from "lucide-react";
import type { BridgeSession, EngineStatus, KanbanStatus } from "../../types/bridge";
import { KanbanCard } from "./KanbanCard";

interface KanbanColumnProps {
  status: KanbanStatus;
  title: string;
  tasks: BridgeSession[];
  installedEngines: EngineStatus[];
  onOpenTerminal: (task: BridgeSession) => void;
  onOpenRemote: (task: BridgeSession) => void;
  onUpdateStatus: (taskId: string, status: KanbanStatus) => void;
  onAssignAgent: (taskId: string, agent: string) => void;
  onToggleSubtask: (taskId: string, subtaskId: string, done: boolean) => void;
  onAddSubtask: (taskId: string, title: string) => void;
  onDeleteSubtask: (taskId: string, subtaskId: string) => void;
  onDeleteTask: (taskId: string) => void;
  onAddTaskToColumn?: (status: KanbanStatus) => void;
  onViewDetail?: (task: BridgeSession) => void;
  onCleanupTodo?: () => void;
}

const COLUMN_CONFIG: Record<
  KanbanStatus,
  { icon: React.ElementType; badgeBg: string; badgeText: string; dotBg: string }
> = {
  todo: {
    icon: ListTodo,
    badgeBg: "bg-slate-800",
    badgeText: "text-slate-300",
    dotBg: "bg-slate-400",
  },
  in_progress: {
    icon: PlayCircle,
    badgeBg: "bg-indigo-950/60",
    badgeText: "text-indigo-300",
    dotBg: "bg-indigo-400",
  },
  review: {
    icon: Eye,
    badgeBg: "bg-amber-950/60",
    badgeText: "text-amber-300",
    dotBg: "bg-amber-400",
  },
  done: {
    icon: CheckCircle2,
    badgeBg: "bg-emerald-950/60",
    badgeText: "text-emerald-300",
    dotBg: "bg-emerald-400",
  },
};

export const KanbanColumn: React.FC<KanbanColumnProps> = ({
  status,
  title,
  tasks,
  installedEngines,
  onOpenTerminal,
  onOpenRemote,
  onUpdateStatus,
  onAssignAgent,
  onToggleSubtask,
  onAddSubtask,
  onDeleteSubtask,
  onDeleteTask,
  onAddTaskToColumn,
  onViewDetail,
  onCleanupTodo,
}) => {
  const [isOver, setIsOver] = useState(false);
  const cfg = COLUMN_CONFIG[status] || COLUMN_CONFIG.todo;
  const Icon = cfg.icon;

  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault();
    e.dataTransfer.dropEffect = "move";
    if (!isOver) setIsOver(true);
  };

  const handleDragLeave = () => {
    setIsOver(false);
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setIsOver(false);
    const taskId = e.dataTransfer.getData("text/plain");
    if (taskId) {
      onUpdateStatus(taskId, status);
    }
  };

  return (
    <div
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
      className={`flex flex-col flex-1 min-w-[280px] max-w-[360px] h-full rounded-2xl border transition-all duration-200 bg-[#0d1017] ${
        isOver
          ? "border-indigo-500/80 bg-indigo-950/15 ring-2 ring-indigo-500/30"
          : "border-slate-800/80 hover:border-slate-700/80"
      }`}
    >
      {/* Column Header */}
      <div className="flex items-center justify-between p-3.5 pb-2 border-b border-slate-800/60">
        <div className="flex items-center gap-2">
          <span className={`h-2 w-2 rounded-full ${cfg.dotBg}`} />
          <Icon className="h-4 w-4 text-slate-400" />
          <h2 className="font-semibold text-xs text-slate-200 uppercase tracking-wider">{title}</h2>
          <span className={`ml-1 rounded-full px-2 py-0.5 text-[11px] font-mono font-medium ${cfg.badgeBg} ${cfg.badgeText}`}>
            {tasks.length}
          </span>
        </div>

        <div className="flex items-center gap-1">
          {status === "todo" && tasks.length > 0 && onCleanupTodo && (
            <button
              onClick={() => {
                if (confirm(`Bạn có chắc muốn xoá sạch toàn bộ ${tasks.length} task trong cột CẦN LÀM để làm mới bảng?`)) {
                  onCleanupTodo();
                }
              }}
              title="Dọn dẹp sạch toàn bộ task trong cột To Do"
              className="flex items-center gap-1 rounded-md px-2 py-1 text-[11px] text-slate-400 hover:text-rose-400 hover:bg-rose-950/30 transition cursor-pointer border border-slate-800/90"
            >
              <Trash2 className="h-3 w-3" />
              <span>Dọn dẹp</span>
            </button>
          )}

          {onAddTaskToColumn && (
            <button
              onClick={() => onAddTaskToColumn(status)}
              title="Thêm task mới vào cột này"
              className="flex items-center gap-1 rounded-lg p-1 text-slate-400 hover:bg-slate-800 hover:text-slate-100 transition cursor-pointer"
            >
              <Plus className="h-4 w-4" />
            </button>
          )}
        </div>
      </div>

      {/* Cards Scroll Area */}
      <div className="flex-1 overflow-y-auto p-3 space-y-3 min-h-0">
        {tasks.map((task) => (
          <KanbanCard
            key={task.id}
            task={task}
            installedEngines={installedEngines}
            onOpenTerminal={onOpenTerminal}
            onOpenRemote={onOpenRemote}
            onUpdateStatus={onUpdateStatus}
            onAssignAgent={onAssignAgent}
            onToggleSubtask={onToggleSubtask}
            onAddSubtask={onAddSubtask}
            onDeleteSubtask={onDeleteSubtask}
            onDeleteTask={onDeleteTask}
            onViewDetail={onViewDetail}
          />
        ))}

        {tasks.length === 0 && (
          <div
            onClick={() => onAddTaskToColumn?.(status)}
            className="flex flex-col items-center justify-center rounded-xl border border-dashed border-slate-800/80 p-6 text-center text-xs text-slate-500 hover:border-slate-700 hover:text-slate-400 transition cursor-pointer"
          >
            <p>Kéo thả task vào đây</p>
            {onAddTaskToColumn && <p className="text-[11px] text-indigo-400/80 mt-1">+ Thêm task mới</p>}
          </div>
        )}
      </div>
    </div>
  );
};
