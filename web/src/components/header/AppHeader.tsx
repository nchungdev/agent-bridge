import React, { useState } from "react";
import {
  Check,
  Copy,
  Layers,
  ChevronLeft,
  ChevronDown,
  Plus,
  MoreVertical,
  BookOpen,
  FileEdit,
  Eye,
  Eraser,
  Terminal as TerminalIcon,
  X,
} from "lucide-react";
import type { BridgeSession, EngineStatus, TermTab } from "../../types/bridge";
import { AGENT_META } from "../../constants/agentMeta";
import { RemoteControl } from "../RemoteControl";
import { UsageMeter } from "../UsageMeter";

interface Props {
  isMobile: boolean;
  activeSessionInfo: BridgeSession | null;
  activeTab: TermTab | null;
  activeSessionId: string | null;
  workspace: string;
  copied: boolean;
  ctx: { agent: string; id: string; title: string; workspace: string } | null;
  onBackToTasks?: () => void;
  onCopySessionId: (id: string) => void;
  onTriggerInput: (text: string, label?: string) => void;
  onSetRemoteOn: (key: string) => void;
  onRestartRemote: (key: string, resume: string, remote: boolean) => void;
  onVerifyRemote: (key: string) => void;
  onSetRemoteOff: (key: string) => void;
  onOpenAgentWeb: (agent: string, dir: string) => void;

  viewMode?: "kanban" | "terminal";
  onSetViewMode?: (mode: "kanban" | "terminal") => void;

  // Mobile unified header props
  tabs?: TermTab[];
  activeKey?: string | null;
  availableSessionAgents?: EngineStatus[];
  hasSessionShell?: boolean;
  onSelectTab?: (key: string) => void;
  onCloseTab?: (key: string) => void;
  onOpenAgentInSession?: (agentId: string) => void;
  onOpenShellInSession?: () => void;
  onTriggerHandoff?: (action: "read" | "write") => void;
  onViewHandoff?: () => void;
}

