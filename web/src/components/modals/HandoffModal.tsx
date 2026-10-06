import React, { useState } from "react";
import { BookOpen, Check, Copy, X } from "lucide-react";
import { relTime } from "../../constants/agentMeta";

interface Props {
  isOpen: boolean;
  content: string;
  path: string;
  updatedAt: string;
  hasActiveTab: boolean;
  onClose: () => void;
  onSendToActiveTab: () => void;
}

export const HandoffModal: React.FC<Props> = ({
  isOpen,
  content,
  path,
  updatedAt,
  hasActiveTab,
  onClose,
  onSendToActiveTab,
}) => {
  const [copied, setCopied] = useState(false);

  if (!isOpen) return null;

  const handleCopy = () => {
    navigator.clipboard?.writeText(content);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm p-4 animate-in fade-in duration-150">
      <div className="w-full max-w-2xl max-h-[85vh] rounded-xl border border-[#2b3548] bg-[#121620] shadow-2xl overflow-hidden flex flex-col text-slate-200">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-[#1f2636] px-5 py-3.5 bg-[#161b27]">
          <div className="flex items-center gap-2">
            <BookOpen className="h-4 w-4 text-indigo-400" />
            <span className="text-sm font-semibold text-slate-100">.agent/handoff.md</span>
            {updatedAt && (
              <span className="text-[11px] text-slate-500 font-mono">
                · {relTime(updatedAt)}
              </span>
            )}
          </div>
          <div className="flex items-center gap-2">
            <button
              onClick={handleCopy}
              className="flex cursor-pointer items-center gap-1 rounded bg-white/[0.06] px-2.5 py-1 text-[11px] font-medium text-slate-300 hover:bg-white/[0.1] transition-colors"
              title="Copy nội dung handoff"
            >
              {copied ? <Check className="h-3 w-3 text-emerald-400" /> : <Copy className="h-3 w-3" />}
              <span>{copied ? "Copied" : "Copy"}</span>
            </button>
            <button
              onClick={onClose}
              className="cursor-pointer p-1 text-slate-500 hover:text-slate-300 rounded"
            >
              <X className="h-4 w-4" />
            </button>
          </div>
        </div>

        {/* Content */}
        <div className="flex-1 overflow-y-auto p-4 font-mono text-xs leading-relaxed text-slate-300 bg-[#0c0e14] whitespace-pre-wrap select-text">
          {content ? (
            content
          ) : (
            <div className="py-8 text-center text-slate-500">
              File .agent/handoff.md hiện đang trống hoặc chưa được khởi tạo.
              <div className="mt-2 text-[11px]">
                Bấm "Write Handoff" ở thanh tab để yêu cầu agent tóm tắt và ghi lại tiến độ.
              </div>
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="flex items-center justify-between border-t border-[#1c2230] px-5 py-3 bg-[#161b27]">
          <span className="truncate font-mono text-[10.5px] text-slate-500" title={path}>
            {path}
          </span>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => {
                onClose();
                onSendToActiveTab();
              }}
              disabled={!content || !hasActiveTab}
              className="cursor-pointer rounded-lg bg-indigo-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-indigo-500 disabled:opacity-50 disabled:cursor-not-allowed shadow transition-colors"
            >
              Send to Active Tab
            </button>
            <button
              type="button"
              onClick={onClose}
              className="cursor-pointer rounded-lg border border-[#2c3549] bg-[#161a24] px-3.5 py-1.5 text-xs font-medium text-slate-300 hover:bg-[#202634] transition-colors"
            >
              Close
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};
