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
  UserCheck
} from "lucide-react";
import { TerminalPanel } from "./components/TerminalPanel";
import { AccountsDialog } from "./components/AccountsDialog";

interface Workspace {
  id: string;
  path: string;
  name: string;
  active_bridge_session_id?: string;
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
  const [modifiedFiles, setModifiedFiles] = useState<string[]>([]);
  const [isRefreshing, setIsRefreshing] = useState<boolean>(false);
  const [showTerminal, setShowTerminal] = useState<boolean>(false);
  const [showAccountsDialog, setShowAccountsDialog] = useState<boolean>(false);
  const [accountsAgent, setAccountsAgent] = useState<string>("agy");
  const [taskGoal, setTaskGoal] = useState<string>("Tối ưu hoá backend và quản lý auth cho CLI");

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

  const loadData = async () => {
    setIsRefreshing(true);
    try {
      // 1. Fetch workspaces
      const wsRes = await fetch("/api/bridge/workspaces");
      if (wsRes.ok) {
        const wsData = await wsRes.json();
        setWorkspaces(wsData || []);
        if (wsData && wsData.length > 0 && !currentWorkspace) {
          setCurrentWorkspace(wsData[0].path);
        }
      }

      // 2. Fetch git state
      const stateRes = await fetch(`/api/bridge/state?workspace=${encodeURIComponent(currentWorkspace)}`);
      if (stateRes.ok) {
        const stateData = await stateRes.json();
        setModifiedFiles(stateData.modified_files || []);
      }

      // 3. Check engine installations
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
      console.error("Error loading bridge data", e);
    } finally {
      setIsRefreshing(false);
    }
  };

  useEffect(() => {
    loadData();
  }, [currentWorkspace]);

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
        loadData();
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
          session_id: "universal_bridge_session",
          from_agent: "agy",
          to_agent: toAgent,
          task_goal: taskGoal,
          extra_context: "Resume via Agent Bridge Admin Dashboard",
          trigger_reason: "manual_dashboard_switch"
        })
      });
      const data = await res.json();
      if (data.success) {
        alert(`✅ Đã xuất Context Handoff sang .agent/handoff.md cho ${toAgent}!`);
        loadData();
      }
    } catch (e) {
      alert("Lỗi khi gửi lệnh handoff");
    }
  };

  return (
    <div className="flex h-screen w-screen flex-col bg-[#0b0e14] text-slate-100 font-sans">
      {/* Header Bar */}
      <header className="flex h-14 shrink-0 items-center justify-between border-b border-[#1d222b] bg-[#101319] px-6">
        <div className="flex items-center gap-3">
          <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-indigo-600/20 text-indigo-400 border border-indigo-500/30">
            <ArrowRightLeft className="h-4 w-4" />
          </div>
          <div>
            <h1 className="text-[14px] font-bold tracking-wide text-slate-100 flex items-center gap-2">
              AGENT BRIDGE
              <span className="text-[10px] font-medium px-1.5 py-0.5 rounded bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
                v2.0 DAEMON
              </span>
            </h1>
            <p className="text-[11px] text-slate-400">Universal Context Switcher & CLI Manager</p>
          </div>
        </div>

        <div className="flex items-center gap-3">
          {/* Workspace Path Selector */}
          <div className="flex items-center gap-2 bg-[#161a23] px-3 py-1.5 rounded-lg border border-[#232a39] text-[12px]">
            <FolderCheck className="h-3.5 w-3.5 text-indigo-400 shrink-0" />
            <select
              value={currentWorkspace}
              onChange={(e) => setCurrentWorkspace(e.target.value)}
              className="bg-transparent text-slate-300 font-mono text-[11.5px] focus:outline-none cursor-pointer"
            >
              {workspaces.map((ws) => (
                <option key={ws.id} value={ws.path} className="bg-[#161a23] text-slate-200">
                  {ws.path}
                </option>
              ))}
              {!workspaces.some((w) => w.path === currentWorkspace) && (
                <option value={currentWorkspace} className="bg-[#161a23] text-slate-200">
                  {currentWorkspace}
                </option>
              )}
            </select>
          </div>

          <button
            onClick={loadData}
            disabled={isRefreshing}
            className="flex items-center gap-1.5 bg-[#1b202c] hover:bg-[#222838] px-3 py-1.5 rounded-lg border border-[#2c3447] text-[12px] font-medium transition"
          >
            <RefreshCw className={`h-3.5 w-3.5 ${isRefreshing ? "animate-spin text-indigo-400" : "text-slate-300"}`} />
            Sync
          </button>

          <button
            onClick={() => setShowTerminal(!showTerminal)}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg border text-[12px] font-medium transition ${
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

      {/* Main Body */}
      <main className="flex-1 flex flex-col lg:flex-row overflow-hidden">
        {/* Left Column: Engines & Context Manager */}
        <div className="flex-1 overflow-y-auto p-6 space-y-6">
          {/* 1. Context Handoff Snapshot Card */}
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

          {/* 2. Engines Grid */}
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
                          className="flex-1 flex items-center justify-center gap-1.5 bg-indigo-600 hover:bg-indigo-500 text-white font-medium text-[11.5px] py-2 rounded-lg transition shadow-sm"
                        >
                          <ArrowRightLeft className="h-3.5 w-3.5" /> Switch Context
                        </button>
                        <button
                          onClick={() => {
                            setAccountsAgent(eng.id);
                            setShowAccountsDialog(true);
                          }}
                          className="bg-[#1b202c] hover:bg-[#242b3b] text-slate-300 px-2.5 py-2 rounded-lg text-[11.5px] border border-[#2b3447] transition flex items-center gap-1"
                          title="Quản lý tài khoản"
                        >
                          <UserCheck className="h-3.5 w-3.5" />
                        </button>
                      </div>
                    ) : (
                      <button
                        onClick={() => handleInstall(eng.id)}
                        className="w-full flex items-center justify-center gap-1.5 bg-emerald-600 hover:bg-emerald-500 text-white font-medium text-[11.5px] py-2 rounded-lg transition"
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
          <div className="h-[360px] lg:h-full lg:w-[480px] border-t lg:border-t-0 lg:border-l border-[#1d222b] bg-[#0c0e14] flex flex-col">
            <div className="flex h-9 shrink-0 items-center justify-between border-b border-[#1d222b] bg-[#101319] px-4">
              <span className="text-[12px] font-medium text-slate-300 flex items-center gap-1.5">
                <TerminalIcon className="h-3.5 w-3.5 text-sky-400" /> Embedded PTY Terminal
              </span>
              <button
                onClick={() => setShowTerminal(false)}
                className="text-slate-400 hover:text-slate-200 text-[11px]"
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

      {/* Account Switcher Dialog */}
      {showAccountsDialog && (
        <AccountsDialog
          addEngine={accountsAgent}
          onChanged={loadData}
          onClose={() => setShowAccountsDialog(false)}
        />
      )}
    </div>
  );
}
export { App };
