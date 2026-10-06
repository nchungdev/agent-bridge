import React, { useEffect, useRef, useState } from "react";
import {
  AlertCircle,
  ArrowUpCircle,
  CheckCircle2,
  Loader2,
  RefreshCw,
  Terminal,
} from "lucide-react";
import type { UpdateStatus } from "../../types/bridge";

interface Props {
  updateStatus: UpdateStatus | null;
  onRefreshUpdateStatus: () => Promise<void>;
}

export const UpdatesTab: React.FC<Props> = ({ updateStatus, onRefreshUpdateStatus }) => {
  const [checkingUpdate, setCheckingUpdate] = useState(false);
  const [updatingApp, setUpdatingApp] = useState(false);
  const [updateError, setUpdateError] = useState<string | null>(null);
  const [updateLogs, setUpdateLogs] = useState<string[]>([]);
  const [currentStep, setCurrentStep] = useState<string>("idle");
  const [reconnecting, setReconnecting] = useState(false);
  const logEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    logEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [updateLogs]);

  const handleCheckUpdate = async () => {
    setCheckingUpdate(true);
    setUpdateError(null);
    try {
      const res = await fetch("/api/admin/update/check", { method: "POST" });
      if (!res.ok) {
        const d = await res.json().catch(() => ({}));
        throw new Error(d.error || "Không thể kiểm tra bản cập nhật");
      }
      await onRefreshUpdateStatus();
    } catch (err: any) {
      setUpdateError(err.message);
    } finally {
      setCheckingUpdate(false);
    }
  };

  const handleApplyUpdate = async () => {
    if (!confirm("Bắt đầu cập nhật ứng dụng lên phiên bản mới nhất từ Git? Quá trình sẽ biên dịch lại và tự động restart service.")) return;
    setUpdatingApp(true);
    setUpdateError(null);
    setUpdateLogs(["[Bắt đầu] Đang gửi yêu cầu cập nhật đến máy chủ..."]);
    setCurrentStep("pulling");

    try {
      const res = await fetch("/api/admin/update/apply", { method: "POST" });
      if (!res.ok) {
        const d = await res.json().catch(() => ({}));
        throw new Error(d.error || "Không thể kích hoạt tiến trình cập nhật");
      }

      const pollTimer = setInterval(async () => {
        try {
          const r = await fetch("/api/admin/update/status");
          if (r.ok) {
            const data: UpdateStatus = await r.json();
            setUpdateLogs(data.logs || []);
            setCurrentStep(data.step);

            if (data.step === "restarting") {
              clearInterval(pollTimer);
              setReconnecting(true);
              pollServerRestart();
            } else if (data.step === "error") {
              clearInterval(pollTimer);
              setUpdatingApp(false);
              setUpdateError(data.error || "Quá trình cập nhật thất bại");
            }
          }
        } catch {
          // Server might be restarting
        }
      }, 1500);
    } catch (err: any) {
      setUpdatingApp(false);
      setUpdateError(err.message);
    }
  };

  const pollServerRestart = () => {
    let attempts = 0;
    const restartTimer = setInterval(async () => {
      attempts++;
      try {
        const r = await fetch("/api/admin/update/status", { cache: "no-store" });
        if (r.ok) {
          clearInterval(restartTimer);
          setUpdatingApp(false);
          setReconnecting(false);
          setCurrentStep("idle");
          await onRefreshUpdateStatus();
          alert("🎉 Ứng dụng đã được cập nhật và khởi động lại thành công!");
          window.location.reload();
        }
      } catch {
        if (attempts > 60) {
          clearInterval(restartTimer);
          setUpdatingApp(false);
          setReconnecting(false);
          setUpdateError("Không thể kết nối lại sau khi restart. Vui lòng kiểm tra terminal.");
        }
      }
    }, 2000);
  };

  return (
    <div className="space-y-6 max-w-2xl">
      <div>
        <h1 className="text-xl font-bold text-slate-100">Updates</h1>
        <p className="text-xs text-slate-400 mt-1">
          Tự động kiểm tra phiên bản mới từ GitHub và cập nhật nhanh chỉ với 1 click.
        </p>
      </div>

      <div className="space-y-4">
        {/* Version Card */}
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
            <div>Tự động kiểm tra: mỗi 24 giờ &amp; mỗi lần vào ứng dụng.</div>
            {updateStatus?.last_checked && (
              <div className="text-slate-500 font-mono text-[11px]">
                Kiểm tra gần nhất: {new Date(updateStatus.last_checked).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}
              </div>
            )}
          </div>
        </div>

        {/* Action buttons */}
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

        {/* Progress indicator */}
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

        {/* Error message */}
        {updateError && (
          <div className="flex items-start gap-2 rounded-xl border border-rose-500/30 bg-rose-500/10 p-3.5 text-xs text-rose-300">
            <AlertCircle className="h-4 w-4 shrink-0 text-rose-400 mt-0.5" />
            <div className="flex-1 break-words leading-relaxed">{updateError}</div>
          </div>
        )}

        {/* Progress log console */}
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
  );
};
