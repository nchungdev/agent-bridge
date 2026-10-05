import { useCallback, useEffect, useMemo, useState } from "react";
import {
  Terminal as TerminalIcon,
  GitBranch,
  RefreshCw,
  FolderOpen,
  Folder,
  ArrowRight,
  Play,
  CheckCircle2,
  AlertCircle,
  UserCheck,
  PanelLeftClose,
  PanelLeftOpen,
  Settings,
  Zap,
  Copy,
  Check,
  MessageSquare,
  X,
} from "lucide-react";
import { TerminalPanel } from "./components/TerminalPanel";
import { AccountsDialog } from "./components/AccountsDialog";

interface Workspace {
  id: string;
  path: string;
  name: string;
}

interface NativeTurn {
  role: string;
  content: string;
}

interface NativeSession {
  agent: string;
  id: string;
  title: string;
  workspace: string;
  updated_at: string;
  last_user: string;
  last_assistant: string;
  turn_count: number;
  turns?: NativeTurn[];
}

interface EngineStatus {
  id: string;
  name: string;
  binary: string;
  installCmd?: string;
  installed: boolean;
  auth_status?: string;
  has_auth?: boolean;
}

interface HandoffResult {
  to: string;
  command: string;
}

const AGENT_META: Record<string, { label: string; badge: string; dot: string }> = {
  agy: { label: "Antigravity", badge: "bg-blue-500/15 text-blue-300 border-blue-500/30", dot: "bg-blue-400" },
  claude: { label: "Claude Code", badge: "bg-orange-500/15 text-orange-300 border-orange-500/30", dot: "bg-orange-400" },
  codex: { label: "Codex", badge: "bg-emerald-500/15 text-emerald-300 border-emerald-500/30", dot: "bg-emerald-400" },
};

const INITIAL_ENGINES: EngineStatus[] = [
  { id: "agy", name: "Antigravity", binary: "antigravity", installed: false },
  { id: "claude", name: "Claude Code", binary: "claude", installCmd: "npm install -g @anthropic-ai/claude-code", installed: false },
  { id: "codex", name: "Codex", binary: "codex", installCmd: "npm install -g @openai/codex", installed: false },
];

function relTime(s?: string): string {
  if (!s) return "";
  const t = Date.parse(s);
  if (Number.isNaN(t) || t < 86400000) return "";
  const m = Math.max(0, Math.round((Date.now() - t) / 60000));
  if (m < 1) return "vừa xong";
  if (m < 60) return `${m} phút`;
  if (m < 1440) return `${Math.floor(m / 60)} giờ`;
  return `${Math.floor(m / 1440)} ngày`;
}

