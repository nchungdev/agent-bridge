import React, { useEffect, useState } from "react";
import { Info, RefreshCw } from "lucide-react";
import type { EngineStatus } from "../../types/bridge";
import type { AccountGroup, AccountInfo } from "../../v2/types";
import { LoginPanel } from "../../v2/LoginPanel";
import { ConfirmDialog } from "../ConfirmDialog";
import { ProviderQuota } from "../ProviderQuota";
import { AgentStatusCard } from "./AgentStatusCard";
import { AccountList } from "./AccountList";
import { AgentSubTabs } from "./AgentSubTabs";

const AGENT_META_COLORS: Record<string, { badge: string; dot: string; border: string }> = {
  agy: { badge: "bg-blue-500/15 text-blue-300 border-blue-500/30", dot: "bg-blue-400", border: "border-blue-500/30" },
  claude: { badge: "bg-orange-500/15 text-orange-300 border-orange-500/30", dot: "bg-orange-400", border: "border-orange-500/30" },
  codex: { badge: "bg-emerald-500/15 text-emerald-300 border-emerald-500/30", dot: "bg-emerald-400", border: "border-emerald-500/30" },
};

interface Props {
  engines: EngineStatus[];
  onRefreshEngines: () => void;
  onOpenAgentTerminal: (agentId: string) => void;
}

