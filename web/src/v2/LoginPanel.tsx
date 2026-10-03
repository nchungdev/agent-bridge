import { useEffect, useRef, useState } from "react";
import { Check, Copy } from "lucide-react";
import type { LoginState } from "./types";

/** Drives an engine's CLI login from the GUI: shows the sign-in link / device code, accepts a pasted code. */
export function LoginPanel({ engine, onDone, onClose, replace = false }: { engine: string; onDone: () => void; onClose: () => void; replace?: boolean }) {
  const [st, setSt] = useState<LoginState | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [code, setCode] = useState("");
  const [busy, setBusy] = useState(false);
  const [copied, setCopied] = useState(false);

  const copyCode = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      // clipboard API needs a secure context; fall back to a temporary selection
      const ta = document.createElement("textarea");
      ta.value = text;
      ta.style.position = "fixed";
      ta.style.opacity = "0";
      document.body.appendChild(ta);
      ta.select();
      try { document.execCommand("copy"); } catch { /* nothing more to try */ }
      document.body.removeChild(ta);
    }
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1800);
  };
  const doneRef = useRef(false);
  const base = `/api/v2/engines/${engine}/login`;

  // The callbacks are kept in refs: the effect below must run once per panel. Re-running it on every
  // parent render would cancel the sign-in that is in progress (and start another).
  const onDoneRef = useRef(onDone);
  onDoneRef.current = onDone;

  useEffect(() => {
    let alive = true;
    let flow = "";
    let cancelled = false;
    const cancel = () => fetch(`${base}?flow=${encodeURIComponent(flow)}`, { method: "DELETE" }).catch(() => {});
    fetch(replace ? `${base}?replace=1` : base, { method: "POST" })
      .then((r) => (r.ok ? r.json() : r.json().then((e) => Promise.reject(new Error(e.error ?? r.statusText)))))
      .then((s: LoginState) => {
        flow = s.flow ?? "";
        if (cancelled) cancel(); // the panel closed before the attempt id was known
        else if (alive) setSt(s);
      })
      .catch((e: Error) => alive && setErr(e.message));
    const t = window.setInterval(() => {
      fetch(base).then((r) => r.json()).then((s: LoginState) => {
        if (!alive || (flow && s.flow && s.flow !== flow)) return; // ignore a different attempt
        setSt(s);
        if (s.finished && s.success && !doneRef.current) { doneRef.current = true; onDoneRef.current(); }
      }).catch(() => {});
    }, 1500);
    return () => {
      alive = false;
      cancelled = true;
      window.clearInterval(t);
      if (flow) cancel();
    };
  }, [base]);

  const submit = () => {
    if (!code.trim()) return;
    setBusy(true);
    fetch(`${base}/input`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ text: code }) })
      .then((r) => r.json()).then((s: LoginState) => { setSt(s); setCode(""); })
      .catch((e: Error) => setErr(e.message))
      .finally(() => setBusy(false));
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4" role="dialog" aria-modal="true">
      <div className="w-full max-w-lg rounded-xl border border-slate-700 bg-[#171b23] p-5 text-sm text-slate-200 shadow-xl">
        <div className="mb-3 flex items-center justify-between">
          <h2 className="text-base font-semibold text-slate-100">Sign in to {engine}</h2>
          <button onClick={onClose} className="text-slate-400 hover:text-white" aria-label="Close">✕</button>
        </div>
        {err && <div className="mb-3 rounded border border-red-500/40 bg-red-500/10 px-3 py-2 text-xs text-red-300">{err}</div>}
        {!st && !err && <div className="text-slate-400">Starting login…</div>}
        {st?.finished && st.success && <div className="rounded border border-emerald-500/40 bg-emerald-500/10 px-3 py-2 text-emerald-300">Signed in. You can close this window.</div>}
        {st?.finished && !st.success && <div className="mb-3 rounded border border-red-500/40 bg-red-500/10 px-3 py-2 text-xs text-red-300">{st.error ?? "Login failed"} <button className="ml-2 underline" onClick={() => location.reload()}>Try again</button></div>}
        {st && !st.finished && (
          <ol className="list-decimal space-y-3 pl-5">
            <li>
              Open the sign-in page and approve access:
              <div className="mt-2 flex flex-wrap gap-2">
                {st.urls.length === 0 && <span className="text-slate-500">waiting for the link…</span>}
                {st.urls.map((u) => (
                  <a key={u} href={u} target="_blank" rel="noopener noreferrer" className="rounded bg-sky-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-sky-500">Open sign-in page ↗</a>
                ))}
              </div>
            </li>
            {st.code && (
              <li>
                Enter this one-time code when asked:
                <div className="mt-2 flex items-center gap-2 rounded bg-black/40 px-3 py-2">
                  <span className="flex-1 select-all text-center font-mono text-xl tracking-widest text-amber-200">{st.code}</span>
                  <button
                    type="button"
                    onClick={() => copyCode(st.code as string)}
                    className="flex shrink-0 items-center gap-1 rounded-md border border-slate-600 bg-slate-800 px-2 py-1 text-xs text-slate-200 hover:bg-slate-700 cursor-pointer"
                    title="Copy the code"
                    aria-label="Copy code"
                  >
                    {copied ? <Check className="h-3.5 w-3.5 text-emerald-400" /> : <Copy className="h-3.5 w-3.5" />}
                    {copied ? "Copied" : "Copy"}
                  </button>
                </div>
              </li>
            )}
            {st.needs_code && (
              <li>
                Paste the code shown after you sign in:
                <div className="mt-2 flex gap-2">
                  <input value={code} onChange={(e) => setCode(e.target.value)} onKeyDown={(e) => e.key === "Enter" && submit()} autoFocus spellCheck={false}
                    className="flex-1 rounded border border-slate-700 bg-black/40 px-2 py-1.5 font-mono text-xs text-slate-100 outline-none focus:border-sky-500" placeholder="paste code here" />
                  <button onClick={submit} disabled={busy} className="rounded bg-emerald-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-emerald-500 disabled:opacity-50">Submit</button>
                </div>
              </li>
            )}
            {!st.needs_code && !st.code && st.urls.length > 0 && <li className="list-none text-xs text-slate-500">This window updates automatically once you finish signing in.</li>}
          </ol>
        )}
        {st && <details className="mt-4 text-xs text-slate-500"><summary className="cursor-pointer">CLI output</summary><pre className="mt-2 max-h-40 overflow-auto whitespace-pre-wrap">{st.output}</pre></details>}
      </div>
    </div>
  );
}
