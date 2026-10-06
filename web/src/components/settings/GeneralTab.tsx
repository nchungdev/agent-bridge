import React from "react";
import { Shield } from "lucide-react";

export const GeneralTab: React.FC = () => {
  return (
    <div className="space-y-6 max-w-2xl">
      <div>
        <h1 className="text-xl font-bold text-slate-100">General Settings</h1>
        <p className="text-xs text-slate-400 mt-1">
          Thiết lập an toàn và cấu hình mặc định cho hệ thống Agent Bridge.
        </p>
      </div>

      <div className="space-y-4">
        <div className="space-y-2">
          <h2 className="text-xs font-semibold uppercase tracking-wider text-slate-400">
            Workspace Safety
          </h2>
          <div className="rounded-xl border border-[#202738] bg-[#131722] p-4 flex items-start gap-3">
            <Shield className="h-5 w-5 text-indigo-400 shrink-0 mt-0.5" />
            <div>
              <div className="text-xs font-medium text-slate-200">
                Bảo vệ thư mục gốc & người dùng
              </div>
              <div className="text-[11.5px] text-slate-400 mt-1 leading-relaxed">
                Hệ thống tự động kích hoạt tính năng khoá an toàn cho các thư mục tài khoản cá nhân và hệ điều hành (
                <code className="text-slate-300 font-mono">/Users/*</code>,{" "}
                <code className="text-slate-300 font-mono">/home/*</code>,{" "}
                <code className="text-slate-300 font-mono">/root</code>,{" "}
                <code className="text-slate-300 font-mono">/tmp</code>), không cho phép xoá khỏi danh mục để tránh mất mát dữ liệu ngoài ý muốn.
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};
