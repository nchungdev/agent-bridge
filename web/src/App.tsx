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
  Menu,
  Settings,
  Zap,
  Copy,
  Check,
  MessageSquare,
  ChevronDown,
  ChevronRight,
  Plus,
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

/** a terminal tab: an agent CLI (resumed session, fresh session or handoff) or a plain shell */
interface TermTab {
  key: string;
  label: string;
  workDir: string;
  createdAt: number;
  agent?: string;
  launch?: { agent: string; resume?: string; fresh?: boolean };
  /** the session this tab was handed off from: it supplies the context for the next switch */
  from?: { agent: string; id: string };
}

const AGENT_META: Record<string, { label: string; badge: string; dot: string; chip: string }> = {
  agy: { label: "Antigravity", badge: "bg-blue-500/15 text-blue-300 border-blue-500/30", dot: "bg-blue-400", chip: "hover:border-blue-500/50 hover:text-blue-300" },
  claude: { label: "Claude Code", badge: "bg-orange-500/15 text-orange-300 border-orange-500/30", dot: "bg-orange-400", chip: "hover:border-orange-500/50 hover:text-orange-300" },
  codex: { label: "Codex", badge: "bg-emerald-500/15 text-emerald-300 border-emerald-500/30", dot: "bg-emerald-400", chip: "hover:border-emerald-500/50 hover:text-emerald-300" },
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

/** true while the viewport matches the media query (kept in sync on resize/rotate) */
function useMedia(query: string): boolean {
  const [match, setMatch] = useState(() => window.matchMedia(query).matches);
  useEffect(() => {
    const mq = window.matchMedia(query);
    const on = () => setMatch(mq.matches);
    on();
    mq.addEventListener("change", on);
    return () => mq.removeEventListener("change", on);
  }, [query]);
  return match;
}

const newKey = (prefix: string) => `${prefix}-${Date.now().toString(36)}`;
const shortTitle = (s: string) => (s.length > 26 ? `${s.slice(0, 25)}…` : s);

export default function App() {
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
  const [workspace, setWorkspace] = useState<string>(() => localStorage.getItem("bridge_workspace") || "/home/chungnh/AI Workspace");
  const [sessions, setSessions] = useState<NativeSession[]>([]);
  const [detail, setDetail] = useState<NativeSession | null>(null);
  const [modifiedFiles, setModifiedFiles] = useState<string[]>([]);
  const [engines, setEngines] = useState<EngineStatus[]>(INITIAL_ENGINES);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const [tabs, setTabs] = useState<TermTab[]>([]);
  const [activeKey, setActiveKey] = useState<string | null>(null);
  const [view, setView] = useState<"terminal" | "settings">("terminal");
  const [switchOpen, setSwitchOpen] = useState(false);
  const [contextOpen, setContextOpen] = useState(false);
  const [treeCollapsed, setTreeCollapsed] = useState(false);
  // on a phone the sidebar starts collapsed so the terminal gets the whole screen
  const isMobile = useMedia("(max-width: 767px)");
  const [sidebarCollapsed, setSidebarCollapsed] = useState(() => window.innerWidth < 768);
  // on a phone the sidebar is a drawer over the terminal: close it once something was picked
  const closeDrawer = () => {
    if (isMobile) setSidebarCollapsed(true);
  };
  const [accountsEngine, setAccountsEngine] = useState<string | null>(null);

  const activeTab = tabs.find((t) => t.key === activeKey) || null;
  // a session is online while an agent CLI resumed on its id is running in a tab
  const onlineIds = useMemo(() => new Set(tabs.filter((t) => t.launch?.resume).map((t) => `${t.agent}:${t.launch!.resume}`)), [tabs]);

  // the session behind the active tab: the one it resumed, the one it was handed off from, or (for a brand
  // new agent session) the newest session of that agent that appeared after the tab was opened
  const ctx = useMemo<NativeSession | null>(() => {
    if (!activeTab?.agent) return null;
    const id = activeTab.launch?.resume || activeTab.from?.id;
    const agent = activeTab.launch?.resume ? activeTab.agent : activeTab.from?.agent || activeTab.agent;
    if (id) return sessions.find((s) => s.agent === agent && s.id === id) || null;
    const since = activeTab.createdAt - 60_000;
    return sessions.filter((s) => s.agent === activeTab.agent && Date.parse(s.updated_at) >= since).sort((a, b) => Date.parse(b.updated_at) - Date.parse(a.updated_at))[0] || null;
  }, [activeTab, sessions]);

  // re-attach to the terminals the server is still running (survives reloads and server restarts)
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
              label: x.agent ? AGENT_META[x.agent]?.label || x.agent : "Shell",
              workDir: x.dir,
              createdAt: 0,
              agent: x.agent || undefined,
              launch: x.agent ? { agent: x.agent, resume: x.resume || undefined } : undefined,
            })),
        ]);
        let last: string | null = null;
        try {
          last = localStorage.getItem("bridge_active_tab");
        } catch {
          /* storage unavailable */
        }
        setActiveKey((cur) => cur ?? (list.some((x) => x.id === last) ? last : list[0].id));
      })
      .catch(() => {});
  }, []);

  useEffect(() => {
    try {
      if (activeKey) localStorage.setItem("bridge_active_tab", activeKey);
    } catch {
      /* storage unavailable */
    }
  }, [activeKey]);

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

  const loadWorkspace = useCallback(async (ws: string, quiet = false) => {
    if (!quiet) setLoading(true);
    try {
      const [sRes, gRes] = await Promise.all([
        fetch(`/api/bridge/native-sessions?workspace=${encodeURIComponent(ws)}`),
        fetch(`/api/bridge/state?workspace=${encodeURIComponent(ws)}`),
      ]);
      if (sRes.ok) setSessions(await sRes.json());
      if (gRes.ok) setModifiedFiles((await gRes.json()).modified_files || []);
    } finally {
      if (!quiet) setLoading(false);
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
    loadWorkspace(workspace);
    // new conversations show up in the tree while you work
    const t = window.setInterval(() => loadWorkspace(workspace, true), 15000);
    return () => window.clearInterval(t);
  }, [workspace, loadWorkspace]);

  // transcript of the active tab's session (context drawer + what a switch hands over)
  const ctxId = ctx ? `${ctx.agent}:${ctx.id}` : "";
  useEffect(() => {
    setDetail(null);
    if (!ctx) return;
    const q = new URLSearchParams({ agent: ctx.agent, id: ctx.id, workspace: ctx.workspace || workspace });
    fetch(`/api/bridge/native-session?${q}`)
      .then((r) => (r.ok ? r.json() : null))
      .then(setDetail)
      .catch(() => {});
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ctxId]);

  // ---------- tabs ----------
  // closing a tab ends its process on the server
  const killSession = (key: string) => fetch(`/api/terminal/sessions/${key}`, { method: "DELETE" }).catch(() => {});

  const openTab = (t: TermTab) => {
    setTabs((prev) => [...prev, t]);
    setActiveKey(t.key);
    setView("terminal");
    closeDrawer();
  };

  const closeTab = (key: string) => {
    killSession(key);
    setTabs((prev) => {
      const next = prev.filter((t) => t.key !== key);
      setActiveKey((cur) => (cur === key ? next[next.length - 1]?.key ?? null : cur));
      return next;
    });
  };

  /** click a session in the tree: jump to its terminal, or resume it in its own agent */
  const openSession = (s: NativeSession) => {
    const existing = tabs.find((t) => t.agent === s.agent && t.launch?.resume === s.id);
    if (existing) {
      setActiveKey(existing.key);
      setView("terminal");
      closeDrawer();
      return;
    }
    openTab({
      key: newKey(s.agent),
      label: shortTitle(s.title || AGENT_META[s.agent]?.label || s.agent),
      workDir: s.workspace || workspace,
      createdAt: Date.now(),
      agent: s.agent,
      launch: { agent: s.agent, resume: s.id },
    });
  };

  const newAgentSession = (agent: string, dir: string) =>
    openTab({ key: newKey(agent), label: `${AGENT_META[agent]?.label || agent} · mới`, workDir: dir, createdAt: Date.now(), agent, launch: { agent, fresh: true } });

  const newShell = (dir: string) => openTab({ key: newKey("sh"), label: "Shell", workDir: dir, createdAt: Date.now() });

  /** switch the active agent tab to another agent: write the handoff, close the current CLI, open the new one */
  const switchAgent = async (to: string) => {
    if (!activeTab || !ctx) return;
    setSwitchOpen(false);
    setBusy(to);
    try {
      const r = await fetch("/api/bridge/handoff", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ workspace_path: activeTab.workDir, from_agent: ctx.agent, from_native_id: ctx.id, to_agent: to }),
      });
      const d = await r.json();
      if (!d.success) {
        alert(`Handoff thất bại: ${d.error || "unknown error"}`);
        return;
      }
      closeTab(activeTab.key); // free the agent we are leaving
      openTab({
        key: newKey(to),
        label: `${AGENT_META[to]?.label || to} ← ${AGENT_META[ctx.agent]?.label || ctx.agent}`,
        workDir: activeTab.workDir,
        createdAt: Date.now(),
        agent: to,
        launch: { agent: to },
        from: { agent: ctx.agent, id: ctx.id },
      });
    } finally {
      setBusy(null);
    }
  };

  const openGui = async (agent: string) => {
    const r = await fetch("/api/bridge/open", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ agent, workspace_path: activeTab?.workDir || workspace }),
    });
    const d = await r.json().catch(() => ({}));
    if (d.url) window.open(d.url, "_blank", "noopener");
    else if (!d.success) alert("Không mở được GUI của agent này");
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

  const copyText = (text: string) => {
    navigator.clipboard?.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };

  const installed = engines.filter((e) => e.installed);
  const switchTargets = installed.filter((e) => e.id !== ctx?.agent);
  const allTurns = detail?.turns || [];
  const lastTurns = allTurns.slice(-6);

  return (
    <div className="flex h-dvh w-screen overflow-hidden bg-[#0b0e14] text-slate-100">
      {/* ---------- Sidebar: folders & sessions, settings pinned at the bottom ---------- */}
      {!sidebarCollapsed && isMobile && <div className="fixed inset-0 z-30 bg-black/60" onClick={() => setSidebarCollapsed(true)} />}
      {!sidebarCollapsed ? (
        <aside
          className={`flex flex-col border-r border-[#1d222b] bg-[#12151c] text-slate-300 select-none ${
            isMobile ? "fixed inset-y-0 left-0 z-40 w-[85vw] max-w-xs pt-[env(safe-area-inset-top)] shadow-2xl" : "h-full w-72 shrink-0"
          }`}
        >
          <div className="flex items-center justify-between border-b border-[#1c212a] px-3.5 pt-3 pb-2">
            <div className="flex items-center gap-2">
              <Zap className="h-3.5 w-3.5 fill-amber-400 text-amber-400" />
              <span className="text-xs font-semibold tracking-wide text-slate-200">Agent Bridge</span>
            </div>
            <button onClick={() => setSidebarCollapsed(true)} className="cursor-pointer p-1 text-slate-500 hover:text-slate-300" title="Thu gọn">
              <PanelLeftClose className="h-3.5 w-3.5" />
            </button>
          </div>

          <div className="px-4 pt-3 pb-1 text-[10px] font-semibold uppercase tracking-wider text-slate-500">Folders &amp; sessions</div>
          <div className="min-h-0 flex-1 space-y-0.5 overflow-y-auto px-2 pb-2">
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
                      {/* start something new in this folder */}
                      <div className="flex flex-wrap items-center gap-1 px-1 py-1">
                        <Plus className="h-3 w-3 text-slate-600" />
                        {installed.map((e) => (
                          <button
                            key={e.id}
                            onClick={() => newAgentSession(e.id, ws.path)}
                            title={`Phiên ${e.name} mới trong folder này`}
                            className={`flex cursor-pointer items-center gap-1 rounded-full border border-[#2a3242] px-2 py-0.5 text-[10.5px] text-slate-400 ${AGENT_META[e.id]?.chip}`}
                          >
                            <span className={`h-1.5 w-1.5 rounded-full ${AGENT_META[e.id]?.dot}`} />
                            {AGENT_META[e.id]?.label.split(" ")[0]}
                          </button>
                        ))}
                        <button
                          onClick={() => newShell(ws.path)}
                          title="Shell mới trong folder này"
                          className="flex cursor-pointer items-center gap-1 rounded-full border border-[#2a3242] px-2 py-0.5 text-[10.5px] text-slate-400 hover:border-sky-500/50 hover:text-sky-300"
                        >
                          <TerminalIcon className="h-2.5 w-2.5" />
                          Shell
                        </button>
                      </div>
                      {sessions.length === 0 && !loading && <div className="px-2 py-2 text-[11px] text-slate-500">Chưa có session nào.</div>}
                      {sessions.map((s) => {
                        const sActive = ctx?.agent === s.agent && ctx?.id === s.id;
                        const meta = AGENT_META[s.agent];
                        return (
                          <button
                            key={`${s.agent}:${s.id}`}
                            onClick={() => openSession(s)}
                            className={`w-full cursor-pointer rounded-md px-2 py-2.5 text-left md:py-1.5 ${sActive ? "bg-[#1d2330]" : "hover:bg-[#171b24]"}`}
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

          {/* pinned at the bottom */}
          <div className="shrink-0 border-t border-[#1d222b] bg-[#101217] p-2.5">
            <button
              onClick={() => setView(view === "settings" ? "terminal" : "settings")}
              className={`flex w-full cursor-pointer items-center gap-2 rounded-lg px-2.5 py-1.5 text-xs ${
                view === "settings" ? "bg-[#1d2330] text-slate-100" : "text-slate-400 hover:bg-[#181c26] hover:text-slate-200"
              }`}
            >
              <Settings className="h-3.5 w-3.5 text-violet-400" />
              Cài đặt agent
            </button>
          </div>
        </aside>
      ) : isMobile ? null : (
        <div className="flex w-11 shrink-0 flex-col items-center justify-between border-r border-[#1d222b] bg-[#12151c] py-3">
          <button onClick={() => setSidebarCollapsed(false)} className="cursor-pointer rounded-lg p-2 text-slate-400 hover:bg-[#1a1e28] hover:text-slate-200" title="Mở rộng">
            <PanelLeftOpen className="h-4 w-4" />
          </button>
          <button onClick={() => setView(view === "settings" ? "terminal" : "settings")} className="cursor-pointer rounded-lg p-2 text-slate-400 hover:bg-[#1a1e28] hover:text-slate-200" title="Cài đặt agent">
            <Settings className="h-4 w-4" />
          </button>
        </div>
      )}

      {/* ---------- Main ---------- */}
      <div className="flex h-full min-w-0 flex-1 flex-col">
        <header className="flex h-12 shrink-0 items-center justify-between gap-2 border-b border-[#1d222b] bg-[#101319] px-3 pt-[env(safe-area-inset-top)] sm:gap-4 sm:px-5">
          <div className="flex min-w-0 items-center gap-2.5">
            {isMobile && (
              <button onClick={() => setSidebarCollapsed(false)} className="-ml-1 shrink-0 cursor-pointer rounded-md p-1.5 text-slate-300 hover:bg-[#1a1e28]" title="Menu">
                <Menu className="h-5 w-5" />
              </button>
            )}
            <FolderOpen className="hidden h-4 w-4 shrink-0 text-amber-400 sm:block" />
            <div className="min-w-0 leading-tight">
              <div className="truncate text-[13px] font-semibold text-slate-100">
                {view === "settings" ? "Cài đặt agent" : activeTab?.label || workspace.split("/").filter(Boolean).pop() || "/"}
              </div>
              <div className="hidden truncate font-mono text-[10.5px] text-slate-500 sm:block" title={activeTab?.workDir || workspace}>
                {activeTab?.workDir || workspace}
              </div>
            </div>
          </div>

          <div className="flex shrink-0 items-center gap-2">
            {view === "terminal" && activeTab?.agent && (
              <>
                {/* switch the running agent: this is the handoff */}
                <div className="relative">
                  <button
                    onClick={() => setSwitchOpen((v) => !v)}
                    disabled={!ctx || busy !== null}
                    title={ctx ? "Chuyển phiên này sang agent khác (bàn giao context)" : "Chưa có session để bàn giao: gửi tin nhắn đầu tiên rồi thử lại"}
                    className="flex cursor-pointer items-center gap-1.5 rounded-md border border-indigo-500/50 bg-indigo-600 px-2.5 py-1 text-[12px] font-medium text-white hover:bg-indigo-500 disabled:cursor-not-allowed disabled:opacity-40"
                  >
                    {busy && busy !== "" && !busy.startsWith("install") ? <RefreshCw className="h-3.5 w-3.5 animate-spin" /> : <ArrowRight className="h-3.5 w-3.5" />}
                    <span className="hidden sm:inline">Chuyển agent</span>
                    <ChevronDown className="h-3 w-3" />
                  </button>
                  {switchOpen && (
                    <div className="absolute right-0 z-20 mt-1 w-56 max-w-[calc(100vw-1.5rem)] rounded-lg border border-[#2c3447] bg-[#161b26] p-1 shadow-xl">
                      {switchTargets.length === 0 && <div className="px-3 py-2 text-[11.5px] text-slate-500">Chưa có agent nào khác được cài.</div>}
                      {switchTargets.map((e) => (
                        <button key={e.id} onClick={() => switchAgent(e.id)} className="flex w-full cursor-pointer items-center gap-2 rounded-md px-3 py-2 text-left text-[12px] text-slate-200 hover:bg-[#222a3a]">
                          <span className={`h-1.5 w-1.5 rounded-full ${AGENT_META[e.id]?.dot}`} />
                          {e.name}
                        </button>
                      ))}
                    </div>
                  )}
                </div>
                <button
                  onClick={() => setContextOpen((v) => !v)}
                  className={`flex cursor-pointer items-center gap-1.5 rounded-md border px-2.5 py-1 text-[12px] ${
                    contextOpen ? "border-indigo-500 bg-[#1d2330] text-slate-100" : "border-[#2c3447] bg-[#1b202c] text-slate-300 hover:bg-[#222838]"
                  }`}
                  title="Hội thoại của session và file đang thay đổi"
                >
                  <MessageSquare className="h-3.5 w-3.5" />
                  <span className="hidden sm:inline">Context</span>
                  {modifiedFiles.length > 0 && <span className="rounded bg-amber-500/20 px-1 text-[10px] text-amber-300">{modifiedFiles.length}</span>}
                </button>
                <button onClick={() => openGui(activeTab.agent!)} className="hidden cursor-pointer sm:block rounded-md border border-[#2c3447] bg-[#1b202c] px-2.5 py-1 text-[12px] text-slate-300 hover:bg-[#222838]" title={`Mở GUI của ${AGENT_META[activeTab.agent]?.label}`}>
                  GUI
                </button>
              </>
            )}
            <button
              onClick={() => {
                loadWorkspace(workspace);
                loadEngines();
              }}
              className="flex cursor-pointer items-center gap-1.5 rounded-md border border-[#2c3447] bg-[#1b202c] px-2.5 py-1 text-[12px] text-slate-300 hover:bg-[#222838]"
              title="Tải lại danh sách session"
            >
              <RefreshCw className={`h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
            </button>
          </div>
        </header>

        {/* ---------- Settings (agent config) ---------- */}
        {view === "settings" && (
          <div className="min-h-0 flex-1 overflow-y-auto p-4 sm:p-6">
            <div className="mx-auto max-w-4xl">
              <div className="mb-4 flex items-center justify-between">
                <h2 className="text-[15px] font-semibold text-slate-100">Agents</h2>
                <div className="flex gap-2">
                  <button onClick={() => setAccountsEngine("")} className="flex cursor-pointer items-center gap-1.5 rounded-md border border-[#2c3447] bg-[#1b202c] px-2.5 py-1 text-[12px] text-slate-300 hover:bg-[#222838]">
                    <UserCheck className="h-3.5 w-3.5" />
                    Tài khoản
                  </button>
                  <button onClick={() => setView("terminal")} className="cursor-pointer rounded-md border border-[#2c3447] bg-[#1b202c] px-2.5 py-1 text-[12px] text-slate-300 hover:bg-[#222838]">
                    Quay lại terminal
                  </button>
                </div>
              </div>
              <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
                {engines.map((eng) => {
                  const mine = sessions.filter((x) => x.agent === eng.id);
                  return (
                    <div key={eng.id} className="flex items-center gap-3 rounded-lg border border-[#232a39] bg-[#121620] px-3.5 py-3">
                      <span className={`h-2 w-2 shrink-0 rounded-full ${AGENT_META[eng.id]?.dot}`} />
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center gap-1.5 text-[12.5px] font-medium text-slate-200">
                          {eng.name}
                          {eng.installed ? eng.has_auth ? <CheckCircle2 className="h-3.5 w-3.5 text-emerald-400" /> : <AlertCircle className="h-3.5 w-3.5 text-amber-400" /> : null}
                        </div>
                        <div className="truncate text-[11px] text-slate-500" title={eng.auth_status}>
                          {eng.installed ? `${eng.auth_status || "Đã cài"} · ${mine.length} session` : "Chưa cài đặt"}
                        </div>
                      </div>
                      {eng.installed ? (
                        <div className="flex gap-1.5">
                          <button onClick={() => openGui(eng.id)} className="cursor-pointer rounded-md border border-[#2b3447] bg-[#1b202c] px-2 py-1 text-[11px] text-slate-300 hover:bg-[#242b3b]">
                            GUI
                          </button>
                          <button onClick={() => setAccountsEngine(eng.id)} className="cursor-pointer rounded-md border border-[#2b3447] bg-[#1b202c] p-1.5 text-slate-400 hover:bg-[#242b3b] hover:text-slate-200" title="Đăng nhập / đổi tài khoản">
                            <UserCheck className="h-3.5 w-3.5" />
                          </button>
                        </div>
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
                  );
                })}
              </div>
            </div>
          </div>
        )}

        {/* ---------- Terminal workspace (default) ---------- */}
        <div className={`min-h-0 flex-1 ${view === "terminal" ? "flex" : "hidden"}`}>
          <div className="flex min-w-0 flex-1 flex-col bg-[#0c0e14]">
            {tabs.length > 0 && (
              <div className="flex h-9 shrink-0 items-center gap-0.5 overflow-x-auto border-b border-[#1d222b] bg-[#101319] px-1.5">
                {tabs.map((t) => (
                  <div
                    key={t.key}
                    onClick={() => setActiveKey(t.key)}
                    className={`flex shrink-0 cursor-pointer items-center gap-1.5 rounded-md px-2.5 py-1 text-[12px] ${
                      t.key === activeKey ? "bg-[#1d2330] text-slate-100" : "text-slate-400 hover:bg-[#171b24] hover:text-slate-200"
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
                <button onClick={() => newShell(activeTab?.workDir || workspace)} className="shrink-0 cursor-pointer rounded-md px-2 py-1 text-slate-400 hover:bg-[#171b24] hover:text-slate-200" title="Shell mới">
                  <Plus className="h-3.5 w-3.5" />
                </button>
              </div>
            )}

            <div className="relative flex-1 overflow-hidden">
              {tabs.map((t) => (
                <div key={t.key} className={`absolute inset-0 ${t.key === activeKey ? "" : "invisible"}`}>
                  <TerminalPanel workDir={t.workDir} launch={t.launch} sessionId={t.key} title={t.label} headless visible={view === "terminal" && t.key === activeKey} onClose={() => closeTab(t.key)} />
                </div>
              ))}

              {tabs.length === 0 && (
                <div className="flex h-full flex-col items-center justify-center gap-5 p-8 text-center">
                  <TerminalIcon className="h-8 w-8 text-slate-600" />
                  <div>
                    <div className="text-[14px] font-medium text-slate-200">Chưa có terminal nào đang chạy</div>
                    <div className="mt-1 text-[12px] text-slate-500">Chọn một session ở bên trái để tiếp tục, hoặc bắt đầu mới trong {workspace.split("/").filter(Boolean).pop() || "/"}.</div>
                  </div>
                  <div className="flex flex-wrap justify-center gap-2">
                    {installed.map((e) => (
                      <button key={e.id} onClick={() => newAgentSession(e.id, workspace)} className="flex cursor-pointer items-center gap-2 rounded-lg border border-[#2c3447] bg-[#1b202c] px-3.5 py-2 text-[12px] text-slate-200 hover:bg-[#232a3a]">
                        <span className={`h-1.5 w-1.5 rounded-full ${AGENT_META[e.id]?.dot}`} />
                        {e.name} mới
                      </button>
                    ))}
                    <button onClick={() => newShell(workspace)} className="flex cursor-pointer items-center gap-2 rounded-lg border border-[#2c3447] bg-[#1b202c] px-3.5 py-2 text-[12px] text-slate-200 hover:bg-[#232a3a]">
                      <TerminalIcon className="h-3.5 w-3.5 text-sky-400" />
                      Shell
                    </button>
                  </div>
                  {installed.length === 0 && <div className="text-[11.5px] text-amber-300/80">Chưa cài agent nào: mở “Cài đặt agent” ở góc dưới bên trái.</div>}
                </div>
              )}
            </div>
          </div>

          {/* ---------- Context drawer: what a switch would hand over ---------- */}
          {contextOpen && activeTab?.agent && (
            <aside className="fixed inset-0 z-30 flex w-full flex-col border-l border-[#1d222b] bg-[#101319] pt-[env(safe-area-inset-top)] md:static md:z-auto md:w-80 md:shrink-0 md:pt-0 xl:w-[360px]">
              <div className="flex items-center justify-between border-b border-[#1d222b] px-4 py-2.5">
                <span className="text-[11px] font-semibold uppercase tracking-wider text-slate-500">Context</span>
                <button onClick={() => setContextOpen(false)} className="cursor-pointer p-0.5 text-slate-500 hover:text-slate-300">
                  <X className="h-3.5 w-3.5" />
                </button>
              </div>
              <div className="min-h-0 flex-1 space-y-4 overflow-y-auto p-4">
                {!ctx ? (
                  <div className="text-[12px] leading-relaxed text-slate-500">Phiên này chưa ghi session nào. Gửi tin nhắn đầu tiên cho agent, vài giây sau context sẽ hiện ở đây.</div>
                ) : (
                  <>
                    <div>
                      <div className="flex items-center gap-2">
                        <span className={`rounded border px-1.5 py-0.5 text-[10px] font-medium ${AGENT_META[ctx.agent]?.badge}`}>{AGENT_META[ctx.agent]?.label}</span>
                        <button onClick={() => copyText(ctx.id)} className="ml-auto flex cursor-pointer items-center gap-1 font-mono text-[10px] text-slate-500 hover:text-slate-300" title="Copy session id">
                          {copied ? <Check className="h-3 w-3 text-emerald-400" /> : <Copy className="h-3 w-3" />}
                          {ctx.id.slice(0, 8)}
                        </button>
                      </div>
                      <div className="mt-1.5 text-[13px] font-semibold leading-snug text-slate-100">{ctx.title || "Untitled"}</div>
                      <div className="mt-0.5 text-[11px] text-slate-500">
                        {ctx.turn_count} tin nhắn{relTime(ctx.updated_at) && ` · cập nhật ${relTime(ctx.updated_at)} trước`}
                      </div>
                    </div>

                    <div>
                      <div className="mb-2 text-[10.5px] font-semibold uppercase tracking-wider text-indigo-300">{lastTurns.length} tin gần nhất · sẽ được bàn giao</div>
                      {!detail && <div className="text-[12px] text-slate-500">Đang đọc transcript…</div>}
                      <div className="space-y-2.5">
                        {lastTurns.map((t, i) => (
                          <div key={i} className={`rounded-lg border px-3 py-2 ${t.role === "user" ? "border-[#2a3347] bg-[#171c28]" : "border-[#232a39] bg-[#121620]"}`}>
                            <div className="mb-0.5 text-[10px] font-medium uppercase tracking-wide text-slate-500">{t.role === "user" ? "Bạn" : AGENT_META[ctx.agent]?.label}</div>
                            <div className="line-clamp-6 whitespace-pre-wrap break-words text-[12px] leading-relaxed text-slate-300">{t.content}</div>
                          </div>
                        ))}
                      </div>
                    </div>
                  </>
                )}

                <div>
                  <div className="mb-2 flex items-center gap-1.5 text-[10.5px] font-semibold uppercase tracking-wider text-slate-500">
                    <GitBranch className="h-3.5 w-3.5 text-emerald-400" />
                    File thay đổi
                    <span className="ml-auto font-normal normal-case tracking-normal">{modifiedFiles.length}</span>
                  </div>
                  {modifiedFiles.length === 0 ? (
                    <div className="text-[11.5px] text-slate-500">Working tree sạch.</div>
                  ) : (
                    <div className="max-h-56 space-y-1 overflow-y-auto">
                      {modifiedFiles.map((f) => (
                        <div key={f} className="truncate rounded bg-[#1a2030] px-2 py-1 font-mono text-[10.5px] text-slate-300" title={f}>
                          {f}
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              </div>
            </aside>
          )}
        </div>
      </div>

      {accountsEngine !== null && <AccountsDialog addEngine={accountsEngine || undefined} onChanged={loadEngines} onClose={() => setAccountsEngine(null)} />}
    </div>
  );
}
export { App };
