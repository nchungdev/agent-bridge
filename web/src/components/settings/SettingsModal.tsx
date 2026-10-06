import React, { useEffect, useState } from "react";
import {
  ArrowUpCircle,
  Bot,
  Palette,
  Sliders,
  X,
} from "lucide-react";
import type { EngineStatus, UpdateStatus } from "../../types/bridge";
import { GeneralTab } from "./GeneralTab";
import { AgentsTab } from "./AgentsTab";
import { AppearanceTab } from "./AppearanceTab";
import { UpdatesTab } from "./UpdatesTab";

export type { UpdateStatus, EngineStatus };

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

  useEffect(() => {
    if (isOpen) {
      setActiveTab(initialTab);
    }
  }, [isOpen, initialTab]);

  useEffect(() => {
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === "Escape" && isOpen) {
        onClose();
      }
    };
    window.addEventListener("keydown", handleKey);
    return () => window.removeEventListener("keydown", handleKey);
  }, [isOpen, onClose]);

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/75 backdrop-blur-md p-4 sm:p-6 animate-in fade-in duration-150">
      <div className="flex h-[88vh] max-h-[820px] w-full max-w-5xl overflow-hidden rounded-2xl border border-[#2b354b] bg-[#10141d] shadow-2xl text-slate-200">
        {/* Left Sidebar */}
        <div className="w-56 sm:w-64 shrink-0 border-r border-[#1a202c] bg-[#121620] flex flex-col justify-between p-3.5 select-none">
          <div className="space-y-4">
            <div className="px-2 pt-1">
              <span className="text-xs font-bold uppercase tracking-wider text-slate-400">Settings</span>
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

          {/* Sidebar Footer info with build number */}
          <div className="border-t border-[#1a202c] pt-3 px-2 text-[11px] text-slate-500 font-mono flex items-center justify-between">
            <span>Agent Bridge {updateStatus?.current_version || "v1.0.0"}</span>
            <span>{updateStatus?.current_commit || "main"}</span>
          </div>
        </div>

        {/* Right Content Area */}
        <div className="relative flex-1 flex flex-col min-w-0 bg-[#0e121a] overflow-hidden">
          <button
            onClick={onClose}
            className="absolute top-4 right-4 z-10 cursor-pointer rounded-lg p-1.5 text-slate-400 hover:bg-[#1a202c] hover:text-slate-200 transition-colors"
            title="Đóng cài đặt"
          >
            <X className="h-5 w-5" />
          </button>

          <div className="flex-1 overflow-y-auto p-6 sm:p-8">
            {activeTab === "general" && <GeneralTab />}
            {activeTab === "agents" && (
              <AgentsTab
                engines={engines}
                onRefreshEngines={onRefreshEngines}
                onOpenAgentTerminal={onOpenAgentTerminal}
              />
            )}
            {activeTab === "appearance" && <AppearanceTab />}
            {activeTab === "updates" && (
              <UpdatesTab
                updateStatus={updateStatus}
                onRefreshUpdateStatus={onRefreshUpdateStatus}
              />
            )}
          </div>
        </div>
      </div>
    </div>
  );
};
