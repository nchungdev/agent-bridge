import React, { useEffect, useRef } from "react";
import { Plus, X } from "lucide-react";
import type { EngineStatus } from "../../types/bridge";
import { AGENT_META } from "../../constants/agentMeta";

interface Props {
  availableAgents: EngineStatus[];
  onAssign: (agentId: string) => void;
  onClose: () => void;
}

export const AssignAgentDropdown: React.FC<Props> = ({ availableAgents, onAssign, onClose }) => {
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handleDown = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        onClose();
      }
    };
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };

    window.addEventListener("mousedown", handleDown);
    window.addEventListener("keydown", handleKey);
    return () => {
      window.removeEventListener("mousedown", handleDown);
      window.removeEventListener("keydown", handleKey);
    };
  }, [onClose]);

  return (
    <div
      ref={ref}
      onClick={(e) => e.stopPropagation()}
      className="absolute left-0 top-full mt-1.5 z-40 w-60 rounded-xl border border-[#2b3548] bg-[#141824] p-1.5 shadow-2xl backdrop-blur animate-in fade-in zoom-in-95 duration-100"
    >
      <div className="flex items-center justify-between border-b border-[#1f2638] px-2 py-1.5 mb-1">
        <span className="text-[10px] font-semibold uppercase tracking-wider text-slate-400">
          Gán Agent cho Task
        </span>
        <button
          onClick={onClose}
          className="cursor-pointer p-0.5 text-slate-500 hover:text-slate-300 rounded"
        >
          <X className="h-3 w-3" />
        </button>
      </div>

      <div className="max-h-48 overflow-y-auto space-y-0.5">
        {availableAgents.length === 0 ? (
          <div className="py-3 px-2 text-center text-[11px] text-slate-500">
            Tất cả agent trên máy đã được gán vào task này.
          </div>
        ) : (
          availableAgents.map((eng) => {
            const meta = AGENT_META[eng.id] || { dot: "bg-purple-400", badge: "border-purple-500/30 text-purple-300" };
            return (
              <button
                key={eng.id}
                onClick={() => onAssign(eng.id)}
                className="group flex w-full cursor-pointer items-center justify-between rounded-lg px-2.5 py-1.5 text-left text-xs transition-colors hover:bg-[#202738]"
              >
                <div className="flex items-center gap-2 min-w-0">
                  <span className={`h-2 w-2 rounded-full shrink-0 ${meta.dot}`} />
                  <div className="min-w-0">
                    <div className="truncate font-medium text-slate-200 group-hover:text-white">
                      {eng.name}
                    </div>
                    <div className="truncate text-[10px] text-slate-500 font-mono">
                      {eng.binary}
                    </div>
                  </div>
                </div>

                <div className="shrink-0 flex items-center gap-1 rounded bg-[#10141d] px-1.5 py-0.5 text-[10px] text-indigo-300 font-medium group-hover:bg-indigo-600 group-hover:text-white transition-colors">
                  <Plus className="h-2.5 w-2.5" />
                  <span>Gán</span>
                </div>
              </button>
            );
          })
        )}
      </div>
    </div>
  );
};
