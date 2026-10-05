import React, { useCallback, useEffect, useRef, useState } from "react";
import {
  Check,
  CheckCircle2,
  AlertCircle,
  LogIn,
  Pencil,
  Plus,
  Trash2,
  Play,
  RefreshCw,
  ChevronLeft,
} from "lucide-react";
import type { AccountGroup, AccountInfo } from "../v2/types";
import { LoginPanel } from "../v2/LoginPanel";
import { ConfirmDialog } from "./ConfirmDialog";

interface EngineStatus {
  id: string;
  name: string;
  binary: string;
  installCmd?: string;
  installed: boolean;
  auth_status?: string;
  has_auth?: boolean;
}


const AGENT_META: Record<string, { label: string; badge: string; dot: string; border: string }> = {
  agy: { label: "Antigravity", badge: "bg-blue-500/15 text-blue-300 border-blue-500/30", dot: "bg-blue-400", border: "border-blue-500/30" },
  claude: { label: "Claude Code", badge: "bg-orange-500/15 text-orange-300 border-orange-500/30", dot: "bg-orange-400", border: "border-orange-500/30" },
  codex: { label: "Codex", badge: "bg-emerald-500/15 text-emerald-300 border-emerald-500/30", dot: "bg-emerald-400", border: "border-emerald-500/30" },
};

interface Props {
  engines: EngineStatus[];
  sessions: any[];
  onBackToTerminal: () => void;
  onRefreshEngines: () => void;
  initialAgent?: string | null;
}

