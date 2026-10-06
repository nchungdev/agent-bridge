import React from "react";
import { AlertCircle, CheckCircle2, Play, RefreshCw, Terminal } from "lucide-react";
import type { EngineStatus } from "../../types/bridge";

interface Props {
  engine: EngineStatus;
  meta: { badge: string; dot: string; border: string };
  installing: boolean;
  onOpenTerminal: (id: string) => void;
  onInstall: (eng: EngineStatus) => void;
}

export const AgentStatusCard: React.FC<Props> = ({
  engine,
  meta,
  installing,
  onOpenTerminal,
  onInstall,
}) => {
  return (
    <div className="rounded-xl border border-[#202738] bg-[#131722] p-4 flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
      <div className="flex items-center gap-3 min-w-0">
        <span className={`h-3 w-3 rounded-full shrink-0 ${meta.dot}`} />
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <span className="text-sm font-semibold text-slate-100 truncate">{engine.name}</span>
            <span className={`rounded border px-1.5 py-0.2 text-[10px] font-mono ${meta.badge}`}>
              {engine.id}
            </span>
            {engine.installed ? (
              <span className="flex items-center gap-1 rounded bg-emerald-500/15 px-2 py-0.5 text-[10.5px] font-medium text-emerald-400">
                <CheckCircle2 className="h-3 w-3" /> Đã cài đặt
              </span>
            ) : (
              <span className="flex items-center gap-1 rounded bg-amber-500/15 px-2 py-0.5 text-[10.5px] font-medium text-amber-400">
                <AlertCircle className="h-3 w-3" /> Chưa cài đặt
              </span>
            )}
          </div>
          <div
            className="text-[11px] text-slate-500 font-mono truncate mt-0.5"
            title={engine.binary}
          >
            {engine.binary} ·{" "}
            {engine.installed
              ? engine.auth_status || "Sẵn sàng hoạt động trong PATH"
              : "Chưa tìm thấy lệnh CLI này trong PATH"}
          </div>
        </div>
      </div>

      <div className="shrink-0 flex items-center gap-2">
        {engine.installed ? (
          <button
            onClick={() => onOpenTerminal(engine.id)}
            className="flex cursor-pointer items-center gap-1.5 rounded-lg border border-[#2e374d] bg-[#191f2c] px-3 py-1.5 text-xs font-medium text-slate-200 hover:bg-[#232c3f] hover:text-white transition-colors shadow-sm"
            title={`Mở terminal chạy trực tiếp CLI ${engine.id}`}
          >
            <Terminal className="h-3.5 w-3.5 text-sky-400" />
            <span>Mở CLI</span>
          </button>
        ) : (
          engine.installCmd && (
            <button
              onClick={() => onInstall(engine)}
              disabled={installing}
              className="flex cursor-pointer items-center gap-1.5 rounded-lg bg-emerald-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-emerald-500 disabled:opacity-50 transition-colors shadow-sm"
            >
              {installing ? (
                <RefreshCw className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <Play className="h-3.5 w-3.5" />
              )}
              <span>Cài đặt ngay</span>
            </button>
          )
        )}
      </div>
    </div>
  );
};