export const AppHeader: React.FC<Props> = ({
  isMobile,
  activeSessionInfo,
  activeTab,
  activeSessionId,
  workspace,
  copied,
  ctx,
  onBackToTasks,
  onCopySessionId,
  onTriggerInput,
  onSetRemoteOn,
  onRestartRemote,
  onVerifyRemote,
  onSetRemoteOff,
  onOpenAgentWeb,
  viewMode = "kanban",
  onSetViewMode,
  tabs = [],
  activeKey,
  availableSessionAgents = [],
  hasSessionShell = true,
  onSelectTab,
  onCloseTab,
  onOpenAgentInSession,
  onOpenShellInSession,
  onTriggerHandoff,
  onViewHandoff,
}) => {
  const [tabMenuOpen, setTabMenuOpen] = useState(false);
  const [addMenuOpen, setAddMenuOpen] = useState(false);
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);

  return (
    <header className="flex h-11 shrink-0 items-center justify-between gap-1.5 border-b border-[#1d222b] bg-[#101319] px-2.5 pt-[env(safe-area-inset-top)] sm:h-12 sm:gap-4 sm:px-5">
      <div className="flex min-w-0 items-center gap-2">
        {isMobile && (
          <button
            onClick={onBackToTasks}
            className="-ml-1 flex h-7 w-7 shrink-0 cursor-pointer items-center justify-center rounded-lg bg-white/[0.06] hover:bg-white/10 active:bg-white/15 text-slate-200 transition-colors"
            title="Quay lại danh sách Tasks"
          >
            <ChevronLeft className="h-4.5 w-4.5 text-slate-300" />
          </button>
        )}
        <Layers className="hidden h-4 w-4 shrink-0 text-indigo-400 sm:block" />
        <div className="min-w-0 leading-tight">
          <div className="flex items-center gap-1.5">
            {activeTab?.agent ? (
              <span className={`h-2 w-2 rounded-full shrink-0 ${AGENT_META[activeTab.agent]?.dot || "bg-sky-400"}`} />
            ) : null}
            <span className="truncate text-[13px] font-semibold text-slate-100 max-w-[100px] min-[380px]:max-w-[140px] sm:max-w-none">
              {activeSessionInfo?.title || activeTab?.label || "Agent Bridge"}
            </span>

            {/* Mobile multi-tab switcher pill */}
            {isMobile && tabs.length > 1 && (
              <div className="relative">
                <button
                  onClick={() => setTabMenuOpen((v) => !v)}
                  className="flex shrink-0 items-center gap-1 rounded-full bg-white/[0.08] px-2 py-0.5 text-[10.5px] font-medium text-slate-300 hover:bg-white/15 cursor-pointer"
                >
                  <span className="max-w-[70px] truncate">{tabs.find((t) => t.key === activeKey)?.label || "Tabs"}</span>
                  <ChevronDown className="h-2.5 w-2.5 text-slate-400" />
                </button>
                {tabMenuOpen && (
                  <>
                    <div className="fixed inset-0 z-40" onClick={() => setTabMenuOpen(false)} />
                    <div className="fixed left-3 top-12 z-50 w-56 rounded-xl border border-[#2c3447] bg-[#161b26] p-1.5 shadow-2xl backdrop-blur">
                      <div className="px-2.5 py-1 text-[10px] font-semibold uppercase tracking-wider text-slate-400 border-b border-[#21293a] mb-1">
                        Tabs trong task này
                      </div>
                      {tabs.map((t) => (
                        <div
                          key={t.key}
                          onClick={() => {
                            setTabMenuOpen(false);
                            onSelectTab?.(t.key);
                          }}
                          className={`flex w-full cursor-pointer items-center justify-between gap-2 rounded-lg px-2.5 py-1.5 text-left text-xs ${
                            t.key === activeKey ? "bg-indigo-600/20 text-indigo-200 font-medium" : "text-slate-300 hover:bg-[#222a3a]"
                          }`}
                        >
                          <div className="flex items-center gap-2 truncate">
                            {t.agent ? (
                              <span className={`h-1.5 w-1.5 rounded-full ${AGENT_META[t.agent]?.dot}`} />
                            ) : (
                              <TerminalIcon className="h-3 w-3 text-sky-400" />
                            )}
                            <span className="truncate">{t.label}</span>
                          </div>
                          {tabs.length > 1 && (
                            <button
                              onClick={(e) => {
                                e.stopPropagation();
                                onCloseTab?.(t.key);
                              }}
                              className="p-0.5 text-slate-500 hover:text-rose-400"
                              title="Đóng tab"
                            >
                              <X className="h-3 w-3" />
                            </button>
                          )}
                        </div>
                      ))}
                    </div>
                  </>
                )}
              </div>
            )}

            {!isMobile && activeSessionId && (
              <button
                onClick={() => onCopySessionId(activeSessionId)}
                className="flex shrink-0 cursor-pointer items-center gap-1 rounded bg-[#1c222e] px-1.5 py-0.5 font-mono text-[10px] text-slate-400 hover:bg-[#252d3d] hover:text-slate-200"
                title={`Copy Session ID: ${activeSessionId}`}
              >
                {copied ? <Check className="h-2.5 w-2.5 text-emerald-400" /> : <Copy className="h-2.5 w-2.5" />}
                <span>{activeSessionId.slice(0, 8)}</span>
              </button>
            )}
          </div>
          <div
            className="hidden truncate font-mono text-[10.5px] text-slate-500 sm:block"
            title={activeTab?.workDir || workspace}
          >
            {activeTab?.workDir || workspace}
          </div>
        </div>
      </div>

      {/* Center: View Switcher (Kanban Board vs Console/Terminal) */}
      {onSetViewMode && (
        <div className="flex items-center rounded-lg bg-black/40 p-0.5 border border-slate-800">
          <button
            onClick={() => onSetViewMode("kanban")}
            className={`flex items-center gap-1.5 rounded-md px-2.5 py-1 text-xs font-medium transition cursor-pointer ${
              viewMode === "kanban"
                ? "bg-indigo-600 text-white shadow-sm"
                : "text-slate-400 hover:text-slate-200"
            }`}
          >
            <Layers className="h-3.5 w-3.5" />
            <span className="hidden sm:inline">Kanban Board</span>
          </button>
          <button
            onClick={() => onSetViewMode("terminal")}
            className={`flex items-center gap-1.5 rounded-md px-2.5 py-1 text-xs font-medium transition cursor-pointer ${
              viewMode === "terminal"
                ? "bg-indigo-600 text-white shadow-sm"
                : "text-slate-400 hover:text-slate-200"
            }`}
          >
            <TerminalIcon className="h-3.5 w-3.5" />
            <span className="hidden sm:inline">Console</span>
          </button>
        </div>
      )}

      <div className="flex shrink-0 items-center gap-1 sm:gap-2">
        {/* Remote Control */}
        {activeTab?.agent && (
          <>
            {!isMobile && (
              <UsageMeter
                agent={activeTab.agent}
                nativeId={(ctx?.agent === activeTab.agent ? ctx?.id : "") || activeTab.launch?.resume || ""}
                workspace={activeTab.workDir || workspace}
              />
            )}
            <RemoteControl
              key={activeTab.key}
              agent={activeTab.agent}
              workDir={activeTab.workDir}
              on={!!(activeTab.launch?.remote || activeTab.remoteOn)}
              sendInput={onTriggerInput}
              onEnabled={() => (activeTab.agent === "claude" ? onVerifyRemote(activeTab.key) : onSetRemoteOn(activeTab.key))}
              nativeId={(ctx?.agent === activeTab.agent ? ctx?.id : "") || activeTab.launch?.resume || ""}
              onDisabled={() => onSetRemoteOff(activeTab.key)}
              onRestart={(remote) => onRestartRemote(activeTab.key, (ctx?.agent === activeTab.agent ? ctx?.id : "") || activeTab.launch?.resume || "", remote)}
              onOpenIde={() => onOpenAgentWeb(activeTab.agent!, activeTab.workDir)}
            />
          </>
        )}

        {/* Mobile: Quick Add Agent/Shell button */}
        {isMobile && (availableSessionAgents.length > 0 || !hasSessionShell) && (
          <div className="relative">
            <button
              onClick={() => setAddMenuOpen((v) => !v)}
              className="flex h-7 w-7 cursor-pointer items-center justify-center rounded-lg bg-white/[0.06] hover:bg-white/10 active:bg-white/15 text-slate-300"
              title="Thêm agent hoặc shell"
            >
              <Plus className="h-3.5 w-3.5" />
            </button>
            {addMenuOpen && (
              <>
                <div className="fixed inset-0 z-40" onClick={() => setAddMenuOpen(false)} />
                <div className="fixed right-3 top-12 z-50 w-52 rounded-xl border border-[#2c3447] bg-[#161b26] p-1.5 shadow-2xl backdrop-blur">
                  <div className="px-2.5 py-1 text-[10px] font-semibold uppercase tracking-wider text-slate-400 border-b border-[#21293a] mb-1">
                    Mở thêm vào task này
                  </div>
                  {availableSessionAgents.map((e) => (
                    <button
                      key={e.id}
                      onClick={() => {
                        setAddMenuOpen(false);
                        onOpenAgentInSession?.(e.id);
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
                        setAddMenuOpen(false);
                        onOpenShellInSession?.();
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

        {/* Mobile: Overflow options menu (⋮) */}
        {isMobile && (
          <div className="relative">
            <button
              onClick={() => setMobileMenuOpen((v) => !v)}
              className="flex h-7 w-7 cursor-pointer items-center justify-center rounded-lg bg-white/[0.06] hover:bg-white/10 active:bg-white/15 text-slate-300"
              title="Tùy chọn khác"
            >
              <MoreVertical className="h-3.5 w-3.5" />
            </button>
            {mobileMenuOpen && (
              <>
                <div className="fixed inset-0 z-40" onClick={() => setMobileMenuOpen(false)} />
                <div className="fixed right-3 top-12 z-50 w-52 rounded-xl border border-[#2c3447] bg-[#161b26] p-1.5 shadow-2xl backdrop-blur">
                  {activeSessionId && (
                    <button
                      onClick={() => {
                        setMobileMenuOpen(false);
                        onCopySessionId(activeSessionId);
                      }}
                      className="flex w-full cursor-pointer items-center gap-2 rounded-lg px-2.5 py-1.5 text-left text-xs text-slate-200 hover:bg-[#222a3a] border-b border-[#21293a] mb-1 pb-2"
                    >
                      <Copy className="h-3.5 w-3.5 text-slate-400" />
                      <span>Copy Session ID ({activeSessionId.slice(0, 8)})</span>
                    </button>
                  )}
                  <button
                    disabled={!activeTab}
                    onClick={() => {
                      setMobileMenuOpen(false);
                      onTriggerHandoff?.("read");
                    }}
                    className="flex w-full cursor-pointer items-center gap-2 rounded-lg px-2.5 py-1.5 text-left text-xs text-slate-200 hover:bg-[#222a3a] disabled:opacity-40"
                  >
                    <BookOpen className="h-3.5 w-3.5 text-indigo-400" />
                    <span>Đọc handoff.md</span>
                  </button>
                  <button
                    disabled={!activeTab}
                    onClick={() => {
                      setMobileMenuOpen(false);
                      onTriggerHandoff?.("write");
                    }}
                    className="flex w-full cursor-pointer items-center gap-2 rounded-lg px-2.5 py-1.5 text-left text-xs text-slate-200 hover:bg-[#222a3a] disabled:opacity-40"
                  >
                    <FileEdit className="h-3.5 w-3.5 text-emerald-400" />
                    <span>Ghi handoff.md</span>
                  </button>
                  <button
                    onClick={() => {
                      setMobileMenuOpen(false);
                      onViewHandoff?.();
                    }}
                    className="flex w-full cursor-pointer items-center gap-2 rounded-lg px-2.5 py-1.5 text-left text-xs text-slate-200 hover:bg-[#222a3a]"
                  >
                    <Eye className="h-3.5 w-3.5 text-slate-400" />
                    <span>Xem handoff.md</span>
                  </button>
                  <button
                    disabled={!activeTab}
                    onClick={() => {
                      setMobileMenuOpen(false);
                      onTriggerInput("/clear\n", "Sent /clear");
                    }}
                    className="flex w-full cursor-pointer items-center gap-2 rounded-lg px-2.5 py-1.5 text-left text-xs text-slate-200 hover:bg-[#222a3a] disabled:opacity-40 border-t border-[#21293a] mt-1 pt-2"
                  >
                    <Eraser className="h-3.5 w-3.5 text-slate-400" />
                    <span>Xóa màn hình (/clear)</span>
                  </button>
                </div>
              </>
            )}
          </div>
        )}
      </div>
    </header>
  );
};

