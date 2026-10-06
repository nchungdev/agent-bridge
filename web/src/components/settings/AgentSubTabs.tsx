import React from "react";
import type { EngineStatus } from "../../types/bridge";
import type { AccountGroup } from "../../v2/types";

interface Props {
  engines: EngineStatus[];
  selectedAgentId: string;
  groups: AccountGroup[] | null;
  agentMetaColors: Record<string, { badge: string; dot: string; border: string }>;
  onSelectAgent: (id: string) => void;
}

export const AgentSubTabs: React.FC<Props> = ({
  engines,
  selectedAgentId,
  groups,
  agentMetaColors,
  onSelectAgent,
}) => {
  return (
    <div className="flex items-center gap-2 border-b border-[#202737] pb-2 overflow-x-auto">
      {engines.map((eng) => {
        const isSelected = eng.id === selectedAgentId;
        const baseKey = eng.base || eng.id;
        const meta = agentMetaColors[baseKey] || {
          badge: "bg-purple-500/15 text-purple-300 border-purple-500/30",
          dot: "bg-purple-400",
          border: "border-purple-500/30",
        };
        const engGroup = groups?.find((g) => g.engine === eng.id);
        const accCount = engGroup?.accounts?.length || 0;

        return (
          <button
            key={eng.id}
            onClick={() => onSelectAgent(eng.id)}
            className={`flex shrink-0 cursor-pointer items-center gap-2 rounded-lg px-3.5 py-1.5 text-xs font-medium transition-all ${
              isSelected
                ? "bg-[#1c2230] text-slate-100 border border-[#2d374b] shadow-sm"
                : "text-slate-400 hover:bg-[#141822] hover:text-slate-200 border border-transparent"
            }`}
          >
            <span className={`h-2 w-2 rounded-full ${meta.dot}`} />
            <span className="font-semibold">{eng.name}</span>
            {eng.installed ? (
              <span className="rounded bg-emerald-500/15 px-1.5 py-0.5 text-[10px] text-emerald-400 font-normal">
                Đã cài
              </span>
            ) : (
              <span className="rounded bg-slate-800 px-1.5 py-0.5 text-[10px] text-slate-500 font-normal">
                Chưa cài
              </span>
            )}
            {accCount > 0 && (
              <span className="rounded bg-sky-950/50 px-1.5 py-0.5 text-[10px] text-sky-400 font-mono">
                {accCount}
              </span>
            )}
          </button>
        );
      })}
    </div>
  );
};
