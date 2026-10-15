import React, { useState } from "react";
import {
  X,
  ExternalLink,
  Terminal,
  Trash2,
  Plus,
  CheckSquare,
  Square,
  CheckCircle2,
  Split,
  Save,
  Clock,
  Download,
  FileText
} from "lucide-react";
import type { BridgeSession, EngineStatus, KanbanStatus } from "../../types/bridge";

interface TaskDetailModalProps {
  task: BridgeSession | null;
  isOpen: boolean;
  installedEngines: EngineStatus[];
  onClose: () => void;
  onOpenTerminal: (task: BridgeSession) => void;
  onOpenRemote: (task: BridgeSession) => void;
  onUpdateStatus: (taskId: string, status: KanbanStatus) => void;
  onAssignAgent: (taskId: string, agent: string) => void;
  onToggleSubtask: (taskId: string, subtaskId: string, done: boolean) => void;
  onAddSubtask: (taskId: string, title: string) => void;
  onDeleteSubtask: (taskId: string, subtaskId: string) => void;
  onDecompose: (taskId: string, titles: string[]) => void;
  onUpdateResult: (taskId: string, summary: string, status?: string) => void;
  onDeleteTask: (taskId: string) => void;
}

export const TaskDetailModal: React.FC<TaskDetailModalProps> = ({
  task,
  isOpen,
  installedEngines: _installedEngines,
  onClose,
  onOpenTerminal,
  onOpenRemote,
  onUpdateStatus,
  onAssignAgent,
  onToggleSubtask,
  onAddSubtask,
  onDeleteSubtask,
  onDecompose,
  onUpdateResult,
  onDeleteTask,
}) => {
  const [subtaskInput, setSubtaskInput] = useState("");
  const [decomposeText, setDecomposeText] = useState("");
  const [showDecompose, setShowDecompose] = useState(false);
  const [resultSummary, setResultSummary] = useState(task?.result_summary || "");
  const [editingResult, setEditingResult] = useState(false);

  if (!isOpen || !task) return null;

  const subtasks = task.subtasks || [];
  const completed = subtasks.filter((s) => s.done).length;
  const percent = subtasks.length > 0 ? Math.round((completed / subtasks.length) * 100) : 0;

  const handleAddSubtask = (e: React.FormEvent) => {
    e.preventDefault();
    if (subtaskInput.trim()) {
      onAddSubtask(task.id, subtaskInput.trim());
      setSubtaskInput("");
    }
  };

  const handleDecomposeSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const lines = decomposeText
      .split("\n")
      .map((l) => l.trim())
      .filter((l) => l.length > 0);
    if (lines.length > 0) {
      onDecompose(task.id, lines);
      setDecomposeText("");
      setShowDecompose(false);
    }
  };

  const handleSaveResult = () => {
    onUpdateResult(task.id, resultSummary, task.kanban_status);
    setEditingResult(false);
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/75 backdrop-blur-xs">
      <div className="relative flex flex-col w-full max-w-2xl max-h-[90vh] rounded-2xl border border-slate-800 bg-[#121622] shadow-2xl text-slate-100 overflow-hidden animate-in fade-in zoom-in-95 duration-150">
        {/* Header */}
        <div className="flex items-center justify-between p-5 border-b border-slate-800">
          <div className="flex items-center gap-2">
            <span className="font-mono text-xs text-slate-500">{task.id}</span>
          </div>

          <div className="flex items-center gap-2">
            {/* Open Remote Button */}
            <button
              onClick={() => {
                onOpenRemote(task);
              }}
              className="flex items-center gap-1.5 rounded-lg bg-indigo-600/30 hover:bg-indigo-600/50 border border-indigo-500/40 px-3 py-1.5 text-xs font-medium text-indigo-200 transition cursor-pointer"
            >
              <ExternalLink className="h-3.5 w-3.5" />
              <span>Mở Native Remote</span>
            </button>

            {/* Inspect Terminal Button */}
            <button
              onClick={() => {
                onOpenTerminal(task);
                onClose();
              }}
              className="flex items-center gap-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 px-3 py-1.5 text-xs font-medium text-slate-200 transition cursor-pointer"
            >
              <Terminal className="h-3.5 w-3.5" />
              <span>Xem Console/CLI</span>
            </button>

            <button
              onClick={onClose}
              className="rounded-lg p-1.5 text-slate-400 hover:bg-slate-800 hover:text-slate-200 transition cursor-pointer"
            >
              <X className="h-4 w-4" />
            </button>
          </div>
        </div>

        {/* Modal Body */}
        <div className="flex-1 overflow-y-auto p-5 space-y-5">
          {/* Title */}
          <div>
            <h1 className="text-lg font-semibold text-slate-100 leading-snug">{task.title}</h1>
            <div className="flex items-center gap-2 mt-2 text-xs text-slate-400">
              <Clock className="h-3.5 w-3.5" />
              <span>Cập nhật: {new Date(task.updated_at).toLocaleString()}</span>
            </div>
          </div>

          {/* Status & Agent Selection Grid */}
          <div className="grid grid-cols-2 gap-3 p-3.5 rounded-xl bg-slate-900/60 border border-slate-800">
            <div>
              <label className="block text-[11px] font-semibold uppercase text-slate-400 mb-1">
                Trạng thái Kanban
              </label>
              <select
                value={task.kanban_status || "todo"}
                onChange={(e) => onUpdateStatus(task.id, e.target.value as KanbanStatus)}
                className="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-1.5 text-xs text-slate-200 focus:border-indigo-500 focus:outline-none cursor-pointer"
              >
                <option value="todo">To Do (Cần làm)</option>
                <option value="in_progress">In Progress (Đang chạy)</option>
                <option value="review">Review & Handoff (Chờ duyệt)</option>
                <option value="done">Done (Hoàn thành)</option>
              </select>
            </div>

            <div>
              <label className="block text-[11px] font-semibold uppercase text-slate-400 mb-1">
                Agent phụ trách
              </label>
              <select
                value={task.current_agent || "agy"}
                onChange={(e) => onAssignAgent(task.id, e.target.value)}
                className="w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-1.5 text-xs text-slate-200 focus:border-indigo-500 focus:outline-none cursor-pointer"
              >
                <option value="agy">Antigravity (Google)</option>
                <option value="claude">Claude Code (Anthropic)</option>
                <option value="codex">Codex CLI (OpenAI)</option>
              </select>
            </div>
          </div>

          {/* Subtasks Checklist */}
          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <h3 className="font-semibold text-xs uppercase tracking-wider text-slate-300">
                  Subtasks Checklist
                </h3>
                {subtasks.length > 0 && (
                  <span className="rounded-full bg-slate-800 px-2 py-0.5 text-xs font-mono text-slate-400">
                    {completed}/{subtasks.length} ({percent}%)
                  </span>
                )}
              </div>

              <button
                onClick={() => setShowDecompose(!showDecompose)}
                className="flex items-center gap-1 rounded-lg border border-indigo-800/60 bg-indigo-950/40 px-2.5 py-1 text-xs text-indigo-300 hover:bg-indigo-900/40 transition cursor-pointer"
              >
                <Split className="h-3 w-3" />
                <span>Phân rã hàng loạt</span>
              </button>
            </div>

            {/* Progress Bar */}
            {subtasks.length > 0 && (
              <div className="h-2 w-full rounded-full bg-slate-800 overflow-hidden">
                <div
                  className={`h-full transition-all duration-300 ${
                    percent === 100 ? "bg-emerald-500" : "bg-indigo-500"
                  }`}
                  style={{ width: `${percent}%` }}
                />
              </div>
            )}

            {/* Bulk Decompose Form */}
            {showDecompose && (
              <form onSubmit={handleDecomposeSubmit} className="rounded-xl border border-indigo-900/60 bg-indigo-950/20 p-3 space-y-2">
                <div className="text-xs text-indigo-300 font-medium">
                  Nhập danh sách subtasks (mỗi dòng 1 subtask):
                </div>
                <textarea
                  rows={3}
                  value={decomposeText}
                  onChange={(e) => setDecomposeText(e.target.value)}
                  placeholder="Bước 1: Khởi tạo module...&#10;Bước 2: Viết test...&#10;Bước 3: Chạy linter..."
                  className="w-full rounded-lg border border-slate-700 bg-slate-900 px-3 py-2 text-xs text-slate-200 focus:border-indigo-500 focus:outline-none"
                />
                <div className="flex justify-end gap-2">
                  <button
                    type="button"
                    onClick={() => setShowDecompose(false)}
                    className="rounded-lg px-2.5 py-1 text-xs text-slate-400 hover:text-slate-200 cursor-pointer"
                  >
                    Huỷ
                  </button>
                  <button
                    type="submit"
                    className="rounded-lg bg-indigo-600 px-3 py-1 text-xs font-semibold text-white hover:bg-indigo-500 cursor-pointer"
                  >
                    Áp dụng phân rã
                  </button>
                </div>
              </form>
            )}

            {/* Subtask List */}
            <div className="space-y-1.5">
              {subtasks.map((st) => (
                <div
                  key={st.id}
                  className="flex items-center justify-between gap-3 rounded-lg border border-slate-800/80 bg-slate-900/40 p-2.5 text-xs hover:border-slate-700 transition"
                >
                  <button
                    onClick={() => onToggleSubtask(task.id, st.id, !st.done)}
                    className="flex items-center gap-2.5 flex-1 text-left cursor-pointer"
                  >
                    {st.done ? (
                      <CheckSquare className="h-4 w-4 text-emerald-400 shrink-0" />
                    ) : (
                      <Square className="h-4 w-4 text-slate-500 hover:text-slate-300 shrink-0" />
                    )}
                    <span
                      className={`text-xs ${
                        st.done ? "text-slate-500 line-through" : "text-slate-200"
                      }`}
                    >
                      {st.title}
                    </span>
                  </button>

                  <button
                    onClick={() => onDeleteSubtask(task.id, st.id)}
                    className="text-slate-500 hover:text-rose-400 transition p-1 cursor-pointer"
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                </div>
              ))}
            </div>

            {/* Quick Add Subtask Form */}
            <form onSubmit={handleAddSubtask} className="flex items-center gap-2 pt-1">
              <input
                type="text"
                placeholder="Thêm subtask mới..."
                value={subtaskInput}
                onChange={(e) => setSubtaskInput(e.target.value)}
                className="flex-1 rounded-lg border border-slate-700 bg-slate-900 px-3 py-2 text-xs text-slate-200 placeholder-slate-500 focus:border-indigo-500 focus:outline-none"
              />
              <button
                type="submit"
                className="flex items-center gap-1 rounded-lg bg-indigo-600 px-3 py-2 text-xs font-semibold text-white hover:bg-indigo-500 transition cursor-pointer"
              >
                <Plus className="h-3.5 w-3.5" />
                <span>Thêm</span>
              </button>
            </form>
          </div>

          {/* Result / Outcome Summary */}
          <div className="space-y-2.5 pt-2 border-t border-slate-800">
            <div className="flex items-center justify-between flex-wrap gap-2">
              <h3 className="font-semibold text-xs uppercase tracking-wider text-slate-300 flex items-center gap-1.5">
                <CheckCircle2 className="h-3.5 w-3.5 text-emerald-400" />
                <span>Kết quả thực hiện & Báo cáo</span>
              </h3>
              <div className="flex items-center gap-2">
                <a
                  href={`/api/bridge/tasks/${task.id}/report.md`}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="flex items-center gap-1.5 rounded-lg bg-indigo-600/30 hover:bg-indigo-600/50 border border-indigo-500/40 px-2.5 py-1 text-xs font-medium text-indigo-200 transition cursor-pointer"
                  title="Mở file báo cáo .md trực tiếp trong tab mới"
                >
                  <ExternalLink className="h-3 w-3" />
                  <span>Xem file .md</span>
                </a>
                <a
                  href={`/api/bridge/tasks/${task.id}/report.md?download=1`}
                  className="flex items-center gap-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 border border-slate-700 px-2.5 py-1 text-xs font-medium text-slate-300 transition cursor-pointer"
                  title="Tải file báo cáo (.md) về máy"
                >
                  <Download className="h-3 w-3" />
                  <span>Tải về (.md)</span>
                </a>
                {!editingResult && (
                  <button
                    onClick={() => {
                      setResultSummary(task.result_summary || "");
                      setEditingResult(true);
                    }}
                    className="text-xs text-slate-400 hover:text-slate-200 cursor-pointer ml-1"
                  >
                    Chỉnh sửa
                  </button>
                )}
              </div>
            </div>

            {editingResult ? (
              <div className="space-y-2">
                <textarea
                  rows={4}
                  value={resultSummary}
                  onChange={(e) => setResultSummary(e.target.value)}
                  placeholder="Ghi chú kết quả hoàn thành task hoặc báo cáo từ agent..."
                  className="w-full rounded-xl border border-slate-700 bg-slate-900 p-3 text-xs text-slate-200 focus:border-indigo-500 focus:outline-none font-mono"
                />
                <div className="flex justify-end gap-2">
                  <button
                    onClick={() => setEditingResult(false)}
                    className="rounded-lg px-3 py-1 text-xs text-slate-400 hover:text-slate-200 cursor-pointer"
                  >
                    Huỷ
                  </button>
                  <button
                    onClick={handleSaveResult}
                    className="flex items-center gap-1 rounded-lg bg-emerald-600 px-3 py-1 text-xs font-semibold text-white hover:bg-emerald-500 cursor-pointer"
                  >
                    <Save className="h-3 w-3" />
                    <span>Lưu kết quả</span>
                  </button>
                </div>
              </div>
            ) : (
              <div className="space-y-2">
                <div className="rounded-xl border border-emerald-900/40 bg-emerald-950/20 p-3.5 text-xs text-emerald-200/90 font-mono whitespace-pre-wrap leading-relaxed">
                  {task.result_summary || (
                    <span className="text-slate-500 italic">Chưa có kết quả báo cáo.</span>
                  )}
                </div>
                {task.result_summary && (
                  <div className="flex items-center gap-2 text-[11px] text-slate-500 font-mono">
                    <FileText className="h-3 w-3 text-slate-400" />
                    <span>File lưu tại:</span>
                    <code className="text-slate-300 bg-slate-900/80 px-1.5 py-0.5 rounded border border-slate-800">
                      .agent/reports/{task.id.slice(0, 18)}.md
                    </code>
                  </div>
                )}
              </div>
            )}
          </div>
        </div>

        {/* Modal Footer */}
        <div className="flex items-center justify-between p-4 border-t border-slate-800 bg-[#0e121b]">
          <button
            onClick={() => {
              if (confirm("Bạn có chắc muốn xoá task này?")) {
                onDeleteTask(task.id);
                onClose();
              }
            }}
            className="flex items-center gap-1.5 rounded-lg text-xs text-rose-400 hover:text-rose-300 hover:bg-rose-950/30 px-3 py-1.5 transition cursor-pointer"
          >
            <Trash2 className="h-3.5 w-3.5" />
            <span>Xoá Task</span>
          </button>

          <button
            onClick={onClose}
            className="rounded-lg bg-slate-800 hover:bg-slate-700 px-4 py-1.5 text-xs font-medium text-slate-200 transition cursor-pointer"
          >
            Đóng
          </button>
        </div>
      </div>
    </div>
  );
};
