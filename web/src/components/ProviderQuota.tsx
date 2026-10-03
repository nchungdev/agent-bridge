import React, { useCallback, useEffect, useState } from "react";
import { RefreshCw } from "lucide-react";

export interface QuotaWindow { label: string; used_percent?: number; resets_at?: string; disabled?: boolean }
export interface QuotaGroup { name: string; windows: QuotaWindow[] }
export interface QuotaResp { engine: string; source?: string; plan?: string; groups: QuotaGroup[]; fetched_at?: string; supported: boolean; error?: string }

function resetsIn(iso?: string): string {
  const t = iso ? Date.parse(iso) : NaN;
  if (Number.isNaN(t)) return "";
  const mins = Math.round((t - Date.now()) / 60000);
  if (mins <= 0) return "resetting now";
  if (mins < 60) return `resets in ${mins}m`;
  if (mins < 60 * 24) return `resets in ${Math.floor(mins / 60)}h ${mins % 60}m`;
  return `resets in ${Math.floor(mins / 1440)}d ${Math.floor((mins % 1440) / 60)}h`;
}

const barColor = (p: number) => (p >= 85 ? "bg-rose-500" : p >= 60 ? "bg-amber-400" : "bg-emerald-500");

/** Quota of one engine from its own CLI; refreshes every 2 minutes (while visible) and whenever `refreshKey` changes. */
export function useQuota(engine: string, refreshKey = 0) {
  const [q, setQ] = useState<QuotaResp | null>(null);
  const [loading, setLoading] = useState(false);

  const load = useCallback((refresh = false) => {
    setLoading(true);
    return fetch(`/api/v2/engines/${engine}/quota${refresh ? "?refresh=1" : ""}`)
      .then((r) => r.json())
      .then((d: QuotaResp) => setQ(d))
      .catch(() => setQ({ engine, groups: [], supported: true, error: "could not reach the hub" }))
      .finally(() => setLoading(false));
  }, [engine]);

  useEffect(() => { setQ(null); load(); }, [load]);
  useEffect(() => { if (refreshKey > 0) load(); }, [refreshKey, load]);
  useEffect(() => {
    const t = window.setInterval(() => { if (document.visibilityState === "visible") load(); }, 120000);
    return () => window.clearInterval(t);
  }, [load]);

  return { q, loading, load };
}

/** Short label for a quota window: "5h", "wk", ... */
export function shortWindowLabel(label: string): string {
  const l = label.toLowerCase();
  if (/(5|five)[ -]?hour/.test(l)) return "5h";
  if (/week/.test(l)) return "wk";
  if (/day/.test(l)) return "day";
  return label.slice(0, 6);
}

/** The quota group that applies to the selected model (Antigravity has separate Gemini / Claude+GPT pools). */
export function groupForModel(q: QuotaResp | null, modelId: string): QuotaGroup | null {
  if (!q || q.groups.length === 0) return null;
  if (q.groups.length === 1) return q.groups[0];
  const m = modelId.toLowerCase();
  const want = m.includes("gemini") ? /gemini/i : /claude|gpt|openai/i;
  return q.groups.find((g) => want.test(g.name)) ?? q.groups[0];
}

/** Real provider quota as reported by each engine's own CLI (nothing is estimated). */
export const ProviderQuota: React.FC<{ engine: string }> = ({ engine }) => {
  const { q, loading, load } = useQuota(engine);

  if (!q) {
    return <div className="rounded-lg border border-[#232a3b] bg-[#141824]/60 p-2.5 text-[11px] text-slate-500">{loading ? "Reading quota from the CLI…" : ""}</div>;
  }
  if (!q.supported) {
    return (
      <div className="rounded-lg border border-[#232a3b] bg-[#141824]/60 p-2.5">
        <div className="flex items-center gap-1.5 text-[11px] font-semibold text-slate-200"><span className="w-1.5 h-1.5 rounded-full bg-slate-400" />Quota unavailable</div>
        <p className="mt-1 text-[10.5px] leading-relaxed text-slate-400">This engine's CLI does not report provider quota, so nothing is shown rather than an estimate.</p>
      </div>
    );
  }
  return (
    <div className="space-y-2.5">
      {q.error && q.groups.length === 0 && (
        <div className="rounded-lg border border-[#3a2a2a] bg-[#1c1416]/60 p-2.5 text-[10.5px] leading-relaxed text-rose-300 break-words">Could not read quota: {q.error}</div>
      )}
      {q.groups.map((g) => (
        <div key={g.name} className="rounded-lg border border-[#232a3b] bg-[#141824]/60 p-2.5 space-y-2">
          <div className="text-[11px] font-semibold text-slate-200">{g.name}{q.plan && g === q.groups[0] ? <span className="ml-1.5 text-[10px] font-normal text-slate-500">{q.plan}</span> : null}</div>
          {g.windows.map((w) => {
            const used = w.used_percent;
            return (
              <div key={w.label} className="space-y-1">
                <div className="flex items-center justify-between text-[11px]">
                  <span className="text-slate-400">{w.label}</span>
                  {w.disabled ? (
                    <span className="text-slate-500">not limited</span>
                  ) : used == null ? (
                    <span className="text-slate-500">unknown</span>
                  ) : (
                    <span className={`font-mono ${used >= 85 ? "text-rose-300" : "text-slate-300"}`}>{Math.round(used)}% used</span>
                  )}
                </div>
                {!w.disabled && used != null && (
                  <div className="w-full h-1.5 rounded-full bg-[#202532] overflow-hidden">
                    <div className={`h-full ${barColor(used)} transition-all duration-300`} style={{ width: `${Math.min(100, Math.max(0, used))}%` }} />
                  </div>
                )}
                {!w.disabled && w.resets_at && (
                  <div className="text-[10px] text-slate-500" title={new Date(w.resets_at).toLocaleString()}>{resetsIn(w.resets_at)}</div>
                )}
              </div>
            );
          })}
        </div>
      ))}
      <div className="flex items-center justify-between text-[10px] text-slate-600">
        <span className="truncate">{q.source ? `from ${q.source}` : ""}</span>
        <button type="button" onClick={() => load(true)} className="flex items-center gap-1 text-slate-500 hover:text-slate-300 cursor-pointer" title="Read the quota from the CLI again">
          <RefreshCw className={`w-3 h-3 ${loading ? "animate-spin" : ""}`} />
          Refresh
        </button>
      </div>
    </div>
  );
};
