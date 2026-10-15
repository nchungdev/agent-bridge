import React, { useState } from "react";
import { X, Plus, Trash2, Sparkles, CheckSquare } from "lucide-react";
import type { EngineStatus, KanbanStatus } from "../../types/bridge";

interface NewTaskModalProps {
  isOpen: boolean;
  initialStatus?: KanbanStatus;
  installedEngines: EngineStatus[];
  onClose: () => void;
  onCreateTask: (
    title: string,
    description: string,
    agent: string,
    status: KanbanStatus,
    subtasks: string[]
  ) => Promise<void>;
}

export const NewTaskModal: React.FC<NewTaskModalProps> = ({
  isOpen,
  initialStatus = "todo",
  installedEngines: _installedEngines,
  onClose,
  onCreateTask,
}) => {
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [agent, setAgent] = useState("agy");
  const [status, setStatus] = useState<KanbanStatus>(initialStatus);
  const [subtasksText, setSubtasksText] = useState("");
  const [subtaskList, setSubtaskList] = useState<string[]>([]);
  const [subtaskInput, setSubtaskInput] = useState("");
  const [submitting, setSubmitting] = useState(false);

  if (!isOpen) return null;

  const handleAddSubtask = (e?: React.FormEvent) => {
    e?.preventDefault();
    if (subtaskInput.trim()) {
      setSubtaskList([...subtaskList, subtaskInput.trim()]);
      setSubtaskInput("");
    }
  };

  const handleRemoveSubtask = (index: number) => {
    setSubtaskList(subtaskList.filter((_, i) => i !== index));
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!title.trim()) return;

    // Combine list items + any pasted multi-line text
    const extra = subtasksText
      .split("\n")
      .map((s) => s.trim())
      .filter((s) => s.length > 0);
    const allSubtasks = [...subtaskList, ...extra];

    setSubmitting(true);
    try {
      await onCreateTask(title.trim(), description.trim(), agent, status, allSubtasks);
      setTitle("");
      setDescription("");
      setSubtaskList([]);
      setSubtasksText("");
      onClose();
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-xs">
      <div className="relative w-full max-w-lg rounded-2xl border border-slate-800 bg-[#121622] p-6 shadow-2xl text-slate-100 animate-in fade-in zoom-in-95 duration-150">
        <div className="flex items-center justify-between border-b border-slate-800 pb-3 mb-4">
          <div className="flex items-center gap-2">
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-indigo-600/20 text-indigo-400 border border-indigo-500/30">
              <Sparkles className="h-4 w-4" />
            </div>
            <div>
              <h2 className="font-semibold text-base text-slate-100">Giao Task mới</h2>
              <p className="text-xs text-slate-400">Tạo nhiệm vụ cho Autonomous AI Agent</p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="rounded-lg p-1.5 text-slate-400 hover:bg-slate-800 hover:text-slate-200 transition cursor-pointer"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          {/* Task Title */}
          <div>
            <label className="block text-xs font-semibold uppercase text-slate-400 mb-1.5">
              Mục tiêu / Tên Task <span className="text-rose-400">*</span>
            </label>
            <input
              type="text"
              autoFocus
              required
              placeholder="VD: Viết endpoint quản lý webhook Zalo và test e2e..."
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              className="w-full rounded-xl border border-slate-700 bg-slate-900/90 px-3.5 py-2.5 text-sm text-slate-100 placeholder-slate-500 focus:border-indigo-500 focus:outline-none focus:ring-1 focus:ring-indigo-500"
            />
          </div>

          {/* Task Description / Content */}
          <div>
            <label className="block text-xs font-semibold uppercase text-slate-400 mb-1.5 flex items-center justify-between">
              <span>Nội dung / Yêu cầu chi tiết (Prompt)</span>
              <span className="text-[10px] text-slate-500 font-normal lowercase">Tuỳ chọn</span>
            </label>
            <textarea
              rows={3}
              placeholder="Ghi rõ bối cảnh, yêu cầu kỹ thuật, file liên quan hoặc nội dung prompt chi tiết để Agent thực hiện..."
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              className="w-full rounded-xl border border-slate-700 bg-slate-900/90 px-3.5 py-2.5 text-xs text-slate-100 placeholder-slate-500 focus:border-indigo-500 focus:outline-none focus:ring-1 focus:ring-indigo-500 leading-relaxed"
            />
          </div>

          {/* Agent and Status selectors */}
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-xs font-semibold uppercase text-slate-400 mb-1.5">
                Chỉ định Agent
              </label>
              <select
                value={agent}
                onChange={(e) => setAgent(e.target.value)}
                className="w-full rounded-xl border border-slate-700 bg-slate-900 px-3 py-2 text-xs text-slate-200 focus:border-indigo-500 focus:outline-none cursor-pointer"
              >
                <option value="agy">Antigravity (Google)</option>
                <option value="claude">Claude Code (Anthropic)</option>
                <option value="codex">Codex CLI (OpenAI)</option>
              </select>
            </div>

            <div>
              <label className="block text-xs font-semibold uppercase text-slate-400 mb-1.5">
                Cột Kanban
              </label>
              <select
                value={status}
                onChange={(e) => setStatus(e.target.value as KanbanStatus)}
                className="w-full rounded-xl border border-slate-700 bg-slate-900 px-3 py-2 text-xs text-slate-200 focus:border-indigo-500 focus:outline-none cursor-pointer"
              >
                <option value="todo">To Do (Cần làm)</option>
                <option value="in_progress">In Progress (Bắt đầu ngay)</option>
                <option value="review">Review & Handoff</option>
                <option value="done">Done (Đã xong)</option>
              </select>
            </div>
          </div>

          {/* Subtasks breakdown */}
          <div>
            <label className="block text-xs font-semibold uppercase text-slate-400 mb-1.5">
              Phân rã Subtasks (Checklist cho Agent)
            </label>

            {/* Subtask input item */}
            <div className="flex items-center gap-2 mb-2">
              <input
                type="text"
                placeholder="Thêm subtask từng bước..."
                value={subtaskInput}
                onChange={(e) => setSubtaskInput(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") {
                    e.preventDefault();
                    handleAddSubtask();
                  }
                }}
                className="flex-1 rounded-lg border border-slate-700 bg-slate-900/80 px-3 py-1.5 text-xs text-slate-200 placeholder-slate-500 focus:border-indigo-500 focus:outline-none"
              />
              <button
                type="button"
                onClick={() => handleAddSubtask()}
                className="rounded-lg bg-slate-800 px-3 py-1.5 text-xs font-medium text-slate-200 hover:bg-slate-700 cursor-pointer"
              >
                <Plus className="h-3.5 w-3.5" />
              </button>
            </div>

            {/* List of added subtasks */}
            {subtaskList.length > 0 && (
              <div className="max-h-28 overflow-y-auto space-y-1 mb-2 pr-1">
                {subtaskList.map((st, i) => (
                  <div
                    key={i}
                    className="flex items-center justify-between gap-2 rounded-md bg-slate-900/60 px-2.5 py-1 text-xs text-slate-300 border border-slate-800/80"
                  >
                    <div className="flex items-center gap-2 overflow-hidden">
                      <CheckSquare className="h-3.5 w-3.5 text-indigo-400 shrink-0" />
                      <span className="truncate">{st}</span>
                    </div>
                    <button
                      type="button"
                      onClick={() => handleRemoveSubtask(i)}
                      className="text-slate-500 hover:text-rose-400 transition cursor-pointer"
                    >
                      <Trash2 className="h-3 w-3" />
                    </button>
                  </div>
                ))}
              </div>
            )}

            {/* Multi-line paste input */}
            <textarea
              rows={2}
              placeholder="Hoặc dán danh sách nhiều dòng (mỗi dòng 1 subtask)..."
              value={subtasksText}
              onChange={(e) => setSubtasksText(e.target.value)}
              className="w-full rounded-xl border border-slate-700/80 bg-slate-900/60 px-3 py-2 text-xs text-slate-200 placeholder-slate-500 focus:border-indigo-500 focus:outline-none"
            />
          </div>

          {/* Footer buttons */}
          <div className="flex items-center justify-end gap-2.5 pt-3 border-t border-slate-800">
            <button
              type="button"
              onClick={onClose}
              className="rounded-xl px-4 py-2 text-xs font-medium text-slate-400 hover:bg-slate-800 hover:text-slate-200 transition cursor-pointer"
            >
              Huỷ
            </button>
            <button
              type="submit"
              disabled={submitting || !title.trim()}
              className="rounded-xl bg-indigo-600 px-5 py-2 text-xs font-semibold text-white shadow-lg shadow-indigo-600/30 hover:bg-indigo-500 disabled:opacity-50 transition cursor-pointer"
            >
              {submitting ? "Đang tạo..." : "Tạo & Giao Task"}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
