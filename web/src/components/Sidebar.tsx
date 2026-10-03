import React, { useState } from "react";
import {
  Plus,
  History,
  Folder,
  SlidersHorizontal,
  ChevronDown,
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
}

export const Sidebar: React.FC<SidebarProps> = ({
  projectGroups,
  activeConversationId,
  onSelectConversation,
  onNewConversation,
  hubConvs = [],
  onConvAction,
}) => {
  const [isProjectsOpen, setIsProjectsOpen] = useState(true);

  return (
    <aside className="w-68 bg-[#14171e] border-r border-[#1d222b] flex flex-col h-full select-none text-slate-300">
      {/* Top Header - Đổi tên thành AGENT HUB */}
      <div className="px-3.5 pt-3 pb-2 flex items-center justify-between border-b border-[#1c212a]">
        <div className="flex items-center gap-2">
          <Zap className="w-3.5 h-3.5 text-amber-400 fill-amber-400" />
          <span className="font-semibold text-xs text-slate-200 tracking-wide">Agent Hub</span>
        </div>
        <button className="text-slate-500 hover:text-slate-300 transition-colors p-1" title="Collapse sidebar">
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
        {/* Conversation History link */}
        <div className="flex items-center gap-2 px-2.5 py-1.5 text-xs text-slate-400 hover:text-slate-200 hover:bg-[#1a1e28] rounded-lg cursor-pointer">
          <History className="w-3.5 h-3.5 text-slate-500" />
          <span className="font-medium">Conversation History</span>
        </div>

        {/* Hub conversations: pinned, groups, archived, with a ⋮ action menu */}
        {onConvAction && (
          <div className="pt-2">
            <HubConversations convs={hubConvs} activeId={activeConversationId} onSelect={onSelectConversation} onAction={onConvAction} />
          </div>
        )}

        {/* Projects Accordion Header */}
        <div className="pt-2">
          <div className="flex items-center justify-between px-2.5 py-1 text-xs text-slate-400 font-medium">
            <div
              className="flex items-center gap-1 cursor-pointer hover:text-slate-200"
              onClick={() => setIsProjectsOpen(!isProjectsOpen)}
            >
              <span>Projects</span>
              <ChevronDown className={`w-3.5 h-3.5 transition-transform text-slate-500 ${isProjectsOpen ? "" : "-rotate-90"}`} />
            </div>
            <div className="flex items-center gap-1.5 text-slate-500">
              <SlidersHorizontal className="w-3.5 h-3.5 hover:text-slate-300 cursor-pointer" />
            </div>
          </div>

          {/* List Projects */}
          {isProjectsOpen && (
            <div className="space-y-3 mt-1.5">
              {projectGroups.map((group) => (
                <div key={group.name} className="space-y-0.5">
                  {/* Project Name Header */}
                  <div className="flex items-center gap-1.5 px-2.5 py-1 text-xs text-slate-400 font-medium">
                    <Folder className="w-3.5 h-3.5 text-slate-500" />
                    <span className="truncate">{group.name}</span>
                  </div>

                  {/* Conversations under project */}
                  <div className="space-y-0.5 pl-1">
                    {group.conversations.map((conv) => {
                      const isActive = conv.id === activeConversationId;
                      return (
                        <div
                          key={conv.id}
                          onClick={() => onSelectConversation(conv.id)}
                          className={`flex items-center justify-between px-2.5 py-1.5 rounded-lg text-xs cursor-pointer transition-colors ${
                            isActive
                              ? "bg-[#212733] text-slate-100 font-medium shadow-sm"
                              : "text-slate-400 hover:bg-[#1a1f28] hover:text-slate-200"
                          }`}
                        >
                          <span className="truncate pr-2">{conv.title}</span>
                          <span className="text-[11px] text-slate-500 shrink-0 font-mono">
                            {conv.relative_time || "now"}
                          </span>
                        </div>
                      );
                    })}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>

      {/* Bottom Footer: Machine & Settings */}
      <div className="p-2.5 border-t border-[#1d222b] space-y-1 bg-[#12141a]">
        {/* Machine dropdown button */}
        <div className="flex items-center justify-between px-2.5 py-1.5 rounded-lg bg-[#181c24] border border-[#222834] text-xs font-medium cursor-pointer hover:bg-[#1e232e]">
          <div className="flex items-center gap-2">
            <span className="w-1.5 h-1.5 rounded-full bg-emerald-400"></span>
            <span className="text-slate-300">nas-duinch</span>
          </div>
          <ChevronDown className="w-3.5 h-3.5 text-slate-500" />
        </div>

        {/* Settings button */}
        <div className="flex items-center gap-2 px-2.5 py-1.5 rounded-lg text-xs text-slate-400 hover:text-slate-200 hover:bg-[#1a1e28] cursor-pointer">
          <Settings className="w-3.5 h-3.5 text-slate-500" />
          <span>Settings</span>
        </div>
      </div>
    </aside>
  );
};
