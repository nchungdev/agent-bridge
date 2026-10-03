import React, { useEffect, useRef, useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { Terminal, FileCode, Bot, User, Sparkles, Image as ImageIcon, X } from "lucide-react";

export interface MediaAttachment {
  mime_type: string;
  uri: string;
}

export interface ToolStepItem {
  name: string;
  action: string;
  summary?: string;
  command?: string;
  cwd?: string;
  path?: string;
  output?: string;
  status?: string;
}

export interface MessageItem {
  id?: string;
  role: "user" | "assistant";
  content: string;
  agent?: string;
  model?: string;
  duration?: string;
  token_count?: number;
  diffPatch?: string;
  diffFile?: string;
  toolCommand?: string;
  toolOutput?: string;
  media?: MediaAttachment[];
  steps?: ToolStepItem[];
  is_running?: boolean;
}

interface ChatStreamProps {
  messages: MessageItem[];
  isStreaming: boolean;
}

export const ChatStream: React.FC<ChatStreamProps> = ({ messages, isStreaming }) => {
  const bottomRef = useRef<HTMLDivElement>(null);
  const [previewImageUri, setPreviewImageUri] = useState<string | null>(null);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages, isStreaming]);

  if (messages.length === 0) {
    return (
      <div className="flex-1 flex flex-col items-center justify-center p-8 text-center select-none overflow-x-hidden">
        <div className="w-11 h-11 rounded-2xl bg-[#1e232d] border border-[#2d3442] flex items-center justify-center mb-3 shadow-inner">
          <Sparkles className="w-5 h-5 text-slate-400" />
        </div>
        <h2 className="text-lg font-medium text-slate-200 mb-1.5">Agent Hub</h2>
        <p className="text-xs text-slate-400 max-w-sm leading-relaxed">
          Switch models and agents seamlessly. Conversation context is preserved.
        </p>
      </div>
    );
  }

  return (
    <div className="flex-1 overflow-y-auto overflow-x-hidden px-4 py-6 space-y-5 w-full max-w-4xl mx-auto">
      {messages.map((msg, idx) => (
        <div
          key={idx}
          className={`flex gap-3 max-w-full ${msg.role === "user" ? "justify-end" : "justify-start"}`}
        >
          {msg.role === "assistant" && (
            <div className="w-6 h-6 rounded-md bg-[#1e232d] border border-[#2d3442] flex items-center justify-center shrink-0 mt-0.5 text-slate-400">
              <Bot className="w-3.5 h-3.5" />
            </div>
          )}

          <div
            className={`max-w-[85%] rounded-xl px-4 py-3 text-[13.5px] leading-relaxed overflow-hidden break-words [overflow-wrap:anywhere] ${
              msg.role === "user"
                ? "bg-[#252b36] border border-[#343c4a] text-slate-100 rounded-tr-sm shadow-sm"
                : "bg-[#161a22] border border-[#242934] text-slate-200 rounded-tl-sm shadow-sm"
            }`}
          >
            {/* Header info for assistant */}
            {msg.role === "assistant" && msg.model && (
              <div className="flex items-center gap-2 mb-2 pb-1.5 border-b border-[#242934] text-[11px] text-slate-400">
                <span className="font-medium text-slate-300">{msg.model}</span>
                {msg.agent && <span className="text-slate-500">• {msg.agent}</span>}
              </div>
            )}

            {/* RENDER HÌNH ẢNH ĐÍNH KÈM CỦA USER (Attachments) */}
            {msg.media && msg.media.length > 0 && (
              <div className="mb-2.5 flex flex-wrap gap-2">
                {msg.media.map((item, mIdx) => {
                  const mediaUrl = `/api/media?path=${encodeURIComponent(item.uri)}`;
                  return (
                    <div
                      key={mIdx}
                      onClick={() => setPreviewImageUri(mediaUrl)}
                      className="relative w-20 h-20 rounded-lg overflow-hidden border border-[#374152] bg-[#161a22] cursor-pointer group shrink-0 transition-transform hover:scale-105"
                      title="View image"
                    >
                      <img
                        src={mediaUrl}
                        alt="attachment"
                        className="w-full h-full object-cover"
                        loading="lazy"
                        onError={(e) => {
                          // Fallback nếu ảnh không load được
                          (e.target as HTMLElement).style.display = "none";
                        }}
                      />
                      <div className="absolute inset-0 bg-black/30 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center text-white text-[10px] gap-0.5">
                        <ImageIcon className="w-3 h-3" />
                      </div>
                    </div>
                  );
                })}
              </div>
            )}

            {/* Markdown content */}
            {msg.content && (
              <div className="prose prose-invert max-w-none text-[13.5px] break-words [overflow-wrap:anywhere] text-slate-200">
                <ReactMarkdown remarkPlugins={[remarkGfm]}>
                  {msg.content}
                </ReactMarkdown>
              </div>
            )}

            {/* Diff Preview Block */}
            {msg.diffPatch && (
              <div className="mt-3 rounded-lg bg-[#11141a] border border-[#242934] overflow-hidden max-w-full text-xs">
                <div className="flex items-center justify-between px-3 py-1.5 bg-[#161a22] border-b border-[#242934] font-mono text-slate-400">
                  <span className="flex items-center gap-1.5 truncate">
                    <FileCode className="w-3.5 h-3.5 text-sky-400 shrink-0" />
                    <span className="truncate">{msg.diffFile || "Changes"}</span>
                  </span>
                  <span className="text-[10px] text-emerald-400 font-semibold shrink-0">Diff</span>
                </div>
                <pre className="p-3 font-mono overflow-x-auto text-slate-300 leading-relaxed max-w-full">
                  {msg.diffPatch.split("\n").map((line, i) => {
                    const isAdd = line.startsWith("+");
                    const isDel = line.startsWith("-");
                    return (
                      <div
                        key={i}
                        className={
                          isAdd
                            ? "bg-emerald-950/30 text-emerald-300"
                            : isDel
                            ? "bg-rose-950/30 text-rose-300"
                            : "text-slate-400"
                        }
                      >
                        {line}
                      </div>
                    );
                  })}
                </pre>
              </div>
            )}

            {/* Tool Execution Block */}
            {msg.toolCommand && (
              <div className="mt-3 rounded-lg bg-[#11141a] border border-[#242934] overflow-hidden max-w-full text-xs">
                <div className="flex items-center gap-2 px-3 py-1.5 bg-[#161a22] border-b border-[#242934] font-mono text-slate-400">
                  <Terminal className="w-3.5 h-3.5 text-amber-400 shrink-0" />
                  <span>Terminal</span>
                </div>
                <div className="p-2.5 font-mono text-amber-200/80 bg-black/30 break-all">
                  $ {msg.toolCommand}
                </div>
                {msg.toolOutput && (
                  <pre className="p-2.5 font-mono text-slate-400 max-h-40 overflow-y-auto overflow-x-auto border-t border-[#202530] max-w-full">
                    {msg.toolOutput}
                  </pre>
                )}
              </div>
            )}
          </div>

          {msg.role === "user" && (
            <div className="w-6 h-6 rounded-md bg-[#252b36] border border-[#343c4a] flex items-center justify-center shrink-0 mt-0.5 text-slate-400">
              <User className="w-3.5 h-3.5" />
            </div>
          )}
        </div>
      ))}

      {isStreaming && (
        <div className="flex items-center gap-2 text-xs text-slate-400 animate-pulse pl-9">
          <Sparkles className="w-3.5 h-3.5 text-slate-500" />
          <span>Generating response...</span>
        </div>
      )}

      {/* Lightbox Preview Modal xem ảnh to */}
      {previewImageUri && (
        <div
          className="fixed inset-0 z-50 bg-black/80 flex items-center justify-center p-4 backdrop-blur-sm cursor-zoom-out"
          onClick={() => setPreviewImageUri(null)}
        >
          <div className="relative max-w-4xl max-h-[90vh]">
            <img
              src={previewImageUri}
              alt="preview"
              className="max-w-full max-h-[85vh] object-contain rounded-lg shadow-2xl border border-slate-700"
            />
            <button
              type="button"
              onClick={() => setPreviewImageUri(null)}
              className="absolute -top-3 -right-3 p-1.5 rounded-full bg-slate-800 text-slate-200 hover:bg-slate-700 border border-slate-600 cursor-pointer shadow-lg"
            >
              <X className="w-4 h-4" />
            </button>
          </div>
        </div>
      )}

      <div ref={bottomRef} />
    </div>
  );
};