export const AgentSettingsView: React.FC<Props> = ({
  engines,
  sessions,
  onBackToTerminal,
  onRefreshEngines,
  initialAgent,
}) => {
  const [selectedId, setSelectedId] = useState<string>(() => initialAgent || engines[0]?.id || "agy");
  const [groups, setGroups] = useState<AccountGroup[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [adding, setAdding] = useState<{ engine: string; label: string } | null>(null);
  const [renaming, setRenaming] = useState<{ id: string; label: string } | null>(null);
  const [removing, setRemoving] = useState<AccountInfo | null>(null);
  const [relogin, setRelogin] = useState<AccountInfo | null>(null);
  const [login, setLogin] = useState<{ id: string; replace: boolean } | null>(null);
  const [installing, setInstalling] = useState(false);
  const addRef = useRef<HTMLInputElement>(null);

  const currentEngine = engines.find((e) => e.id === selectedId) || engines[0];
  const meta = AGENT_META[selectedId];
  const engineSessions = sessions.filter((s) => s.agent === selectedId || s.current_agent === selectedId);

  const loadAccounts = useCallback((force = false) => {
    fetch(`/api/v2/accounts${force ? "?refresh=1" : ""}`)
      .then((r) => r.json())
      .then((g: AccountGroup[]) => setGroups(Array.isArray(g) ? g : []))
      .catch(() => setError("Không thể tải danh sách tài khoản"));
  }, []);

  useEffect(() => {
    loadAccounts(true);
  }, [loadAccounts]);

  useEffect(() => {
    if (adding) addRef.current?.focus();
  }, [adding]);

  const changed = () => {
    loadAccounts(true);
    onRefreshEngines();
  };

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

  const json = (body: object): RequestInit => ({
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });

  const create = async () => {
    if (!adding || !adding.label.trim()) return;
    const a = await call("/api/v2/accounts", {
      method: "POST",
      ...json({ engine: adding.engine, label: adding.label.trim() }),
    });
    if (a) {
      setAdding(null);
      changed();
      setLogin({ id: (a as { id: string }).id, replace: false });
    }
  };

  const use = async (a: AccountInfo) => {
    if (await call("/api/v2/accounts/active", { method: "PUT", ...json({ engine: a.engine, id: a.id }) })) {
      changed();
    }
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
    if (await call(`/api/v2/accounts/${encodeURIComponent(a.id)}`, { method: "DELETE" })) {
      changed();
    }
  };

  const installEngine = async (eng: EngineStatus) => {
    if (!eng.installCmd) return;
    setInstalling(true);
    try {
      const r = await fetch("/api/agents/install", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ command: eng.installCmd, binary: eng.binary }),
      });
      const d = await r.json();
      if (!d.success) alert(`Cài đặt thất bại: ${d.error || d.output}`);
      onRefreshEngines();
    } finally {
      setInstalling(false);
    }
  };

  const currentGroup = groups?.find((g) => g.engine === selectedId);

  return (
    <div className="flex h-full flex-col overflow-y-auto bg-[#0b0e14] p-4 sm:p-6 text-slate-200">
      <div className="mx-auto w-full max-w-4xl space-y-6">
        {/* Header navigation */}
        <div className="flex items-center justify-between border-b border-[#1d222b] pb-4">
          <div>
            <h1 className="text-lg font-semibold text-slate-100">Cài đặt Agent</h1>
            <p className="text-xs text-slate-500 mt-0.5">
              Cấu hình CLI, trạng thái cài đặt và quản lý tài khoản theo từng Agent.
            </p>
          </div>
          <button
            onClick={onBackToTerminal}
            className="flex cursor-pointer items-center gap-1.5 rounded-lg border border-[#2c3447] bg-[#171b25] px-3 py-1.5 text-xs font-medium text-slate-300 hover:bg-[#202736] hover:text-slate-100 transition-colors"
          >
            <ChevronLeft className="h-4 w-4" />
            Quay lại terminal
          </button>
        </div>

        {/* Tab Strip: Each agent is a tab */}
        <div>
          <div className="flex items-center gap-1.5 border-b border-[#202634] pb-2 overflow-x-auto">
            {engines.map((eng) => {
              const isSelected = eng.id === selectedId;
              const engMeta = AGENT_META[eng.id];
              const engGroup = groups?.find((g) => g.engine === eng.id);
              const accCount = engGroup?.accounts?.length || 0;
              return (
                <button
                  key={eng.id}
                  onClick={() => setSelectedId(eng.id)}
                  className={`flex shrink-0 cursor-pointer items-center gap-2 rounded-lg px-4 py-2 text-xs font-medium transition-all ${
                    isSelected
                      ? "bg-[#1c2230] text-slate-100 border border-[#2d374b] shadow-sm"
                      : "text-slate-400 hover:bg-[#141822] hover:text-slate-200 border border-transparent"
                  }`}
                >
                  <span className={`h-2 w-2 rounded-full ${engMeta?.dot || "bg-slate-500"}`} />
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
                    <span className="rounded bg-sky-950/40 px-1.5 py-0.5 text-[10px] text-sky-400 font-mono">
                      {accCount}
                    </span>
                  )}
                </button>
              );
            })}
          </div>
        </div>

        {/* Tab Content for Selected Agent */}
        {currentEngine && (
          <div className="space-y-6">
            {/* Status & CLI Section */}
            <div className="rounded-xl border border-[#202737] bg-[#121620] p-4 sm:p-5">
              <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
                <div className="flex items-center gap-3">
                  <span className={`h-3 w-3 rounded-full ${meta?.dot || "bg-slate-400"}`} />
                  <div>
                    <div className="flex items-center gap-2">
                      <span className="text-base font-semibold text-slate-100">{currentEngine.name}</span>
                      <span className="font-mono text-xs text-slate-500">({currentEngine.binary})</span>
                      {currentEngine.installed ? (
                        <span className="flex items-center gap-1 rounded bg-emerald-500/15 px-2 py-0.5 text-[11px] font-medium text-emerald-400">
                          <CheckCircle2 className="h-3 w-3" /> Đã cài đặt
                        </span>
                      ) : (
                        <span className="flex items-center gap-1 rounded bg-amber-500/15 px-2 py-0.5 text-[11px] font-medium text-amber-400">
                          <AlertCircle className="h-3 w-3" /> Chưa cài đặt
                        </span>
                      )}
                    </div>
                    <div className="mt-1 text-xs text-slate-400">
                      {currentEngine.installed
                        ? currentEngine.auth_status || "Sẵn sàng hoạt động"
                        : "Chưa tìm thấy lệnh CLI này trên hệ thống"}
                      {" · "}
                      <span className="text-slate-500">{engineSessions.length} session đang lưu</span>
                    </div>
                  </div>
                </div>

                {!currentEngine.installed && currentEngine.installCmd && (
                  <button
                    onClick={() => installEngine(currentEngine)}
                    disabled={installing}
                    className="flex cursor-pointer items-center gap-1.5 rounded-lg bg-emerald-600 px-3.5 py-2 text-xs font-semibold text-white hover:bg-emerald-500 disabled:opacity-50 transition-colors"
                  >
                    {installing ? <RefreshCw className="h-3.5 w-3.5 animate-spin" /> : <Play className="h-3.5 w-3.5" />}
                    Cài đặt ngay
                  </button>
                )}
              </div>
            </div>

            {/* Accounts Management Section */}
            <div className="rounded-xl border border-[#202737] bg-[#121620] p-4 sm:p-5">
              <div className="mb-4 flex items-center justify-between border-b border-[#1e2535] pb-3">
                <div>
                  <h3 className="text-sm font-semibold text-slate-100">
                    Tài khoản {currentEngine.name}
                  </h3>
                  <p className="text-[11.5px] text-slate-500 mt-0.5">
                    Quản lý danh sách tài khoản, đăng nhập và hạn mức (quota) cho {currentEngine.name}.
                  </p>
                </div>
                {selectedId !== "agy" && !adding && (
                  <button
                    onClick={() => setAdding({ engine: selectedId, label: "" })}
                    className="flex cursor-pointer items-center gap-1 rounded-md border border-indigo-500/40 bg-indigo-600/20 px-2.5 py-1 text-xs font-medium text-indigo-300 hover:bg-indigo-600/30 transition-colors"
                  >
                    <Plus className="h-3.5 w-3.5" />
                    Thêm tài khoản
                  </button>
                )}
              </div>

              {error && (
                <div className="mb-4 rounded-lg border border-rose-800/50 bg-rose-950/30 px-3 py-2 text-xs text-rose-300">
                  {error}
                </div>
              )}

              {/* Accounts list */}
              <div className="space-y-2.5">
                {!currentGroup && !error && (
                  <div className="py-4 text-center text-xs text-slate-500">Đang tải danh sách tài khoản…</div>
                )}

                {currentGroup?.accounts.length === 0 && (
                  <div className="py-6 text-center text-xs text-slate-500">
                    Chưa có tài khoản nào được lưu cho agent này.
                  </div>
                )}

                {currentGroup?.accounts.map((a) => (
                  <div
                    key={a.id}
                    className={`rounded-xl border px-3.5 py-2.5 transition-colors ${
                      a.active ? "border-sky-700/60 bg-sky-950/20" : "border-[#242b3b] bg-[#141824]/60"
                    }`}
                  >
                    <div className="flex items-center justify-between gap-3">
                      <div className="min-w-0">
                        {renaming?.id === a.id ? (
                          <input
                            autoFocus
                            value={renaming.label}
                            onChange={(e) => setRenaming({ id: a.id, label: e.target.value })}
                            onKeyDown={(e) => {
                              if (e.key === "Enter") rename();
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
                            {a.logged_in ? "Đã đăng nhập" : a.known ? "Chưa đăng nhập" : "Không xác định"}
                          </span>
                          {a.detail && (
                            <span className="truncate text-slate-500 font-mono text-[10.5px]">· {a.detail}</span>
                          )}
                        </div>
                      </div>

                      <div className="flex shrink-0 items-center gap-1.5">
                        {!a.active && (
                          <button
                            onClick={() => use(a)}
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
                        {!a.default && selectedId !== "agy" && (
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
                ))}

                {/* Add account input field */}
                {adding && adding.engine === selectedId && (
                  <div className="mt-3 flex items-center gap-2 rounded-xl border border-[#2b3548] bg-[#141824] p-2.5">
                    <input
                      ref={addRef}
                      value={adding.label}
                      onChange={(e) => setAdding({ engine: selectedId, label: e.target.value })}
                      onKeyDown={(e) => {
                        if (e.key === "Enter") create();
                        if (e.key === "Escape") setAdding(null);
                      }}
                      placeholder="Tên tài khoản (ví dụ: Công việc, Cá nhân)"
                      className="flex-1 rounded-md border border-sky-600/50 bg-[#0f1218] px-2.5 py-1.5 text-xs text-slate-100 outline-none"
                    />
                    <button
                      onClick={create}
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

                {selectedId === "agy" && (
                  <p className="mt-2 text-[11px] text-slate-500 leading-relaxed">
                    Hồ sơ Antigravity được đồng bộ từ AGY Manager / ADC. Việc đổi hồ sơ sẽ áp dụng cho tất cả phiên làm việc Antigravity trên máy chủ này.
                  </p>
                )}
              </div>
            </div>
          </div>
        )}
      </div>

      {login && (
        <LoginPanel
          engine={login.id}
          replace={login.replace}
          onClose={() => {
            setLogin(null);
            changed();
          }}
          onDone={changed}
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
          onConfirm={() => remove(removing)}
          onCancel={() => setRemoving(null)}
        />
      )}
    </div>
  );
};
