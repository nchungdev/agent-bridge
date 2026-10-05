import React, { useEffect, useRef, useState } from "react";
import { ProviderQuota, groupForModel, shortWindowLabel, useQuota } from "./ProviderQuota";

interface ContextResp { agent: string; model?: string; tokens: number; window: number; supported: boolean }

const fmtTokens = (n: number) => (n >= 1e6 ? `${(n / 1e6).toFixed(2)}M` : n >= 1e3 ? `${Math.round(n / 1e3)}k` : String(n));
const tone = (p: number, base: string) => (p >= 90 ? "bg-rose-500" : p >= 70 ? "bg-amber-400" : base);

/** Context window of the agent's session, as recorded by its own transcript; refreshed every 10s while visible. */
function useContextUsage(agent: string, nativeId: string, workspace: string) {
  const [c, setC] = useState<ContextResp | null>(null);
  useEffect(() => {
    setC(null);
    if (!nativeId) return;
    let alive = true;
    const load = () => {
      if (document.visibilityState !== "visible") return;
      const q = new URLSearchParams({ agent, id: nativeId, workspace });
      fetch(`/api/bridge/context-usage?${q}`)
        .then((r) => (r.ok ? r.json() : null))
        .then((d: ContextResp | null) => { if (alive && d) setC(d); })
        .catch(() => {});
    };
    load();
    const t = window.setInterval(load, 10000);
    return () => { alive = false; window.clearInterval(t); };
  }, [agent, nativeId, workspace]);
  return c;
}

/** One small vertical bar: `percent` null means "no data yet" (empty track). */
const MiniBar: React.FC<{ percent: number | null; base: string }> = ({ percent, base }) => (
  <div className="flex h-5 w-1.5 flex-col justify-end overflow-hidden rounded-full bg-[#232b3b]">
    {percent != null && <div className={`w-full rounded-full transition-all duration-300 ${tone(percent, base)}`} style={{ height: `${Math.max(6, Math.min(100, percent))}%` }} />}
  </div>
);

/**
 * Tab-bar widget for the active agent: three mini bars (context, 5h quota, weekly quota).
 * Click opens a popup with the full numbers.
 */
export const UsageMeter: React.FC<{ agent: string; nativeId: string; workspace: string }> = ({ agent, nativeId, workspace }) => {
  const ctx = useContextUsage(agent, nativeId, workspace);
  const { q } = useQuota(agent);
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => { if (rootRef.current && !rootRef.current.contains(e.target as Node)) setOpen(false); };
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") setOpen(false); };
    document.addEventListener("mousedown", onDown);
    document.addEventListener("keydown", onKey);
    return () => { document.removeEventListener("mousedown", onDown); document.removeEventListener("keydown", onKey); };
  }, [open]);
  useEffect(() => setOpen(false), [agent]);

  const ctxPct = ctx && ctx.supported && ctx.window > 0 ? Math.min(100, (ctx.tokens / ctx.window) * 100) : null;
  const windows = (groupForModel(q, ctx?.model ?? "")?.windows ?? []).filter((w) => !w.disabled && w.used_percent != null);
  const [w1, w2] = windows;

  const cell = (label: string, percent: number | null, base: string) => (
    <div className="flex items-center gap-1" key={label}>
      <MiniBar percent={percent} base={base} />
      <div className="flex flex-col leading-none">
        <span className="text-[9px] uppercase tracking-wide text-slate-500">{label}</span>
        <span className={`text-[10.5px] font-medium tabular-nums ${percent != null && percent >= 85 ? "text-rose-300" : "text-slate-300"}`}>{percent != null ? `${Math.round(percent)}%` : "–"}</span>
      </div>
    </div>
  );

  return (
    <div ref={rootRef} className="relative shrink-0">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className={`flex cursor-pointer items-center gap-3 rounded-md border px-2 py-0.5 ${open ? "border-indigo-500 bg-[#1d2330]" : "border-white/5 bg-[#161a24] hover:bg-[#1c2230]"}`}
        title="Context window & provider quota"
      >
        {cell("ctx", ctxPct, "bg-sky-400")}
        {cell(w1 ? shortWindowLabel(w1.label) : "5h", w1 ? (w1.used_percent as number) : null, "bg-emerald-500")}
        {cell(w2 ? shortWindowLabel(w2.label) : "wk", w2 ? (w2.used_percent as number) : null, "bg-violet-400")}
      </button>

      {open && (
        <div className="absolute right-0 top-full z-30 mt-1.5 w-[300px] max-w-[calc(100vw-1.5rem)] space-y-2.5 rounded-xl border border-[#272e3d] bg-[#161920] p-3 text-slate-200 shadow-2xl">
          <div className="rounded-lg border border-[#232a3b] bg-[#141824]/60 p-2.5">
            <div className="flex items-center justify-between text-[11px]">
              <span className="font-semibold text-slate-200">Context window</span>
              <span className="font-mono text-slate-300">{ctxPct != null ? `${ctxPct.toFixed(1)}% used` : "–"}</span>
            </div>
            <div className="mt-1.5 h-1.5 w-full overflow-hidden rounded-full bg-[#202532]">
              {ctxPct != null && <div className={`h-full transition-all duration-300 ${tone(ctxPct, "bg-sky-400")}`} style={{ width: `${ctxPct}%` }} />}
            </div>
            <div className="mt-1 text-[10.5px] text-slate-500">
              {ctx && ctx.supported
                ? `${fmtTokens(ctx.tokens)} of ${fmtTokens(ctx.window)} tokens${ctx.model ? ` · ${ctx.model}` : ""}`
                : agent === "agy"
                  ? "Antigravity does not report context size."
                  : "Appears after the first reply of this session."}
            </div>
          </div>
          <ProviderQuota engine={agent} />
        </div>
      )}
    </div>
  );
};
