import React, { useState, useEffect, useRef } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import {
  ChevronRight,
  ChevronDown,
  Terminal,
  FileCode,
  Sparkles,
  Copy,
  Check,
  X,
} from "lucide-react";
import type { MessageItem, ToolStepItem } from "./ChatStream";

export interface ConversationTurn {
  id: string;
  userMessage: MessageItem;
  assistantMessage?: MessageItem;
}

interface TurnStreamProps {
  conversationId?: string | null;
  messages: MessageItem[];
  isStreaming: boolean;
}

const ScrollContext = React.createContext<React.RefObject<HTMLDivElement | null> | null>(null);

export const TurnStream: React.FC<TurnStreamProps> = ({ conversationId, messages, isStreaming }) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const bottomRef = useRef<HTMLDivElement>(null);
  const [previewImageUri, setPreviewImageUri] = useState<string | null>(null);
  const [isReady, setIsReady] = useState(false);

  // Nhóm các messages thành từng cặp Turn: [User -> Assistant]
  const turns: ConversationTurn[] = [];
  let currentTurn: ConversationTurn | null = null;

  messages.forEach((msg, idx) => {
    if (msg.role === "user") {
      if (currentTurn) {
        turns.push(currentTurn);
      }
      currentTurn = {
        id: `turn-${idx}`,
        userMessage: msg,
      };
    } else if (msg.role === "assistant") {
      if (currentTurn) {
        currentTurn.assistantMessage = msg;
        turns.push(currentTurn);
        currentTurn = null;
      } else {
        // Trường hợp assistant đầu tiên không có user
        turns.push({
          id: `turn-${idx}`,
          userMessage: { role: "user", content: "" },
          assistantMessage: msg,
        });
      }
    }
  });

  if (currentTurn) {
    turns.push(currentTurn);
  }

  const isAutoScrollLockedRef = useRef(false);
  const prevConvIdRef = useRef<string | null | undefined>(undefined);
  const prevMessagesLenRef = useRef<number>(0);

  // Khi chuyển sang conversation mới: Reset và nhảy tức thì (instant) xuống đáy, không cuộn từ trên xuống
  useEffect(() => {
    if (conversationId !== prevConvIdRef.current) {
      prevConvIdRef.current = conversationId;
      isAutoScrollLockedRef.current = false;
      setIsReady(false);

      if (messages.length > 0) {
        requestAnimationFrame(() => {
          if (containerRef.current) {
            containerRef.current.scrollTop = containerRef.current.scrollHeight;
          }
          setIsReady(true);
        });
      }
    }
  }, [conversationId, messages.length]);

  // Sau khi load xong dữ liệu lần đầu của conversation
  useEffect(() => {
    if (!isReady && messages.length > 0 && containerRef.current) {
      containerRef.current.scrollTop = containerRef.current.scrollHeight;
      setIsReady(true);
    }
  }, [messages.length, isReady]);

  // Theo dõi hành vi scroll của user: Nếu user cuộn lên (cách đáy > 100px) thì khoá auto-scroll
  const handleScroll = () => {
    const el = containerRef.current;
    if (!el) return;
    const distanceFromBottom = el.scrollHeight - el.scrollTop - el.clientHeight;
    if (distanceFromBottom > 100) {
      isAutoScrollLockedRef.current = true;
    } else {
      isAutoScrollLockedRef.current = false;
    }
  };

  // Khi có token stream hoặc turn mới: chỉ cuộn mượt nếu user không khóa và ĐÃ xong bước init
  useEffect(() => {
    if (isReady && !isAutoScrollLockedRef.current && (isStreaming || messages.length > prevMessagesLenRef.current)) {
      bottomRef.current?.scrollIntoView({ behavior: "smooth" });
    }
    prevMessagesLenRef.current = messages.length;
  }, [messages, isStreaming, isReady]);

  // Khi user gửi câu hỏi mới: mở khóa và cuộn mượt xuống câu hỏi mới
  useEffect(() => {
    const lastMsg = messages[messages.length - 1];
    if (lastMsg && lastMsg.role === "user") {
      isAutoScrollLockedRef.current = false;
      bottomRef.current?.scrollIntoView({ behavior: "smooth" });
    }
  }, [messages.length]);

  if (turns.length === 0) {
    return (
      <div className="flex-1 flex flex-col items-center justify-center p-8 text-center select-none overflow-x-hidden">
        <div className="w-11 h-11 rounded-2xl bg-[#1e232d] border border-[#2d3442] flex items-center justify-center mb-3 shadow-inner">
          <Sparkles className="w-5 h-5 text-slate-400" />
        </div>
        <h2 className="text-lg font-medium text-slate-200 mb-1.5">Agent Hub</h2>
        <p className="text-xs text-slate-400 max-w-sm leading-relaxed">
          Ask questions, inspect diffs, and orchestrate CLI agents.
        </p>
      </div>
    );
  }

  return (
    <ScrollContext.Provider value={containerRef}>
      <div
        ref={containerRef}
        onScroll={handleScroll}
        className="flex-1 overflow-y-auto overflow-x-hidden px-4 pb-6 pt-0 w-full"
      >
        <div className="max-w-4xl mx-auto space-y-8 pt-3">
          {turns.map((turn, tIdx) => (
            <TurnItem
              key={turn.id || tIdx}
              turn={turn}
              onPreviewImage={(url) => setPreviewImageUri(url)}
            />
          ))}

          {isStreaming && (
            <div className="flex items-center gap-2 text-xs text-slate-400 animate-pulse py-2">
              <Sparkles className="w-3.5 h-3.5 text-slate-500" />
              <span>Generating response...</span>
            </div>
          )}
        </div>

        {/* Lightbox Xem ảnh */}
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
    </ScrollContext.Provider>
  );
};

