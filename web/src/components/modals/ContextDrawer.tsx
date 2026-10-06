import React from "react";
import { Check, Copy, GitBranch, X } from "lucide-react";
import type { NativeSession } from "../../types/bridge";
import { AGENT_META, relTime } from "../../constants/agentMeta";

interface Props {
  isOpen: boolean;
  ctx: { agent: string; id: string; title: string; workspace: string } | null;
  detail: NativeSession | null;
  modifiedFiles: string[];
  copied: boolean;
  onClose: () => void;
  onCopySessionId: (id: string) => void;
}

export const ContextDrawer: React.FC<Props> = ({
  isOpen,
  ctx,
  detail,
  modifiedFiles,
  copied,
  onClose,
  onCopySessionId,
}) => {
  if (!isOpen) return null;

  const allTurns = detail?.turns || [];
  const lastTurns = allTurns.slice(-6);

  return (
    <aside className="fixed inset-0 z-30 flex w-full flex-col border-l border-[#1d222b] bg-[#101319] pt-[env(safe-area-inset-top)] md:static md:z-auto md:w-80 md:shrink-0 md:pt-0 xl:w-[360px]">
      <div className="flex items-center justify-between border-b border-[#1d222b] px-4 py-2.5">
        <span className="text-[11px] font-semibold uppercase tracking-wider text-slate-500">Context</span>
        <button onClick={onClose} className="cursor-pointer p-0.5 text-slate-500 hover:text-slate-300">
          <X className="h-3.5 w-3.5" />
        </button>
      </div>

      <div className="min-h-0 flex-1 space-y-4 overflow-y-auto p-4">
        {!ctx ? (
          <div className="text-[12px] leading-relaxed text-slate-500">
            Chưa có session nào. Gửi lệnh đầu tiên để xem ngữ cảnh tại đây.
          </div>
        ) : (
          <>
            <div>
              <div className="flex items-center gap-2">
                <span className={`rounded border px-1.5 py-0.5 text-[10px] font-medium ${AGENT_META[ctx.agent]?.badge}`}>
                  {AGENT_META[ctx.agent]?.label}
                </span>
                <button
                  onClick={() => onCopySessionId(ctx.id)}
                  className="ml-auto flex cursor-pointer items-center gap-1 font-mono text-[10px] text-slate-500 hover:text-slate-300"
                  title="Copy session id"
                >
                  {copied ? <Check className="h-3 w-3 text-emerald-400" /> : <Copy className="h-3 w-3" />}
                  {ctx.id.slice(0, 8)}
                </button>
              </div>
              <div className="mt-1.5 text-[13px] font-semibold leading-snug text-slate-100">
                {ctx.title || "Untitled"}
              </div>
              <div className="mt-0.5 text-[11px] text-slate-500">
                {detail?.turn_count ?? 0} messages
                {detail?.updated_at && relTime(detail.updated_at) && ` · updated ${relTime(detail.updated_at)}`}
              </div>
            </div>

            <div>
              <div className="mb-2 text-[10.5px] font-semibold uppercase tracking-wider text-indigo-300">
                {lastTurns.length} tin nhắn gần nhất · handoff payload
              </div>
              {!detail && <div className="text-[12px] text-slate-500">Đang đọc transcript…</div>}
              <div className="space-y-2.5">
                {lastTurns.map((t, i) => (
                  <div
                    key={i}
                    className={`rounded-lg border px-3 py-2 ${
                      t.role === "user" ? "border-[#2a3347] bg-[#171c28]" : "border-[#232a39] bg-[#121620]"
                    }`}
                  >
                    <div className="mb-0.5 text-[10px] font-medium uppercase tracking-wide text-slate-500">
                      {t.role === "user" ? "User" : AGENT_META[ctx.agent]?.label}
                    </div>
                    <div className="line-clamp-6 whitespace-pre-wrap break-words text-[12px] leading-relaxed text-slate-300">
                      {t.content}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          </>
        )}

        <div>
          <div className="mb-2 flex items-center gap-1.5 text-[10.5px] font-semibold uppercase tracking-wider text-slate-500">
            <GitBranch className="h-3.5 w-3.5 text-emerald-400" />
            Changed Files
            <span className="ml-auto font-normal normal-case tracking-normal">{modifiedFiles.length}</span>
          </div>
          {modifiedFiles.length === 0 ? (
            <div className="text-[11.5px] text-slate-500">Working tree sạch.</div>
          ) : (
            <div className="max-h-56 space-y-1 overflow-y-auto">
              {modifiedFiles.map((f) => (
                <div
                  key={f}
                  className="truncate rounded bg-[#1a2030] px-2 py-1 font-mono text-[10.5px] text-slate-300"
                  title={f}
                >
                  {f}
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </aside>
  );
};
