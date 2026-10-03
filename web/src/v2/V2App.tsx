import { useEffect, useMemo, useRef, useState } from "react";
import { fold, type ConvSummary, type EngineInfo, type Item } from "./types";
import { useHub } from "./useHub";

const MODE_LABEL: Record<string, string> = { ask: "Ask before acting", plan: "Plan (read-only)", "accept-edits": "Auto-accept edits", bypass: "Full access (this session)" };

function Badge({ children, tone = "slate" }: { children: React.ReactNode; tone?: "slate" | "red" | "amber" | "green" | "blue" }) {
  const tones = { slate: "bg-slate-700/60 text-slate-300", red: "bg-red-500/20 text-red-300", amber: "bg-amber-500/20 text-amber-300", green: "bg-emerald-500/20 text-emerald-300", blue: "bg-sky-500/20 text-sky-300" };
  return <span className={`rounded px-1.5 py-0.5 text-[11px] font-medium ${tones[tone]}`}>{children}</span>;
}

function ApprovalCard({ it, onDecide }: { it: Extract<Item, { kind: "approval" }>; onDecide: (id: string, allow: boolean, scope?: string) => void }) {
  const cmd = (it.args?.command ?? it.args?.file_path ?? it.args?.path ?? it.args?.url) as string | undefined;
  const riskTone = it.risk === "high" ? "red" : it.risk === "medium" ? "amber" : "slate";
  return (
    <div className="rounded-lg border border-amber-500/40 bg-amber-500/5 p-3 text-sm">
      <div className="mb-2 flex items-center gap-2">
        <Badge tone="amber">Permission</Badge>
        <span className="font-medium text-slate-100">{it.tool}</span>
        {it.risk && <Badge tone={riskTone}>{it.risk} risk</Badge>}
      </div>
      {cmd && <pre className="mb-2 overflow-x-auto rounded bg-black/40 p-2 text-xs text-slate-200">{cmd}</pre>}
      {!cmd && it.args && <pre className="mb-2 max-h-40 overflow-auto rounded bg-black/40 p-2 text-xs text-slate-300">{JSON.stringify(it.args, null, 2)}</pre>}
      {it.resolved ? (
        <Badge tone={it.resolved.allow ? "green" : "red"}>{it.resolved.allow ? "Allowed" : "Denied"} by {it.resolved.by}</Badge>
      ) : (
        <div className="flex gap-2">
          <button onClick={() => onDecide(it.id, true)} className="rounded bg-emerald-600 px-3 py-1 text-xs font-medium text-white hover:bg-emerald-500">Allow once</button>
          <button onClick={() => onDecide(it.id, true, "session")} className="rounded bg-emerald-900/70 px-3 py-1 text-xs font-medium text-emerald-100 hover:bg-emerald-800">Allow {it.tool} for this session</button>
          <button onClick={() => onDecide(it.id, false)} className="rounded bg-slate-700 px-3 py-1 text-xs font-medium text-slate-100 hover:bg-slate-600">Deny</button>
        </div>
      )}
    </div>
  );
}

function ItemView({ it, onDecide }: { it: Item; onDecide: (id: string, allow: boolean, scope?: string) => void }) {
  switch (it.kind) {
    case "user":
      return <div className="ml-auto max-w-[80%] whitespace-pre-wrap rounded-lg bg-sky-600/20 px-3 py-2 text-sm text-slate-100">{it.text}</div>;
    case "assistant":
      return (
        <div className="max-w-[90%] text-sm text-slate-200">
          {it.engine && <div className="mb-1"><Badge tone="blue">{it.engine}</Badge></div>}
          <div className="whitespace-pre-wrap">{it.text}</div>
        </div>
      );
    case "tool":
      return (
        <details className="rounded border border-slate-700 bg-black/20 px-3 py-1.5 text-xs text-slate-400">
          <summary className="cursor-pointer select-none">
            <span className="font-medium text-slate-300">{it.name}</span>{" "}
            <span className="text-slate-500">{String(it.args?.command ?? it.args?.file_path ?? it.args?.pattern ?? "").slice(0, 100)}</span>
          </summary>
          {it.output && <pre className="mt-2 max-h-60 overflow-auto whitespace-pre-wrap text-slate-400">{it.output}</pre>}
        </details>
      );
    case "approval":
      return <ApprovalCard it={it} onDecide={onDecide} />;
    case "switch":
      return <div className="text-center text-xs text-slate-500">— engine switched {it.from && `${it.from} → `}{it.to} —</div>;
    case "error":
      return <div className="rounded border border-red-500/40 bg-red-500/10 px-3 py-2 text-xs text-red-300"><Badge tone="red">{it.errKind}</Badge> {it.message}</div>;
  }
}

