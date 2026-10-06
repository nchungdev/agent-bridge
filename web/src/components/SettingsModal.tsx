import React, { useEffect, useRef, useState } from "react";
import {
  Sliders,
  Bot,
  Palette,
  ArrowUpCircle,
  X,
  Terminal,
  CheckCircle2,
  AlertCircle,
  RefreshCw,
  Sparkles,
  Loader2,
  GitCommit,
  Shield,
  Info,
} from "lucide-react";

export interface UpdateStatus {
  current_commit: string;
  current_message: string;
  current_date: string;
  branch: string;
  remote_commit: string;
  has_update: boolean;
  commits_behind: number;
  commits: string[];
  is_updating: boolean;
  step: "idle" | "checking" | "pulling" | "building_web" | "building_binary" | "restarting" | "success" | "error";
  error?: string;
  logs: string[];
  last_checked?: string;
}

export interface EngineStatus {
  id: string;
  name: string;
  base?: string;
  binary: string;
  installCmd?: string;
  installed: boolean;
  auth_status?: string;
  has_auth?: boolean;
}

interface Props {
  isOpen: boolean;
  onClose: () => void;
  initialTab?: "general" | "agents" | "appearance" | "updates";
  engines: EngineStatus[];
  onRefreshEngines: () => void;
  onOpenAgentTerminal: (agentId: string) => void;
  updateStatus: UpdateStatus | null;
  onRefreshUpdateStatus: () => Promise<void>;
}

const AGENT_META_COLORS: Record<string, { badge: string; dot: string; border: string }> = {
  agy: { badge: "bg-blue-500/15 text-blue-300 border-blue-500/30", dot: "bg-blue-400", border: "border-blue-500/30" },
  claude: { badge: "bg-orange-500/15 text-orange-300 border-orange-500/30", dot: "bg-orange-400", border: "border-orange-500/30" },
  codex: { badge: "bg-emerald-500/15 text-emerald-300 border-emerald-500/30", dot: "bg-emerald-400", border: "border-emerald-500/30" },
};

