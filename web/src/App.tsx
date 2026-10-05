import { useCallback, useEffect, useState } from "react";
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
  LayoutDashboard,
  ChevronDown,
  ChevronRight,
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

/** a tab in the right dock: an agent CLI (launched from a handoff) or a plain shell */
interface DockTab {
  key: string;
  label: string;
  workDir: string;
  agent?: string;
  launch?: { agent: string; resume?: string };
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
  const [tabs, setTabs] = useState<DockTab[]>([]);
  const [activeShell, setActiveShell] = useState<string | null>(null);
  const [view, setView] = useState<"dashboard" | "agents" | "terminal">("dashboard");
  const agentTabs = tabs.filter((t) => t.agent);
  const shellTabs = tabs;
  // a session is online while an agent CLI resumed on its id is running in the dock
  const onlineIds = new Set(agentTabs.filter((t) => t.launch?.resume).map((t) => `${t.agent}:${t.launch!.resume}`));
  const [treeCollapsed, setTreeCollapsed] = useState(false);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [accountsEngine, setAccountsEngine] = useState<string | null>(null);

  // re-attach to the shells the server is still running (survives reloads)
  useEffect(() => {
    fetch("/api/terminal/sessions")
      .then((r) => r.json())
      .then((list: { id: string; dir: string; agent?: string; resume?: string }[]) => {
        if (!list?.length) return;
        setTabs((prev) => [
          ...prev,
          ...list
            .filter((x) => !prev.some((t) => t.key === x.id))
            .map((x) => ({
              key: x.id,
              label: x.agent ? `${AGENT_META[x.agent]?.label || x.agent}${x.resume ? " · resume" : ""}` : "Shell",
              workDir: x.dir,
              agent: x.agent,
              launch: x.agent ? { agent: x.agent, resume: x.resume } : undefined,
            })),
        ]);
        let last: string | null = null;
        try {
          last = localStorage.getItem("bridge_active_tab");
        } catch {
          /* storage unavailable */
        }
        setActiveShell((cur) => cur ?? (list.some((x) => x.id === last) ? last : list[0].id));
      })
      .catch(() => {});
  }, []);

  useEffect(() => {
    try {
      if (activeShell) localStorage.setItem("bridge_active_tab", activeShell);
    } catch {
      /* storage unavailable */
    }
  }, [activeShell]);

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

  const handoff = async (to: string) => {
    if (!selected) return;
    const same = to === selected.agent;
    setBusy(to);
    setResult(null);
    try {
      // same agent, same session: it already has its context, so just resume it; otherwise write the handoff first
      if (!same) {
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
        if (!d.success) {
          alert(`Handoff thất bại: ${d.error || "unknown error"}`);
          return;
        }
        setResult({ to, command: d.resume_command });
      }
      // free the agent we are leaving (and any stale copy of the one we are opening): closing a tab kills its process
      const stale = tabs.filter((t) => t.agent && t.workDir === workspace && (t.agent === selected.agent || t.agent === to));
      stale.forEach((t) => killSession(t.key));
      setTabs((prev) => prev.filter((t) => !stale.some((x) => x.key === t.key)));
      openTab({
        key: `${to}-${Date.now().toString(36)}`,
        label: `${AGENT_META[to]?.label || to}${same ? " · resume" : ""}`,
        workDir: workspace,
        agent: to,
        launch: { agent: to, resume: same ? selected.id : undefined },
      });
    } finally {
      setBusy(null);
    }
  };