export const AgentsTab: React.FC<Props> = ({ engines, onRefreshEngines, onOpenAgentTerminal }) => {
  const [selectedAgentId, setSelectedAgentId] = useState<string>(engines[0]?.id || "agy");
  const [refreshingEngines, setRefreshingEngines] = useState(false);
  const [installingEngine, setInstallingEngine] = useState(false);

  // Accounts state
  const [groups, setGroups] = useState<AccountGroup[] | null>(null);
  const [accountError, setAccountError] = useState<string | null>(null);
  const [login, setLogin] = useState<{ id: string; replace: boolean } | null>(null);
  const [relogin, setRelogin] = useState<AccountInfo | null>(null);
  const [removing, setRemoving] = useState<AccountInfo | null>(null);
  const [adding, setAdding] = useState<{ engine: string; label: string } | null>(null);
  const [renaming, setRenaming] = useState<{ id: string; label: string } | null>(null);

  // force re-reads each CLI's login state (which account is signed in); without it the server may answer from cache
  const loadAccounts = async (force = false) => {
    try {
      const r = await fetch(`/api/v2/accounts${force ? "?refresh=1" : ""}`);
      const body = await r.json().catch(() => null);
      if (!r.ok || !Array.isArray(body)) {
        throw new Error((body && typeof body === "object" && (body as { error?: string }).error) || `HTTP ${r.status}`);
      }
      setGroups(body as AccountGroup[]);
    } catch (err: any) {
      // never swallow this: an empty list would otherwise read as "this agent has no account"
      setAccountError(`Không đọc được danh sách tài khoản: ${err?.message || err}`);
      setGroups((prev) => prev ?? []);
    }
  };

  useEffect(() => {
    loadAccounts(true);
  }, []);

  const handleRefresh = async () => {
    setRefreshingEngines(true);
    setAccountError(null);
    try {
      await onRefreshEngines();
      await loadAccounts(true);
    } finally {
      setRefreshingEngines(false);
    }
  };

  const useAccount = async (a: AccountInfo) => {
    setAccountError(null);
    try {
      const r = await fetch("/api/v2/accounts/active", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ engine: a.engine, id: a.id }),
      });
      if (!r.ok) {
        const d = await r.json().catch(() => ({}));
        throw new Error(d.error || "Không thể chuyển tài khoản");
      }
      await loadAccounts(true);
    } catch (err: any) {
      setAccountError(err.message);
    }
  };

  const renameAccount = async () => {
    if (!renaming || !renaming.label.trim()) return;
    setAccountError(null);
    try {
      const r = await fetch(`/api/v2/accounts/${encodeURIComponent(renaming.id)}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ label: renaming.label.trim() }),
      });
      if (!r.ok) {
        const d = await r.json().catch(() => ({}));
        throw new Error(d.error || "Không thể đổi tên");
      }
      setRenaming(null);
      await loadAccounts(true);
    } catch (err: any) {
      setAccountError(err.message);
    }
  };

  const createAccount = async () => {
    if (!adding || !adding.label.trim()) return;
    setAccountError(null);
    try {
      const r = await fetch("/api/v2/accounts", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ engine: adding.engine, label: adding.label.trim() }),
      });
      if (!r.ok) {
        const d = await r.json().catch(() => ({}));
        throw new Error(d.error || "Không thể tạo tài khoản");
      }
      const created = await r.json();
      setAdding(null);
      await loadAccounts(true);
      setLogin({ id: created.id, replace: false });
    } catch (err: any) {
      setAccountError(err.message);
    }
  };

  const removeAccount = async (a: AccountInfo) => {
    setAccountError(null);
    try {
      const r = await fetch(`/api/v2/accounts/${encodeURIComponent(a.id)}`, { method: "DELETE" });
      if (!r.ok) {
        const d = await r.json().catch(() => ({}));
        throw new Error(d.error || "Không thể xóa tài khoản");
      }
      setRemoving(null);
      await loadAccounts(true);
    } catch (err: any) {
      setAccountError(err.message);
    }
  };

  const installEngine = async (eng: EngineStatus) => {
    if (!eng.installCmd) return;
    if (!confirm(`Chạy lệnh cài đặt cho ${eng.name}?\n\nLệnh: ${eng.installCmd}`)) return;
    setInstallingEngine(true);
    try {
      const res = await fetch("/api/agents/install", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ engine: eng.id, cmd: eng.installCmd }),
      });
      if (!res.ok) {
        const d = await res.json().catch(() => ({}));
        alert(d.error || "Cài đặt thất bại");
      } else {
        alert(`Đã gửi lệnh cài đặt cho ${eng.name}!`);
        await handleRefresh();
      }
    } finally {
      setInstallingEngine(false);
    }
  };

  const selectedEngine = engines.find((e) => e.id === selectedAgentId) || engines[0];
  const activeBaseKey = selectedEngine ? selectedEngine.base || selectedEngine.id : "agy";
  const activeMeta = AGENT_META_COLORS[activeBaseKey] || {
    badge: "bg-purple-500/15 text-purple-300 border-purple-500/30",
    dot: "bg-purple-400",
    border: "border-purple-500/30",
  };
  const currentGroup = groups?.find((g) => g.engine === selectedEngine?.id);

  return (
    <div className="space-y-6 max-w-2xl">
      {/* Header */}
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0 pr-2">
          <h1 className="text-xl font-bold text-slate-100">Agents</h1>
          <p className="text-xs text-slate-400 mt-1">
            Cấu hình CLI, trạng thái cài đặt và quản lý tài khoản theo từng Agent.
          </p>
        </div>
        <button
          onClick={handleRefresh}
          disabled={refreshingEngines}
          className="shrink-0 whitespace-nowrap flex cursor-pointer items-center gap-1.5 rounded-lg border border-[#2b3548] bg-[#161a25] px-3 py-1.5 text-xs font-medium text-slate-300 hover:bg-[#202636] hover:text-white transition-colors"
          title="Quét lại các agent trong PATH"
        >
          <RefreshCw className={`h-3.5 w-3.5 ${refreshingEngines ? "animate-spin" : ""}`} />
          <span>Quét lại</span>
        </button>
      </div>

      {/* Agent Selector Sub-tabs */}
      <AgentSubTabs
        engines={engines}
        selectedAgentId={selectedEngine?.id || selectedAgentId}
        groups={groups}
        agentMetaColors={AGENT_META_COLORS}
        onSelectAgent={setSelectedAgentId}
      />

      {/* Active Agent Details */}
      {selectedEngine && (
        <div className="space-y-5">
          <AgentStatusCard
            engine={selectedEngine}
            meta={activeMeta}
            installing={installingEngine}
            onOpenTerminal={onOpenAgentTerminal}
            onInstall={installEngine}
          />

          <AccountList
            selectedEngine={selectedEngine}
            currentGroup={currentGroup}
            accountError={accountError}
            adding={adding}
            renaming={renaming}
            onSetAdding={setAdding}
            onSetRenaming={setRenaming}
            onUseAccount={useAccount}
            onRenameAccount={renameAccount}
            onCreateAccount={createAccount}
            onRemoveAccountClick={setRemoving}
            onLoginClick={(id) => setLogin({ id, replace: false })}
            onReloginClick={setRelogin}
          />

          {/* 5-hour and weekly usage, as reported by the agent's own CLI */}
          <div className="rounded-xl border border-[#202738] bg-[#131722] p-4 sm:p-5">
            <div className="mb-3 border-b border-[#1e2535] pb-3">
              <h3 className="text-sm font-semibold text-slate-100">Mức sử dụng {selectedEngine.name}</h3>
              <p className="text-[11.5px] text-slate-500 mt-0.5">
                Hạn mức theo 5 giờ và theo tuần do chính CLI của agent báo cáo cho tài khoản đang dùng (không ước lượng).
              </p>
            </div>
            <ProviderQuota key={selectedEngine.id} engine={selectedEngine.id} />
          </div>

          {/* Independent config note banner */}
          <div className="rounded-xl border border-[#222a3b] bg-[#111520] p-4 flex items-start gap-3">
            <Info className="h-4 w-4 text-sky-400 shrink-0 mt-0.5" />
            <div className="text-[11.5px] text-slate-400 leading-relaxed">
              <strong className="text-slate-200">Nguyên tắc cấu hình độc lập:</strong> Mỗi agent CLI (Claude, Antigravity, Codex) lưu trữ thông tin đăng nhập, token và thiết lập trong thư mục người dùng riêng (<code className="font-mono text-slate-300">~/.claude</code>, <code className="font-mono text-slate-300">~/.gemini</code>, <code className="font-mono text-slate-300">~/.codex</code>). Khi bạn cần thay đổi tham số hoặc login nâng cao, chỉ cần bấm <strong>Mở CLI</strong> để thao tác trực tiếp.
            </div>
          </div>
        </div>
      )}

      {login && (
        <LoginPanel
          engine={login.id}
          replace={login.replace}
          onClose={() => {
            setLogin(null);
            loadAccounts(true);
          }}
          onDone={() => loadAccounts(true)}
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
