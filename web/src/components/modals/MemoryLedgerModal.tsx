import React, { useState, useEffect } from "react";
import { BookOpen, X, Edit3, Save, Check, RefreshCw, Sparkles } from "lucide-react";

interface MemoryLedgerModalProps {
  isOpen: boolean;
  workspace: string;
  onClose: () => void;
}

export const MemoryLedgerModal: React.FC<MemoryLedgerModalProps> = ({
  isOpen,
  workspace,
  onClose,
}) => {
  const [content, setContent] = useState("");
  const [isEditing, setIsEditing] = useState(false);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [savedSuccess, setSavedSuccess] = useState(false);

  const loadMemory = async () => {
    if (!workspace) return;
    setLoading(true);
    try {
      const q = new URLSearchParams({ workspace });
      const r = await fetch(`/api/bridge/workspace/memory?${q}`);
      if (r.ok) {
        const data = await r.json();
        setContent(data.content || "");
      }
    } catch (e) {
      console.error("Load memory error:", e);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (isOpen) {
      loadMemory();
      setIsEditing(false);
      setSavedSuccess(false);
    }
  }, [isOpen, workspace]);

  const handleSave = async () => {
    if (!workspace) return;
    setSaving(true);
    try {
      const r = await fetch("/api/bridge/workspace/memory", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ workspace, content }),
      });
      if (r.ok) {
        setIsEditing(false);
        setSavedSuccess(true);
        setTimeout(() => setSavedSuccess(false), 2000);
      }
    } catch (e) {
      console.error("Save memory error:", e);
    } finally {
      setSaving(false);
    }
  };

  if (!isOpen) return null;

  const currentWsName = workspace.split("/").filter(Boolean).pop() || "Project";

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/75 p-4 backdrop-blur-sm animate-in fade-in duration-150">
      <div className="flex h-[82vh] w-full max-w-3xl flex-col rounded-2xl border border-slate-800 bg-[#121622] shadow-2xl overflow-hidden">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-slate-800/80 px-6 py-4 bg-[#0e121c]">
          <div className="flex items-center gap-3">
            <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-indigo-600/20 text-indigo-400 border border-indigo-500/30">
              <BookOpen className="h-5 w-5" />
            </div>
            <div>
              <div className="flex items-center gap-2">
                <h2 className="text-base font-bold text-slate-100">Project Memory & Learnings</h2>
                <span className="inline-flex items-center gap-1 rounded-full bg-indigo-950/60 border border-indigo-800/50 px-2 py-0.5 text-[10px] font-medium text-indigo-300">
                  <Sparkles className="h-2.5 w-2.5" />
                  Auto-Ledger
                </span>
              </div>
              <p className="text-xs text-slate-400 font-mono mt-0.5">
                {currentWsName} &bull; <span className="text-slate-500">.agent/memory.md</span>
              </p>
            </div>
          </div>

          <div className="flex items-center gap-2">
            {isEditing ? (
              <button
                onClick={handleSave}
                disabled={saving}
                className="flex items-center gap-1.5 rounded-lg bg-indigo-600 hover:bg-indigo-500 active:bg-indigo-700 text-white px-3 py-1.5 text-xs font-semibold shadow transition cursor-pointer disabled:opacity-50"
              >
                {saving ? <RefreshCw className="h-3.5 w-3.5 animate-spin" /> : <Save className="h-3.5 w-3.5" />}
                <span>Lưu nhật ký</span>
              </button>
            ) : (
              <button
                onClick={() => setIsEditing(true)}
                className="flex items-center gap-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-200 border border-slate-700 px-3 py-1.5 text-xs font-medium transition cursor-pointer"
              >
                <Edit3 className="h-3.5 w-3.5 text-indigo-400" />
                <span>Chỉnh sửa</span>
              </button>
            )}

            <button
              onClick={onClose}
              className="rounded-lg p-1.5 text-slate-400 hover:bg-slate-800 hover:text-slate-200 transition cursor-pointer"
            >
              <X className="h-5 w-5" />
            </button>
          </div>
        </div>

        {/* Content Body */}
        <div className="flex-1 overflow-y-auto p-6 bg-[#0c0f17]">
          {loading ? (
            <div className="flex h-full items-center justify-center text-slate-500 gap-2">
              <RefreshCw className="h-4 w-4 animate-spin text-indigo-400" />
              <span className="text-xs">Đang tải nhật ký dự án...</span>
            </div>
          ) : isEditing ? (
            <div className="flex h-full flex-col">
              <textarea
                value={content}
                onChange={(e) => setContent(e.target.value)}
                placeholder="# Project Memory & Knowledge Ledger&#10;&#10;Ghi nhận các quy tắc, ghi chú hoặc bài học kinh nghiệm để Agent tự động tuân theo..."
                className="flex-1 w-full resize-none rounded-xl border border-slate-800 bg-[#121620] p-4 font-mono text-xs text-slate-200 placeholder-slate-600 focus:border-indigo-500 focus:outline-none focus:ring-1 focus:ring-indigo-500"
              />
              <p className="text-[11px] text-slate-500 mt-2">
                Tip: Mọi Agent (Codex, Claude, Antigravity) khi nhận task mới trong dự án này sẽ đọc các lưu ý tại đây.
              </p>
            </div>
          ) : (
            <div className="prose prose-invert prose-sm max-w-none font-sans space-y-3">
              {content ? (
                <pre className="whitespace-pre-wrap font-sans text-xs text-slate-300 leading-relaxed bg-[#121620] border border-slate-800/80 rounded-xl p-5 select-text overflow-x-auto">
                  {content}
                </pre>
              ) : (
                <div className="flex flex-col items-center justify-center h-48 text-center text-slate-500 text-xs">
                  <BookOpen className="h-8 w-8 text-slate-600 mb-2" />
                  <p>Chưa có nhật ký nào được ghi nhận cho dự án này.</p>
                  <p className="text-[11px] text-slate-600 mt-1">Khi bạn hoàn thành các task, Agent sẽ tự động tích lũy bài học vào đây.</p>
                </div>
              )}
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="flex items-center justify-between border-t border-slate-800/80 px-6 py-3 bg-[#0e121c] text-xs text-slate-400">
          <div className="flex items-center gap-2">
            <span className="h-2 w-2 rounded-full bg-emerald-400"></span>
            <span>Tự động đồng bộ với mọi Agent CLI và Terminal</span>
          </div>

          {savedSuccess && (
            <div className="flex items-center gap-1.5 text-emerald-400 font-medium">
              <Check className="h-4 w-4" />
              <span>Đã lưu thành công!</span>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
