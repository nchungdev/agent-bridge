import React, { useRef } from "react";
import { Check, LogIn, Pencil, Plus, Trash2 } from "lucide-react";
import type { AccountGroup, AccountInfo } from "../../v2/types";
import type { EngineStatus } from "../../types/bridge";

interface Props {
  selectedEngine: EngineStatus;
  currentGroup?: AccountGroup;
  accountError: string | null;
  adding: { engine: string; label: string } | null;
  renaming: { id: string; label: string } | null;
  onSetAdding: (val: { engine: string; label: string } | null) => void;
  onSetRenaming: (val: { id: string; label: string } | null) => void;
  onUseAccount: (a: AccountInfo) => void;
  onRenameAccount: () => void;
  onCreateAccount: () => void;
  onRemoveAccountClick: (a: AccountInfo) => void;
  onLoginClick: (id: string) => void;
  onReloginClick: (a: AccountInfo) => void;
}

export const AccountList: React.FC<Props> = ({
  selectedEngine,
  currentGroup,
  accountError,
  adding,
  renaming,
  onSetAdding,
  onSetRenaming,
  onUseAccount,
  onRenameAccount,
  onCreateAccount,
  onRemoveAccountClick,
  onLoginClick,
  onReloginClick,
}) => {
  const addRef = useRef<HTMLInputElement>(null);

  return (
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
            onClick={() => onSetAdding({ engine: selectedEngine.id, label: "" })}
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
                      onChange={(e) => onSetRenaming({ id: a.id, label: e.target.value })}
                      onKeyDown={(e) => {
                        if (e.key === "Enter") onRenameAccount();
                        if (e.key === "Escape") onSetRenaming(null);
                      }}
                      onBlur={() => onSetRenaming(null)}
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
                      onClick={() => onUseAccount(a)}
                      className="cursor-pointer rounded-md bg-[#222938] px-2.5 py-1 text-[11px] font-medium text-slate-200 hover:bg-[#2b3447] transition-colors"
                    >
                      Sử dụng
                    </button>
                  )}
                  {a.can_login && (
                    <button
                      onClick={() => (a.logged_in ? onReloginClick(a) : onLoginClick(a.id))}
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
                        onClick={() => onSetRenaming({ id: a.id, label: a.label })}
                        className="cursor-pointer rounded-md p-1.5 text-slate-400 hover:bg-[#222938] hover:text-slate-100 transition-colors"
                        title="Đổi tên"
                      >
                        <Pencil className="h-3.5 w-3.5" />
                      </button>
                      <button
                        onClick={() => onRemoveAccountClick(a)}
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
              onChange={(e) => onSetAdding({ engine: selectedEngine.id, label: e.target.value })}
              onKeyDown={(e) => {
                if (e.key === "Enter") onCreateAccount();
                if (e.key === "Escape") onSetAdding(null);
              }}
              placeholder="Tên tài khoản (ví dụ: Công việc, Cá nhân)"
              className="flex-1 rounded-md border border-sky-600/50 bg-[#0f1218] px-2.5 py-1.5 text-xs text-slate-100 outline-none"
            />
            <button
              onClick={onCreateAccount}
              disabled={!adding.label.trim()}
              className="cursor-pointer rounded-md bg-sky-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-sky-500 disabled:opacity-40 transition-colors"
            >
              Tạo &amp; Đăng nhập
            </button>
            <button
              onClick={() => onSetAdding(null)}
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
  );
};