type GroupType = "explore" | "edit" | "command" | "other";

interface StepGroup {
  id: string;
  type: GroupType;
  title: string;
  steps: ToolStepItem[];
}

function formatCwd(cwd?: string): string {
  if (!cwd) return "agent-hub";
  const clean = cwd.replace(/\\/g, "/").replace(/\/+$/, "");
  const parts = clean.split("/").filter(Boolean);
  if (parts.length === 0) return "agent-hub";
  const hubIdx = parts.indexOf("agent-hub");
  if (hubIdx !== -1) {
    return parts.slice(hubIdx).join("/");
  }
  if (parts.length >= 2) {
    return `${parts[parts.length - 2]}/${parts[parts.length - 1]}`;
  }
  return parts[parts.length - 1];
}

function renderCommandLine(cmd: string) {
  if (!cmd) return null;
  const cleanCmd = cmd.replace(/\\n/g, " \n").replace(/\\"/g, '"').trim();
  const tokens = cleanCmd.split(/(\s+|&&|\|\||;|\|)/);
  let expectCommand = true;

  return tokens.map((part, idx) => {
    const trimmed = part.trim();
    if (trimmed === "&&" || trimmed === "||" || trimmed === ";" || trimmed === "|") {
      expectCommand = true;
      return <span key={idx} className="text-amber-400 font-semibold">{part}</span>;
    }
    if (trimmed === "") {
      return <span key={idx}>{part}</span>;
    }

    if (expectCommand) {
      expectCommand = false;
      return <span key={idx} className="text-[#e5c07b] font-medium">{part}</span>;
    }

    if (trimmed.startsWith("-")) {
      return <span key={idx} className="text-sky-300/90">{part}</span>;
    }

    if (trimmed === "sudo") {
      expectCommand = true;
      return <span key={idx} className="text-[#e5c07b] font-medium">{part}</span>;
    }

    return <span key={idx} className="text-slate-200">{part}</span>;
  });
}

function groupSteps(steps: ToolStepItem[]): StepGroup[] {
  const groups: StepGroup[] = [];

  const categorize = (step: ToolStepItem): GroupType => {
    const name = (step.name || "").toLowerCase();
    if (name === "run_command" || Boolean(step.command)) return "command";
    if (
      name === "view_file" ||
      name.includes("read") ||
      name.includes("search") ||
      name.includes("grep") ||
      name.includes("list")
    ) {
      return "explore";
    }
    if (
      name.includes("edit") ||
      name.includes("write") ||
      name.includes("replace") ||
      name.includes("patch")
    ) {
      return "edit";
    }
    return "other";
  };

  steps.forEach((step, idx) => {
    const type = categorize(step);
    const last = groups[groups.length - 1];

    if (last && last.type === type) {
      last.steps.push(step);
    } else {
      groups.push({
        id: `group-${idx}`,
        type,
        title: "",
        steps: [step],
      });
    }
  });

  groups.forEach((g) => {
    const count = g.steps.length;
    if (g.type === "explore") {
      g.title = `Explored ${count} file${count > 1 ? "s" : ""}`;
    } else if (g.type === "edit") {
      g.title = `Edited ${count} file${count > 1 ? "s" : ""}`;
    } else if (g.type === "command") {
      g.title = `Ran ${count} command${count > 1 ? "s" : ""}`;
    } else {
      g.title = count === 1 ? (g.steps[0].summary || g.steps[0].action || g.steps[0].name) : `${count} actions`;
    }
  });

  return groups;
}

const CommandStepItem: React.FC<{
  step: ToolStepItem;
  defaultExpanded?: boolean;
}> = ({ step, defaultExpanded = true }) => {
  const [isOpen, setIsOpen] = useState(defaultExpanded);
  const [copied, setCopied] = useState(false);

  const commandText = step.command || step.action;
  const cwdText = formatCwd(step.cwd);
  const title = step.summary || step.action || "Run command";

  const handleCopy = (e: React.MouseEvent) => {
    e.stopPropagation();
    navigator.clipboard.writeText(commandText);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="space-y-1.5 my-1 text-xs">
      <div
        onClick={() => setIsOpen(!isOpen)}
        className="inline-flex items-center gap-1.5 text-slate-300 hover:text-white cursor-pointer select-none py-0.5 transition-colors"
      >
        <span className="text-[12.5px] font-sans font-medium">{title}</span>
        {isOpen ? (
          <ChevronDown className="w-3.5 h-3.5 text-slate-500" />
        ) : (
          <ChevronRight className="w-3.5 h-3.5 text-slate-500" />
        )}
      </div>

      {isOpen && (
        <div className="relative group rounded-xl bg-[#0d1117] border border-[#21262d] p-3.5 my-1 shadow-sm font-mono text-[12px] select-text">
          <button
            type="button"
            onClick={handleCopy}
            className="absolute top-2.5 right-2.5 p-1 rounded bg-[#161b22] text-slate-400 hover:text-slate-200 opacity-0 group-hover:opacity-100 transition-opacity z-10 border border-[#30363d]"
            title="Copy command"
          >
            {copied ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
          </button>

          <div className="flex items-start gap-2 break-all leading-relaxed">
            <span className="text-[#8b949e] shrink-0 select-none">{cwdText} $</span>
            <div className="flex-1 overflow-x-auto whitespace-pre-wrap">
              {renderCommandLine(commandText)}
            </div>
          </div>

          {step.output && (
            <div className="mt-2.5 pt-2 text-[#c9d1d9] whitespace-pre-wrap break-all max-h-72 overflow-y-auto leading-relaxed border-t border-[#21262d]/60 select-text font-mono text-[11.5px] selection:bg-sky-900/50">
              {step.output}
            </div>
          )}
        </div>
      )}
    </div>
  );
};

const StepGroupRow: React.FC<{
  group: StepGroup;
  defaultExpanded?: boolean;
}> = ({ group, defaultExpanded = false }) => {
  const [isOpen, setIsOpen] = useState(defaultExpanded);

  return (
    <div className="space-y-1">
      <div
        onClick={() => setIsOpen(!isOpen)}
        className="inline-flex items-center gap-1.5 text-slate-400 hover:text-slate-200 cursor-pointer select-none font-normal py-0.5 text-[13px] transition-colors"
      >
        <span>{group.title}</span>
        {isOpen ? (
          <ChevronDown className="w-3.5 h-3.5 text-slate-500" />
        ) : (
          <ChevronRight className="w-3.5 h-3.5 text-slate-500" />
        )}
      </div>

      {isOpen && (
        <div className="pl-3 py-0.5 space-y-1">
          {group.type === "command" ? (
            group.steps.map((st, idx) => (
              <CommandStepItem key={idx} step={st} defaultExpanded={true} />
            ))
          ) : (
            group.steps.map((st, idx) => {
              const label =
                st.summary ||
                st.action ||
                (st.path
                  ? `${group.type === "explore" ? "Viewed" : "Edited"} ${st.path.split("/").pop()}`
                  : st.name);
              return (
                <div key={idx} className="flex items-center gap-2 text-[12px] text-slate-400 py-0.5">
                  <span className="text-slate-600">•</span>
                  <span className="font-mono text-slate-300">{label}</span>
                </div>
              );
            })
          )}
        </div>
      )}
    </div>
  );
};

const TurnStepsBlock: React.FC<{
  steps: ToolStepItem[];
  duration?: string;
  isRunning?: boolean;
}> = ({ steps, duration, isRunning }) => {
  const [isMainOpen, setIsMainOpen] = useState(true);

  if (!steps || steps.length === 0) return null;

  const groups = groupSteps(steps);
  const headerText = isRunning
    ? "Working..."
    : duration
    ? `Worked for ${duration}`
    : `Worked for ${steps.length} step${steps.length > 1 ? "s" : ""}`;

  return (
    <div className="pt-3 pb-1 space-y-1.5 text-xs">
      <div
        onClick={() => setIsMainOpen(!isMainOpen)}
        className="inline-flex items-center gap-1.5 text-slate-400 hover:text-slate-200 cursor-pointer py-1 select-none font-normal text-[13px] transition-colors"
      >
        {isRunning && <span className="w-2 h-2 rounded-full bg-sky-400 animate-pulse inline-block mr-0.5" />}
        <span>{headerText}</span>
        {isMainOpen ? (
          <ChevronDown className="w-3.5 h-3.5 text-slate-500" />
        ) : (
          <ChevronRight className="w-3.5 h-3.5 text-slate-500" />
        )}
      </div>

      {isMainOpen && (
        <div className="pl-3 space-y-1">
          {groups.map((group) => (
            <StepGroupRow
              key={group.id}
              group={group}
              defaultExpanded={group.type === "command" || Boolean(isRunning)}
            />
          ))}
        </div>
      )}
    </div>
  );
};


// Component từng Turn: Có Pinned Question Card ở đỉnh
const TurnItem: React.FC<{
  turn: ConversationTurn;
  onPreviewImage: (url: string) => void;
}> = ({ turn, onPreviewImage }) => {
  const scrollContainerRef = React.useContext(ScrollContext);
  const sentinelRef = useRef<HTMLDivElement>(null);
  const [isSticky, setIsSticky] = useState(false);
  const [isStickyExpanded, setIsStickyExpanded] = useState(false);
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    const sentinel = sentinelRef.current;
    if (!sentinel) return;

    const observer = new IntersectionObserver(
      ([entry]) => {
        const root = scrollContainerRef?.current;
        const rootTop = root ? root.getBoundingClientRect().top : 0;
        const isAbove = entry.boundingClientRect.top < rootTop;
        setIsSticky(!entry.isIntersecting && isAbove);
      },
      {
        root: scrollContainerRef?.current || null,
        threshold: [0],
      }
    );

    observer.observe(sentinel);
    return () => observer.disconnect();
  }, [scrollContainerRef]);

  const handleCopy = () => {
    navigator.clipboard.writeText(turn.userMessage.content);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const hasMedia = turn.userMessage.media && turn.userMessage.media.length > 0;
  const hasSteps = turn.assistantMessage?.steps && turn.assistantMessage.steps.length > 0;

  return (
    <div className="relative border-b border-[#1c222c] pb-8 last:border-b-0">
      {/* Vạch sentinel ngay trước card để nhận biết khi card chạm đỉnh container */}
      <div ref={sentinelRef} className="h-0 w-full pointer-events-none -mt-4" />

      {/* 📌 QUESTION CARD */}
      <div
        className={`sticky top-0 z-20 transition-all ${
          isSticky
            ? "py-1.5 pointer-events-none"
            : "py-2 bg-transparent"
        }`}
      >
        {isSticky ? (
          /* KHI BỊ STICKY Ở TOP: Thu gọn thành thanh compact có shadow dropdown trực tiếp */
          <div className="pointer-events-auto max-w-4xl mx-auto rounded-xl bg-[#141720]/95 backdrop-blur-md border border-[#2a3244] shadow-2xl shadow-black/80 overflow-hidden text-slate-200 transition-all">
            <div
              onClick={() => setIsStickyExpanded(!isStickyExpanded)}
              className="flex items-center justify-between px-3.5 py-2.5 cursor-pointer hover:bg-[#1f222b] transition-colors"
            >
              <div className="flex items-center gap-2.5 flex-1 min-w-0 pr-3">
                <button
                  type="button"
                  className="text-slate-400 hover:text-slate-200 p-0.5 shrink-0"
                >
                  {isStickyExpanded ? (
                    <ChevronDown className="w-3.5 h-3.5 text-slate-400" />
                  ) : (
                    <ChevronRight className="w-3.5 h-3.5 text-slate-400" />
                  )}
                </button>

                {hasMedia && (
                  <div className="w-5 h-5 rounded bg-[#252c3b] border border-[#374256] overflow-hidden shrink-0 flex items-center justify-center">
                    <img
                      src={`/api/media?path=${encodeURIComponent(turn.userMessage.media![0].uri)}`}
                      alt="thumb"
                      className="w-full h-full object-cover"
                    />
                  </div>
                )}

                <span className={`text-[13px] text-slate-200 ${isStickyExpanded ? "" : "truncate"}`}>
                  {turn.userMessage.content || "Empty prompt"}
                </span>
              </div>

              <div className="flex items-center gap-2 shrink-0">
                <button
                  type="button"
                  onClick={(e) => {
                    e.stopPropagation();
                    handleCopy();
                  }}
                  className="p-1 rounded text-slate-400 hover:text-slate-200 hover:bg-[#252833] transition-colors"
                  title="Copy prompt"
                >
                  {copied ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
                </button>
              </div>
            </div>

            {isStickyExpanded && (
              <div className="px-4 pb-3.5 pt-1 border-t border-[#232630] bg-[#14161c] text-xs space-y-2">
                {hasMedia && (
                  <div className="flex flex-wrap gap-2 pt-1">
                    {turn.userMessage.media!.map((m, mIdx) => (
                      <div
                        key={mIdx}
                        className="w-14 h-14 rounded-lg overflow-hidden border border-[#2b3240] bg-[#12151b] cursor-pointer hover:border-slate-400 transition-all shrink-0"
                        onClick={() => onPreviewImage(`/api/media?path=${encodeURIComponent(m.uri)}`)}
                      >
                        <img
                          src={`/api/media?path=${encodeURIComponent(m.uri)}`}
                          alt="attachment"
                          className="w-full h-full object-cover"
                        />
                      </div>
                    ))}
                  </div>
                )}
                <div className="text-slate-200 leading-relaxed whitespace-pre-wrap text-[13px]">
                  {turn.userMessage.content}
                </div>
              </div>
            )}
          </div>
        ) : (
          /* KHI BÌNH THƯỜNG TRONG DÒNG CHAT: Hiện đầy đủ, KHÔNG collapse, KHÔNG có nút chevron */
          <div className="rounded-2xl bg-[#181a20] border border-[#232630] shadow-sm p-4 text-slate-200 transition-all">
            {hasMedia && (
              <div className="flex flex-wrap gap-2 mb-2.5">
                {turn.userMessage.media!.map((m, mIdx) => (
                  <div
                    key={mIdx}
                    className="relative w-20 h-20 rounded-lg overflow-hidden border border-[#2b3240] bg-[#12151b] group cursor-zoom-in shadow-sm hover:border-slate-400 transition-all shrink-0"
                    onClick={() => onPreviewImage(`/api/media?path=${encodeURIComponent(m.uri)}`)}
                    title="Click to zoom image"
                  >
                    <img
                      src={`/api/media?path=${encodeURIComponent(m.uri)}`}
                      alt="attachment"
                      className="w-full h-full object-cover group-hover:scale-105 transition-transform duration-200"
                    />
                  </div>
                ))}
              </div>
            )}

            <div className="flex items-start justify-between gap-3">
              <div className="text-slate-100 leading-relaxed whitespace-pre-wrap text-[14px] flex-1">
                {turn.userMessage.content || "Empty prompt"}
              </div>
              <button
                type="button"
                onClick={handleCopy}
                className="p-1 rounded text-slate-400 hover:text-slate-200 hover:bg-[#252833] transition-colors shrink-0"
                title="Copy prompt"
              >
                {copied ? <Check className="w-4 h-4 text-emerald-400" /> : <Copy className="w-4 h-4" />}
              </button>
            </div>
          </div>
        )}
      </div>

      {/* 🛠️ CÁC BƯỚC TOOL CALL / HÀNH ĐỘNG CỦA AGENT (Giống Hình 2 Antigravity) */}
      {hasSteps && (
        <TurnStepsBlock
          steps={turn.assistantMessage!.steps!}
          duration={turn.assistantMessage?.duration}
          isRunning={turn.assistantMessage?.is_running}
        />
      )}

      {/* ⏳ Trạng thái đang chạy realtime (Working...) giống Antigravity */}
      {turn.assistantMessage?.is_running && (
        <div className="flex items-center gap-2 pt-2 pb-1 text-xs text-sky-400 animate-pulse font-medium">
          <div className="w-2 h-2 rounded-full bg-sky-400" />
          <span>Working...</span>
        </div>
      )}

      {/* 📄 PHẦN TRẢ LỜI CỦA ASSISTANT (Thẳng hàng, căn lề trái như tài liệu) */}
      {turn.assistantMessage && turn.assistantMessage.content && (
        <div className="pt-4 px-0 space-y-4 text-slate-200">
          {/* Markdown Content - Chỉnh font và màu sắc sắc nét chuẩn Antigravity */}
          <div className="text-[14px] leading-[1.65] text-[#d1d5db] space-y-3.5">
            <ReactMarkdown
              remarkPlugins={[remarkGfm]}
              components={{
                h1: ({ node, ...props }) => <h1 className="text-lg font-semibold text-white mt-4 mb-2" {...props} />,
                h2: ({ node, ...props }) => <h2 className="text-[16px] font-semibold text-white mt-4 mb-2" {...props} />,
                h3: ({ node, ...props }) => <h3 className="text-[14.5px] font-semibold text-white mt-3 mb-1.5" {...props} />,
                p: ({ node, ...props }) => <p className="mb-3 text-[#d1d5db] leading-relaxed" {...props} />,
                ul: ({ node, ...props }) => <ul className="list-disc pl-5 space-y-1.5 text-[#d1d5db]" {...props} />,
                ol: ({ node, ...props }) => <ol className="list-decimal pl-5 space-y-1.5 text-[#d1d5db]" {...props} />,
                li: ({ node, ...props }) => <li className="leading-relaxed" {...props} />,
                strong: ({ node, ...props }) => <strong className="font-semibold text-slate-100" {...props} />,
                code: ({ node, className, children, ...props }: any) => {
                  const content = String(children || "");
                  const isBlock = content.includes("\n") || Boolean(className);
                  if (!isBlock) {
                    return (
                      <code className="bg-[#1e222b] text-[#f1f5f9] px-1.5 py-0.5 rounded text-[12.5px] font-mono border border-[#2d3340]" {...props}>
                        {children}
                      </code>
                    );
                  }
                  return (
                    <div className="my-3 rounded-xl bg-[#14171f] border border-[#232733] overflow-hidden">
                      <pre className="p-3.5 overflow-x-auto text-[12.5px] font-mono text-[#e2e8f0] leading-relaxed">
                        <code className={className} {...props}>
                          {children}
                        </code>
                      </pre>
                    </div>
                  );
                },
              }}
            >
              {turn.assistantMessage.content}
            </ReactMarkdown>
          </div>

          {/* Diff Block (nếu có) */}
          {turn.assistantMessage.diffPatch && (
            <div className="mt-3 rounded-lg bg-[#11141a] border border-[#242934] overflow-hidden max-w-full text-xs">
              <div className="flex items-center justify-between px-3 py-1.5 bg-[#161a22] border-b border-[#242934] font-mono text-slate-400">
                <span className="flex items-center gap-1.5 truncate">
                  <FileCode className="w-3.5 h-3.5 text-sky-400 shrink-0" />
                  <span className="truncate">{turn.assistantMessage.diffFile || "Changes"}</span>
                </span>
                <span className="text-[10px] text-emerald-400 font-semibold shrink-0">Diff View</span>
              </div>
              <pre className="p-3 font-mono overflow-x-auto text-slate-300 leading-relaxed max-w-full">
                {turn.assistantMessage.diffPatch.split("\n").map((line, i) => {
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

          {/* Tool Execution Block (nếu có) */}
          {turn.assistantMessage.toolCommand && (
            <div className="mt-3 rounded-lg bg-[#11141a] border border-[#242934] overflow-hidden max-w-full text-xs">
              <div className="flex items-center gap-2 px-3 py-1.5 bg-[#161a22] border-b border-[#242934] font-mono text-slate-400">
                <Terminal className="w-3.5 h-3.5 text-amber-400 shrink-0" />
                <span>Command Execution</span>
              </div>
              <div className="p-2.5 font-mono text-amber-200/80 bg-black/30 break-all">
                $ {turn.assistantMessage.toolCommand}
              </div>
              {turn.assistantMessage.toolOutput && (
                <pre className="p-2.5 font-mono text-slate-400 max-h-40 overflow-y-auto overflow-x-auto border-t border-[#202530] max-w-full">
                  {turn.assistantMessage.toolOutput}
                </pre>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
};
