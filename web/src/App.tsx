import { useEffect, useState } from "react";
import { 
  Cpu, 
  Terminal as TerminalIcon, 
  GitBranch, 
  RefreshCw, 
  FolderCheck, 
  ArrowRightLeft, 
  Play, 
  CheckCircle2, 
  AlertCircle, 
  Sparkles,
  UserCheck,
  PanelLeftClose,
  PanelLeftOpen,
  Plus,
  Folder,
  Layers,
  Settings,
  Zap,
  Activity,
  ChevronRight
} from "lucide-react";
import { TerminalPanel } from "./components/TerminalPanel";
import { AccountsDialog } from "./components/AccountsDialog";

interface Workspace {
  id: string;
  path: string;
  name: string;
  active_bridge_session_id?: string;
}

interface BridgeSession {
  id: string;
  workspace_id: string;
  title: string;
  status: string;
  current_agent: string;
  last_handoff_summary?: string;
  created_at: string;
  updated_at: string;
}

interface EngineStatus {
  id: string;
  name: string;
  binary: string;
  installed: boolean;
  path?: string;
  auth_status?: string;
  has_auth?: boolean;
  models: string[];
}

export default function App() {
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
  const [currentWorkspace, setCurrentWorkspace] = useState<string>("/home/chungnh/AI Workspace");
  const [currentWorkspaceId, setCurrentWorkspaceId] = useState<string>("");
  
  const [sessions, setSessions] = useState<BridgeSession[]>([]);
  const [activeSessionId, setActiveSessionId] = useState<string>("");
  const [activeSession, setActiveSession] = useState<BridgeSession | null>(null);

  const [modifiedFiles, setModifiedFiles] = useState<string[]>([]);
  const [isRefreshing, setIsRefreshing] = useState<boolean>(false);
  const [showTerminal, setShowTerminal] = useState<boolean>(false);
  const [sidebarCollapsed, setSidebarCollapsed] = useState<boolean>(false);
  const [showAccountsDialog, setShowAccountsDialog] = useState<boolean>(false);
  const [accountsAgent, setAccountsAgent] = useState<string>("agy");
  const [taskGoal, setTaskGoal] = useState<string>("");

  const [engines, setEngines] = useState<EngineStatus[]>([
    {
      id: "agy",
      name: "Google Antigravity",
      binary: "antigravity",
      installed: true,
      has_auth: true,
      auth_status: "OAuth active (~/.gemini)",
      models: ["gemini-2.5-pro", "gemini-2.5-flash"]
    },
    {
      id: "claude",
      name: "Claude Code",
      binary: "claude",
      installed: false,
      has_auth: false,
      auth_status: "Not authenticated",
      models: ["claude-3-7-sonnet", "claude-3-5-haiku"]
    },
    {
      id: "codex",
      name: "OpenAI Codex",
      binary: "codex",
      installed: false,
      has_auth: false,
      auth_status: "API Key / OAuth required",
      models: ["o3-mini", "gpt-4o"]
    }
  ]);

  // Load Workspaces on mount
  const loadWorkspaces = async () => {
    try {
      const wsRes = await fetch("/api/bridge/workspaces");
      if (wsRes.ok) {
        const wsData = await wsRes.json();
        setWorkspaces(wsData || []);
        if (wsData && wsData.length > 0) {
          const matched = wsData.find((w: Workspace) => w.path === currentWorkspace) || wsData[0];
          setCurrentWorkspace(matched.path);
          setCurrentWorkspaceId(matched.id);
        }
      }
    } catch (e) {
      console.error("Error loading workspaces", e);
    }
  };

  // Load Sessions for current workspace
  const loadSessions = async (wsId: string) => {
    if (!wsId) return;
    try {
      const sessRes = await fetch(`/api/bridge/sessions?workspace_id=${encodeURIComponent(wsId)}`);
      if (sessRes.ok) {
        const sessData = await sessRes.json();
        setSessions(sessData || []);
        if (sessData && sessData.length > 0) {
          // If no active session or current active is not in list, pick the first
          const found = sessData.find((s: BridgeSession) => s.id === activeSessionId) || sessData[0];
          setActiveSessionId(found.id);
          setActiveSession(found);
          setTaskGoal(found.title || found.last_handoff_summary || "");
        } else {
          setActiveSessionId("");
          setActiveSession(null);
          setTaskGoal("");
        }
      }
    } catch (e) {
      console.error("Error loading sessions", e);
    }
  };

  // Load Git State & Engine status
  const loadWorkspaceState = async () => {
    setIsRefreshing(true);
    try {
      // 1. Fetch git state
      const stateRes = await fetch(`/api/bridge/state?workspace=${encodeURIComponent(currentWorkspace)}`);
      if (stateRes.ok) {
        const stateData = await stateRes.json();
        setModifiedFiles(stateData.modified_files || []);
      }

      // 2. Check engine installations
      const updatedEngines = await Promise.all(
        engines.map(async (eng) => {
          try {
            const checkRes = await fetch("/api/agents/check", {
              method: "POST",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify({ binary: eng.binary })
            });
            if (checkRes.ok) {
              const res = await checkRes.json();
              return {
                ...eng,
                installed: res.found,
                path: res.path,
                has_auth: res.has_auth,
                auth_status: res.auth_status || eng.auth_status
              };
            }
          } catch (e) {
            console.error(e);
          }
          return eng;
        })
      );
      setEngines(updatedEngines);
    } catch (e) {
      console.error("Error loading state", e);
    } finally {
      setIsRefreshing(false);
    }
  };

  useEffect(() => {
    loadWorkspaces();
  }, []);

  useEffect(() => {
    if (currentWorkspaceId) {
      loadSessions(currentWorkspaceId);
    }
    loadWorkspaceState();
  }, [currentWorkspace, currentWorkspaceId]);

  const handleSelectWorkspace = (ws: Workspace) => {
    setCurrentWorkspace(ws.path);
    setCurrentWorkspaceId(ws.id);
  };

  const handleSelectSession = (s: BridgeSession) => {
    setActiveSessionId(s.id);
    setActiveSession(s);
    setTaskGoal(s.title || s.last_handoff_summary || "");
  };

  const handleNewSession = async () => {
    const title = prompt("Tên phiên làm việc mới (Task Session):", "New Task Session");
    if (!title || !currentWorkspaceId) return;

    try {
      const res = await fetch("/api/bridge/sessions", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          workspace_id: currentWorkspaceId,
          title: title.trim(),
          initial_agent: "agy"
        })
      });
      if (res.ok) {
        const newSess = await res.json();
        setSessions((prev) => [newSess, ...prev]);
        setActiveSessionId(newSess.id);
        setActiveSession(newSess);
        setTaskGoal(newSess.title);
      }
    } catch (e) {
      alert("Lỗi khi tạo phiên mới");
    }
  };

  const handleInstall = async (engineId: string) => {
    let cmd = "";
    if (engineId === "claude") {
      cmd = "npm install -g @anthropic-ai/claude-code";
    } else if (engineId === "codex") {
      cmd = "npm install -g @openai/codex";
    }
    if (!cmd) return;

    try {
      const res = await fetch("/api/agents/install", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ command: cmd, binary: engineId })
      });
      const data = await res.json();
      if (data.success) {
        alert(`✅ Cài đặt ${engineId} thành công!`);
        loadWorkspaceState();
      } else {
        alert(`❌ Cài đặt thất bại: ${data.error || data.output}`);
      }
    } catch (e) {
      alert("Lỗi khi kết nối tới daemon");
    }
  };

  const handleHandoff = async (toAgent: string) => {
    try {
      const res = await fetch("/api/bridge/handoff", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          workspace_path: currentWorkspace,
          session_id: activeSessionId || "default_session",
          from_agent: activeSession?.current_agent || "agy",
          to_agent: toAgent,
          task_goal: taskGoal,
          extra_context: "Resume via Agent Bridge Admin Dashboard",
          trigger_reason: "manual_dashboard_switch"
        })
      });
      const data = await res.json();
      if (data.success) {
        alert(`✅ Đã xuất Context Handoff sang .agent/handoff.md cho ${toAgent}!`);
        if (currentWorkspaceId) {
          loadSessions(currentWorkspaceId);
        }
        loadWorkspaceState();
      }
    } catch (e) {
      alert("Lỗi khi gửi lệnh handoff");
    }
  };

  return (
    <div className="flex h-screen w-screen overflow-hidden bg-[#0b0e14] text-slate-100 font-sans select-none">
      {/* 1. LEFT SIDEBAR: Quản lý theo Workspace Folder & Sessions */}
      {!sidebarCollapsed ? (
        <aside className="w-72 bg-[#12151c] border-r border-[#1d222b] flex flex-col h-full shrink-0 select-none text-slate-300">
          {/* Top Brand Header */}
          <div className="px-3.5 pt-3 pb-2 flex items-center justify-between border-b border-[#1c212a]">
            <div className="flex items-center gap-2">
              <Zap className="w-4 h-4 text-amber-400 fill-amber-400" />
              <div className="flex flex-col">
                <span className="font-bold text-xs text-slate-100 tracking-wide flex items-center gap-1.5">
                  AGENT BRIDGE
                  <span className="text-[9px] px-1 py-0.2 rounded bg-indigo-500/20 text-indigo-400 border border-indigo-500/30 font-normal">
                    DAEMON
                  </span>
                </span>
                <span className="text-[10px] text-slate-500">Universal Context Switcher</span>
              </div>
            </div>
            <button
              type="button"
              onClick={() => setSidebarCollapsed(true)}
              className="text-slate-500 hover:text-slate-300 transition-colors p-1 cursor-pointer"
              title="Thu nhỏ Sidebar"
            >
              <PanelLeftClose className="w-3.5 h-3.5" />
            </button>
          </div>

          {/* New Session Button */}
          <div className="px-3 py-2 border-b border-[#1c212a]">
            <button
              onClick={handleNewSession}
              className="w-full flex items-center justify-center gap-2 py-1.5 px-3 rounded-lg bg-[#1a1f29] hover:bg-[#222836] text-slate-200 text-xs font-medium transition-all border border-[#262c3a] cursor-pointer shadow-sm"
            >
              <Plus className="w-3.5 h-3.5 text-slate-400" />
              <span>New Task Session</span>
            </button>
          </div>

          {/* Workspaces & Sessions List */}
          <div className="flex-1 overflow-y-auto px-2 py-2 space-y-4">
            {/* Workspace Selection Section */}
            <div>
              <div className="px-2 py-1 text-[10px] uppercase font-semibold tracking-wider text-slate-500 flex items-center gap-1.5">
                <Folder className="w-3 h-3 text-indigo-400" />
                <span>Workspaces ({workspaces.length})</span>
              </div>
              <div className="mt-1 space-y-0.5">
                {workspaces.map((ws) => {
                  const isActive = ws.path === currentWorkspace;
                  return (
                    <div
                      key={ws.id}
                      onClick={() => handleSelectWorkspace(ws)}
                      className={`group flex items-center justify-between px-2.5 py-1.5 rounded-lg text-xs cursor-pointer transition ${
                        isActive
                          ? "bg-indigo-600/15 text-indigo-300 font-medium border border-indigo-500/30"
                          : "text-slate-400 hover:bg-[#181d26] hover:text-slate-200 border border-transparent"
                      }`}
                    >
                      <div className="flex items-center gap-2 min-w-0">
                        <FolderCheck className={`w-3.5 h-3.5 shrink-0 ${isActive ? "text-indigo-400" : "text-slate-500"}`} />
                        <span className="truncate">{ws.name}</span>
                      </div>
                      {isActive && <ChevronRight className="w-3 h-3 text-indigo-400 shrink-0" />}
                    </div>
                  );
                })}
              </div>
            </div>

            {/* Sessions in current workspace */}
            <div>
              <div className="px-2 py-1 text-[10px] uppercase font-semibold tracking-wider text-slate-500 flex items-center justify-between">
                <span className="flex items-center gap-1.5">
                  <Layers className="w-3 h-3 text-amber-400" />
                  Sessions ({sessions.length})
                </span>
                <span className="text-[9px] text-slate-600 font-mono">in active folder</span>
              </div>
              <div className="mt-1 space-y-1">
                {sessions.length === 0 ? (
                  <div className="px-2 py-3 text-center text-[11px] text-slate-500">
                    Chưa có session nào. Bấm New để tạo.
                  </div>
                ) : (
                  sessions.map((s) => {
                    const isSessActive = s.id === activeSessionId;
                    return (
                      <div
                        key={s.id}
                        onClick={() => handleSelectSession(s)}
                        className={`px-2.5 py-2 rounded-lg text-xs cursor-pointer transition border ${
                          isSessActive
                            ? "bg-[#181d28] text-slate-100 border-[#2e374a] shadow-sm"
                            : "text-slate-400 hover:bg-[#151922] hover:text-slate-300 border-transparent"
                        }`}
                      >
                        <div className="flex items-center justify-between mb-0.5">
                          <span className="font-medium truncate max-w-[170px] text-slate-200">
                            {s.title || "Untitled Session"}
                          </span>
                          <span className={`text-[9.5px] px-1.5 py-0.2 rounded font-mono ${
                            s.current_agent === "claude"
                              ? "bg-purple-500/20 text-purple-300"
                              : s.current_agent === "agy"
                              ? "bg-blue-500/20 text-blue-300"
                              : "bg-emerald-500/20 text-emerald-300"
                          }`}>
                            {s.current_agent.toUpperCase()}
                          </span>
                        </div>
                        <div className="text-[10px] text-slate-500 flex items-center justify-between">
                          <span className="truncate max-w-[180px]">{s.last_handoff_summary || "Ready for context switch"}</span>
                        </div>
                      </div>
                    );
                  })
                )}
              </div>
            </div>
          </div>

          {/* Bottom Footer: Machine & Settings */}
          <div className="p-2.5 border-t border-[#1d222b] space-y-1 bg-[#101217]">
            <div className="flex items-center justify-between px-2.5 py-1.5 rounded-lg bg-[#161a22] border border-[#202532] text-xs font-medium cursor-default">
              <div className="flex items-center gap-2">
                <span className="w-1.5 h-1.5 rounded-full bg-emerald-400"></span>
                <span className="text-slate-300">Daemon :8088</span>
              </div>
              <span className="text-[10px] text-slate-500 font-mono">SQLite WAL</span>
            </div>

            <div
              onClick={() => setShowAccountsDialog(true)}
              className="flex items-center gap-2 px-2.5 py-1.5 rounded-lg text-xs text-slate-400 hover:text-slate-200 hover:bg-[#181c26] cursor-pointer transition"
            >
              <Settings className="w-3.5 h-3.5 text-slate-500" />
              <span>Multi-Account Manager</span>
            </div>
          </div>
        </aside>
      ) : (
        /* Collapsed Sidebar trigger */
        <div className="w-12 bg-[#12151c] border-r border-[#1d222b] flex flex-col items-center py-3 shrink-0">
          <button
            onClick={() => setSidebarCollapsed(false)}
            className="text-slate-400 hover:text-slate-200 p-2 rounded-lg hover:bg-[#1a1e28] transition cursor-pointer"
            title="Mở rộng Sidebar"
          >
            <PanelLeftOpen className="w-4 h-4" />
          </button>
        </div>
      )}

      {/* 2. MAIN ADMIN CONTENT */}
      <div className="flex-1 flex flex-col h-full overflow-hidden">
        {/* Top Header Bar */}
        <header className="flex h-14 shrink-0 items-center justify-between border-b border-[#1d222b] bg-[#101319] px-6">
          <div className="flex items-center gap-3">
            <div className="flex items-center gap-2">
              <span className="text-[11px] text-slate-500 uppercase font-semibold tracking-wider">Active Workspace:</span>
              <span className="text-[12px] font-mono bg-[#161a23] text-indigo-300 px-2.5 py-1 rounded border border-[#232a39]">
                {currentWorkspace}
              </span>
            </div>

            {activeSession && (
              <div className="flex items-center gap-1.5 text-[11.5px] bg-[#181d28] text-slate-300 px-2.5 py-1 rounded border border-[#283246]">
                <Activity className="w-3 h-3 text-emerald-400" />
                <span className="font-semibold text-slate-200">{activeSession.title}</span>
                <span className="text-[10px] text-slate-500">({activeSession.id})</span>
              </div>
            )}
          </div>

          <div className="flex items-center gap-3">
            <button
              onClick={loadWorkspaceState}
              disabled={isRefreshing}
              className="flex items-center gap-1.5 bg-[#1b202c] hover:bg-[#222838] px-3 py-1.5 rounded-lg border border-[#2c3447] text-[12px] font-medium transition cursor-pointer"
            >
              <RefreshCw className={`h-3.5 w-3.5 ${isRefreshing ? "animate-spin text-indigo-400" : "text-slate-300"}`} />
              Sync
            </button>

            <button
              onClick={() => setShowTerminal(!showTerminal)}
              className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg border text-[12px] font-medium transition cursor-pointer ${
                showTerminal
                  ? "bg-indigo-600 text-white border-indigo-500 shadow-sm shadow-indigo-500/20"
                  : "bg-[#1b202c] hover:bg-[#222838] text-slate-300 border-[#2c3447]"
              }`}
            >
              <TerminalIcon className="h-3.5 w-3.5" />
              Terminal {showTerminal ? "On" : "Off"}
            </button>
          </div>
        </header>

        {/* Workspace Body */}
        <main className="flex-1 flex flex-col lg:flex-row overflow-hidden">
          {/* Main Controls Area */}
          <div className="flex-1 overflow-y-auto p-6 space-y-6">
            {/* Context Handoff Snapshot Card */}
            <section className="rounded-xl border border-[#232a39] bg-[#121620] p-5 shadow-sm">
              <div className="flex items-center justify-between mb-3">
                <div className="flex items-center gap-2">
                  <Sparkles className="h-4 w-4 text-amber-400" />
                  <h2 className="text-[13px] font-semibold text-slate-200 uppercase tracking-wider">
                    Active Context & Handoff Target
                  </h2>
                </div>
                <span className="text-[11px] text-slate-400 bg-[#1b202c] px-2 py-0.5 rounded border border-[#293245]">
                  Saved to .agent/handoff.md
                </span>
              </div>

              <div className="space-y-3">
                <div>
                  <label className="text-[11px] text-slate-400 font-medium block mb-1">
                    Mục tiêu công việc hiện tại (Task Goal)
                  </label>
                  <input
                    type="text"
                    value={taskGoal}
                    onChange={(e) => setTaskGoal(e.target.value)}
                    className="w-full bg-[#181d28] border border-[#2b3447] rounded-lg px-3 py-2 text-[12px] text-slate-100 focus:outline-none focus:border-indigo-500"
                    placeholder="Nhập task đang làm..."
                  />
                </div>

                {modifiedFiles.length > 0 && (
                  <div className="rounded-lg bg-[#181d28]/70 border border-[#242b3b] p-3 text-[11.5px] space-y-1.5">
                    <div className="flex items-center gap-2 text-slate-300 font-medium">
                      <GitBranch className="h-3.5 w-3.5 text-emerald-400" />
                      <span>File thay đổi gần nhất ({modifiedFiles.length} files):</span>
                    </div>
                    <div className="flex flex-wrap gap-1.5">
                      {modifiedFiles.slice(0, 8).map((f) => (
                        <span key={f} className="font-mono text-[10.5px] bg-[#202636] text-slate-300 px-2 py-0.5 rounded border border-[#2c354b]">
                          {f}
                        </span>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            </section>

            {/* AI Engines Management */}
            <section className="space-y-3">
              <div className="flex items-center justify-between">
                <h2 className="text-[13px] font-semibold text-slate-200 uppercase tracking-wider flex items-center gap-2">
                  <Cpu className="h-4 w-4 text-indigo-400" />
                  AI Engines Management
                </h2>
                <span className="text-[11px] text-slate-400">Click Switch to transfer context</span>
              </div>

              <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                {engines.map((eng) => (
                  <div
                    key={eng.id}
                    className="rounded-xl border border-[#232a39] bg-[#121620] p-4 flex flex-col justify-between hover:border-[#354057] transition shadow-sm space-y-4"
                  >
                    <div className="space-y-2">
                      <div className="flex items-center justify-between">
                        <h3 className="text-[13px] font-bold text-slate-100">{eng.name}</h3>
                        {eng.installed ? (
                          <span className="flex items-center gap-1 text-[10.5px] font-medium text-emerald-400 bg-emerald-500/10 px-2 py-0.5 rounded border border-emerald-500/20">
                            <CheckCircle2 className="h-3 w-3" /> Ready
                          </span>
                        ) : (
                          <span className="flex items-center gap-1 text-[10.5px] font-medium text-amber-400 bg-amber-500/10 px-2 py-0.5 rounded border border-amber-500/20">
                            <AlertCircle className="h-3 w-3" /> Not Installed
                          </span>
                        )}
                      </div>

                      <div className="text-[11px] text-slate-400 font-mono truncate">
                        cmd: <span className="text-slate-200">{eng.binary}</span>
                      </div>

                      <div className="text-[11px] text-slate-400 space-y-0.5">
                        <div className="text-slate-500">Auth Status:</div>
                        <div className="text-slate-300 font-medium truncate">{eng.auth_status}</div>
                      </div>
                    </div>

                    <div className="space-y-2 pt-2 border-t border-[#1d2331]">
                      {eng.installed ? (
                        <div className="flex items-center gap-2">
                          <button
                            onClick={() => handleHandoff(eng.id)}
                            className="flex-1 flex items-center justify-center gap-1.5 bg-indigo-600 hover:bg-indigo-500 text-white font-medium text-[11.5px] py-2 rounded-lg transition shadow-sm cursor-pointer"
                          >
                            <ArrowRightLeft className="h-3.5 w-3.5" /> Switch Context
                          </button>
                          <button
                            onClick={() => {
                              setAccountsAgent(eng.id);
                              setShowAccountsDialog(true);
                            }}
                            className="bg-[#1b202c] hover:bg-[#242b3b] text-slate-300 px-2.5 py-2 rounded-lg text-[11.5px] border border-[#2b3447] transition flex items-center gap-1 cursor-pointer"
                            title="Quản lý tài khoản"
                          >
                            <UserCheck className="h-3.5 w-3.5" />
                          </button>
                        </div>
                      ) : (
                        <button
                          onClick={() => handleInstall(eng.id)}
                          className="w-full flex items-center justify-center gap-1.5 bg-emerald-600 hover:bg-emerald-500 text-white font-medium text-[11.5px] py-2 rounded-lg transition cursor-pointer"
                        >
                          <Play className="h-3.5 w-3.5" /> 1-Click Auto Install
                        </button>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            </section>
          </div>

          {/* Right / Bottom Terminal Drawer */}
          {showTerminal && (
            <div className="h-[360px] lg:h-full lg:w-[480px] border-t lg:border-t-0 lg:border-l border-[#1d222b] bg-[#0c0e14] flex flex-col shrink-0">
              <div className="flex h-9 shrink-0 items-center justify-between border-b border-[#1d222b] bg-[#101319] px-4">
                <span className="text-[12px] font-medium text-slate-300 flex items-center gap-1.5">
                  <TerminalIcon className="h-3.5 w-3.5 text-sky-400" /> Embedded PTY Terminal
                </span>
                <button
                  onClick={() => setShowTerminal(false)}
                  className="text-slate-400 hover:text-slate-200 text-[11px] cursor-pointer"
                >
                  Close
                </button>
              </div>
              <div className="flex-1 overflow-hidden">
                <TerminalPanel workDir={currentWorkspace} visible={showTerminal} onClose={() => setShowTerminal(false)} />
              </div>
            </div>
          )}
        </main>
      </div>

      {/* Account Switcher Dialog */}
      {showAccountsDialog && (
        <AccountsDialog
          addEngine={accountsAgent}
          onChanged={loadWorkspaceState}
          onClose={() => setShowAccountsDialog(false)}
        />
      )}
    </div>
  );
}
export { App };