export default function V2App() {
  const [convs, setConvs] = useState<ConvSummary[]>([]);
  const [conv, setConv] = useState<string | null>(() => localStorage.getItem("v2_conv"));
  const [engines, setEngines] = useState<EngineInfo[]>([]);
  const [engine, setEngine] = useState("claude");
  const [mode, setMode] = useState("ask");
  const [text, setText] = useState("");
  const bottom = useRef<HTMLDivElement>(null);

  const refreshConvs = () => fetch("/api/sessions").then((r) => r.json()).then((l: ConvSummary[]) => setConvs(l ?? [])).catch(() => {});
  useEffect(() => { refreshConvs(); fetch("/api/v2/engines").then((r) => r.json()).then(setEngines).catch(() => {}); }, []);

  const { events, snapshot, connected, error, clearError, send } = useHub(conv, (id) => { setConv(id); localStorage.setItem("v2_conv", id); refreshConvs(); });
  useEffect(() => { if (snapshot?.conv?.ActiveEngine) setEngine(snapshot.conv.ActiveEngine); if (snapshot?.conv?.Mode) setMode(snapshot.conv.Mode); }, [snapshot?.conv?.ActiveEngine, snapshot?.conv?.Mode]);

  const items = useMemo(() => fold(events), [events]);
  useEffect(() => { bottom.current?.scrollIntoView({ block: "end" }); }, [items.length, events.length]);

  const state = snapshot?.live?.[engine] ?? "";
  const busy = state === "running" || state === "awaiting_approval" || state === "starting";
  const eng = engines.find((e) => e.id === engine);
  const unusable = (e?: EngineInfo) => !!e?.auth && e.auth.known && (!e.auth.installed || !e.auth.logged_in);
  const modes = eng?.capabilities.permission_modes ?? ["ask", "plan", "accept-edits", "bypass"];

  const submit = () => {
    const t = text.trim();
    if (!t || unusable(eng)) return;
    if (mode === "bypass" && !confirm("Full access: the agent can run any command without asking. Continue?")) return;
    send({ type: "send", conv: conv ?? "", engine, text: t, mode });
    setText("");
  };
  const changeMode = (m: string) => {
    if (m === "bypass" && !confirm("Full access lets the agent act without asking, for this conversation. Enable?")) return;
    setMode(m);
    if (conv) send({ type: "set_mode", conv, mode: m });
  };
  const changeEngine = (id: string) => {
    setEngine(id);
    // keep the permission mode valid for the new engine (fall back to its most restrictive mode)
    const allowed = engines.find((e) => e.id === id)?.capabilities.permission_modes;
    if (allowed && allowed.length && !allowed.includes(mode)) {
      const next = allowed.includes("plan") ? "plan" : allowed[0];
      setMode(next);
      if (conv) send({ type: "set_mode", conv, mode: next });
    }
    if (conv && snapshot?.conv?.ActiveEngine && snapshot.conv.ActiveEngine !== id) send({ type: "switch_engine", conv, engine: id });
  };
  const newChat = () => { setConv(null); localStorage.removeItem("v2_conv"); };
  const pick = (id: string) => { setConv(id); localStorage.setItem("v2_conv", id); };

  return (
    <div className="flex h-full bg-[#11141a] text-slate-300">
      <aside className="flex w-64 shrink-0 flex-col border-r border-slate-800">
        <div className="flex items-center justify-between p-3">
          <span className="text-sm font-semibold text-slate-100">Conversations</span>
          <button onClick={newChat} className="rounded bg-slate-700 px-2 py-1 text-xs hover:bg-slate-600">New</button>
        </div>
        <div className="flex-1 overflow-y-auto">
          {convs.map((c) => (
            <button key={c.id} onClick={() => pick(c.id)} className={`block w-full truncate px-3 py-2 text-left text-sm ${c.id === conv ? "bg-slate-800 text-white" : "hover:bg-slate-800/50"}`}>{c.name || c.id.slice(0, 8)}</button>
          ))}
        </div>
        <div className="border-t border-slate-800 p-3 text-xs">
          <Badge tone={connected ? "green" : "red"}>{connected ? "connected" : "reconnecting…"}</Badge>
          <a href="#/" className="ml-2 text-slate-500 hover:text-slate-300">classic UI</a>
        </div>
      </aside>
      <main className="flex min-w-0 flex-1 flex-col">
        <header className="flex flex-wrap items-center gap-3 border-b border-slate-800 px-4 py-2 text-xs">
          <label className="flex items-center gap-1">Engine
            <select value={engine} onChange={(e) => changeEngine(e.target.value)} disabled={busy} className="rounded bg-slate-800 px-2 py-1 text-slate-100">
              {engines.map((e) => <option key={e.id} value={e.id}>{e.id}{unusable(e) ? " — not logged in" : ""}</option>)}
            </select>
          </label>
          <label className="flex items-center gap-1">Permissions
            <select value={mode} onChange={(e) => changeMode(e.target.value)} className={`rounded px-2 py-1 text-slate-100 ${mode === "bypass" ? "bg-red-900/60" : "bg-slate-800"}`}>
              {modes.map((m) => <option key={m} value={m}>{MODE_LABEL[m] ?? m}</option>)}
            </select>
          </label>
          {state && <Badge tone={state === "awaiting_approval" ? "amber" : state === "running" ? "blue" : state === "failed" ? "red" : "slate"}>{state.replace("_", " ")}</Badge>}
          {busy && <button onClick={() => conv && send({ type: "cancel", conv })} className="rounded bg-red-600/80 px-2 py-1 text-white hover:bg-red-500">Stop</button>}
        </header>
        <div className="flex-1 space-y-3 overflow-y-auto p-4">
          {items.length === 0 && <div className="mt-20 text-center text-sm text-slate-500">Start a conversation. You can switch engine at any time — the hub carries the context over.</div>}
          {items.map((it) => <ItemView key={it.key} it={it} onDecide={(id, allow, scope) => conv && send({ type: "decide", conv, approval_id: id, allow, scope: scope ?? "once" })} />)}
          <div ref={bottom} />
        </div>
        {unusable(eng) && (
          <div className="mx-4 mt-3 flex items-center justify-between gap-3 rounded border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-xs text-amber-200">
            <span><b>{engine}</b> is not logged in{eng?.auth?.login_hint ? <> — <code className="rounded bg-black/40 px-1">{eng.auth.login_hint}</code></> : null}</span>
            <button onClick={() => fetch("/api/v2/engines?refresh=1").then((r) => r.json()).then(setEngines)} className="rounded bg-amber-600/70 px-2 py-1 text-white hover:bg-amber-500">Re-check</button>
          </div>
        )}
        {eng && !eng.capabilities.permission_prompts && (
          <div className="mx-4 mt-3 rounded border border-slate-700 bg-slate-800/40 px-3 py-2 text-xs text-slate-300">
            <b>{engine}</b> cannot ask for permission mid-task. Safety comes from the mode: <i>Plan</i> is read-only, <i>Auto-accept edits</i> lets it act within its own limits{eng.capabilities.mode_requires_restart ? "; changing mode restarts the engine process" : ""}.
          </div>
        )}
        {error && <div className="mx-4 mb-2 flex items-center justify-between rounded border border-red-500/40 bg-red-500/10 px-3 py-1.5 text-xs text-red-300">{error}<button onClick={clearError}>×</button></div>}
        <div className="border-t border-slate-800 p-3">
          <div className="flex items-end gap-2">
            <textarea value={text} onChange={(e) => setText(e.target.value)} rows={2} placeholder={busy ? "Queued until the current turn finishes…" : "Message the agent"}
              onKeyDown={(e) => { if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) { e.preventDefault(); submit(); } }}
              className="min-h-[44px] flex-1 resize-none rounded-lg border border-slate-700 bg-[#171b23] px-3 py-2 text-sm text-slate-100 outline-none focus:border-sky-500" />
            <button onClick={submit} disabled={unusable(eng)} className="rounded-lg bg-sky-600 px-4 py-2 text-sm font-medium text-white hover:bg-sky-500 disabled:cursor-not-allowed disabled:opacity-40">Send</button>
          </div>
        </div>
      </main>
    </div>
  );
}
