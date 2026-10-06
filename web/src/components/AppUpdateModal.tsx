import React, { useEffect, useRef, useState } from "react";
import {
  ArrowUpCircle,
  RefreshCw,
  CheckCircle2,
  AlertCircle,
  GitCommit,
  Terminal,
  X,
  Sparkles,
  Loader2,
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

interface Props {
  isOpen: boolean;
  onClose: () => void;
  status: UpdateStatus | null;
  onRefreshStatus: () => Promise<void>;
}

export const AppUpdateModal: React.FC<Props> = ({
  isOpen,
  onClose,
  status,
  onRefreshStatus,
}) => {
  const [checking, setChecking] = useState(false);
  const [updating, setUpdating] = useState(false);
  const [currentStep, setCurrentStep] = useState<string>("idle");
  const [logs, setLogs] = useState<string[]>([]);
  const [reconnecting, setReconnecting] = useState(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const logEndRef = useRef<HTMLDivElement>(null);

  // Sync internal state with props status
  useEffect(() => {
    if (status) {
      if (status.is_updating) {
        setUpdating(true);
      }
      setCurrentStep(status.step);
      if (status.logs && status.logs.length > 0) {
        setLogs(status.logs);
      }
      if (status.error) {
        setErrorMessage(status.error);
      }
    }
  }, [status]);

  // Auto-scroll logs
  useEffect(() => {
    logEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [logs]);

  // Polling loop while updating
  useEffect(() => {
    let timer: any = null;
    if (updating || currentStep === "pulling" || currentStep === "building_web" || currentStep === "building_binary" || currentStep === "restarting") {
      timer = setInterval(async () => {
        try {
          const res = await fetch("/api/admin/update/status");
          if (res.ok) {
            const data: UpdateStatus = await res.json();
            setCurrentStep(data.step);
            setLogs(data.logs || []);
            if (data.error) {
              setErrorMessage(data.error);
              setUpdating(false);
            }
            if (data.step === "restarting") {
              setReconnecting(true);
              pollReconnect();
              clearInterval(timer);
            }
          }
        } catch {
          // If server stops responding during restart, start polling for reconnection
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
  }, [updating, currentStep]);

  // Reconnection polling when restarting
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
          setUpdating(false);
          setCurrentStep("success");
          setLogs((prev) => [...prev, "Máy chủ đã hoạt động trở lại! Đang tải lại ứng dụng..."]);
          setTimeout(() => {
            window.location.reload();
          }, 1200);
        }
      } catch {
        // Still down, keep trying
        if (attempts > 60) {
          clearInterval(interval);
          setErrorMessage("Không thể kết nối lại sau 60 giây. Vui lòng tải lại trang thủ công.");
        }
      }
    }, 1500);
  };

  const handleCheckUpdate = async () => {
    setChecking(true);
    setErrorMessage(null);
    try {
      const res = await fetch("/api/admin/update/check", { method: "POST" });
      if (res.ok) {
        await onRefreshStatus();
      } else {
        const err = await res.json().catch(() => ({}));
        setErrorMessage(err.error || "Không thể kiểm tra bản cập nhật.");
      }
    } catch (e: any) {
      setErrorMessage(e.message || "Lỗi mạng khi kiểm tra cập nhật.");
    } finally {
      setChecking(false);
    }
  };

  const handleApplyUpdate = async () => {
    if (!confirm("Bắt đầu quá trình tải mã nguồn mới, biên dịch và khởi động lại dịch vụ Agent Bridge?")) {
      return;
    }
    setUpdating(true);
    setErrorMessage(null);
    setLogs(["Bắt đầu yêu cầu cập nhật..."]);
    try {
      const res = await fetch("/api/admin/update/apply", { method: "POST" });
      if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        setErrorMessage(err.error || "Không thể khởi chạy cập nhật.");
        setUpdating(false);
      }
    } catch (e: any) {
      setErrorMessage(e.message || "Lỗi gửi yêu cầu cập nhật.");
      setUpdating(false);
    }
  };

  if (!isOpen) return null;

  const isBusy = updating || checking || reconnecting;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
      <div className="relative flex w-full max-w-xl flex-col rounded-xl border border-[#2c3447] bg-[#121620] shadow-2xl overflow-hidden animate-in fade-in zoom-in-95 duration-150">
        {/* Modal Header */}
        <div className="flex items-center justify-between border-b border-[#222838] px-5 py-4 bg-[#161b27]">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-indigo-500/10 border border-indigo-500/20 text-indigo-400">
              <ArrowUpCircle className="h-5 w-5" />
            </div>
            <div>
              <h2 className="text-sm font-semibold text-slate-100 flex items-center gap-2">
                Cập nhật ứng dụng
                {status?.has_update && (
                  <span className="rounded-full bg-amber-500/20 border border-amber-500/30 px-2 py-0.5 text-[10px] font-medium text-amber-300">
                    Bản mới
                  </span>
                )}
              </h2>
              <p className="text-[11px] text-slate-400">
                Agent Bridge · Git Pull, Frontend Vite & Go Single-Binary
              </p>
            </div>
          </div>
          <button
            onClick={onClose}
            disabled={updating && !errorMessage}
            className="cursor-pointer rounded-lg p-1.5 text-slate-400 hover:bg-[#202737] hover:text-slate-200 disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* Modal Body */}
        <div className="space-y-4 p-5 max-h-[75vh] overflow-y-auto">
          {/* Status summary banner */}
          <div className="rounded-lg border border-[#22293b] bg-[#161b28] p-3.5 space-y-2.5">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2 text-xs font-medium text-slate-300">
                <GitCommit className="h-3.5 w-3.5 text-indigo-400" />
                <span>Nhánh: <strong className="text-slate-100 font-mono">{status?.branch || "main"}</strong></span>
              </div>
              <div className="text-[11px] text-slate-500">
                {status?.last_checked ? `Kiểm tra ${new Date(status.last_checked).toLocaleTimeString()}` : ""}
              </div>
            </div>

            <div className="grid grid-cols-2 gap-2 text-[11.5px]">
              <div className="rounded border border-[#1f2637] bg-[#11141e] p-2.5">
                <div className="text-slate-500 text-[10.5px]">Phiên bản hiện tại</div>
                <div className="font-mono font-semibold text-slate-200 mt-0.5">
                  {status?.current_commit || "---"}
                </div>
                <div className="text-slate-400 text-[10.5px] truncate mt-0.5" title={status?.current_message}>
                  {status?.current_message || "Chưa xác định"}
                </div>
              </div>

              <div className="rounded border border-[#1f2637] bg-[#11141e] p-2.5">
                <div className="text-slate-500 text-[10.5px]">Bản mới nhất trên Git</div>
                <div className="font-mono font-semibold text-slate-200 mt-0.5">
                  {status?.remote_commit || status?.current_commit || "---"}
                </div>
                <div className="mt-0.5">
                  {status?.has_update ? (
                    <span className="inline-flex items-center gap-1 text-[10.5px] text-amber-400 font-medium">
                      <Sparkles className="h-3 w-3" />
                      Sau {status.commits_behind || status.commits?.length || 1} commit
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

          {/* Incoming Changelog if update available */}
          {status?.has_update && status.commits && status.commits.length > 0 && (
            <div className="space-y-1.5">
              <div className="text-[11px] font-semibold uppercase tracking-wider text-slate-400">
                Các thay đổi trong bản mới ({status.commits.length}):
              </div>
              <div className="rounded-lg border border-[#232b3d] bg-[#141824] p-2.5 max-h-36 overflow-y-auto space-y-1.5 font-mono text-[11.5px]">
                {status.commits.map((c, i) => (
                  <div key={i} className="flex items-start gap-2 text-slate-300">
                    <span className="text-indigo-400 select-none">•</span>
                    <span className="truncate">{c}</span>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Progress / Step indicator while updating */}
          {updating && (
            <div className="rounded-lg border border-indigo-500/20 bg-indigo-500/5 p-3 space-y-2">
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
              {/* Progress bar */}
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

          {/* Error Message */}
          {errorMessage && (
            <div className="flex items-start gap-2 rounded-lg border border-rose-500/30 bg-rose-500/10 p-3 text-xs text-rose-300">
              <AlertCircle className="h-4 w-4 shrink-0 text-rose-400 mt-0.5" />
              <div className="flex-1 break-words leading-relaxed">{errorMessage}</div>
            </div>
          )}

          {/* Log Console Box (shown if logs exist) */}
          {logs.length > 0 && (
            <div className="space-y-1.5">
              <div className="flex items-center gap-1.5 text-[10.5px] font-semibold uppercase tracking-wider text-slate-500">
                <Terminal className="h-3 w-3" />
                <span>Nhật ký cập nhật</span>
              </div>
              <div className="rounded-lg border border-[#1f2638] bg-[#0c0e15] p-3 font-mono text-[11px] leading-relaxed text-slate-300 max-h-40 overflow-y-auto space-y-1">
                {logs.map((line, idx) => (
                  <div key={idx} className="whitespace-pre-wrap break-all">
                    {line}
                  </div>
                ))}
                <div ref={logEndRef} />
              </div>
            </div>
          )}
        </div>

        {/* Modal Actions */}
        <div className="flex items-center justify-between border-t border-[#222838] bg-[#141824] px-5 py-3.5">
          <button
            onClick={handleCheckUpdate}
            disabled={isBusy}
            className="flex cursor-pointer items-center gap-1.5 rounded-lg border border-[#2c3549] bg-[#1a202e] px-3 py-1.5 text-xs font-medium text-slate-300 hover:bg-[#232c3f] hover:text-slate-100 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
          >
            <RefreshCw className={`h-3.5 w-3.5 ${checking ? "animate-spin" : ""}`} />
            <span>Kiểm tra cập nhật</span>
          </button>

          <div className="flex items-center gap-2">
            <button
              onClick={onClose}
              disabled={updating && !errorMessage}
              className="cursor-pointer rounded-lg px-3 py-1.5 text-xs text-slate-400 hover:bg-[#1f2535] hover:text-slate-200 disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
            >
              Đóng
            </button>
            <button
              onClick={handleApplyUpdate}
              disabled={isBusy}
              className={`flex cursor-pointer items-center gap-1.5 rounded-lg px-4 py-1.5 text-xs font-semibold shadow-md transition-all ${
                status?.has_update
                  ? "bg-indigo-600 hover:bg-indigo-500 text-white shadow-indigo-600/30"
                  : "bg-slate-700 hover:bg-slate-600 text-slate-200"
              } disabled:opacity-40 disabled:cursor-not-allowed`}
            >
              <ArrowUpCircle className="h-3.5 w-3.5" />
              <span>{updating ? "Đang cập nhật..." : status?.has_update ? "Cập nhật ngay" : "Rebuild & Cập nhật"}</span>
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};
