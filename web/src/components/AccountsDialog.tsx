import React, { useCallback, useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { Check, LogIn, Pencil, Plus, Trash2, X } from "lucide-react";
import type { AccountGroup, AccountInfo } from "../v2/types";
import { LoginPanel } from "../v2/LoginPanel";
import { ConfirmDialog } from "./ConfirmDialog";

const ENGINE_NAME: Record<string, string> = { claude: "Claude Code", codex: "OpenAI Codex", agy: "Antigravity" };

interface Props {
  onClose: () => void;
  /** called after anything changed so the app can refresh its account list */
  onChanged: () => void;
  /** open with the "add account" form for this engine already showing */
  addEngine?: string;
}

/** Sign in / switch / add / rename / remove the accounts of every engine that supports several logins. */
export const AccountsDialog: React.FC<Props> = ({ onClose, onChanged, addEngine }) => {
  const [groups, setGroups] = useState<AccountGroup[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [adding, setAdding] = useState<{ engine: string; label: string } | null>(addEngine ? { engine: addEngine, label: "" } : null);
  const [renaming, setRenaming] = useState<{ id: string; label: string } | null>(null);
  const [removing, setRemoving] = useState<AccountInfo | null>(null);
  const [relogin, setRelogin] = useState<AccountInfo | null>(null);
  const [login, setLogin] = useState<{ id: string; replace: boolean } | null>(null);
  const addRef = useRef<HTMLInputElement>(null);

  const load = useCallback((force = false) => {
    fetch(`/api/v2/accounts${force ? "?refresh=1" : ""}`)
      .then((r) => r.json())
      .then((g: AccountGroup[]) => setGroups(Array.isArray(g) ? g : []))
      .catch(() => setError("Could not load accounts"));
  }, []);
  useEffect(() => { load(true); }, [load]);
  useEffect(() => { if (adding) addRef.current?.focus(); }, [adding?.engine]);

  const changed = () => { load(true); onChanged(); };
  const call = async (url: string, init: RequestInit) => {
    setError(null);
    const r = await fetch(url, init);
    if (!r.ok) {
      const e = await r.json().catch(() => ({ error: r.statusText }));
      setError(e.error ?? r.statusText);
      return null;
    }
    return r.status === 204 ? {} : r.json();
  };
  const json = (body: object): RequestInit => ({ headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });

  const create = async () => {
    if (!adding || !adding.label.trim()) return;
    const a = await call("/api/v2/accounts", { method: "POST", ...json({ engine: adding.engine, label: adding.label.trim() }) });
    if (a) {
      setAdding(null);
      changed();
      setLogin({ id: (a as { id: string }).id, replace: false }); // go straight to signing in
    }
  };
  const use = async (a: AccountInfo) => {
    if (await call("/api/v2/accounts/active", { method: "PUT", ...json({ engine: a.engine, id: a.id }) })) changed();
  };
  const captureAgy = async () => {
    if (await call("/api/v2/accounts/agy/capture", { method: "POST" })) changed();
  };
  const rename = async () => {
    if (!renaming || !renaming.label.trim()) return;
    if (await call(`/api/v2/accounts/${encodeURIComponent(renaming.id)}`, { method: "PATCH", ...json({ label: renaming.label.trim() }) })) {
      setRenaming(null);
      changed();
    }
  };
  const remove = async (a: AccountInfo) => {
    setRemoving(null);
    if (await call(`/api/v2/accounts/${encodeURIComponent(a.id)}`, { method: "DELETE" })) changed();
  };

  return createPortal(
    <>
      <div className="fixed inset-0 z-[90] flex items-center justify-center bg-black/60 p-4" role="dialog" aria-modal="true" aria-label="Accounts" onMouseDown={(e) => { if (e.target === e.currentTarget) onClose(); }}>
        <div className="flex max-h-[85vh] w-full max-w-xl flex-col rounded-2xl border border-[#2c3344] bg-[#171b23] text-slate-200 shadow-2xl">
          <div className="flex items-center justify-between border-b border-[#232a38] px-5 py-3.5">
            <div>
              <h2 className="text-[15px] font-semibold text-slate-100">Accounts</h2>
              <p className="text-[11.5px] text-slate-500">Each account keeps its own login, quota and sessions. The one marked “In use” is used for new messages.</p>
            </div>
            <button onClick={onClose} className="text-slate-400 hover:text-white cursor-pointer" aria-label="Close"><X className="h-4 w-4" /></button>
          </div>

          <div className="space-y-5 overflow-y-auto px-5 py-4">
            {error && <div className="rounded-lg border border-rose-800/50 bg-rose-950/30 px-3 py-2 text-xs text-rose-300 break-words">{error}</div>}
            {!groups && !error && <div className="text-xs text-slate-500">Loading…</div>}
            {groups?.map((g) => (
              <section key={g.engine} className="space-y-2">
                <h3 className="text-[11px] font-semibold uppercase tracking-wider text-slate-500">{ENGINE_NAME[g.engine] ?? g.engine}</h3>
                {g.accounts.map((a) => (
                  <div key={a.id} className={`rounded-xl border px-3.5 py-2.5 ${a.active ? "border-sky-700/60 bg-sky-950/20" : "border-[#272e3d] bg-[#141824]/60"}`}>
                    <div className="flex items-center justify-between gap-3">
                      <div className="min-w-0">
                        {renaming?.id === a.id ? (
                          <input
                            autoFocus
                            value={renaming.label}
                            onChange={(e) => setRenaming({ id: a.id, label: e.target.value })}
                            onKeyDown={(e) => { if (e.key === "Enter") rename(); if (e.key === "Escape") setRenaming(null); }}
                            onBlur={() => setRenaming(null)}
                            className="w-44 rounded-md border border-sky-600/60 bg-[#0f1218] px-2 py-0.5 text-sm text-slate-100 outline-none"
                          />
                        ) : (
                          <div className="flex items-center gap-2">
                            <span className="truncate text-[13.5px] font-medium text-slate-100">{a.default ? "Default account" : a.label}</span>
                            {a.active && <span className="flex items-center gap-1 rounded bg-sky-900/50 px-1.5 py-0.5 text-[10px] font-medium text-sky-300"><Check className="h-3 w-3" />In use</span>}
                          </div>
                        )}
                        <div className="mt-0.5 flex items-center gap-1.5 text-[11.5px]">
                          <span className={`h-1.5 w-1.5 rounded-full ${a.logged_in ? "bg-emerald-400" : a.known ? "bg-rose-400" : "bg-slate-500"}`} />
                          <span className={a.logged_in ? "text-emerald-300" : "text-slate-400"}>{a.logged_in ? "Signed in" : a.known ? "Signed out" : "Status unknown"}</span>
                          {a.detail && (a.logged_in || g.engine === "agy") && <span className="truncate text-slate-500">· {a.detail}</span>}
                        </div>
                      </div>
                      <div className="flex shrink-0 items-center gap-1">
                        {!a.active && <button onClick={() => use(a)} className="rounded-md bg-[#232a38] px-2.5 py-1 text-[11.5px] text-slate-200 hover:bg-[#2c3445] cursor-pointer">Use</button>}
                        {a.can_login && (
                          <button
                            onClick={() => (a.logged_in ? setRelogin(a) : setLogin({ id: a.id, replace: false }))}
                            className={`flex items-center gap-1 rounded-md px-2.5 py-1 text-[11.5px] cursor-pointer ${a.logged_in ? "bg-[#232a38] text-slate-300 hover:bg-[#2c3445]" : "bg-sky-600 text-white hover:bg-sky-500"}`}
                          >
                            <LogIn className="h-3 w-3" />{a.logged_in ? "Sign in again" : "Sign in"}
                          </button>
                        )}
                        {!a.default && g.engine !== "agy" && (
                          <>
                            <button onClick={() => setRenaming({ id: a.id, label: a.label })} className="rounded-md p-1.5 text-slate-400 hover:bg-[#232a38] hover:text-slate-100 cursor-pointer" title="Rename"><Pencil className="h-3.5 w-3.5" /></button>
                            <button onClick={() => setRemoving(a)} className="rounded-md p-1.5 text-slate-400 hover:bg-rose-950/40 hover:text-rose-300 cursor-pointer" title="Remove"><Trash2 className="h-3.5 w-3.5" /></button>
                          </>
                        )}
                      </div>
                    </div>
                  </div>
                ))}
                {g.engine === "agy" ? (
                  <div className="space-y-1.5">
                    <button onClick={captureAgy} className="flex items-center gap-1.5 rounded-md px-1 py-1 text-[12px] text-sky-400 hover:text-sky-300 cursor-pointer">
                      <Plus className="h-3.5 w-3.5" />Save current login as account
                    </button>
                    <p className="text-[11px] text-slate-500">To add an account: run <code>agy</code> in a terminal and sign in, then save it here. Switching applies to all Antigravity conversations on this server.</p>
                  </div>
                ) : adding?.engine === g.engine ? (
                  <div className="flex items-center gap-2">
                    <input
                      ref={addRef}
                      value={adding.label}
                      onChange={(e) => setAdding({ engine: g.engine, label: e.target.value })}
                      onKeyDown={(e) => { if (e.key === "Enter") create(); if (e.key === "Escape") setAdding(null); }}
                      placeholder="Name, e.g. Work or Personal"
                      className="flex-1 rounded-md border border-sky-600/60 bg-[#0f1218] px-2.5 py-1.5 text-[12.5px] text-slate-100 outline-none"
                    />
                    <button onClick={create} disabled={!adding.label.trim()} className="rounded-md bg-sky-600 px-3 py-1.5 text-[12px] font-medium text-white hover:bg-sky-500 disabled:opacity-40 cursor-pointer">Create & sign in</button>
                    <button onClick={() => setAdding(null)} className="rounded-md bg-[#232a38] px-2.5 py-1.5 text-[12px] text-slate-300 hover:bg-[#2c3445] cursor-pointer">Cancel</button>
                  </div>
                ) : (
                  <button onClick={() => setAdding({ engine: g.engine, label: "" })} className="flex items-center gap-1.5 rounded-md px-1 py-1 text-[12px] text-sky-400 hover:text-sky-300 cursor-pointer">
                    <Plus className="h-3.5 w-3.5" />Add {ENGINE_NAME[g.engine] ?? g.engine} account
                  </button>
                )}
              </section>
            ))}
            {groups && (
              <p className="border-t border-[#232a38] pt-3 text-[11px] leading-relaxed text-slate-500">
                Antigravity uses the saved profiles from AGY Manager. Finish running Antigravity tasks before switching.
                Switching account in the middle of a conversation continues it from the hub's summary of what was said so far.
              </p>
            )}
          </div>
        </div>
      </div>

      {login && <LoginPanel engine={login.id} replace={login.replace} onClose={() => { setLogin(null); changed(); }} onDone={changed} />}
      {relogin && (
        <ConfirmDialog
          danger
          title="Sign in again?"
          message={<>This replaces the stored login of <b>{relogin.default ? "the default account" : relogin.label}</b>{relogin.detail ? <> ({relogin.detail})</> : null}. Use it to switch that account to a different user.</>}
          confirmLabel="Sign in again"
          onConfirm={() => { setLogin({ id: relogin.id, replace: true }); setRelogin(null); }}
          onCancel={() => setRelogin(null)}
        />
      )}
      {removing && (
        <ConfirmDialog
          danger
          title="Remove account?"
          message={<>“{removing.label}” and its stored login will be deleted from this server. Conversations that used it keep their history.</>}
          confirmLabel="Remove"
          onConfirm={() => remove(removing)}
          onCancel={() => setRemoving(null)}
        />
      )}
    </>,
    document.body
  );
};
