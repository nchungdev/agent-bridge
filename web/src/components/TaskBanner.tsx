import React, { useState } from "react";
import { Loader2, ChevronDown, ChevronUp, ShieldAlert, Check, X } from "lucide-react";

interface TaskBannerProps {
  isRunning: boolean;
  commandText?: string;
  requiresApproval?: boolean;
  onApprove?: () => void;
  onReject?: () => void;
  onApproveSession?: () => void;
  toolName?: string;
}

export const TaskBanner: React.FC<TaskBannerProps> = ({
  isRunning,
  commandText,
  requiresApproval,
  onApprove,
  onReject,
  onApproveSession,
  toolName,
}) => {
  const [isOpen, setIsOpen] = useState(true);

  if (!isRunning && !requiresApproval) return null;

  return (
    <div className="w-full max-w-4xl mx-auto px-4 mb-2 select-none animate-in fade-in slide-in-from-bottom-2 duration-150">
      <div
        className={`rounded-xl border shadow-md overflow-hidden text-slate-300 transition-all ${
          requiresApproval
            ? "bg-[#1c1816] border-amber-600/40"
            : "bg-[#181c24] border-[#262c38]"
        }`}
      >
        {/* Header */}
        <div
          onClick={() => setIsOpen(!isOpen)}
          className="flex items-center justify-between px-3.5 py-2 cursor-pointer hover:bg-black/20 transition-colors text-xs font-medium"
        >
          <div className="flex items-center gap-2">
            {requiresApproval ? (
              <>
                <ShieldAlert className="w-4 h-4 text-amber-400 shrink-0" />
                <span className="text-amber-300 font-semibold">Permission required to execute command</span>
              </>
            ) : (
              <>
                <span className="w-2 h-2 rounded-full bg-sky-400 animate-ping" />
                <span className="text-slate-300">1 task running</span>
              </>
            )}
          </div>
          {isOpen ? (
            <ChevronUp className="w-3.5 h-3.5 text-slate-500" />
          ) : (
            <ChevronDown className="w-3.5 h-3.5 text-slate-500" />
          )}
        </div>

        {/* Content: spinner / command / approve buttons */}
        {isOpen && (
          <div className="px-3.5 py-2.5 bg-[#12151c]/80 border-t border-black/30 flex flex-col sm:flex-row sm:items-center justify-between gap-3 text-xs font-mono">
            <div className="flex items-center gap-2.5 text-slate-300 min-w-0 flex-1">
              {requiresApproval ? (
                <span className="px-1.5 py-0.5 rounded bg-amber-950/60 border border-amber-800/40 text-amber-300 font-bold text-[10px] shrink-0">
                  ACTION
                </span>
              ) : (
                <Loader2 className="w-3.5 h-3.5 text-sky-400 animate-spin shrink-0" />
              )}
              <span className="truncate text-slate-200 break-all select-all">
                {commandText || "Executing command..."}
              </span>
            </div>

            {requiresApproval && (
              <div className="flex items-center gap-2 shrink-0 select-none">
                <button
                  type="button"
                  onClick={onReject}
                  className="px-3 py-1 rounded-lg bg-rose-950/40 hover:bg-rose-900/60 border border-rose-800/50 text-rose-300 flex items-center gap-1.5 font-sans font-medium text-[11.5px] transition-all cursor-pointer"
                >
                  <X className="w-3 h-3 text-rose-400" />
                  <span>Reject</span>
                </button>
                {onApproveSession && (
                  <button
                    type="button"
                    onClick={onApproveSession}
                    title={`Allow every ${toolName || "such"} request in this chat`}
                    className="px-3 py-1 rounded-lg bg-emerald-950/50 hover:bg-emerald-900/60 border border-emerald-800/50 text-emerald-300 font-sans font-medium text-[11.5px] transition-all cursor-pointer"
                  >
                    Allow {toolName || "all"} this chat
                  </button>
                )}
                <button
                  type="button"
                  onClick={onApprove}
                  className="px-3 py-1 rounded-lg bg-emerald-700/80 hover:bg-emerald-600 border border-emerald-500/60 text-white flex items-center gap-1.5 font-sans font-medium text-[11.5px] shadow-sm transition-all cursor-pointer"
                >
                  <Check className="w-3.5 h-3.5 text-emerald-200 stroke-[2.5]" />
                  <span>Approve</span>
                </button>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
};