export const SettingsModal: React.FC<Props> = ({
  isOpen,
  onClose,
  initialTab = "general",
  engines,
  onRefreshEngines,
  onOpenAgentTerminal,
  updateStatus,
  onRefreshUpdateStatus,
}) => {
  const [activeTab, setActiveTab] = useState<"general" | "agents" | "appearance" | "updates">(initialTab);

  // General settings state
  const [defaultAgent, setDefaultAgent] = useState<string>(() => localStorage.getItem("bridge_default_agent") || "agy");
  const [autoRemote, setAutoRemote] = useState<boolean>(() => localStorage.getItem("bridge_auto_remote") === "1");

  // Appearance settings state
  const [fontSize, setFontSize] = useState<string>(() => localStorage.getItem("bridge_term_fontsize") || "12.5");
  const [fontFamily, setFontFamily] = useState<string>(() => localStorage.getItem("bridge_term_font") || 'ui-monospace, SFMono-Regular, Menlo, Consolas, "Liberation Mono", monospace');

  // Update states
  const [checkingUpdate, setCheckingUpdate] = useState(false);
  const [updatingApp, setUpdatingApp] = useState(false);
  const [currentStep, setCurrentStep] = useState<string>("idle");
  const [updateLogs, setUpdateLogs] = useState<string[]>([]);
  const [reconnecting, setReconnecting] = useState(false);
  const [updateError, setUpdateError] = useState<string | null>(null);
  const logEndRef = useRef<HTMLDivElement>(null);

  // Sync tab when opened
  useEffect(() => {
    if (isOpen && initialTab) {
      setActiveTab(initialTab);
    }
  }, [isOpen, initialTab]);

  // Sync update status
  useEffect(() => {
    if (updateStatus) {
      if (updateStatus.is_updating) {
        setUpdatingApp(true);
      }
      setCurrentStep(updateStatus.step);
      if (updateStatus.logs && updateStatus.logs.length > 0) {
        setUpdateLogs(updateStatus.logs);
      }
      if (updateStatus.error) {
        setUpdateError(updateStatus.error);
      }
    }
  }, [updateStatus]);

  // Auto-scroll logs
  useEffect(() => {
    logEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [updateLogs]);

  // Save General settings
  const handleDefaultAgentChange = (val: string) => {
    setDefaultAgent(val);
    localStorage.setItem("bridge_default_agent", val);
  };

  const handleAutoRemoteToggle = () => {
    const next = !autoRemote;
    setAutoRemote(next);
    localStorage.setItem("bridge_auto_remote", next ? "1" : "0");
  };

  // Save Appearance settings
  const handleFontSizeChange = (val: string) => {
    setFontSize(val);
    localStorage.setItem("bridge_term_fontsize", val);
    window.dispatchEvent(new Event("bridge-terminal-settings-changed"));
  };

  const handleFontFamilyChange = (val: string) => {
    setFontFamily(val);
    localStorage.setItem("bridge_term_font", val);
    window.dispatchEvent(new Event("bridge-terminal-settings-changed"));
  };

  // Update Actions
  const handleCheckUpdate = async () => {
    setCheckingUpdate(true);
    setUpdateError(null);
    try {
      const res = await fetch("/api/admin/update/check", { method: "POST" });
      if (res.ok) {
        await onRefreshUpdateStatus();
      } else {
        const err = await res.json().catch(() => ({}));
        setUpdateError(err.error || "Không thể kiểm tra bản cập nhật.");
      }
    } catch (e: any) {
      setUpdateError(e.message || "Lỗi mạng khi kiểm tra cập nhật.");
    } finally {
      setCheckingUpdate(false);
    }
  };

  const handleApplyUpdate = async () => {
    if (!confirm("Bắt đầu quá trình tải mã nguồn mới, biên dịch và khởi động lại dịch vụ Agent Bridge?")) {
      return;
    }
    setUpdatingApp(true);
    setUpdateError(null);
    setUpdateLogs(["Bắt đầu yêu cầu cập nhật..."]);
    try {
      const res = await fetch("/api/admin/update/apply", { method: "POST" });
      if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        setUpdateError(err.error || "Không thể khởi chạy cập nhật.");
        setUpdatingApp(false);
      }
    } catch (e: any) {
      setUpdateError(e.message || "Lỗi gửi yêu cầu cập nhật.");
      setUpdatingApp(false);
    }
  };

  // Polling loop while updating
  useEffect(() => {
    let timer: any = null;
    if (updatingApp || currentStep === "pulling" || currentStep === "building_web" || currentStep === "building_binary" || currentStep === "restarting") {
      timer = setInterval(async () => {
        try {
          const res = await fetch("/api/admin/update/status");
          if (res.ok) {
            const data: UpdateStatus = await res.json();
            setCurrentStep(data.step);
            setUpdateLogs(data.logs || []);
            if (data.error) {
              setUpdateError(data.error);
              setUpdatingApp(false);
            }
            if (data.step === "restarting") {
              setReconnecting(true);
              pollReconnect();
              clearInterval(timer);
            }
          }
        } catch {
          if (currentStep === "restarting" || currentStep === "building_binary") {
            setReconnecting(true);
            pollReconnect();
            clearInterval(timer);
          }
        }
      }, 1000);
    }
    return () => {
      if (timer) clearInterval(timer);
    };
  }, [updatingApp, currentStep]);

  const pollReconnect = () => {
    setReconnecting(true);
    let attempts = 0;
    const interval = setInterval(async () => {
      attempts++;
      try {
        const res = await fetch("/api/admin/update/status");
        if (res.ok) {
          clearInterval(interval);
          setReconnecting(false);
          setUpdatingApp(false);
          setCurrentStep("success");
          setUpdateLogs((prev) => [...prev, "Máy chủ đã hoạt động trở lại! Đang tải lại ứng dụng..."]);
          setTimeout(() => {
            window.location.reload();
          }, 1200);
        }
      } catch {
        if (attempts > 60) {
          clearInterval(interval);
          setUpdateError("Không thể kết nối lại sau 60 giây. Vui lòng tải lại trang thủ công.");
        }
      }
    }, 1500);
  };

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-md p-3 sm:p-6 animate-in fade-in duration-150">
      <div className="relative flex w-full max-w-4xl h-[85vh] max-h-[760px] min-h-[520px] rounded-2xl border border-[#232938] bg-[#0e121a] shadow-2xl overflow-hidden text-slate-200">
        {/* Left Sidebar */}
        <div className="flex w-52 sm:w-60 shrink-0 flex-col border-r border-[#1a202c] bg-[#0a0d14] p-3 justify-between select-none">
          <div>
            <div className="px-3 pt-3 pb-2 text-[11px] font-bold uppercase tracking-wider text-slate-500">
              Settings
            </div>

            <nav className="space-y-1 mt-1">
              <button
                onClick={() => setActiveTab("general")}
                className={`flex w-full cursor-pointer items-center gap-2.5 rounded-lg px-3 py-2 text-xs font-medium transition-colors ${
                  activeTab === "general"
                    ? "bg-white/[0.08] text-slate-100 shadow-sm"
                    : "text-slate-400 hover:bg-white/[0.04] hover:text-slate-200"
                }`}
              >
                <Sliders className="h-4 w-4 text-indigo-400" />
                <span>General</span>
              </button>

              <button
                onClick={() => setActiveTab("agents")}
                className={`flex w-full cursor-pointer items-center gap-2.5 rounded-lg px-3 py-2 text-xs font-medium transition-colors ${
                  activeTab === "agents"
                    ? "bg-white/[0.08] text-slate-100 shadow-sm"
                    : "text-slate-400 hover:bg-white/[0.04] hover:text-slate-200"
                }`}
              >
                <Bot className="h-4 w-4 text-sky-400" />
                <span>Agents</span>
              </button>

              <button
                onClick={() => setActiveTab("appearance")}
                className={`flex w-full cursor-pointer items-center gap-2.5 rounded-lg px-3 py-2 text-xs font-medium transition-colors ${
                  activeTab === "appearance"
                    ? "bg-white/[0.08] text-slate-100 shadow-sm"
                    : "text-slate-400 hover:bg-white/[0.04] hover:text-slate-200"
                }`}
              >
                <Palette className="h-4 w-4 text-amber-400" />
                <span>Appearance</span>
              </button>

              <button
                onClick={() => setActiveTab("updates")}
                className={`flex w-full cursor-pointer items-center justify-between rounded-lg px-3 py-2 text-xs font-medium transition-colors ${
                  activeTab === "updates"
                    ? "bg-white/[0.08] text-slate-100 shadow-sm"
                    : "text-slate-400 hover:bg-white/[0.04] hover:text-slate-200"
                }`}
              >
                <div className="flex items-center gap-2.5">
                  <ArrowUpCircle className="h-4 w-4 text-emerald-400" />
                  <span>Updates</span>
                </div>
                {updateStatus?.has_update && (
                  <span className="flex items-center gap-1 rounded-full bg-amber-500/15 border border-amber-500/30 px-1.5 py-0.5 text-[10px] font-semibold text-amber-300">
                    <span className="h-1.5 w-1.5 rounded-full bg-amber-400 animate-pulse"></span>
                    <span>{updateStatus.commits_behind > 0 ? `${updateStatus.commits_behind} mới` : "Mới"}</span>
                  </span>
                )}
              </button>
            </nav>
          </div>

          {/* Sidebar Footer info */}
          <div className="border-t border-[#1a202c] pt-3 px-2 text-[11px] text-slate-500 font-mono flex items-center justify-between">
            <span>Agent Bridge</span>
            <span>{updateStatus?.current_commit || "main"}</span>
          </div>
        </div>

        {/* Right Content Area */}
        <div className="relative flex-1 flex flex-col min-w-0 bg-[#0e121a] overflow-hidden">
          {/* Close button */}
          <button
            onClick={onClose}
            className="absolute top-4 right-4 z-10 cursor-pointer rounded-lg p-1.5 text-slate-400 hover:bg-[#1a202c] hover:text-slate-200 transition-colors"
            title="Đóng cài đặt"
          >
            <X className="h-5 w-5" />
          </button>

          {/* Scrollable View Content */}
          <div className="flex-1 overflow-y-auto p-6 sm:p-8">
            {/* TAB: GENERAL */}
            {activeTab === "general" && (
              <div className="space-y-6 max-w-2xl">
                <div>
                  <h1 className="text-xl font-bold text-slate-100">General</h1>
                  <p className="text-xs text-slate-400 mt-1">
                    Cấu hình hành vi khởi tạo tác vụ, quyền hạn và điều khiển từ xa.
                  </p>
                </div>

                <div className="space-y-4">
                  {/* Execution section */}
                  <div className="space-y-2">
                    <h2 className="text-xs font-semibold uppercase tracking-wider text-slate-400">
                      Task Execution
                    </h2>
                    <div className="rounded-xl border border-[#202738] bg-[#131722] p-4 space-y-4">
                      <div className="flex items-center justify-between gap-4">
                        <div>
                          <div className="text-xs font-medium text-slate-200">Agent mặc định khi tạo Task</div>
                          <div className="text-[11.5px] text-slate-400 mt-0.5">
                            CLI khởi chạy đầu tiên khi bạn bấm nút "+ Task" mới.
                          </div>
                        </div>
                        <select
                          value={defaultAgent}
                          onChange={(e) => handleDefaultAgentChange(e.target.value)}
                          className="rounded-lg border border-[#2c3549] bg-[#1a202e] px-3 py-1.5 text-xs text-slate-200 focus:outline-none focus:border-indigo-500 cursor-pointer"
                        >
                          <option value="agy">Antigravity (agy)</option>
                          <option value="claude">Claude Code (claude)</option>
                          <option value="codex">Codex (codex)</option>
                        </select>
                      </div>

                      <div className="border-t border-[#1c2230] pt-3 flex items-center justify-between gap-4">
                        <div>
                          <div className="text-xs font-medium text-slate-200">Tự động gắn cờ Remote Control</div>
                          <div className="text-[11.5px] text-slate-400 mt-0.5">
                            Luôn mở session với cờ <code className="text-indigo-300 font-mono">--remote-control</code> để đồng bộ với web/mobile app của agent.
                          </div>
                        </div>
                        <button
                          onClick={handleAutoRemoteToggle}
                          className={`relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none ${
                            autoRemote ? "bg-indigo-600" : "bg-slate-700"
                          }`}
                        >
                          <span
                            className={`pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out ${
                              autoRemote ? "translate-x-4" : "translate-x-0"
                            }`}
                          />
                        </button>
                      </div>
                    </div>
                  </div>

                  {/* Security & Workspace Rules */}
                  <div className="space-y-2">
                    <h2 className="text-xs font-semibold uppercase tracking-wider text-slate-400">
                      Workspace Safety
                    </h2>
                    <div className="rounded-xl border border-[#202738] bg-[#131722] p-4 flex items-start gap-3">
                      <Shield className="h-5 w-5 text-indigo-400 shrink-0 mt-0.5" />
                      <div>
                        <div className="text-xs font-medium text-slate-200">Bảo vệ thư mục gốc & người dùng</div>
                        <div className="text-[11.5px] text-slate-400 mt-1 leading-relaxed">
                          Hệ thống tự động kích hoạt tính năng khoá an toàn cho các thư mục tài khoản cá nhân và hệ điều hành (<code className="text-slate-300 font-mono">/Users/*</code>, <code className="text-slate-300 font-mono">/home/*</code>, <code className="text-slate-300 font-mono">/root</code>, <code className="text-slate-300 font-mono">/tmp</code>), không cho phép xoá khỏi danh mục để tránh mất mát dữ liệu ngoài ý muốn.
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            )}

            {/* TAB: AGENTS */}
            {activeTab === "agents" && (
              <div className="space-y-6 max-w-2xl">
                <div className="flex items-center justify-between">
                  <div>
                    <h1 className="text-xl font-bold text-slate-100">Agents</h1>
                    <p className="text-xs text-slate-400 mt-1">
                      Các công cụ CLI phát hiện được trong hệ thống. Để đăng nhập hoặc sửa cấu hình, hãy mở trực tiếp CLI đó.
                    </p>
                  </div>
                  <button
                    onClick={onRefreshEngines}
                    className="flex cursor-pointer items-center gap-1.5 rounded-lg border border-[#2b3548] bg-[#161a25] px-2.5 py-1.5 text-xs text-slate-300 hover:bg-[#202636] hover:text-white transition-colors"
                    title="Quét lại PATH"
                  >
                    <RefreshCw className="h-3.5 w-3.5" />
                    <span>Quét lại</span>
                  </button>
                </div>

                {/* List of discovered engines */}
                <div className="space-y-3">
                  {engines.map((eng) => {
                    const baseKey = eng.base || eng.id;
                    const meta = AGENT_META_COLORS[baseKey] || {
                      badge: "bg-purple-500/15 text-purple-300 border-purple-500/30",
                      dot: "bg-purple-400",
                      border: "border-purple-500/30",
                    };

                    return (
                      <div
                        key={eng.id}
                        className="rounded-xl border border-[#202738] bg-[#131722] p-4 flex items-center justify-between gap-4 transition-colors hover:border-[#2b344b]"
                      >
                        <div className="flex items-center gap-3 min-w-0">
                          <span className={`h-2.5 w-2.5 rounded-full shrink-0 ${meta.dot}`} />
                          <div className="min-w-0">
                            <div className="flex items-center gap-2">
                              <span className="text-xs font-semibold text-slate-100 truncate">{eng.name}</span>
                              <span className={`rounded border px-1.5 py-0.2 text-[10px] font-mono ${meta.badge}`}>
                                {eng.id}
                              </span>
                            </div>
                            <div className="text-[11px] text-slate-500 font-mono truncate mt-0.5" title={eng.binary}>
                              {eng.binary} · {eng.installed ? "Đã cài đặt trong PATH" : "Chưa tìm thấy binary"}
                            </div>
                          </div>
                        </div>

                        <div className="shrink-0 flex items-center gap-2">
                          {eng.installed ? (
                            <button
                              onClick={() => {
                                onClose();
                                onOpenAgentTerminal(eng.id);
                              }}
                              className="flex cursor-pointer items-center gap-1.5 rounded-lg border border-[#2e374d] bg-[#191f2c] px-3 py-1.5 text-xs font-medium text-slate-200 hover:bg-[#232c3f] hover:text-white transition-colors shadow-sm"
                              title={`Mở terminal chạy trực tiếp CLI ${eng.id} để cấu hình/đăng nhập`}
                            >
                              <Terminal className="h-3.5 w-3.5 text-sky-400" />
                              <span>Mở CLI</span>
                            </button>
                          ) : (
                            eng.installCmd && (
                              <button
                                onClick={async () => {
                                  try {
                                    const r = await fetch("/api/agents/install", {
                                      method: "POST",
                                      headers: { "Content-Type": "application/json" },
                                      body: JSON.stringify({ command: eng.installCmd, binary: eng.binary }),
                                    });
                                    const d = await r.json();
                                    if (!d.success) alert(`Lỗi: ${d.error || d.output}`);
                                    onRefreshEngines();
                                  } catch (e: any) {
                                    alert(e.message);
                                  }
                                }}
                                className="cursor-pointer rounded-lg bg-indigo-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-indigo-500 transition-colors shadow-sm"
                              >
                                Cài đặt
                              </button>
                            )
                          )}
                        </div>
                      </div>
                    );
                  })}
                </div>

                {/* Helpful note banner */}
                <div className="rounded-xl border border-[#222a3b] bg-[#111520] p-4 flex items-start gap-3">
                  <Info className="h-4 w-4 text-sky-400 shrink-0 mt-0.5" />
                  <div className="text-[11.5px] text-slate-400 leading-relaxed">
                    <strong className="text-slate-200">Nguyên tắc cấu hình độc lập:</strong> Mỗi agent CLI (Claude, Antigravity, Codex) lưu trữ thông tin đăng nhập, token và thiết lập trong thư mục người dùng riêng (<code className="font-mono text-slate-300">~/.claude</code>, <code className="font-mono text-slate-300">~/.gemini</code>, <code className="font-mono text-slate-300">~/.codex</code>). Khi bạn cần đăng nhập tài khoản khác hoặc thay đổi tham số, chỉ cần bấm <strong>Mở CLI</strong> và gõ trực tiếp lệnh của CLI đó.
                  </div>
                </div>
              </div>
            )}

            {/* TAB: APPEARANCE */}
            {activeTab === "appearance" && (
              <div className="space-y-6 max-w-2xl">
                <div>
                  <h1 className="text-xl font-bold text-slate-100">Appearance</h1>
                  <p className="text-xs text-slate-400 mt-1">
                    Tuỳ chỉnh hiển thị giao diện, kích thước và font chữ terminal.
                  </p>
                </div>

                <div className="space-y-4">
                  <div className="rounded-xl border border-[#202738] bg-[#131722] p-4 space-y-4">
                    <div className="flex items-center justify-between gap-4">
                      <div>
                        <div className="text-xs font-medium text-slate-200">Cỡ chữ Terminal</div>
                        <div className="text-[11.5px] text-slate-400 mt-0.5">
                          Font size hiển thị cho các cửa sổ terminal console.
                        </div>
                      </div>
                      <select
                        value={fontSize}
                        onChange={(e) => handleFontSizeChange(e.target.value)}
                        className="rounded-lg border border-[#2c3549] bg-[#1a202e] px-3 py-1.5 text-xs text-slate-200 focus:outline-none focus:border-indigo-500 cursor-pointer"
                      >
                        <option value="11">11 px</option>
                        <option value="12">12 px</option>
                        <option value="12.5">12.5 px (Default)</option>
                        <option value="13">13 px</option>
                        <option value="14">14 px</option>
                        <option value="15">15 px</option>
                        <option value="16">16 px</option>
                      </select>
                    </div>

                    <div className="border-t border-[#1c2230] pt-3 flex items-center justify-between gap-4">
                      <div>
                        <div className="text-xs font-medium text-slate-200">Font chữ Terminal</div>
                        <div className="text-[11.5px] text-slate-400 mt-0.5">
                          Font đơn khoảng (Monospace) sử dụng cho xterm.
                        </div>
                      </div>
                      <select
                        value={fontFamily}
                        onChange={(e) => handleFontFamilyChange(e.target.value)}
                        className="rounded-lg border border-[#2c3549] bg-[#1a202e] px-3 py-1.5 text-xs text-slate-200 focus:outline-none focus:border-indigo-500 cursor-pointer font-mono"
                      >
                        <option value='ui-monospace, SFMono-Regular, Menlo, Consolas, "Liberation Mono", monospace'>System Default</option>
                        <option value='"JetBrains Mono", ui-monospace, monospace'>JetBrains Mono</option>
                        <option value='"Fira Code", ui-monospace, monospace'>Fira Code</option>
                        <option value='"Cascadia Code", Consolas, monospace'>Cascadia Code</option>
                      </select>
                    </div>
                  </div>
                </div>
              </div>
            )}

            {/* TAB: UPDATES */}
            {activeTab === "updates" && (
              <div className="space-y-6 max-w-2xl">
                <div>
                  <h1 className="text-xl font-bold text-slate-100">Updates & System</h1>
                  <p className="text-xs text-slate-400 mt-1">
                    Cập nhật mã nguồn mới nhất từ GitHub, biên dịch Vite & Go binary và khởi động lại dịch vụ.
                  </p>
                </div>

                <div className="space-y-4">
                  {/* Status banner */}
                  <div className="rounded-xl border border-[#22293b] bg-[#131722] p-4 space-y-3">
                    <div className="flex items-center justify-between">
                      <div className="flex items-center gap-2 text-xs font-medium text-slate-300">
                        <GitCommit className="h-4 w-4 text-indigo-400" />
                        <span>Nhánh: <strong className="text-slate-100 font-mono">{updateStatus?.branch || "main"}</strong></span>
                      </div>
                      <div className="text-[11px] text-slate-500">
                        {updateStatus?.last_checked ? `Kiểm tra ${new Date(updateStatus.last_checked).toLocaleTimeString()}` : ""}
                      </div>
                    </div>

                    <div className="grid grid-cols-2 gap-3 text-[11.5px]">
                      <div className="rounded-lg border border-[#1f2637] bg-[#0d1017] p-3">
                        <div className="text-slate-500 text-[10.5px]">Phiên bản hiện tại</div>
                        <div className="font-mono font-semibold text-slate-200 mt-0.5">
                          {updateStatus?.current_commit || "---"}
                        </div>
                        <div className="text-slate-400 text-[10.5px] truncate mt-0.5" title={updateStatus?.current_message}>
                          {updateStatus?.current_message || "Chưa xác định"}
                        </div>
                      </div>

                      <div className="rounded-lg border border-[#1f2637] bg-[#0d1017] p-3">
                        <div className="text-slate-500 text-[10.5px]">Bản mới nhất trên Git</div>
                        <div className="font-mono font-semibold text-slate-200 mt-0.5">
                          {updateStatus?.remote_commit || updateStatus?.current_commit || "---"}
                        </div>
                        <div className="mt-0.5">
                          {updateStatus?.has_update ? (
                            <span className="inline-flex items-center gap-1 text-[10.5px] text-amber-400 font-medium">
                              <Sparkles className="h-3 w-3" />
                              Sau {updateStatus.commits_behind || updateStatus.commits?.length || 1} commit
                            </span>
                          ) : (
                            <span className="inline-flex items-center gap-1 text-[10.5px] text-emerald-400 font-medium">
                              <CheckCircle2 className="h-3 w-3" />
                              Mới nhất
                            </span>
                          )}
                        </div>
                      </div>
                    </div>
                  </div>

                  {/* Actions */}
                  <div className="flex items-center justify-between gap-3">
                    <button
                      onClick={handleCheckUpdate}
                      disabled={checkingUpdate || updatingApp}
                      className="flex cursor-pointer items-center gap-1.5 rounded-lg border border-[#2c3549] bg-[#161a25] px-3.5 py-2 text-xs font-medium text-slate-200 hover:bg-[#202737] hover:text-white disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
                    >
                      <RefreshCw className={`h-3.5 w-3.5 ${checkingUpdate ? "animate-spin" : ""}`} />
                      <span>Kiểm tra cập nhật</span>
                    </button>

                    <button
                      onClick={handleApplyUpdate}
                      disabled={checkingUpdate || updatingApp}
                      className={`flex cursor-pointer items-center gap-1.5 rounded-lg px-4 py-2 text-xs font-semibold shadow-md transition-all ${
                        updateStatus?.has_update
                          ? "bg-indigo-600 hover:bg-indigo-500 text-white shadow-indigo-600/30"
                          : "bg-slate-700 hover:bg-slate-600 text-slate-200"
                      } disabled:opacity-40 disabled:cursor-not-allowed`}
                    >
                      <ArrowUpCircle className="h-4 w-4" />
                      <span>{updatingApp ? "Đang cập nhật..." : updateStatus?.has_update ? "Cập nhật ngay" : "Rebuild & Cập nhật"}</span>
                    </button>
                  </div>

                  {/* Incoming commits changelog */}
                  {updateStatus?.has_update && updateStatus.commits && updateStatus.commits.length > 0 && (
                    <div className="space-y-1.5">
                      <div className="text-[11px] font-semibold uppercase tracking-wider text-slate-400">
                        Danh sách thay đổi ({updateStatus.commits.length} commit):
                      </div>
                      <div className="rounded-xl border border-[#232b3d] bg-[#11141e] p-3 max-h-36 overflow-y-auto space-y-1.5 font-mono text-[11px]">
                        {updateStatus.commits.map((c, i) => (
                          <div key={i} className="flex items-start gap-2 text-slate-300">
                            <span className="text-indigo-400 select-none">•</span>
                            <span className="truncate">{c}</span>
                          </div>
                        ))}
                      </div>
                    </div>
                  )}

                  {/* Progress bar when updating */}
                  {updatingApp && (
                    <div className="rounded-xl border border-indigo-500/20 bg-indigo-500/5 p-3.5 space-y-2">
                      <div className="flex items-center justify-between text-xs">
                        <span className="font-medium text-indigo-300 flex items-center gap-2">
                          <Loader2 className="h-3.5 w-3.5 animate-spin text-indigo-400" />
                          {currentStep === "pulling" && "1/4. Đang kéo mã nguồn (git pull)..."}
                          {currentStep === "building_web" && "2/4. Đang biên dịch giao diện Web (Vite)..."}
                          {currentStep === "building_binary" && "3/4. Đang biên dịch Go executable..."}
                          {currentStep === "restarting" && "4/4. Đang khởi động lại ứng dụng..."}
                          {currentStep === "success" && "Hoàn thành! Đang kết nối lại..."}
                        </span>
                        <span className="text-[11px] font-mono text-indigo-400">
                          {reconnecting ? "Đang chờ kết nối lại" : "Đang xử lý"}
                        </span>
                      </div>
                      <div className="h-1.5 w-full bg-[#1e2536] rounded-full overflow-hidden">
                        <div
                          className="h-full bg-indigo-500 transition-all duration-300"
                          style={{
                            width:
                              currentStep === "pulling"
                                ? "25%"
                                : currentStep === "building_web"
                                ? "55%"
                                : currentStep === "building_binary"
                                ? "85%"
                                : "100%",
                          }}
                        />
                      </div>
                    </div>
                  )}

                  {/* Error display */}
                  {updateError && (
                    <div className="flex items-start gap-2 rounded-xl border border-rose-500/30 bg-rose-500/10 p-3.5 text-xs text-rose-300">
                      <AlertCircle className="h-4 w-4 shrink-0 text-rose-400 mt-0.5" />
                      <div className="flex-1 break-words leading-relaxed">{updateError}</div>
                    </div>
                  )}

                  {/* Live Logs console */}
                  {updateLogs.length > 0 && (
                    <div className="space-y-1.5">
                      <div className="flex items-center gap-1.5 text-[10.5px] font-semibold uppercase tracking-wider text-slate-500">
                        <Terminal className="h-3 w-3" />
                        <span>Nhật ký cập nhật</span>
                      </div>
                      <div className="rounded-xl border border-[#1f2638] bg-[#0c0e15] p-3.5 font-mono text-[11px] leading-relaxed text-slate-300 max-h-44 overflow-y-auto space-y-1">
                        {updateLogs.map((line, idx) => (
                          <div key={idx} className="whitespace-pre-wrap break-all">
                            {line}
                          </div>
                        ))}
                        <div ref={logEndRef} />
                      </div>
                    </div>
                  )}
                </div>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
};
