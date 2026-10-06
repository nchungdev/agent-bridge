import React from "react";
import { Layers, Terminal as TerminalIcon } from "lucide-react";
import type { EngineStatus } from "../../types/bridge";
import { AGENT_META } from "../../constants/agentMeta";

interface Props {
  activeSessionTitle?: string;
  activeSessionId: string | null;
  installedEngines: EngineStatus[];
  onOpenAgent: (id: string) => void;
  onOpenShell: () => void;
  onCreateSessionWithAgent: (id: string) => void;
}

export const EmptyTaskState: React.FC<Props> = ({
  activeSessionTitle,
  activeSessionId,
  installedEngines,
  onOpenAgent,
  onOpenShell,
  onCreateSessionWithAgent,
}) => {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-5 p-8 text-center select-none">
      <Layers className="h-8 w-8 text-indigo-400/80" />
      <div>
        <div className="text-[14px] font-medium text-slate-200">
          {activeSessionTitle || "Chưa có tab nào mở trong task này"}
        </div>
        <div className="mt-1 text-[12px] text-slate-500">
          {activeSessionId
            ? `Task ID: ${activeSessionId} · Chọn một agent bên dưới để bắt đầu.`
            : "Chọn một task từ thanh bên hoặc tạo mới."}
        </div>
      </div>
      <div className="flex flex-wrap justify-center gap-2">
        {installedEngines.map((e) => (
          <button
            key={e.id}
            onClick={() => (activeSessionId ? onOpenAgent(e.id) : onCreateSessionWithAgent(e.id))}
            className="flex cursor-pointer items-center gap-2 rounded-lg border border-[#2c3447] bg-[#1b202c] px-3.5 py-2 text-[12px] text-slate-200 hover:bg-[#232a3a] transition-colors"
          >
            <span className={`h-1.5 w-1.5 rounded-full ${AGENT_META[e.id]?.dot}`} />
            {e.name}
          </button>
        ))}
        <button
          onClick={() => (activeSessionId ? onOpenShell() : onCreateSessionWithAgent("agy"))}
          className="flex cursor-pointer items-center gap-2 rounded-lg border border-[#2c3447] bg-[#1b202c] px-3.5 py-2 text-[12px] text-slate-200 hover:bg-[#232a3a] transition-colors"
        >
          <TerminalIcon className="h-3.5 w-3.5 text-sky-400" />
          Shell
        </button>
      </div>
    </div>
  );
};
