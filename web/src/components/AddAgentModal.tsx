import React, { useState } from "react";
import {
  X,
  Sparkles,
  Terminal,
  Code2,
  Cpu,
  Check,
  AlertCircle,
  Loader2,
  Plus,
  ShieldCheck,
  KeyRound,
  Eye,
  EyeOff,
  Copy,
  Download,
  LogOut,
} from "lucide-react";

export type AuthMode = "oauth" | "api_key" | "env";

export interface CustomAgentConfig {
  id: string;
  name: string;
  desc: string;
  binaryPath: string;
  defaultModel?: string;
  authMode: AuthMode;
  envKey?: string;
  envValue?: string;
  loginCmd?: string;
  color?: string;
  isAvailable?: boolean;
}

interface AddAgentModalProps {
  isOpen: boolean;
  onClose: () => void;
  onAddAgent: (agent: CustomAgentConfig) => void;
}

const PRESETS = [
  {
    id: "claude",
    name: "Claude Code",
    desc: "Anthropic Claude CLI Assistant",
    binary: "claude",
    model: "claude-sonnet-5-5",
    authMode: "oauth" as AuthMode,
    loginCmd: "claude login",
    envKey: "ANTHROPIC_API_KEY",
    installCmd: "npm install -g @anthropic-ai/claude-code",
    color: "text-purple-400",
    icon: Terminal,
  },
  {
    id: "codex",
    name: "OpenAI Codex",
    desc: "OpenAI Codex CLI Subprocess",
    binary: "codex",
    model: "gpt-6-astra",
    authMode: "oauth" as AuthMode,
    loginCmd: "codex login",
    envKey: "OPENAI_API_KEY",
    installCmd: "npm install -g @openai/codex",
    color: "text-emerald-400",
    icon: Code2,
  },
  {
    id: "aider",
    name: "Aider AI Pair",
    desc: "Autonomous AI Pair Programming CLI",
    binary: "aider",
    model: "claude-sonnet-5-5",
    authMode: "api_key" as AuthMode,
    loginCmd: "",
    envKey: "ANTHROPIC_API_KEY",
    installCmd: "pip install aider-chat",
    color: "text-sky-400",
    icon: Cpu,
  },
  {
    id: "custom",
    name: "Custom CLI Tool",
    desc: "Any local terminal-based AI CLI",
    binary: "",
    model: "default",
    authMode: "oauth" as AuthMode,
    loginCmd: "",
    envKey: "",
    installCmd: "",
    color: "text-amber-400",
    icon: Sparkles,
  },
];

