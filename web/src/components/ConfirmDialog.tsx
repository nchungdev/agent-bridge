import React, { useEffect, useRef } from "react";
import { createPortal } from "react-dom";
import { AlertTriangle } from "lucide-react";

interface Props {
  title: string;
  message: React.ReactNode;
  confirmLabel?: string;
  cancelLabel?: string;
  danger?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}

/** In-app modal confirmation (rendered into <body>, so it is never clipped by the sidebar). */
export const ConfirmDialog: React.FC<Props> = ({ title, message, confirmLabel = "Confirm", cancelLabel = "Cancel", danger, onConfirm, onCancel }) => {
  const cancelRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    cancelRef.current?.focus(); // safe default for destructive actions
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") { e.stopPropagation(); onCancel(); }
    };
    document.addEventListener("keydown", onKey, true);
    return () => document.removeEventListener("keydown", onKey, true);
  }, [onCancel]);

  return createPortal(
    <div
      className="fixed inset-0 z-[100] flex items-center justify-center bg-black/60 p-4 animate-in fade-in duration-100"
      onMouseDown={(e) => { if (e.target === e.currentTarget) onCancel(); }}
      role="alertdialog"
      aria-modal="true"
      aria-label={title}
    >
      <div className="w-full max-w-sm rounded-2xl border border-[#2c3344] bg-[#171b23] p-5 shadow-2xl text-slate-200">
        <div className="flex items-start gap-3">
          {danger && (
            <span className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-rose-950/60 border border-rose-800/50">
              <AlertTriangle className="h-4 w-4 text-rose-400" />
            </span>
          )}
          <div className="min-w-0">
            <h2 className="text-[15px] font-semibold text-slate-100">{title}</h2>
            <div className="mt-1.5 text-[13px] leading-relaxed text-slate-400 break-words">{message}</div>
          </div>
        </div>
        <div className="mt-5 flex justify-end gap-2">
          <button
            ref={cancelRef}
            type="button"
            onClick={onCancel}
            className="px-3.5 py-1.5 rounded-lg bg-[#222836] hover:bg-[#2a3142] text-[13px] text-slate-200 cursor-pointer focus:outline-none focus:ring-2 focus:ring-sky-500/60"
          >
            {cancelLabel}
          </button>
          <button
            type="button"
            onClick={onConfirm}
            className={`px-3.5 py-1.5 rounded-lg text-[13px] font-medium text-white cursor-pointer focus:outline-none focus:ring-2 ${
              danger ? "bg-rose-700 hover:bg-rose-600 focus:ring-rose-400/60" : "bg-sky-600 hover:bg-sky-500 focus:ring-sky-400/60"
            }`}
          >
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>,
    document.body
  );
};
