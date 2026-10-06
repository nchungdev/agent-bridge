import React, { useCallback, useEffect, useRef, useState } from "react";
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
  Loader2,
  Shield,
  Info,
  Check,
  LogIn,
  Pencil,
  Trash2,
  Plus,
  Play,
} from "lucide-react";
import type { AccountGroup, AccountInfo } from "../v2/types";
import { LoginPanel } from "../v2/LoginPanel";
import { ConfirmDialog } from "./ConfirmDialog";

export interface UpdateStatus {
  current_version?: string;
  latest_version?: string;
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

  // Agent accounts & profiles state
  const [selectedAgentId, setSelectedAgentId] = useState<string>(() => engines[0]?.id || "agy");
  const [groups, setGroups] = useState<AccountGroup[] | null>(null);
  const [accountError, setAccountError] = useState<string | null>(null);
  const [adding, setAdding] = useState<{ engine: string; label: string } | null>(null);
  const [renaming, setRenaming] = useState<{ id: string; label: string } | null>(null);
  const [removing, setRemoving] = useState<AccountInfo | null>(null);
  const [relogin, setRelogin] = useState<AccountInfo | null>(null);
  const [login, setLogin] = useState<{ id: string; replace: boolean } | null>(null);
  const [installingEngine, setInstallingEngine] = useState(false);
  const [refreshingEngines, setRefreshingEngines] = useState(false);
  const addRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (adding) addRef.current?.focus();
  }, [adding]);

  const loadAccounts = useCallback((force = false) => {
    fetch(`/api/v2/accounts${force ? "?refresh=1" : ""}`)
      .then((r) => r.json())
      .then((g: AccountGroup[]) => setGroups(Array.isArray(g) ? g : []))
      .catch(() => setAccountError("Không thể tải danh sách tài khoản"));
  }, []);

  useEffect(() => {
    if (isOpen) {
      loadAccounts(true);
    }
  }, [isOpen, loadAccounts]);

  const changedAccounts = () => {
    loadAccounts(true);
    onRefreshEngines();
  };

  const handleRefreshEngines = async () => {
    setRefreshingEngines(true);
    try {
      await onRefreshEngines();
      await loadAccounts(true);
    } finally {
      setTimeout(() => setRefreshingEngines(false), 400);
    }
  };

  const callAccountApi = async (url: string, init: RequestInit) => {
    setAccountError(null);
    const r = await fetch(url, init);
    if (!r.ok) {
      const e = await r.json().catch(() => ({ error: r.statusText }));
      setAccountError(e.error ?? r.statusText);
      return null;
    }
    return r.status === 204 ? {} : r.json();
  };

  const jsonBody = (body: object): RequestInit => ({
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });

  const createAccount = async () => {
    if (!adding || !adding.label.trim()) return;
    const a = await callAccountApi("/api/v2/accounts", {
      method: "POST",
      ...jsonBody({ engine: adding.engine, label: adding.label.trim() }),
    });
    if (a) {
      setAdding(null);
      changedAccounts();
      setLogin({ id: (a as { id: string }).id, replace: false });
    }
  };

  const useAccount = async (a: AccountInfo) => {
    if (await callAccountApi("/api/v2/accounts/active", { method: "PUT", ...jsonBody({ engine: a.engine, id: a.id }) })) {
      changedAccounts();
    }
  };

  const renameAccount = async () => {
    if (!renaming || !renaming.label.trim()) return;
    if (await callAccountApi(`/api/v2/accounts/${encodeURIComponent(renaming.id)}`, { method: "PATCH", ...jsonBody({ label: renaming.label.trim() }) })) {
      setRenaming(null);
      changedAccounts();
    }
  };

  const removeAccount = async (a: AccountInfo) => {
    setRemoving(null);
    if (await callAccountApi(`/api/v2/accounts/${encodeURIComponent(a.id)}`, { method: "DELETE" })) {
      changedAccounts();
    }
  };

  const installEngine = async (eng: EngineStatus) => {
    if (!eng.installCmd) return;
    setInstallingEngine(true);
    try {
      const r = await fetch("/api/agents/install", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ command: eng.installCmd, binary: eng.binary }),
      });
      const d = await r.json();
      if (!d.success) alert(`Cài đặt thất bại: ${d.error || d.output}`);
      changedAccounts();
    } catch (e: any) {
      alert(`Lỗi: ${e.message}`);
    } finally {
      setInstallingEngine(false);
    }
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
            <span>Agent Bridge v1.0.0</span>
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
            {activeTab === "agents" && (() => {
              const selectedEngine = engines.find((e) => e.id === selectedAgentId) || engines[0];
              const activeBaseKey = selectedEngine ? (selectedEngine.base || selectedEngine.id) : "agy";
              const activeMeta = AGENT_META_COLORS[activeBaseKey] || {
                badge: "bg-purple-500/15 text-purple-300 border-purple-500/30",
                dot: "bg-purple-400",
                border: "border-purple-500/30",
              };
              const currentGroup = groups?.find((g) => g.engine === selectedEngine?.id);

              return (
                <div className="space-y-6 max-w-2xl">
                  {/* Header: title + Quét lại button (nowrap & shrink-0) */}
                  <div className="flex items-start justify-between gap-4">
                    <div className="min-w-0 pr-2">
                      <h1 className="text-xl font-bold text-slate-100">Agents</h1>
                      <p className="text-xs text-slate-400 mt-1">
                        Cấu hình CLI, trạng thái cài đặt và quản lý tài khoản theo từng Agent.
                      </p>
                    </div>
                    <button
                      onClick={handleRefreshEngines}
                      disabled={refreshingEngines}
                      className="shrink-0 whitespace-nowrap flex cursor-pointer items-center gap-1.5 rounded-lg border border-[#2b3548] bg-[#161a25] px-3 py-1.5 text-xs font-medium text-slate-300 hover:bg-[#202636] hover:text-white transition-colors"
                      title="Quét lại các agent trong PATH"
                    >
                      <RefreshCw className={`h-3.5 w-3.5 ${refreshingEngines ? "animate-spin" : ""}`} />
                      <span>Quét lại</span>
                    </button>
                  </div>

                  {/* Agent Selector Sub-tabs */}
                  <div className="flex items-center gap-2 border-b border-[#202737] pb-2 overflow-x-auto">
                    {engines.map((eng) => {
                      const isSelected = eng.id === (selectedEngine?.id || selectedAgentId);
                      const baseKey = eng.base || eng.id;
                      const meta = AGENT_META_COLORS[baseKey] || {
                        badge: "bg-purple-500/15 text-purple-300 border-purple-500/30",
                        dot: "bg-purple-400",
                        border: "border-purple-500/30",
                      };
                      const engGroup = groups?.find((g) => g.engine === eng.id);
                      const accCount = engGroup?.accounts?.length || 0;

                      return (
                        <button
                          key={eng.id}
                          onClick={() => setSelectedAgentId(eng.id)}
                          className={`flex shrink-0 cursor-pointer items-center gap-2 rounded-lg px-3.5 py-1.5 text-xs font-medium transition-all ${
                            isSelected
                              ? "bg-[#1c2230] text-slate-100 border border-[#2d374b] shadow-sm"
                              : "text-slate-400 hover:bg-[#141822] hover:text-slate-200 border border-transparent"
                          }`}
                        >
                          <span className={`h-2 w-2 rounded-full ${meta.dot}`} />
                          <span className="font-semibold">{eng.name}</span>
                          {eng.installed ? (
                            <span className="rounded bg-emerald-500/15 px-1.5 py-0.5 text-[10px] text-emerald-400 font-normal">
                              Đã cài
                            </span>
                          ) : (
                            <span className="rounded bg-slate-800 px-1.5 py-0.5 text-[10px] text-slate-500 font-normal">
                              Chưa cài
                            </span>
                          )}
                          {accCount > 0 && (
                            <span className="rounded bg-sky-950/50 px-1.5 py-0.5 text-[10px] text-sky-400 font-mono">
                              {accCount}
                            </span>
                          )}
                        </button>
                      );
                    })}
                  </div>

                  {/* Active Agent Configuration Details */}
                  {selectedEngine && (
                    <div className="space-y-5">
                      {/* Status & CLI Section */}
                      <div className="rounded-xl border border-[#202738] bg-[#131722] p-4 flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
                        <div className="flex items-center gap-3 min-w-0">
                          <span className={`h-3 w-3 rounded-full shrink-0 ${activeMeta.dot}`} />
                          <div className="min-w-0">
                            <div className="flex items-center gap-2">
                              <span className="text-sm font-semibold text-slate-100 truncate">{selectedEngine.name}</span>
                              <span className={`rounded border px-1.5 py-0.2 text-[10px] font-mono ${activeMeta.badge}`}>
                                {selectedEngine.id}
                              </span>
                              {selectedEngine.installed ? (
                                <span className="flex items-center gap-1 rounded bg-emerald-500/15 px-2 py-0.5 text-[10.5px] font-medium text-emerald-400">
                                  <CheckCircle2 className="h-3 w-3" /> Đã cài đặt
                                </span>
                              ) : (
                                <span className="flex items-center gap-1 rounded bg-amber-500/15 px-2 py-0.5 text-[10.5px] font-medium text-amber-400">
                                  <AlertCircle className="h-3 w-3" /> Chưa cài đặt
                                </span>
                              )}
                            </div>
                            <div className="text-[11px] text-slate-500 font-mono truncate mt-0.5" title={selectedEngine.binary}>
                              {selectedEngine.binary} · {selectedEngine.installed ? (selectedEngine.auth_status || "Sẵn sàng hoạt động trong PATH") : "Chưa tìm thấy lệnh CLI này trong PATH"}
                            </div>
                          </div>
                        </div>

                        <div className="shrink-0 flex items-center gap-2">
                          {selectedEngine.installed ? (
                            <button
                              onClick={() => {
                                onClose();
                                onOpenAgentTerminal(selectedEngine.id);
                              }}
                              className="flex cursor-pointer items-center gap-1.5 rounded-lg border border-[#2e374d] bg-[#191f2c] px-3 py-1.5 text-xs font-medium text-slate-200 hover:bg-[#232c3f] hover:text-white transition-colors shadow-sm"
                              title={`Mở terminal chạy trực tiếp CLI ${selectedEngine.id}`}
                            >
                              <Terminal className="h-3.5 w-3.5 text-sky-400" />
                              <span>Mở CLI</span>
                            </button>
                          ) : (
                            selectedEngine.installCmd && (
                              <button
                                onClick={() => installEngine(selectedEngine)}
                                disabled={installingEngine}
                                className="flex cursor-pointer items-center gap-1.5 rounded-lg bg-emerald-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-emerald-500 disabled:opacity-50 transition-colors shadow-sm"
                              >
                                {installingEngine ? <RefreshCw className="h-3.5 w-3.5 animate-spin" /> : <Play className="h-3.5 w-3.5" />}
                                <span>Cài đặt ngay</span>
                              </button>
                            )
                          )}
                        </div>
                      </div>

                      {/* Accounts Management Section */}
                      <div className="rounded-xl border border-[#202738] bg-[#131722] p-4 sm:p-5">
                        <div className="mb-4 flex items-center justify-between border-b border-[#1e2535] pb-3">
                          <div>
                            <h3 className="text-sm font-semibold text-slate-100">
                              Tài khoản {selectedEngine.name}
                            </h3>
                            <p className="text-[11.5px] text-slate-500 mt-0.5">
                              Quản lý danh sách tài khoản, chuyển đổi hồ sơ và phiên đăng nhập.
                            </p>
                          </div>
                          {selectedEngine.id !== "agy" && !adding && (
                            <button
                              onClick={() => setAdding({ engine: selectedEngine.id, label: "" })}
                              className="flex cursor-pointer items-center gap-1 rounded-md border border-indigo-500/40 bg-indigo-600/20 px-2.5 py-1 text-xs font-medium text-indigo-300 hover:bg-indigo-600/30 transition-colors"
                            >
                              <Plus className="h-3.5 w-3.5" />
                              <span>Thêm tài khoản</span>
                            </button>
                          )}
                        </div>

                        {accountError && (
                          <div className="mb-3 rounded-lg border border-rose-500/30 bg-rose-500/10 p-2.5 text-xs text-rose-300">
                            {accountError}
                          </div>
                        )}

                        {/* Accounts list */}
                        <div className="space-y-2.5">
                          {currentGroup?.accounts && currentGroup.accounts.length > 0 ? (
                            currentGroup.accounts.map((a) => (
                              <div
                                key={a.id}
                                className={`rounded-xl border p-3 transition-colors ${
                                  a.active
                                    ? "border-sky-500/40 bg-[#161c28]"
                                    : "border-[#202737] bg-[#10131c] hover:border-[#2b3548]"
                                }`}
                              >
                                <div className="flex items-center justify-between gap-3">
                                  <div className="min-w-0 flex-1">
                                    {renaming?.id === a.id ? (
                                      <input
                                        value={renaming.label}
                                        autoFocus
                                        onChange={(e) => setRenaming({ id: a.id, label: e.target.value })}
                                        onKeyDown={(e) => {
                                          if (e.key === "Enter") renameAccount();
                                          if (e.key === "Escape") setRenaming(null);
                                        }}
                                        onBlur={() => setRenaming(null)}
                                        className="w-44 rounded-md border border-sky-600/60 bg-[#0f1218] px-2 py-0.5 text-xs text-slate-100 outline-none"
                                      />
                                    ) : (
                                      <div className="flex items-center gap-2">
                                        <span className="truncate text-xs font-semibold text-slate-100">
                                          {a.default ? "Tài khoản mặc định" : a.label}
                                        </span>
                                        {a.active && (
                                          <span className="flex items-center gap-1 rounded bg-sky-900/50 px-1.5 py-0.5 text-[10px] font-medium text-sky-300">
                                            <Check className="h-3 w-3" /> Đang dùng
                                          </span>
                                        )}
                                      </div>
                                    )}
                                    <div className="mt-1 flex items-center gap-2 text-[11px]">
                                      <span
                                        className={`h-1.5 w-1.5 rounded-full ${
                                          a.logged_in ? "bg-emerald-400" : a.known ? "bg-rose-400" : "bg-slate-500"
                                        }`}
                                      />
                                      <span className={a.logged_in ? "text-emerald-300" : "text-slate-400"}>
                                        {a.logged_in ? "Đã đăng nhập" : a.known ? "Chưa đăng nhập" : "Chưa xác định"}
                                      </span>
                                      {a.detail && (
                                        <span className="truncate text-slate-500 font-mono text-[10.5px]">· {a.detail}</span>
                                      )}
                                    </div>
                                  </div>

                                  <div className="flex shrink-0 items-center gap-1.5">
                                    {!a.active && (
                                      <button
                                        onClick={() => useAccount(a)}
                                        className="cursor-pointer rounded-md bg-[#222938] px-2.5 py-1 text-[11px] font-medium text-slate-200 hover:bg-[#2b3447] transition-colors"
                                      >
                                        Sử dụng
                                      </button>
                                    )}
                                    {a.can_login && (
                                      <button
                                        onClick={() => (a.logged_in ? setRelogin(a) : setLogin({ id: a.id, replace: false }))}
                                        className={`flex cursor-pointer items-center gap-1 rounded-md px-2.5 py-1 text-[11px] font-medium transition-colors ${
                                          a.logged_in
                                            ? "bg-[#222938] text-slate-300 hover:bg-[#2b3447]"
                                            : "bg-sky-600 text-white hover:bg-sky-500"
                                        }`}
                                      >
                                        <LogIn className="h-3 w-3" />
                                        {a.logged_in ? "Đăng nhập lại" : "Đăng nhập"}
                                      </button>
                                    )}
                                    {!a.default && selectedEngine.id !== "agy" && (
                                      <>
                                        <button
                                          onClick={() => setRenaming({ id: a.id, label: a.label })}
                                          className="cursor-pointer rounded-md p-1.5 text-slate-400 hover:bg-[#222938] hover:text-slate-100 transition-colors"
                                          title="Đổi tên"
                                        >
                                          <Pencil className="h-3.5 w-3.5" />
                                        </button>
                                        <button
                                          onClick={() => setRemoving(a)}
                                          className="cursor-pointer rounded-md p-1.5 text-slate-400 hover:bg-rose-950/40 hover:text-rose-300 transition-colors"
                                          title="Xóa"
                                        >
                                          <Trash2 className="h-3.5 w-3.5" />
                                        </button>
                                      </>
                                    )}
                                  </div>
                                </div>
                              </div>
                            ))
                          ) : (
                            <div className="rounded-xl border border-dashed border-[#232b3b] p-4 text-center text-xs text-slate-500">
                              Chưa tìm thấy thông tin tài khoản cho agent này.
                            </div>
                          )}

                          {/* Add account input field */}
                          {adding && adding.engine === selectedEngine.id && (
                            <div className="mt-3 flex items-center gap-2 rounded-xl border border-[#2b3548] bg-[#141824] p-2.5">
                              <input
                                ref={addRef}
                                value={adding.label}
                                onChange={(e) => setAdding({ engine: selectedEngine.id, label: e.target.value })}
                                onKeyDown={(e) => {
                                  if (e.key === "Enter") createAccount();
                                  if (e.key === "Escape") setAdding(null);
                                }}
                                placeholder="Tên tài khoản (ví dụ: Công việc, Cá nhân)"
                                className="flex-1 rounded-md border border-sky-600/50 bg-[#0f1218] px-2.5 py-1.5 text-xs text-slate-100 outline-none"
                              />
                              <button
                                onClick={createAccount}
                                disabled={!adding.label.trim()}
                                className="cursor-pointer rounded-md bg-sky-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-sky-500 disabled:opacity-40 transition-colors"
                              >
                                Tạo &amp; Đăng nhập
                              </button>
                              <button
                                onClick={() => setAdding(null)}
                                className="cursor-pointer rounded-md bg-[#222938] px-2.5 py-1.5 text-xs text-slate-300 hover:bg-[#2b3447] transition-colors"
                              >
                                Hủy
                              </button>
                            </div>
                          )}

                          {selectedEngine.id === "agy" && (
                            <p className="mt-2 text-[11px] text-slate-500 leading-relaxed">
                              Hồ sơ Antigravity được đồng bộ từ AGY Manager / ADC. Việc đổi hồ sơ sẽ áp dụng cho tất cả phiên làm việc Antigravity trên máy chủ này.
                            </p>
                          )}
                        </div>
                      </div>

                      {/* Helpful note banner */}
                      <div className="rounded-xl border border-[#222a3b] bg-[#111520] p-4 flex items-start gap-3">
                        <Info className="h-4 w-4 text-sky-400 shrink-0 mt-0.5" />
                        <div className="text-[11.5px] text-slate-400 leading-relaxed">
                          <strong className="text-slate-200">Nguyên tắc cấu hình độc lập:</strong> Mỗi agent CLI (Claude, Antigravity, Codex) lưu trữ thông tin đăng nhập, token và thiết lập trong thư mục người dùng riêng (<code className="font-mono text-slate-300">~/.claude</code>, <code className="font-mono text-slate-300">~/.gemini</code>, <code className="font-mono text-slate-300">~/.codex</code>). Khi bạn cần thay đổi tham số hoặc login nâng cao, chỉ cần bấm <strong>Mở CLI</strong> để thao tác trực tiếp.
                        </div>
                      </div>
                    </div>
                  )}
                </div>
              );
            })()}

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
                  <h1 className="text-xl font-bold text-slate-100">Updates</h1>
                  <p className="text-xs text-slate-400 mt-1">
                    Tự động kiểm tra phiên bản mới từ GitHub và cập nhật nhanh chỉ với 1 click.
                  </p>
                </div>

                <div className="space-y-4">
                  {/* Clean & Simple Version Card */}
                  <div className="rounded-2xl border border-[#22293b] bg-[#121622] p-5 space-y-4 shadow-sm">
                    <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
                      <div className="space-y-1">
                        <div className="text-[11px] font-semibold uppercase tracking-wider text-slate-400">
                          Phiên bản hiện tại
                        </div>
                        <div className="flex items-baseline gap-2">
                          <span className="text-2xl font-bold font-mono text-slate-100">
                            {updateStatus?.current_version || "v1.0.0"}
                          </span>
                        </div>
                      </div>

                      {updateStatus?.has_update ? (
                        <div className="inline-flex items-center gap-2 rounded-full border border-amber-500/30 bg-amber-500/15 px-3.5 py-1.5 text-xs font-medium text-amber-300">
                          <span className="h-2 w-2 rounded-full bg-amber-400 animate-pulse" />
                          <span>Có phiên bản mới: <strong>{updateStatus.latest_version || "mới nhất"}</strong></span>
                        </div>
                      ) : (
                        <div className="inline-flex items-center gap-2 rounded-full border border-emerald-500/30 bg-emerald-500/15 px-3.5 py-1.5 text-xs font-medium text-emerald-300">
                          <CheckCircle2 className="h-4 w-4" />
                          <span>Bạn đang dùng phiên bản mới nhất</span>
                        </div>
                      )}
                    </div>

                    <div className="border-t border-[#1e2537] pt-3 flex flex-col sm:flex-row sm:items-center sm:justify-between gap-2 text-[11.5px] text-slate-400">
                      <div>
                        Tự động kiểm tra: mỗi 24 giờ &amp; mỗi lần vào ứng dụng.
                      </div>
                      {updateStatus?.last_checked && (
                        <div className="text-slate-500 font-mono text-[11px]">
                          Kiểm tra gần nhất: {new Date(updateStatus.last_checked).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
                        </div>
                      )}
                    </div>
                  </div>

                  {/* Actions */}
                  <div className="flex items-center gap-3">
                    {updateStatus?.has_update && (
                      <button
                        onClick={handleApplyUpdate}
                        disabled={checkingUpdate || updatingApp}
                        className="flex cursor-pointer items-center gap-2 rounded-xl bg-indigo-600 px-5 py-2.5 text-xs font-semibold text-white shadow-lg shadow-indigo-600/30 hover:bg-indigo-500 disabled:opacity-50 transition-all"
                      >
                        <ArrowUpCircle className="h-4 w-4" />
                        <span>{updatingApp ? "Đang cập nhật..." : `Cập nhật lên ${updateStatus.latest_version || "mới nhất"}`}</span>
                      </button>
                    )}

                    <button
                      onClick={handleCheckUpdate}
                      disabled={checkingUpdate || updatingApp}
                      className="flex cursor-pointer items-center gap-1.5 rounded-xl border border-[#2b3548] bg-[#161a25] px-4 py-2.5 text-xs font-medium text-slate-200 hover:bg-[#202737] hover:text-white disabled:opacity-40 transition-colors"
                    >
                      <RefreshCw className={`h-3.5 w-3.5 ${checkingUpdate ? "animate-spin" : ""}`} />
                      <span>{checkingUpdate ? "Đang kiểm tra..." : "Kiểm tra cập nhật"}</span>
                    </button>
                  </div>

                  {/* Progress bar when updating */}
                  {updatingApp && (
                    <div className="rounded-xl border border-indigo-500/20 bg-indigo-500/5 p-4 space-y-2.5">
                      <div className="flex items-center justify-between text-xs">
                        <span className="font-medium text-indigo-300 flex items-center gap-2">
                          <Loader2 className="h-3.5 w-3.5 animate-spin text-indigo-400" />
                          {currentStep === "pulling" && "1/4. Đang tải mã nguồn từ GitHub..."}
                          {currentStep === "building_web" && "2/4. Đang biên dịch giao diện Web..."}
                          {currentStep === "building_binary" && "3/4. Đang biên dịch ứng dụng Go..."}
                          {currentStep === "restarting" && "4/4. Đang khởi động lại dịch vụ..."}
                          {currentStep === "success" && "Hoàn thành! Đang kết nối lại..."}
                        </span>
                        <span className="text-[11px] font-mono text-indigo-400">
                          {reconnecting ? "Đang kết nối lại" : "Đang xử lý"}
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
                        <span>Nhật ký tiến trình</span>
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

      {login && (
        <LoginPanel
          engine={login.id}
          replace={login.replace}
          onClose={() => {
            setLogin(null);
            changedAccounts();
          }}
          onDone={changedAccounts}
        />
      )}

      {relogin && (
        <ConfirmDialog
          danger
          title="Đăng nhập lại?"
          message={
            <>
              Hành động này sẽ thay thế phiên đăng nhập của{" "}
              <b>{relogin.default ? "tài khoản mặc định" : relogin.label}</b>
              {relogin.detail ? <> ({relogin.detail})</> : null}. Sử dụng thao tác này khi bạn muốn chuyển tài khoản sang người dùng khác.
            </>
          }
          confirmLabel="Đăng nhập lại"
          onConfirm={() => {
            setLogin({ id: relogin.id, replace: true });
            setRelogin(null);
          }}
          onCancel={() => setRelogin(null)}
        />
      )}

      {removing && (
        <ConfirmDialog
          danger
          title="Xóa tài khoản?"
          message={
            <>
              Tài khoản “{removing.label}” và phiên đăng nhập đã lưu sẽ bị xóa khỏi máy chủ. Các session đã chạy trước đó vẫn được giữ nguyên lịch sử.
            </>
          }
          confirmLabel="Xóa"
          onConfirm={() => removeAccount(removing)}
          onCancel={() => setRemoving(null)}
        />
      )}
    </div>
  );
};