export default function App() {
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
  const [workspace, setWorkspace] = useState<string>(() => localStorage.getItem("bridge_workspace") || "/home/chungnh/AI Workspace");
  const [sessions, setSessions] = useState<NativeSession[]>([]);
  const [selected, setSelected] = useState<NativeSession | null>(null);
  const [detail, setDetail] = useState<NativeSession | null>(null);
  const [modifiedFiles, setModifiedFiles] = useState<string[]>([]);
  const [engines, setEngines] = useState<EngineStatus[]>(INITIAL_ENGINES);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const [result, setResult] = useState<HandoffResult | null>(null);
  const [copied, setCopied] = useState(false);
  const [showTerminal, setShowTerminal] = useState(false);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [accountsEngine, setAccountsEngine] = useState<string | null>(null);

  const loadEngines = useCallback(async () => {
    const next = await Promise.all(
      INITIAL_ENGINES.map(async (eng) => {
        try {
          const r = await fetch("/api/agents/check", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ binary: eng.binary }),
          });
          const d = await r.json();
          return { ...eng, installed: !!d.found, has_auth: d.has_auth, auth_status: d.auth_status };
        } catch {
          return eng;
        }
      })
    );
    setEngines(next);
  }, []);

  const loadWorkspace = useCallback(async (ws: string) => {
    setLoading(true);
    try {
      const [sRes, gRes] = await Promise.all([
        fetch(`/api/bridge/native-sessions?workspace=${encodeURIComponent(ws)}`),
        fetch(`/api/bridge/state?workspace=${encodeURIComponent(ws)}`),
      ]);
      const list: NativeSession[] = sRes.ok ? await sRes.json() : [];
      setSessions(list);
      setSelected((prev) => list.find((s) => prev && s.id === prev.id) || list[0] || null);
      if (gRes.ok) setModifiedFiles((await gRes.json()).modified_files || []);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetch("/api/bridge/workspaces")
      .then((r) => r.json())
      .then((d: Workspace[]) => setWorkspaces(d || []))
      .catch(() => {});
    loadEngines();
  }, [loadEngines]);

  useEffect(() => {
    localStorage.setItem("bridge_workspace", workspace);
    setResult(null);
    loadWorkspace(workspace);
  }, [workspace, loadWorkspace]);

  useEffect(() => {
    setDetail(null);
    if (!selected) return;
    const q = new URLSearchParams({ agent: selected.agent, id: selected.id, workspace });
    fetch(`/api/bridge/native-session?${q}`)
      .then((r) => (r.ok ? r.json() : null))
      .then(setDetail)
      .catch(() => {});
  }, [selected, workspace]);

  const counts = useMemo(() => {
    const c: Record<string, number> = {};
    sessions.forEach((s) => (c[s.agent] = (c[s.agent] || 0) + 1));
    return c;
  }, [sessions]);

  const handoff = async (to: string) => {
    if (!selected) return;
    setBusy(to);
    setResult(null);
    try {
      const r = await fetch("/api/bridge/handoff", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          workspace_path: workspace,
          from_agent: selected.agent,
          from_native_id: selected.id,
          to_agent: to,
        }),
      });
      const d = await r.json();
      if (d.success) setResult({ to, command: d.resume_command });
    } finally {
      setBusy(null);
    }
  };

  const install = async (eng: EngineStatus) => {
    if (!eng.installCmd) return;
    setBusy(`install:${eng.id}`);
    try {
      const r = await fetch("/api/agents/install", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ command: eng.installCmd, binary: eng.binary }),
      });
      const d = await r.json();
      if (!d.success) alert(`Cài đặt thất bại: ${d.error || d.output}`);
      await loadEngines();
    } finally {
      setBusy(null);
    }
  };

  const copyCommand = (cmd: string) => {
    navigator.clipboard?.writeText(`cd "${workspace}" && ${cmd}`);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };

  const recentTurns = (detail?.turns || []).slice(-6);

  return (
    <div className="flex h-screen w-screen overflow-hidden bg-[#0b0e14] text-slate-100">
      {/* ---------- Sidebar: folder → session gốc của từng CLI ---------- */}
      {!sidebarCollapsed ? (
        <aside className="flex h-full w-72 shrink-0 flex-col border-r border-[#1d222b] bg-[#12151c] text-slate-300 select-none">
          <div className="flex items-center justify-between border-b border-[#1c212a] px-3.5 pt-3 pb-2">
            <div className="flex items-center gap-2">
              <Zap className="h-3.5 w-3.5 fill-amber-400 text-amber-400" />
              <span className="text-xs font-semibold tracking-wide text-slate-200">Agent Bridge</span>
            </div>
            <button onClick={() => setSidebarCollapsed(true)} className="cursor-pointer p-1 text-slate-500 hover:text-slate-300" title="Thu gọn">
              <PanelLeftClose className="h-3.5 w-3.5" />
            </button>
          </div>

          {/* Folder picker */}
          <div className="border-b border-[#1c212a] px-2 py-2">
            <div className="px-2 pb-1 text-[10px] font-semibold uppercase tracking-wider text-slate-500">Folder</div>
            <div className="max-h-44 space-y-0.5 overflow-y-auto">
              {workspaces.map((ws) => {
                const active = ws.path === workspace;
                return (
                  <button
                    key={ws.id}
                    onClick={() => setWorkspace(ws.path)}
                    title={ws.path}
                    className={`flex w-full cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-left text-xs ${
                      active ? "bg-[#1d2330] text-slate-100" : "text-slate-400 hover:bg-[#171b24] hover:text-slate-200"
                    }`}
                  >
                    {active ? <FolderOpen className="h-3.5 w-3.5 shrink-0 text-amber-400" /> : <Folder className="h-3.5 w-3.5 shrink-0 text-slate-500" />}
                    <span className="truncate">{ws.name}</span>
                  </button>
                );
              })}
            </div>
          </div>

          {/* Sessions */}
          <div className="flex items-center justify-between px-4 pt-3 pb-1">
            <span className="text-[10px] font-semibold uppercase tracking-wider text-slate-500">Sessions</span>
            <div className="flex items-center gap-1.5">
              {Object.entries(counts).map(([a, n]) => (
                <span key={a} className="flex items-center gap-1 text-[10px] text-slate-500">
                  <span className={`h-1.5 w-1.5 rounded-full ${AGENT_META[a]?.dot}`} />
                  {n}
                </span>
              ))}
            </div>
          </div>
          <div className="flex-1 space-y-0.5 overflow-y-auto px-2 pb-2">
            {sessions.length === 0 && !loading && (
              <div className="px-3 py-6 text-center text-[11px] leading-relaxed text-slate-500">
                Chưa có session nào của Antigravity / Claude / Codex trong folder này.
              </div>
            )}
            {sessions.map((s) => {
              const active = selected?.agent === s.agent && selected?.id === s.id;
              const meta = AGENT_META[s.agent];
              return (
                <button
                  key={`${s.agent}:${s.id}`}
                  onClick={() => setSelected(s)}
                  className={`w-full cursor-pointer rounded-md px-2.5 py-2 text-left ${active ? "bg-[#1d2330]" : "hover:bg-[#171b24]"}`}
                >
                  <div className="flex items-center gap-2">
                    <span className={`h-1.5 w-1.5 shrink-0 rounded-full ${meta?.dot}`} />
                    <span className={`truncate text-xs ${active ? "text-slate-100" : "text-slate-300"}`}>{s.title || "Untitled"}</span>
                    <span className="ml-auto shrink-0 text-[10px] text-slate-500">{relTime(s.updated_at)}</span>
                  </div>
                  {s.last_user && <div className="mt-0.5 truncate pl-3.5 text-[11px] text-slate-500">{s.last_user}</div>}
                </button>
              );
            })}
          </div>

          <div className="space-y-1 border-t border-[#1d222b] bg-[#101217] p-2.5">
            <button
              onClick={() => setAccountsEngine("")}
              className="flex w-full cursor-pointer items-center gap-2 rounded-lg px-2.5 py-1.5 text-xs text-slate-400 hover:bg-[#181c26] hover:text-slate-200"
            >
              <Settings className="h-3.5 w-3.5 text-slate-500" />
              Accounts
            </button>
          </div>
        </aside>
      ) : (
        <div className="flex w-11 shrink-0 flex-col items-center border-r border-[#1d222b] bg-[#12151c] py-3">
          <button onClick={() => setSidebarCollapsed(false)} className="cursor-pointer rounded-lg p-2 text-slate-400 hover:bg-[#1a1e28] hover:text-slate-200" title="Mở rộng">
            <PanelLeftOpen className="h-4 w-4" />
          </button>
        </div>
      )}

      {/* ---------- Main ---------- */}
      <div className="flex h-full min-w-0 flex-1 flex-col">
        <header className="flex h-12 shrink-0 items-center justify-between border-b border-[#1d222b] bg-[#101319] px-5">
          <span className="truncate font-mono text-[12px] text-slate-400" title={workspace}>
            {workspace}
          </span>
          <div className="flex items-center gap-2">
            <button
              onClick={() => {
                loadWorkspace(workspace);
                loadEngines();
              }}
              className="flex cursor-pointer items-center gap-1.5 rounded-md border border-[#2c3447] bg-[#1b202c] px-2.5 py-1 text-[12px] text-slate-300 hover:bg-[#222838]"
            >
              <RefreshCw className={`h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
              Refresh
            </button>
            <button
              onClick={() => setShowTerminal((v) => !v)}
              className={`flex cursor-pointer items-center gap-1.5 rounded-md border px-2.5 py-1 text-[12px] ${
                showTerminal ? "border-indigo-500 bg-indigo-600 text-white" : "border-[#2c3447] bg-[#1b202c] text-slate-300 hover:bg-[#222838]"
              }`}
            >
              <TerminalIcon className="h-3.5 w-3.5" />
              Terminal
            </button>
          </div>
        </header>

        <main className="flex min-h-0 flex-1">
          <div className="min-w-0 flex-1 space-y-5 overflow-y-auto p-6">
            {/* Selected session */}
            {selected ? (
              <section className="rounded-xl border border-[#232a39] bg-[#121620]">
                <div className="flex items-start justify-between gap-4 border-b border-[#1d2331] px-5 py-4">
                  <div className="min-w-0">
                    <div className="mb-1 flex items-center gap-2">
                      <span className={`rounded border px-1.5 py-0.5 text-[10px] font-medium ${AGENT_META[selected.agent]?.badge}`}>
                        {AGENT_META[selected.agent]?.label}
                      </span>
                      <span className="font-mono text-[10.5px] text-slate-500">{selected.id}</span>
                    </div>
                    <h2 className="truncate text-[15px] font-semibold text-slate-100">{selected.title}</h2>
                    <div className="mt-0.5 text-[11px] text-slate-500">
                      {selected.turn_count} tin nhắn{relTime(selected.updated_at) && ` · cập nhật ${relTime(selected.updated_at)} trước`}
                    </div>
                  </div>
                </div>

                {/* Recent conversation — this is what gets handed off */}
                <div className="max-h-[340px] space-y-3 overflow-y-auto px-5 py-4">
                  {!detail && <div className="text-[12px] text-slate-500">Đang đọc transcript…</div>}
                  {recentTurns.map((t, i) => (
                    <div key={i} className="flex gap-2.5">
                      <MessageSquare className={`mt-0.5 h-3.5 w-3.5 shrink-0 ${t.role === "user" ? "text-slate-400" : "text-indigo-400"}`} />
                      <div className="min-w-0">
                        <div className="text-[10.5px] font-medium uppercase tracking-wide text-slate-500">{t.role === "user" ? "Bạn" : AGENT_META[selected.agent]?.label}</div>
                        <div className="line-clamp-4 whitespace-pre-wrap break-words text-[12.5px] leading-relaxed text-slate-300">{t.content}</div>
                      </div>
                    </div>
                  ))}
                </div>

                {modifiedFiles.length > 0 && (
                  <div className="flex flex-wrap items-center gap-1.5 border-t border-[#1d2331] px-5 py-3">
                    <GitBranch className="h-3.5 w-3.5 text-emerald-400" />
                    <span className="mr-1 text-[11px] text-slate-400">{modifiedFiles.length} file thay đổi:</span>
                    {modifiedFiles.slice(0, 6).map((f) => (
                      <span key={f} className="rounded border border-[#2c354b] bg-[#1a2030] px-1.5 py-0.5 font-mono text-[10.5px] text-slate-300">
                        {f}
                      </span>
                    ))}
                    {modifiedFiles.length > 6 && <span className="text-[11px] text-slate-500">+{modifiedFiles.length - 6}</span>}
                  </div>
                )}

                {/* Continue in … */}
                <div className="border-t border-[#1d2331] px-5 py-4">
                  <div className="mb-2.5 text-[11px] font-medium text-slate-400">Tiếp tục session này bằng</div>
                  <div className="flex flex-wrap gap-2">
                    {engines.map((eng) => {
                      const same = eng.id === selected.agent;
                      return (
                        <button
                          key={eng.id}
                          disabled={!eng.installed || busy !== null}
                          onClick={() => handoff(eng.id)}
                          className={`flex cursor-pointer items-center gap-2 rounded-lg border px-3.5 py-2 text-[12px] font-medium transition disabled:cursor-not-allowed disabled:opacity-40 ${
                            same ? "border-[#2c3447] bg-[#1b202c] text-slate-200 hover:bg-[#232a3a]" : "border-indigo-500/50 bg-indigo-600 text-white hover:bg-indigo-500"
                          }`}
                          title={eng.installed ? "" : "Chưa cài đặt"}
                        >
                          <span className={`h-1.5 w-1.5 rounded-full ${AGENT_META[eng.id]?.dot}`} />
                          {same ? `Resume trong ${eng.name}` : eng.name}
                          {busy === eng.id ? <RefreshCw className="h-3.5 w-3.5 animate-spin" /> : <ArrowRight className="h-3.5 w-3.5" />}
                        </button>
                      );
                    })}
                  </div>

                  {result && (
                    <div className="mt-3 rounded-lg border border-emerald-500/30 bg-emerald-500/5 p-3">
                      <div className="mb-2 flex items-center gap-1.5 text-[12px] text-emerald-300">
                        <CheckCircle2 className="h-3.5 w-3.5" />
                        Đã ghi context vào <code className="font-mono">.agent/handoff.md</code>. Chạy lệnh sau để tiếp tục trong {AGENT_META[result.to]?.label}:
                      </div>
                      <div className="flex items-center gap-2">
                        <code className="flex-1 truncate rounded bg-[#0c0f15] px-2.5 py-1.5 font-mono text-[12px] text-slate-200">{result.command}</code>
                        <button
                          onClick={() => copyCommand(result.command)}
                          className="flex cursor-pointer items-center gap-1 rounded-md border border-[#2c3447] bg-[#1b202c] px-2.5 py-1.5 text-[11.5px] text-slate-300 hover:bg-[#232a3a]"
                        >
                          {copied ? <Check className="h-3.5 w-3.5 text-emerald-400" /> : <Copy className="h-3.5 w-3.5" />}
                          Copy
                        </button>
                        <button
                          onClick={() => {
                            copyCommand(result.command);
                            setShowTerminal(true);
                          }}
                          className="flex cursor-pointer items-center gap-1 rounded-md border border-[#2c3447] bg-[#1b202c] px-2.5 py-1.5 text-[11.5px] text-slate-300 hover:bg-[#232a3a]"
                          title="Mở terminal (lệnh đã được copy, dán vào là chạy)"
                        >
                          <TerminalIcon className="h-3.5 w-3.5" />
                          Mở terminal
                        </button>
                        <button onClick={() => setResult(null)} className="cursor-pointer p-1 text-slate-500 hover:text-slate-300">
                          <X className="h-3.5 w-3.5" />
                        </button>
                      </div>
                    </div>
                  )}
                </div>
              </section>
            ) : (
              <section className="rounded-xl border border-dashed border-[#2a3242] p-10 text-center text-[13px] text-slate-500">
                {loading ? "Đang tải…" : "Chọn một session bên trái để xem và chuyển sang agent khác."}
              </section>
            )}

            {/* Engines: install + account, compact */}
            <section>
              <div className="mb-2 text-[11px] font-semibold uppercase tracking-wider text-slate-500">Engines</div>
              <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
                {engines.map((eng) => (
                  <div key={eng.id} className="flex items-center gap-3 rounded-lg border border-[#232a39] bg-[#121620] px-3.5 py-3">
                    <span className={`h-2 w-2 shrink-0 rounded-full ${AGENT_META[eng.id]?.dot}`} />
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-1.5 text-[12.5px] font-medium text-slate-200">
                        {eng.name}
                        {eng.installed ? (
                          eng.has_auth ? <CheckCircle2 className="h-3.5 w-3.5 text-emerald-400" /> : <AlertCircle className="h-3.5 w-3.5 text-amber-400" />
                        ) : null}
                      </div>
                      <div className="truncate text-[11px] text-slate-500" title={eng.auth_status}>
                        {eng.installed ? eng.auth_status || "Đã cài" : "Chưa cài đặt"}
                      </div>
                    </div>
                    {eng.installed ? (
                      <button
                        onClick={() => setAccountsEngine(eng.id)}
                        className="cursor-pointer rounded-md border border-[#2b3447] bg-[#1b202c] p-1.5 text-slate-400 hover:bg-[#242b3b] hover:text-slate-200"
                        title="Đăng nhập / đổi tài khoản"
                      >
                        <UserCheck className="h-3.5 w-3.5" />
                      </button>
                    ) : eng.installCmd ? (
                      <button
                        onClick={() => install(eng)}
                        disabled={busy !== null}
                        className="flex cursor-pointer items-center gap-1 rounded-md bg-emerald-600 px-2.5 py-1.5 text-[11.5px] font-medium text-white hover:bg-emerald-500 disabled:opacity-50"
                      >
                        {busy === `install:${eng.id}` ? <RefreshCw className="h-3.5 w-3.5 animate-spin" /> : <Play className="h-3.5 w-3.5" />}
                        Cài
                      </button>
                    ) : null}
                  </div>
                ))}
              </div>
            </section>
          </div>

          {showTerminal && (
            <div className="flex w-[480px] shrink-0 flex-col border-l border-[#1d222b] bg-[#0c0e14]">
              <div className="flex h-9 shrink-0 items-center justify-between border-b border-[#1d222b] bg-[#101319] px-3">
                <span className="flex items-center gap-1.5 text-[12px] text-slate-300">
                  <TerminalIcon className="h-3.5 w-3.5 text-sky-400" /> Terminal
                </span>
                <button onClick={() => setShowTerminal(false)} className="cursor-pointer p-1 text-slate-500 hover:text-slate-300">
                  <X className="h-3.5 w-3.5" />
                </button>
              </div>
              <div className="flex-1 overflow-hidden">
                <TerminalPanel key={workspace} workDir={workspace} visible={showTerminal} onClose={() => setShowTerminal(false)} />
              </div>
            </div>
          )}
        </main>
      </div>

      {accountsEngine !== null && (
        <AccountsDialog addEngine={accountsEngine || undefined} onChanged={loadEngines} onClose={() => setAccountsEngine(null)} />
      )}
    </div>
  );
}
export { App };