export const AddAgentModal: React.FC<AddAgentModalProps> = ({
  isOpen,
  onClose,
  onAddAgent,
}) => {
  const [selectedPreset, setSelectedPreset] = useState("claude");
  const [name, setName] = useState("Claude Code");
  const [id, setId] = useState("claude");
  const [desc, setDesc] = useState("Anthropic Claude CLI Assistant");
  const [binaryPath, setBinaryPath] = useState("claude");
  const [defaultModel, setDefaultModel] = useState("claude-sonnet-5-5");
  const [authMode, setAuthMode] = useState<AuthMode>("oauth");
  const [loginCmd, setLoginCmd] = useState("claude login");
  const [envKey, setEnvKey] = useState("ANTHROPIC_API_KEY");
  const [envValue, setEnvValue] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [installCmd, setInstallCmd] = useState("npm install -g @anthropic-ai/claude-code");
  const [copiedLoginCmd, setCopiedLoginCmd] = useState(false);

  // Checking binary & auth status
  const [isChecking, setIsChecking] = useState(false);
  const [checkResult, setCheckResult] = useState<{
    tested: boolean;
    found: boolean;
    path?: string;
    has_auth?: boolean;
    auth_method?: string;
    auth_status?: string;
    error?: string;
  } | null>(null);

  // Install state
  const [isInstalling, setIsInstalling] = useState(false);
  const [installResult, setInstallResult] = useState<{
    success: boolean;
    output?: string;
    found?: boolean;
    path?: string;
    error?: string;
  } | null>(null);

	// Auth management state (logout / switch account)
	const [isLoggingOut, setIsLoggingOut] = useState(false);
	const [authActionMessage, setAuthActionMessage] = useState<string | null>(null);
	const [isStartingCodexLogin, setIsStartingCodexLogin] = useState(false);
	const [codexLoginState, setCodexLoginState] = useState<{
		running: boolean;
		finished: boolean;
		output?: string;
		error?: string;
		logged_in: boolean;
	} | null>(null);

  const handleLogout = async () => {
    setIsLoggingOut(true);
    setAuthActionMessage(null);
    try {
      const res = await fetch("/api/agents/auth-action", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ action: "logout", agent: id }),
      });
      const data = await res.json();
      if (data.success) {
        setAuthActionMessage("Logged out successfully. You can now login with another account.");
        // Re-check auth status
        setTimeout(() => {
          handleVerifyBinary();
        }, 500);
      } else {
        setAuthActionMessage(`Logout error: ${data.error || "Failed"}`);
      }
    } catch {
      setAuthActionMessage("Failed to reach server");
    } finally {
      setIsLoggingOut(false);
    }
	};

	const handleCodexDeviceLogin = async (action: "start" | "status") => {
		setIsStartingCodexLogin(true);
		setAuthActionMessage(null);
		try {
			const res = await fetch("/api/agents/codex-login", {
				method: "POST",
				headers: { "Content-Type": "application/json" },
				body: JSON.stringify({ action }),
			});
			const data = await res.json();
			if (!data.success) {
				setAuthActionMessage(data.error || "Could not start Codex login");
				return;
			}

			setCodexLoginState(data.state);
			if (data.state?.logged_in) {
				setCheckResult({
					tested: true,
					found: true,
					has_auth: true,
					auth_method: "oauth",
					auth_status: "Signed in with Codex CLI",
				});
				setAuthActionMessage("Codex is signed in and ready to use.");
			}
		} catch {
			setAuthActionMessage("Failed to reach the Codex login service");
		} finally {
			setIsStartingCodexLogin(false);
		}
	};

	if (!isOpen) return null;

  const handleSelectPreset = (presetId: string) => {
    setSelectedPreset(presetId);
    const p = PRESETS.find((item) => item.id === presetId);
    if (!p) return;
    setName(p.name);
    setId(p.id === "custom" ? `agent-${Date.now().toString().slice(-4)}` : p.id);
    setDesc(p.desc);
    setBinaryPath(p.binary);
    setDefaultModel(p.model);
    setAuthMode(p.authMode);
    setLoginCmd(p.loginCmd);
    setEnvKey(p.envKey);
    setInstallCmd(p.installCmd);
    setCheckResult(null);
    setInstallResult(null);
  };

  const handleInstallBinary = async () => {
    if (!installCmd.trim()) return;
    setIsInstalling(true);
    setInstallResult(null);

    try {
      const res = await fetch("/api/agents/install", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ command: installCmd.trim(), binary: binaryPath.trim() }),
      });
      const data = await res.json();
      setInstallResult({
        success: !!data.success,
        output: data.output,
        found: !!data.found,
        path: data.path,
        error: data.error,
      });

      // If install succeeded and binary is now found, update checkResult
      if (data.success && data.found) {
        setCheckResult({
          tested: true,
          found: true,
          path: data.path,
          has_auth: false,
          auth_method: "none",
          auth_status: "Installed — run login or set API key",
        });
      }
    } catch {
      setInstallResult({
        success: false,
        error: "Cannot connect to server install endpoint",
      });
    } finally {
      setIsInstalling(false);
    }
  };

  const handleVerifyBinary = async () => {
    if (!binaryPath.trim()) return;
    setIsChecking(true);
    setCheckResult(null);

    try {
      const res = await fetch("/api/agents/check", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ binary: binaryPath.trim() }),
      });
      const data = await res.json();
      setCheckResult({
        tested: true,
        found: !!data.found,
        path: data.path,
        has_auth: !!data.has_auth,
        auth_method: data.auth_method,
        auth_status: data.auth_status,
        error: data.error,
      });

      // Auto-set auth mode if detected
      if (data.has_auth && data.auth_method === "oauth") {
        setAuthMode("oauth");
      }
    } catch {
      setCheckResult({
        tested: true,
        found: false,
        error: "Cannot connect to server verification endpoint",
      });
    } finally {
      setIsChecking(false);
    }
  };

  const handleCopyCommand = (text: string) => {
    navigator.clipboard.writeText(text);
    setCopiedLoginCmd(true);
    setTimeout(() => setCopiedLoginCmd(false), 2000);
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim() || !binaryPath.trim()) return;

    // Auto-install if binary not detected and install command is available
    const binaryFound = checkResult?.found ?? false;
    if (!binaryFound && installCmd.trim()) {
      setIsInstalling(true);
      setInstallResult(null);
      try {
        const res = await fetch("/api/agents/install", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ command: installCmd.trim(), binary: binaryPath.trim() }),
        });
        const data = await res.json();
        setInstallResult({
          success: !!data.success,
          output: data.output,
          found: !!data.found,
          path: data.path,
          error: data.error,
        });

        if (data.success && data.found) {
          setCheckResult({
            tested: true,
            found: true,
            path: data.path,
            has_auth: false,
            auth_method: "none",
            auth_status: "Installed — run login or set API key",
          });
        } else {
          // Install failed or binary still not found, don't close modal
          setIsInstalling(false);
          return;
        }
      } catch {
        setInstallResult({
          success: false,
          error: "Cannot connect to server install endpoint",
        });
        setIsInstalling(false);
        return;
      }
      setIsInstalling(false);
    }

    const newAgent: CustomAgentConfig = {
      id: id.trim().toLowerCase() || `agent-${Date.now().toString().slice(-4)}`,
      name: name.trim(),
      desc: desc.trim() || "Custom AI Engine",
      binaryPath: binaryPath.trim(),
      defaultModel: defaultModel.trim() || "default",
      authMode,
      loginCmd: loginCmd.trim(),
      envKey: authMode === "api_key" ? envKey.trim() : undefined,
      envValue: authMode === "api_key" ? envValue.trim() : undefined,
      color:
        selectedPreset === "claude"
          ? "text-purple-400"
          : selectedPreset === "codex"
          ? "text-emerald-400"
          : selectedPreset === "aider"
          ? "text-sky-400"
          : "text-amber-400",
      isAvailable: checkResult?.found ?? installResult?.found ?? true,
    };

    // Register with Go backend dispatcher
    try {
      await fetch("/api/agents/register", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          id: newAgent.id,
          name: newAgent.name,
          binaryPath: newAgent.binaryPath,
          defaultModel: newAgent.defaultModel,
          envKey: newAgent.envKey,
          envValue: newAgent.envValue,
        }),
      });
    } catch (e) {
      console.warn("Failed to register agent with backend:", e);
    }

    onAddAgent(newAgent);
    onClose();
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm p-4 animate-in fade-in duration-150">
      <div className="w-full max-w-xl bg-[#141720] border border-[#272e3d] rounded-2xl shadow-2xl overflow-hidden flex flex-col text-slate-200">
        {/* Modal Header */}
        <div className="flex items-center justify-between px-5 py-4 border-b border-[#222836] bg-[#12151d]">
          <div className="flex items-center gap-2.5">
            <div className="w-8 h-8 rounded-xl bg-sky-500/10 border border-sky-500/20 flex items-center justify-center text-sky-400">
              <Plus className="w-4 h-4" />
            </div>
            <div>
              <h2 className="text-sm font-semibold text-slate-100">Add AI Engine</h2>
              <p className="text-[11px] text-slate-400">Connect a CLI agent or custom AI subprocess</p>
            </div>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="p-1.5 rounded-lg text-slate-400 hover:text-slate-200 hover:bg-[#202736] transition-colors cursor-pointer"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* Modal Body */}
        <form onSubmit={handleSubmit} className="p-5 space-y-4 overflow-y-auto max-h-[78vh]">
          {/* 1. Quick Presets */}
          <div>
            <label className="block text-xs font-medium text-slate-300 mb-2">
              Select Engine Preset
            </label>
            <div className="grid grid-cols-2 gap-2">
              {PRESETS.map((p) => {
                const Icon = p.icon;
                const isSelected = selectedPreset === p.id;
                return (
                  <button
                    key={p.id}
                    type="button"
                    onClick={() => handleSelectPreset(p.id)}
                    className={`flex items-center gap-2.5 p-2.5 rounded-xl border text-left transition-all cursor-pointer ${
                      isSelected
                        ? "bg-[#1c2230] border-sky-500/60 shadow-sm shadow-sky-500/10"
                        : "bg-[#161a24] border-[#252c3c] hover:bg-[#1a202c] text-slate-400 hover:text-slate-200"
                    }`}
                  >
                    <Icon className={`w-4 h-4 ${p.color} shrink-0`} />
                    <div className="truncate">
                      <div className="text-xs font-medium text-slate-200 truncate">{p.name}</div>
                      <div className="text-[10px] text-slate-500 truncate">{p.desc}</div>
                    </div>
                  </button>
                );
              })}
            </div>
          </div>

          {/* 2. Display Name & ID */}
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-xs font-medium text-slate-300 mb-1">
                Display Name <span className="text-rose-400">*</span>
              </label>
              <input
                type="text"
                required
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="e.g. Claude Code"
                className="w-full px-3 py-1.5 rounded-xl bg-[#161a24] border border-[#272e3d] text-xs text-slate-100 placeholder-slate-500 focus:outline-none focus:border-sky-500/60"
              />
            </div>
            <div>
              <label className="block text-xs font-medium text-slate-300 mb-1">
                Engine ID <span className="text-rose-400">*</span>
              </label>
              <input
                type="text"
                required
                value={id}
                onChange={(e) => setId(e.target.value)}
                placeholder="e.g. claude"
                className="w-full px-3 py-1.5 rounded-xl bg-[#161a24] border border-[#272e3d] text-xs font-mono text-slate-100 placeholder-slate-500 focus:outline-none focus:border-sky-500/60"
              />
            </div>
          </div>

          {/* 3. CLI Binary Path & System Detection */}
          <div>
            <div className="flex items-center justify-between mb-1">
              <label className="block text-xs font-medium text-slate-300">
                CLI Binary / Command <span className="text-rose-400">*</span>
              </label>
              <button
                type="button"
                onClick={handleVerifyBinary}
                disabled={isChecking || !binaryPath.trim()}
                className="text-[11px] text-sky-400 hover:text-sky-300 disabled:opacity-40 flex items-center gap-1 cursor-pointer font-medium"
              >
                {isChecking && <Loader2 className="w-3 h-3 animate-spin" />}
                <span>Detect on system</span>
              </button>
            </div>
            <div className="relative">
              <input
                type="text"
                required
                value={binaryPath}
                onChange={(e) => {
                  setBinaryPath(e.target.value);
                  setCheckResult(null);
                }}
                placeholder="e.g. claude, /usr/bin/antigravity, aider"
                className="w-full px-3 py-1.5 rounded-xl bg-[#161a24] border border-[#272e3d] text-xs font-mono text-slate-100 placeholder-slate-500 focus:outline-none focus:border-sky-500/60 pr-8"
              />
              {checkResult && (
                <div className="absolute right-2.5 top-1/2 -translate-y-1/2">
                  {checkResult.found ? (
                    <Check className="w-4 h-4 text-emerald-400" />
                  ) : (
                    <AlertCircle className="w-4 h-4 text-amber-400" />
                  )}
                </div>
              )}
            </div>

            {/* Check Result status hint */}
            {checkResult && (
              <div
                className={`mt-1.5 px-3 py-2 rounded-xl text-[11.5px] space-y-1 ${
                  checkResult.found
                    ? "bg-emerald-500/10 border border-emerald-500/20 text-emerald-300"
                    : "bg-amber-500/10 border border-amber-500/20 text-amber-300"
                }`}
              >
                <div className="flex items-center justify-between">
                  <span>
                    {checkResult.found
                      ? `✓ Binary detected at: ${checkResult.path}`
                      : `⚠️ Binary not found in system PATH.`}
                  </span>
                </div>
                {checkResult.auth_status && (
                  <div className="text-[10.5px] text-slate-400 font-mono">
                    Auth: {checkResult.auth_status}
                  </div>
                )}
              </div>
            )}

            {installCmd && !checkResult?.found && (
              <div className="mt-2 space-y-2">
                <div className="flex items-center justify-between gap-2">
                  <div className="flex items-center gap-1.5 text-[10.5px] text-slate-400 min-w-0">
                    <span className="shrink-0">Install CLI:</span>
                    <code className="text-slate-200 font-mono bg-[#1c2230] px-1.5 py-0.5 rounded text-[10px] select-all truncate">
                      {installCmd}
                    </code>
                  </div>
                  <button
                    type="button"
                    onClick={handleInstallBinary}
                    disabled={isInstalling}
                    className="shrink-0 flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-sky-500/15 text-sky-300 border border-sky-500/30 hover:bg-sky-500/25 disabled:opacity-50 text-[11px] font-semibold transition-all cursor-pointer"
                  >
                    {isInstalling ? (
                      <>
                        <Loader2 className="w-3 h-3 animate-spin" />
                        <span>Installing...</span>
                      </>
                    ) : (
                      <>
                        <Download className="w-3 h-3" />
                        <span>Install Now</span>
                      </>
                    )}
                  </button>
                </div>

                {/* Install result */}
                {installResult && (
                  <div
                    className={`px-3 py-2 rounded-xl text-[11px] space-y-1 ${
                      installResult.success
                        ? "bg-emerald-500/10 border border-emerald-500/20 text-emerald-300"
                        : "bg-rose-500/10 border border-rose-500/20 text-rose-300"
                    }`}
                  >
                    <div className="font-medium">
                      {installResult.success
                        ? installResult.found
                          ? `✓ Installed successfully — ${installResult.path}`
                          : "✓ Install command completed, but binary not yet in PATH. Try restarting terminal."
                        : `✗ Install failed: ${installResult.error}`}
                    </div>
                    {installResult.output && (
                      <details className="mt-1">
                        <summary className="text-[10px] text-slate-400 cursor-pointer hover:text-slate-300">
                          Show output
                        </summary>
                        <pre className="mt-1 text-[9.5px] text-slate-400 font-mono bg-[#10131a] px-2 py-1.5 rounded max-h-28 overflow-auto whitespace-pre-wrap break-words">
                          {installResult.output}
                        </pre>
                      </details>
                    )}
                  </div>
                )}
              </div>
            )}
          </div>

          {/* 4. AUTHENTICATION METHOD (Xác thực tài khoản hoặc API Key) */}
          <div className="pt-2 border-t border-[#222836]">
            <label className="block text-xs font-medium text-slate-200 mb-2">
              Authentication Method (Phương thức xác thực)
            </label>

            {/* Auth Mode Tabs */}
            <div className="grid grid-cols-3 gap-2 mb-3">
              <button
                type="button"
                onClick={() => setAuthMode("oauth")}
                className={`flex flex-col items-center gap-1.5 p-2 rounded-xl border text-center transition-all cursor-pointer ${
                  authMode === "oauth"
                    ? "bg-[#1c2333] border-sky-500/60 text-sky-300 shadow-sm"
                    : "bg-[#161a24] border-[#252c3c] text-slate-400 hover:text-slate-200 hover:bg-[#1a202c]"
                }`}
              >
                <ShieldCheck className="w-4 h-4 text-sky-400" />
                <span className="text-[11px] font-medium">CLI Session Login</span>
              </button>

              <button
                type="button"
                onClick={() => setAuthMode("api_key")}
                className={`flex flex-col items-center gap-1.5 p-2 rounded-xl border text-center transition-all cursor-pointer ${
                  authMode === "api_key"
                    ? "bg-[#1c2333] border-sky-500/60 text-sky-300 shadow-sm"
                    : "bg-[#161a24] border-[#252c3c] text-slate-400 hover:text-slate-200 hover:bg-[#1a202c]"
                }`}
              >
                <KeyRound className="w-4 h-4 text-amber-400" />
                <span className="text-[11px] font-medium">API Key</span>
              </button>

              <button
                type="button"
                onClick={() => setAuthMode("env")}
                className={`flex flex-col items-center gap-1.5 p-2 rounded-xl border text-center transition-all cursor-pointer ${
                  authMode === "env"
                    ? "bg-[#1c2333] border-sky-500/60 text-sky-300 shadow-sm"
                    : "bg-[#161a24] border-[#252c3c] text-slate-400 hover:text-slate-200 hover:bg-[#1a202c]"
                }`}
              >
                <Terminal className="w-4 h-4 text-emerald-400" />
                <span className="text-[11px] font-medium">Inherit Env</span>
              </button>
            </div>

            {/* Auth Mode Details */}
            {authMode === "oauth" && (
              <div className="p-3 rounded-xl bg-[#161a24] border border-[#262d3d] space-y-2.5 text-xs text-slate-300">
                <div className="flex items-start gap-2">
                  <ShieldCheck className="w-4 h-4 text-emerald-400 shrink-0 mt-0.5" />
                  <div>
                    <div className="font-medium text-slate-200">
                      Tự động kế thừa phiên đăng nhập CLI (OAuth)
                    </div>
                    <div className="text-[11px] text-slate-400 leading-relaxed mt-0.5">
                      Subprocess tự động đọc token phiên làm việc đã lưu trong máy (ví dụ: <code className="text-slate-300 font-mono text-[10.5px]">~/.claude.json</code> hoặc <code className="text-slate-300 font-mono text-[10.5px]">~/.gemini/oauth_creds.json</code>). Không cần lưu trữ API Key thủ công.
                    </div>
                  </div>
                </div>

                {/* Account status & Login/Switch account actions */}
                <div className="pt-2 border-t border-[#222836] space-y-2">
                  <div className="flex items-center justify-between">
                    <div className="text-[11px] font-medium text-slate-300 flex items-center gap-1.5">
                      <KeyRound className="w-3.5 h-3.5 text-sky-400" />
                      <span>Account Session:</span>
                      <span className={checkResult?.has_auth ? "text-emerald-400 font-semibold" : "text-amber-400 font-semibold"}>
                        {checkResult?.has_auth ? "Signed in" : "Not signed in"}
                      </span>
                    </div>

                    {checkResult?.has_auth && (
                      <button
                        type="button"
                        onClick={handleLogout}
                        disabled={isLoggingOut}
                        className="px-2 py-0.5 rounded-lg bg-rose-500/15 hover:bg-rose-500/25 border border-rose-500/30 text-rose-300 text-[10.5px] font-medium transition-colors flex items-center gap-1 cursor-pointer disabled:opacity-50"
                        title="Log out from current CLI session to switch account"
                      >
                        {isLoggingOut ? (
                          <Loader2 className="w-3 h-3 animate-spin" />
                        ) : (
                          <LogOut className="w-3 h-3" />
                        )}
                        <span>Switch Account / Logout</span>
                      </button>
                    )}
                  </div>

					{authActionMessage && (
						<div className="text-[10.5px] text-sky-300 bg-sky-500/10 px-2 py-1 rounded border border-sky-500/20">
							{authActionMessage}
						</div>
					)}

					{id === "codex" && !checkResult?.has_auth && (
						<div className="space-y-2 pt-1">
							<button
								type="button"
								onClick={() => handleCodexDeviceLogin("start")}
								disabled={isStartingCodexLogin}
								className="w-full px-3 py-2 rounded-lg bg-emerald-500/15 hover:bg-emerald-500/25 border border-emerald-500/30 text-emerald-200 text-[11px] font-semibold transition-colors flex items-center justify-center gap-1.5 cursor-pointer disabled:opacity-50"
							>
								{isStartingCodexLogin ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <ShieldCheck className="w-3.5 h-3.5" />}
								<span>{isStartingCodexLogin ? "Starting secure login..." : "Sign in with Codex"}</span>
							</button>

							{codexLoginState && (
								<div className="space-y-2 rounded-lg border border-emerald-500/20 bg-emerald-500/5 p-2.5">
									<div className="text-[10.5px] text-emerald-200">
										Open the URL or enter the device code below in your browser, then return here to check sign-in. This page never receives your token.
									</div>
									{codexLoginState.output && (
										<pre className="max-h-32 overflow-auto whitespace-pre-wrap break-words rounded bg-[#10131a] p-2 text-[10px] text-slate-300 font-mono">{codexLoginState.output}</pre>
									)}
									{codexLoginState.error && <div className="text-[10.5px] text-rose-300">{codexLoginState.error}</div>}
									<button
										type="button"
										onClick={() => handleCodexDeviceLogin("status")}
										disabled={isStartingCodexLogin}
										className="w-full rounded-md border border-sky-500/30 bg-sky-500/10 px-2.5 py-1.5 text-[10.5px] font-medium text-sky-200 hover:bg-sky-500/20 disabled:opacity-50"
									>
										Check sign-in status
									</button>
								</div>
							)}
						</div>
					)}

					{loginCmd && (
                    <div className="flex items-center justify-between gap-2 pt-1">
                      <div className="flex items-center gap-2 font-mono text-[11px] text-slate-300 bg-[#10131a] px-2.5 py-1.5 rounded-lg border border-[#222838] truncate flex-1">
                        <Terminal className="w-3 h-3 text-slate-400 shrink-0" />
                        <span className="truncate">{loginCmd}</span>
                      </div>
                      <button
                        type="button"
                        onClick={() => handleCopyCommand(loginCmd)}
                        className="px-2.5 py-1.5 rounded-lg bg-[#202738] hover:bg-[#283248] text-[11px] text-slate-200 font-medium transition-colors flex items-center gap-1.5 cursor-pointer shrink-0"
                      >
                        {copiedLoginCmd ? (
                          <>
                            <Check className="w-3 h-3 text-emerald-400" />
                            <span>Copied</span>
                          </>
                        ) : (
                          <>
                            <Copy className="w-3 h-3 text-slate-400" />
                            <span>Copy Login Command</span>
                          </>
                        )}
                      </button>
                    </div>
                  )}

                  <p className="text-[10px] text-slate-400 leading-normal">
                    Tip: Để đổi tài khoản (Switch Account), bấm <strong className="text-slate-300">Switch Account / Logout</strong> hoặc mở Terminal chạy <code className="text-slate-300 bg-[#141822] px-1 rounded">{loginCmd}</code> để trình duyệt mở trang đăng nhập Anthropic/Google mới.
                  </p>
                </div>
              </div>
            )}

            {authMode === "api_key" && (
              <div className="p-3 rounded-xl bg-[#161a24] border border-[#262d3d] space-y-3">
                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <label className="block text-xs font-medium text-slate-300 mb-1">
                      Env Variable Name
                    </label>
                    <input
                      type="text"
                      value={envKey}
                      onChange={(e) => setEnvKey(e.target.value)}
                      placeholder="e.g. ANTHROPIC_API_KEY"
                      className="w-full px-3 py-1.5 rounded-xl bg-[#11141c] border border-[#272e3d] text-xs font-mono text-slate-100 placeholder-slate-500 focus:outline-none focus:border-sky-500/60"
                    />
                  </div>
                  <div>
                    <label className="block text-xs font-medium text-slate-300 mb-1">
                      API Key Token
                    </label>
                    <div className="relative">
                      <input
                        type={showPassword ? "text" : "password"}
                        value={envValue}
                        onChange={(e) => setEnvValue(e.target.value)}
                        placeholder="sk-ant-... or sk-..."
                        className="w-full px-3 py-1.5 rounded-xl bg-[#11141c] border border-[#272e3d] text-xs font-mono text-slate-100 placeholder-slate-500 focus:outline-none focus:border-sky-500/60 pr-8"
                      />
                      <button
                        type="button"
                        onClick={() => setShowPassword(!showPassword)}
                        className="absolute right-2.5 top-1/2 -translate-y-1/2 text-slate-400 hover:text-slate-200 p-0.5 cursor-pointer"
                      >
                        {showPassword ? <EyeOff className="w-3.5 h-3.5" /> : <Eye className="w-3.5 h-3.5" />}
                      </button>
                    </div>
                  </div>
                </div>
                <p className="text-[10.5px] text-slate-400">
                  Key sẽ được inject trực tiếp vào biến môi trường khi khởi chạy CLI subprocess.
                </p>
              </div>
            )}

            {authMode === "env" && (
              <div className="p-3 rounded-xl bg-[#161a24] border border-[#262d3d] text-xs text-slate-300 space-y-1">
                <div className="font-medium text-slate-200 flex items-center gap-2">
                  <Terminal className="w-4 h-4 text-emerald-400" />
                  <span>Kế thừa toàn bộ biến môi trường Shell</span>
                </div>
                <p className="text-[11px] text-slate-400 leading-relaxed">
                  Subprocess sẽ tự động kế thừa tất cả API Key và cấu hình đã export trong file <code className="text-slate-300 font-mono text-[10.5px]">~/.bashrc</code>, <code className="text-slate-300 font-mono text-[10.5px]">~/.zshrc</code> hoặc systemd service.
                </p>
              </div>
            )}
          </div>



          {/* Modal Actions */}
          <div className="flex items-center justify-end gap-2.5 pt-3 border-t border-[#222836]">
            <button
              type="button"
              onClick={onClose}
              className="px-4 py-2 rounded-xl bg-[#181d28] hover:bg-[#202738] text-xs font-medium text-slate-300 hover:text-white transition-colors cursor-pointer border border-[#262f40]"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={isInstalling}
              className="px-4 py-2 rounded-xl bg-sky-500 hover:bg-sky-600 disabled:opacity-60 text-xs font-semibold text-slate-950 transition-colors cursor-pointer shadow-sm shadow-sky-500/20 flex items-center gap-1.5"
            >
              {isInstalling ? (
                <>
                  <Loader2 className="w-3 h-3 animate-spin" />
                  <span>Installing...</span>
                </>
              ) : (
                <span>Connect & Save Engine</span>
              )}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
