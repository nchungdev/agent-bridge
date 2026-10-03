import React, { useState } from "react";
import { ChevronDown, ChevronUp, ArrowRight, Pencil, Trash2 } from "lucide-react";

export interface QueuedItem {
  id: string;
  text: string;
  modelId: string;
  agent: string;
  media?: { uri: string; mime_type: string }[];
}

interface QueuedMessagesProps {
  queue: QueuedItem[];
  onRemove: (id: string) => void;
  onSendNow: (id: string) => void;
  onEdit: (id: string) => void;
}

export const QueuedMessages: React.FC<QueuedMessagesProps> = ({
  queue,
  onRemove,
  onSendNow,
  onEdit,
}) => {
  const [isOpen, setIsOpen] = useState(true);

  if (queue.length === 0) return null;

  return (
    <div className="w-full max-w-4xl mx-auto px-4 mb-2 select-none animate-in fade-in slide-in-from-bottom-2 duration-150">
      <div className="rounded-xl bg-[#181c24] border border-[#262c38] shadow-md overflow-hidden text-slate-300">
        {/* Header: Queued Messages (2) Sends after agent finishes working */}
        <div
          onClick={() => setIsOpen(!isOpen)}
          className="flex items-center justify-between px-3.5 py-2 cursor-pointer hover:bg-[#1e232e] transition-colors text-xs font-medium"
        >
          <div className="flex items-center gap-2">
            <span className="font-semibold text-slate-200">Queued Messages</span>
            <span className="px-1.5 py-0.2 rounded-full bg-[#293242] text-[11px] font-mono text-slate-300">
              {queue.length}
            </span>
            <span className="text-slate-500 text-[11px] hidden sm:inline">
              Sends after agent finishes working
            </span>
          </div>

          <div>
            {isOpen ? (
              <ChevronUp className="w-3.5 h-3.5 text-slate-500" />
            ) : (
              <ChevronDown className="w-3.5 h-3.5 text-slate-500" />
            )}
          </div>
        </div>

        {/* Danh sách các tin nhắn trong Queue */}
        {isOpen && (
          <div className="divide-y divide-[#202530] bg-[#12151c]">
            {queue.map((item) => (
              <div
                key={item.id}
                className="flex items-center justify-between px-3.5 py-2.5 hover:bg-[#161a22] transition-colors text-xs text-slate-300 group"
              >
                <div className="flex items-center gap-2.5 truncate pr-3 flex-1">
                  <span className="truncate text-slate-200 text-[13px]">{item.text}</span>
                </div>

                {/* 3 nút thao tác: Send now (→), Edit (✎), Delete (🗑) */}
                <div className="flex items-center gap-1 shrink-0 text-slate-500">
                  <button
                    type="button"
                    onClick={() => onSendNow(item.id)}
                    className="p-1 rounded hover:text-slate-200 hover:bg-[#252b36] transition-colors"
                    title="Send now (Prioritize)"
                  >
                    <ArrowRight className="w-3.5 h-3.5" />
                  </button>
                  <button
                    type="button"
                    onClick={() => onEdit(item.id)}
                    className="p-1 rounded hover:text-slate-200 hover:bg-[#252b36] transition-colors"
                    title="Edit message"
                  >
                    <Pencil className="w-3.5 h-3.5" />
                  </button>
                  <button
                    type="button"
                    onClick={() => onRemove(item.id)}
                    className="p-1 rounded hover:text-rose-400 hover:bg-[#252b36] transition-colors"
                    title="Delete from queue"
                  >
                    <Trash2 className="w-3.5 h-3.5" />
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
};
