import React, { useState } from "react";
import {
  BookOpen,
  Eraser,
  Eye,
  FileEdit,
  MoreHorizontal,
  Plus,
  Radio,
  Terminal as TerminalIcon,
  X,
} from "lucide-react";
import type { EngineStatus, TermTab } from "../../types/bridge";
import { AGENT_META } from "../../constants/agentMeta";
import { UsageMeter } from "../UsageMeter";

interface Props {
  tabs: TermTab[];
  activeKey: string | null;
  availableSessionAgents: EngineStatus[];
  hasSessionShell: boolean;
  activeTab: TermTab | null;
  isMobile: boolean;
  wideToolbar: boolean;
  workspace: string;
  ctx: { agent: string; id: string; title: string; workspace: string } | null;
  onSelectTab: (key: string) => void;
  onCloseTab: (key: string) => void;
  onOpenAgentInSession: (agentId: string) => void;
  onOpenShellInSession: () => void;
  onTriggerHandoff: (action: "read" | "write") => void;
  onViewHandoff: () => void;
  onTriggerInput: (text: string, label?: string) => void;
}

export const SessionTabBar: React.FC<Props> = ({
  tabs,
  activeKey,
  availableSessionAgents,
  hasSessionShell,
  activeTab,
  isMobile,
  wideToolbar,
  workspace,
  ctx,
  onSelectTab,
  onCloseTab,
  onOpenAgentInSession,
  onOpenShellInSession,
  onTriggerHandoff,
  onViewHandoff,
  onTriggerInput,
}) => {
  const [switchOpen, setSwitchOpen] = useState(false);
  const [switchPos, setSwitchPos] = useState({ x: 0, y: 0 });
  const [moreOpen, setMoreOpen] = useState(false);
  const [morePos, setMorePos] = useState({ x: 0, y: 0 });

  return (
    <div className="flex h-9 shrink-0 items-center gap-1 overflow-x-auto border-b border-[#1d222b] bg-[#101319] px-2 select-none">
      {tabs.map((t) => {
        const isActive = t.key === activeKey;
        return (
          <div
            key={t.key}
            onClick={() => onSelectTab(t.key)}
            className={`flex shrink-0 cursor-pointer items-center gap-1.5 rounded-md px-2.5 py-1 text-[12px] transition-colors ${
              isActive
                ? "bg-white/[0.08] font-medium text-slate-100 border border-white/10 shadow-sm"
                : "text-slate-400 hover:bg-white/[0.04] hover:text-slate-200 border border-transparent"
            }`}
          >
            {t.agent ? (
              <span className={`h-1.5 w-1.5 rounded-full ${AGENT_META[t.agent]?.dot}`} />
            ) : (
              <TerminalIcon className="h-3.5 w-3.5 text-sky-400" />
            )}
            <span title={t.workDir}>{t.label}</span>
            {(t.launch?.remote || t.remoteOn) && (
              <span title="Remote: điều khiển được từ app web/mobile của agent">
                <Radio className="h-3 w-3 text-emerald-400" />
              </span>
            )}
            <button
              onClick={(e) => {
                e.stopPropagation();
                onCloseTab(t.key);
              }}
              className="cursor-pointer rounded p-0.5 text-slate-500 hover:text-rose-400 transition-colors"
              title="Đóng tab này"
            >
              <X className="h-3 w-3" />
            </button>
          </div>
        );
      })}

      {/* Add agent or shell to current session */}
      {(availableSessionAgents.length > 0 || !hasSessionShell) && (
        <div className="relative shrink-0">
          <button
            onClick={(e) => {
              const r = e.currentTarget.getBoundingClientRect();
              setSwitchPos({
                x: Math.max(8, Math.min(r.left, window.innerWidth - 232)),
                y: r.bottom + 4,
              });
              setSwitchOpen((v) => !v);
            }}
            className="flex cursor-pointer items-center gap-1 rounded-md px-2 py-1 text-[11.5px] text-slate-400 hover:bg-white/[0.04] hover:text-slate-200"
            title="Thêm agent hoặc shell vào task này"
          >
            <Plus className="h-3.5 w-3.5" />
            <span className="text-[11px]">Add</span>
          </button>

          {switchOpen && (
            <>
              <div className="fixed inset-0 z-40" onClick={() => setSwitchOpen(false)} />
              <div
                style={{ left: switchPos.x, top: switchPos.y }}
                className="fixed z-50 w-56 max-w-[calc(100vw-1.5rem)] rounded-xl border border-[#2c3447] bg-[#161b26] p-1.5 shadow-2xl backdrop-blur"
              >
                <div className="px-2.5 py-1 text-[10.5px] font-semibold uppercase tracking-wider text-slate-400 border-b border-[#21293a] mb-1">
                  Mở trong task này
                </div>
                {availableSessionAgents.map((e) => (
                  <button
                    key={e.id}
                    onClick={() => {
                      setSwitchOpen(false);
                      onOpenAgentInSession(e.id);
                    }}
                    className="flex w-full cursor-pointer items-center gap-2 rounded-lg px-2.5 py-1.5 text-left text-xs text-slate-200 hover:bg-[#222a3a]"
                  >
                    <span className={`h-1.5 w-1.5 rounded-full ${AGENT_META[e.id]?.dot}`} />
                    <span>{e.name}</span>
                  </button>
                ))}
                {!hasSessionShell && (
                  <button
                    onClick={() => {
                      setSwitchOpen(false);
                      onOpenShellInSession();
                    }}
                    className="flex w-full cursor-pointer items-center gap-2 rounded-lg px-2.5 py-1.5 text-left text-xs text-slate-200 hover:bg-[#222a3a]"
                  >
                    <TerminalIcon className="h-3.5 w-3.5 text-sky-400" />
                    <span>Shell</span>
                  </button>
                )}
              </div>
            </>
          )}
        </div>
      )}

      {/* Right side: handoff actions and /clear */}
      <div className="ml-auto flex shrink-0 items-center gap-1.5 pr-1">
        {isMobile && activeTab?.agent && (
          <UsageMeter
            agent={activeTab.agent}
            nativeId={(ctx?.agent === activeTab.agent ? ctx.id : "") || activeTab.launch?.resume || ""}
            workspace={activeTab.workDir || workspace}
          />
        )}

        <div className={`${wideToolbar ? "flex" : "hidden"} items-center gap-1.5`}>
          <div className="flex items-center gap-0.5 rounded-md bg-[#161a24] p-0.5 border border-white/5">
            <button
              onClick={() => onTriggerHandoff("read")}
              disabled={!activeTab}
              className="flex cursor-pointer items-center gap-1 rounded px-2 py-0.5 text-[11px] font-medium text-slate-300 hover:bg-white/[0.08] hover:text-indigo-300 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
              title="Gửi lệnh 'Đọc .agent/handoff.md' cho active tab"
            >
              <BookOpen className="h-3 w-3 text-indigo-400" />
              <span className="hidden sm:inline">Read Handoff</span>
            </button>
            <button
              onClick={() => onTriggerHandoff("write")}
              disabled={!activeTab}
              className="flex cursor-pointer items-center gap-1 rounded px-2 py-0.5 text-[11px] font-medium text-slate-300 hover:bg-white/[0.08] hover:text-emerald-300 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
              title="Yêu cầu agent tóm tắt và ghi vào .agent/handoff.md"
            >
              <FileEdit className="h-3 w-3 text-emerald-400" />
              <span className="hidden sm:inline">Write Handoff</span>
            </button>
            <button
              onClick={onViewHandoff}
              className="flex cursor-pointer items-center gap-1 rounded px-1.5 py-0.5 text-[11px] text-slate-400 hover:bg-white/[0.08] hover:text-slate-200 transition-colors"
              title="Xem nội dung .agent/handoff.md hiện tại"
            >
              <Eye className="h-3 w-3 text-slate-400" />
            </button>
          </div>

          <button
            onClick={() => onTriggerInput("/clear\n", "Sent /clear to active tab")}
            disabled={!activeTab}
            className="flex cursor-pointer items-center gap-1 rounded px-2 py-1 text-[11px] text-slate-400 hover:bg-white/[0.04] hover:text-slate-200 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
            title="Gửi lệnh /clear đến active tab"
          >
            <Eraser className="h-3 w-3 text-slate-500" />
            <span>/clear</span>
          </button>
        </div>

        <div className={wideToolbar ? "hidden" : ""}>
          <button
            onClick={(e) => {
              const r = e.currentTarget.getBoundingClientRect();
              setMorePos({
                x: Math.max(8, Math.min(r.right - 208, window.innerWidth - 216)),
                y: r.bottom + 4,
              });
              setMoreOpen((v) => !v);
            }}
            className="flex cursor-pointer items-center rounded-md border border-white/5 bg-[#161a24] px-1.5 py-1 text-slate-300 hover:bg-white/[0.08] hover:text-slate-100"
            title="Thao tác khác"
          >
            <MoreHorizontal className="h-4 w-4" />
          </button>

          {moreOpen && (
            <>
              <div className="fixed inset-0 z-40" onClick={() => setMoreOpen(false)} />
              <div
                style={{ left: morePos.x, top: morePos.y }}
                className="fixed z-50 w-52 rounded-xl border border-[#2c3447] bg-[#161b26] p-1.5 shadow-2xl backdrop-blur"
              >
                {(
                  [
                    [BookOpen, "text-indigo-400", "Read Handoff", () => onTriggerHandoff("read"), !activeTab],
                    [FileEdit, "text-emerald-400", "Write Handoff", () => onTriggerHandoff("write"), !activeTab],
                    [Eye, "text-slate-400", "Xem handoff.md", onViewHandoff, false],
                    [Eraser, "text-slate-500", "/clear", () => onTriggerInput("/clear\n", "Sent /clear to active tab"), !activeTab],
                  ] as const
                ).map(([Icon, color, label, run, disabled]) => (
                  <button
                    key={label}
                    disabled={disabled}
                    onClick={() => {
                      setMoreOpen(false);
                      run();
                    }}
                    className="flex w-full cursor-pointer items-center gap-2 rounded-lg px-2.5 py-1.5 text-left text-xs text-slate-200 hover:bg-[#222a3a] disabled:cursor-not-allowed disabled:opacity-40"
                  >
                    <Icon className={`h-3.5 w-3.5 ${color}`} />
                    {label}
                  </button>
                ))}
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  );
};