  const openGui = async (agent: string) => {
    const r = await fetch("/api/bridge/open", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ agent, workspace_path: workspace }),
    });
    const d = await r.json().catch(() => ({}));
    if (d.url) window.open(d.url, "_blank", "noopener");
    else if (!d.success) alert("Không mở được GUI của agent này");
  };

  const openTab = (t: DockTab) => {
    setTabs((prev) => [...prev, t]);
    setActiveShell(t.key);
    setView("terminal");
  };

  // closing a tab ends its process on the server
  const killSession = (key: string) => fetch(`/api/terminal/sessions/${key}`, { method: "DELETE" }).catch(() => {});

  const closeTab = (key: string) => {
    killSession(key);
    setTabs((prev) => {
      const next = prev.filter((t) => t.key !== key);
      setActiveShell((cur) => (cur === key ? next[next.length - 1]?.key ?? null : cur));
      return next;
    });
  };

  const openShell = () => openTab({ key: `sh${Date.now().toString(36)}`, label: "Shell", workDir: workspace });

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

          {/* Navigation */}
          <div className="space-y-0.5 border-b border-[#1c212a] px-2 py-2">
            {([
              ["dashboard", "Dashboard", LayoutDashboard, "text-amber-400"],
              ["agents", "Cấu hình agent", Settings, "text-violet-400"],
              ["terminal", "Terminal", TerminalIcon, "text-sky-400"],
            ] as const).map(([id, label, Icon, color]) => (
              <button
                key={id}
                onClick={() => {
                  setView(id);
                  if (id === "terminal" && shellTabs.length === 0) openShell();
                }}
                className={`flex w-full cursor-pointer items-center gap-2 rounded-md px-2.5 py-1.5 text-xs ${
                  view === id ? "bg-[#1d2330] text-slate-100" : "text-slate-400 hover:bg-[#171b24] hover:text-slate-200"
                }`}
              >
                <Icon className={`h-3.5 w-3.5 ${color}`} />
                {label}
                {id === "terminal" && shellTabs.length > 0 && <span className="ml-auto text-[10px] text-slate-500">{shellTabs.length}</span>}
              </button>
            ))}
          </div>

          {/* Tree: folder → sessions of every agent */}
          <div className="px-4 pt-3 pb-1 text-[10px] font-semibold uppercase tracking-wider text-slate-500">Folders</div>
          <div className="flex-1 space-y-0.5 overflow-y-auto px-2 pb-2">
            {workspaces.map((ws) => {
              const active = ws.path === workspace;
              const open = active && !treeCollapsed;
              return (
                <div key={ws.id}>
                  <button
                    onClick={() => {
                      if (active) setTreeCollapsed((v) => !v);
                      else {
                        setWorkspace(ws.path);
                        setTreeCollapsed(false);
                      }
                      setView("dashboard");
                    }}
                    title={ws.path}
                    className={`flex w-full cursor-pointer items-center gap-1.5 rounded-md px-1.5 py-1.5 text-left text-xs ${
                      active ? "text-slate-100" : "text-slate-400 hover:bg-[#171b24] hover:text-slate-200"
                    }`}
                  >
                    {open ? <ChevronDown className="h-3 w-3 shrink-0 text-slate-500" /> : <ChevronRight className="h-3 w-3 shrink-0 text-slate-500" />}
                    {open ? <FolderOpen className="h-3.5 w-3.5 shrink-0 text-amber-400" /> : <Folder className="h-3.5 w-3.5 shrink-0 text-slate-500" />}
                    <span className="truncate">{ws.name}</span>
                    {active && sessions.length > 0 && <span className="ml-auto shrink-0 text-[10px] text-slate-500">{sessions.length}</span>}
                  </button>
                  {open && (
                    <div className="ml-3.5 space-y-0.5 border-l border-[#232a39] pl-1.5">
                      {sessions.length === 0 && !loading && <div className="px-2 py-2 text-[11px] text-slate-500">Chưa có session nào.</div>}
                      {sessions.map((s) => {
                        const sActive = selected?.agent === s.agent && selected?.id === s.id;
                        const meta = AGENT_META[s.agent];
                        return (
                          <button
                            key={`${s.agent}:${s.id}`}
                            onClick={() => {
                              setSelected(s);
                              setView("dashboard");
                            }}
                            className={`w-full cursor-pointer rounded-md px-2 py-1.5 text-left ${sActive ? "bg-[#1d2330]" : "hover:bg-[#171b24]"}`}
                          >
                            <div className="flex items-center gap-2">
                              {onlineIds.has(`${s.agent}:${s.id}`) ? (
                                <span className={`h-1.5 w-1.5 shrink-0 rounded-full ${meta?.dot}`} title={`Đang chạy trong ${meta?.label}`} />
                              ) : (
                                <span className="h-1.5 w-1.5 shrink-0" />
                              )}
                              <span className={`truncate text-xs ${sActive ? "text-slate-100" : "text-slate-300"}`}>{s.title || "Untitled"}</span>
                              <span className="ml-auto shrink-0 text-[10px] text-slate-500">{relTime(s.updated_at)}</span>
                            </div>
                            <div className="mt-0.5 truncate pl-3.5 text-[10.5px] text-slate-500">
                              {meta?.label}
                              {s.last_user ? ` · ${s.last_user}` : ""}
                            </div>
                          </button>
                        );
                      })}
                    </div>
                  )}
                </div>
              );
            })}
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
          </div>
        </header>

        <main className={`min-h-0 flex-1 ${view === "dashboard" ? "flex" : "hidden"}`}>
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
                        <div key={eng.id} className="flex items-stretch">
                        <button
                          disabled={!eng.installed || busy !== null}
                          onClick={() => handoff(eng.id)}
                          className={`flex cursor-pointer items-center gap-2 rounded-l-lg border px-3.5 py-2 text-[12px] font-medium transition disabled:cursor-not-allowed disabled:opacity-40 ${
                            same ? "border-[#2c3447] bg-[#1b202c] text-slate-200 hover:bg-[#232a3a]" : "border-indigo-500/50 bg-indigo-600 text-white hover:bg-indigo-500"
                          }`}
                          title={eng.installed ? "" : "Chưa cài đặt"}
                        >
                          <span className={`h-1.5 w-1.5 rounded-full ${AGENT_META[eng.id]?.dot}`} />
                          {same ? `Resume trong ${eng.name}` : eng.name}
                          {busy === eng.id ? <RefreshCw className="h-3.5 w-3.5 animate-spin" /> : <ArrowRight className="h-3.5 w-3.5" />}
                        </button>
                          <button
                            disabled={!eng.installed}
                            onClick={() => openGui(eng.id)}
                            className="cursor-pointer rounded-r-lg border border-l-0 border-[#2c3447] bg-[#1b202c] px-2 text-[11px] text-slate-300 hover:bg-[#232a3a] disabled:cursor-not-allowed disabled:opacity-40"
                            title={`Mở GUI của ${eng.name}`}
                          >
                            Mở GUI
                          </button>
                        </div>
                      );
                    })}
                  </div>

                  {result && (
                    <div className="mt-3 rounded-lg border border-emerald-500/30 bg-emerald-500/5 p-3">
                      <div className="mb-2 flex items-center gap-1.5 text-[12px] text-emerald-300">
                        <CheckCircle2 className="h-3.5 w-3.5" />
                        Đã ghi context vào <code className="font-mono">.agent/handoff.md</code> và mở {AGENT_META[result.to]?.label} ở tab Terminal. Lệnh tương đương nếu muốn chạy ở terminal ngoài:
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

          </div>

        </main>

        {/* ---------- Agent config view ---------- */}
        <div className={`min-h-0 flex-1 overflow-y-auto p-6 ${view === "agents" ? "block" : "hidden"}`}>
          <div className="mb-4 flex items-center justify-between">
            <h2 className="text-[15px] font-semibold text-slate-100">Cấu hình agent</h2>
            <button
              onClick={() => setAccountsEngine("")}
              className="flex cursor-pointer items-center gap-1.5 rounded-md border border-[#2c3447] bg-[#1b202c] px-2.5 py-1 text-[12px] text-slate-300 hover:bg-[#222838]"
            >
              <Settings className="h-3.5 w-3.5" />
              Tài khoản
            </button>
          </div>
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

        {/* ---------- Terminal view: plain shells, opened from the sidebar ---------- */}
        <div className={`min-h-0 flex-1 flex-col bg-[#0c0e14] ${view === "terminal" ? "flex" : "hidden"}`}>
          <div className="flex h-9 shrink-0 items-center gap-0.5 overflow-x-auto border-b border-[#1d222b] bg-[#101319] px-1.5">
            {shellTabs.map((t) => (
              <div
                key={t.key}
                onClick={() => setActiveShell(t.key)}
                className={`flex shrink-0 cursor-pointer items-center gap-1.5 rounded-md px-2.5 py-1 text-[12px] ${
                  t.key === activeShell ? "bg-[#1d2330] text-slate-100" : "text-slate-400 hover:bg-[#171b24] hover:text-slate-200"
                }`}
              >
                {t.agent ? <span className={`h-1.5 w-1.5 rounded-full ${AGENT_META[t.agent]?.dot}`} /> : <TerminalIcon className="h-3.5 w-3.5 text-sky-400" />}
                <span title={t.workDir}>{t.label}</span>
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    closeTab(t.key);
                  }}
                  className="cursor-pointer rounded p-0.5 text-slate-500 hover:text-rose-400"
                  title="Đóng (kết thúc tiến trình)"
                >
                  <X className="h-3 w-3" />
                </button>
              </div>
            ))}
            <button onClick={openShell} className="shrink-0 cursor-pointer rounded-md px-2 py-1 text-[13px] text-slate-400 hover:bg-[#171b24] hover:text-slate-200" title="Mở shell mới trong folder đang chọn">
              +
            </button>
          </div>
          <div className="relative flex-1 overflow-hidden">
            {shellTabs.map((t) => (
              <div key={t.key} className={`absolute inset-0 ${t.key === activeShell ? "" : "invisible"}`}>
                <TerminalPanel workDir={t.workDir} launch={t.launch} sessionId={t.key} title={t.label} headless visible={view === "terminal" && t.key === activeShell} onClose={() => closeTab(t.key)} />
              </div>
            ))}
          </div>
        </div>
      </div>

      {accountsEngine !== null && (
        <AccountsDialog addEngine={accountsEngine || undefined} onChanged={loadEngines} onClose={() => setAccountsEngine(null)} />
      )}
    </div>
  );
}
export { App };
