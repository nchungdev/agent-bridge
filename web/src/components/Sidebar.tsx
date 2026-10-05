import React from "react";
import {
  Plus,
  Settings,
  PanelLeftClose,
  Zap,
} from "lucide-react";
import { HubConversations, type HubConv, type ConvAction } from "./HubConversations";

export interface ConversationItem {
  id: string;
  title: string;
  relative_time?: string;
}

export interface ProjectGroupItem {
  name: string;
  conversations: ConversationItem[];
}

interface SidebarProps {
  projectGroups: ProjectGroupItem[];
  activeConversationId: string | null;
  onSelectConversation: (id: string) => void;
  onNewConversation: () => void;
  hubConvs?: HubConv[];
  onConvAction?: (a: ConvAction) => void;
  onToggleCollapse?: () => void;
  onOpenSettings?: () => void;
}

export const Sidebar: React.FC<SidebarProps> = ({
  activeConversationId,
  onSelectConversation,
  onNewConversation,
  hubConvs = [],
  onConvAction,
  onToggleCollapse,
  onOpenSettings,
}) => {
  return (
    <aside className="w-68 bg-[#14171e] border-r border-[#1d222b] flex flex-col h-full select-none text-slate-300">
      {/* Top Header - Đổi tên thành AGENT BRIDGE */}
      <div className="px-3.5 pt-3 pb-2 flex items-center justify-between border-b border-[#1c212a]">
        <div className="flex items-center gap-2">
          <Zap className="w-3.5 h-3.5 text-amber-400 fill-amber-400" />
          <span className="font-semibold text-xs text-slate-200 tracking-wide">Agent Bridge</span>
        </div>
        <button type="button" onClick={onToggleCollapse} className="text-slate-500 hover:text-slate-300 transition-colors p-1 cursor-pointer" title="Collapse sidebar">
          <PanelLeftClose className="w-3.5 h-3.5" />
        </button>
      </div>

      {/* Action: New Conversation */}
      <div className="px-3 py-2">
        <button
          onClick={onNewConversation}
          className="w-full flex items-center gap-2 py-1.5 px-3 rounded-lg bg-[#1a1f29] hover:bg-[#222836] text-slate-200 text-xs font-medium transition-all border border-[#262c3a] cursor-pointer shadow-sm"
        >
          <Plus className="w-3.5 h-3.5 text-slate-400" />
          <span>New Conversation</span>
        </button>
      </div>

      {/* History & Projects section */}
      <div className="flex-1 overflow-y-auto px-2 space-y-1 py-1">
        {/* Hub conversations: pinned, groups, archived, with a ⋮ action menu */}
        {onConvAction && (
          <div className="pt-2">
            <HubConversations convs={hubConvs} activeId={activeConversationId} onSelect={onSelectConversation} onAction={onConvAction} />
          </div>
        )}

      </div>

      {/* Bottom Footer: Machine & Settings */}
      <div className="p-2.5 border-t border-[#1d222b] space-y-1 bg-[#12141a]">
        {/* Machine dropdown button */}
        <div className="flex items-center justify-between px-2.5 py-1.5 rounded-lg bg-[#181c24] border border-[#222834] text-xs font-medium cursor-default" title="This server">
          <div className="flex items-center gap-2">
            <span className="w-1.5 h-1.5 rounded-full bg-emerald-400"></span>
            <span className="text-slate-300">nas-duinch</span>
          </div>
        </div>

        {/* Settings button */}
        <div onClick={onOpenSettings} className="flex items-center gap-2 px-2.5 py-1.5 rounded-lg text-xs text-slate-400 hover:text-slate-200 hover:bg-[#1a1e28] cursor-pointer">
          <Settings className="w-3.5 h-3.5 text-slate-500" />
          <span>Accounts</span>
        </div>
      </div>
    </aside>
  );
};
