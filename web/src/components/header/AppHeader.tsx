import React from "react";
import { Check, Copy, Layers, Menu } from "lucide-react";
import type { BridgeSession, TermTab } from "../../types/bridge";
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
  onToggleSidebar: () => void;
  onCopySessionId: (id: string) => void;
  onTriggerInput: (text: string, label?: string) => void;
  onSetRemoteOn: (key: string) => void;
  onOpenAgentWeb: (agent: string, dir: string) => void;
}

export const AppHeader: React.FC<Props> = ({
  isMobile,
  activeSessionInfo,
  activeTab,
  activeSessionId,
  workspace,
  copied,
  ctx,
  onToggleSidebar,
  onCopySessionId,
  onTriggerInput,
  onSetRemoteOn,
  onOpenAgentWeb,
}) => {
  return (
    <header className="flex h-12 shrink-0 items-center justify-between gap-2 border-b border-[#1d222b] bg-[#101319] px-3 pt-[env(safe-area-inset-top)] sm:gap-4 sm:px-5">
      <div className="flex min-w-0 items-center gap-2.5">
        {isMobile && (
          <button
            onClick={onToggleSidebar}
            className="-ml-1 shrink-0 cursor-pointer rounded-md p-1.5 text-slate-300 hover:bg-[#1a1e28]"
            title="Menu"
          >
            <Menu className="h-5 w-5" />
          </button>
        )}
        <Layers className="hidden h-4 w-4 shrink-0 text-indigo-400 sm:block" />
        <div className="min-w-0 leading-tight">
          <div className="flex items-center gap-2">
            <span className="truncate text-[13px] font-semibold text-slate-100">
              {activeSessionInfo?.title || activeTab?.label || "Agent Bridge"}
            </span>
            {activeSessionId && (
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

      <div className="flex shrink-0 items-center gap-2">
        {activeTab?.agent && (
          <>
            {!isMobile && (
              <UsageMeter
                agent={activeTab.agent}
                nativeId={(ctx?.agent === activeTab.agent ? ctx.id : "") || activeTab.launch?.resume || ""}
                workspace={activeTab.workDir || workspace}
              />
            )}
            <RemoteControl
              key={activeTab.key}
              agent={activeTab.agent}
              workDir={activeTab.workDir}
              on={!!(activeTab.launch?.remote || activeTab.remoteOn)}
              sendInput={onTriggerInput}
              onEnabled={() => onSetRemoteOn(activeTab.key)}
              onOpenIde={() => onOpenAgentWeb(activeTab.agent!, activeTab.workDir)}
            />
          </>
        )}
      </div>
    </header>
  );
};
