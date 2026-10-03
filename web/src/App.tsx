import { useState, useEffect, useRef } from "react";
import { Sidebar, type ProjectGroupItem } from "./components/Sidebar";
import { TurnStream } from "./components/TurnStream";
import type { MessageItem } from "./components/ChatStream";
import { ChatInput, type AttachedMedia } from "./components/ChatInput";
import { TaskBanner } from "./components/TaskBanner";
import { QueuedMessages, type QueuedItem } from "./components/QueuedMessages";
import { FileTreePanel } from "./components/FileTreePanel";
import { GitDiffPanel } from "./components/GitDiffPanel";
import { type SelectedModelConfig } from "./components/ModelSelector";
import { ALL_MODELS, PROVIDER_GROUPS } from "./lib/models";
import { TerminalPanel } from "./components/TerminalPanel";
import { BrowserPanel } from "./components/BrowserPanel";
import { GitBranch, FolderTree, Terminal as TerminalIcon, Globe, Settings } from "lucide-react";

export function App() {
  const [projectGroups, setProjectGroups] = useState<ProjectGroupItem[]>([]);
  const [activeConversationId, setActiveConversationId] = useState<string | null>(null);
  const [activeConversationTitle, setActiveConversationTitle] = useState<string>("AI CLI Safety");
  const [messages, setMessages] = useState<MessageItem[]>([]);
  const [currentConfig, setCurrentConfig] = useState<SelectedModelConfig>(() => {
    try {
      const saved = localStorage.getItem("clara_current_config");
      if (saved) {
        const parsed = JSON.parse(saved);
        const found = ALL_MODELS.find((m) => m.id === parsed.model?.id);
        if (found) {
          return {
            model: found,
            effort: parsed.effort || "Medium",
          };
        }
      }
    } catch (e) {
      console.error("Error reading saved model config:", e);
    }
    // Mặc định ưu tiên Gemini (agy) vì là engine active sẵn có, thay vì claude bị inactive
    const defaultModel = ALL_MODELS.find((m) => m.agent === "agy") || ALL_MODELS[0];
    return {
      model: defaultModel,
      effort: "Medium",
    };
  });
  const [isStreaming, setIsStreaming] = useState(false);
  const [runningCommand, setRunningCommand] = useState<string | null>(null);
  const [pendingApproval, setPendingApproval] = useState<boolean>(false);

  // Quản lý hàng đợi tin nhắn (Queued Messages)
  const [queue, setQueue] = useState<QueuedItem[]>([]);
  const [editingText, setEditingText] = useState<string>("");

  // Toggle các cột phụ: Cột trái (FileTree), Cột phải (Tab: "terminal" | "changes" | "browser" | null)
  const [showFileTree, setShowFileTree] = useState<boolean>(() => {
    const saved = localStorage.getItem("clara_show_file_tree");
    return saved !== null ? saved === "true" : false; // Mặc định collapse
  });
  const handleToggleFileTree = () => {
    setShowFileTree((prev) => {
      const next = !prev;
      localStorage.setItem("clara_show_file_tree", String(next));
      return next;
    });
  };
  const [activeRightTab, setActiveRightTab] = useState<"terminal" | "changes" | "browser" | "settings" | null>(null);
  const [rightPanelWidth, setRightPanelWidth] = useState<number>(() => {
    const saved = localStorage.getItem("clara_right_panel_width");
    return saved ? parseInt(saved, 10) : 380;
  });
  const isResizingRef = useRef(false);

  // Resize handler cho cột bên phải
  useEffect(() => {
    const handleMouseMove = (e: MouseEvent) => {
      if (!isResizingRef.current) return;
      const newWidth = window.innerWidth - e.clientX;
      if (newWidth >= 260 && newWidth <= Math.min(window.innerWidth * 0.75, 900)) {
        setRightPanelWidth(newWidth);
        localStorage.setItem("clara_right_panel_width", newWidth.toString());
      }
    };

    const handleMouseUp = () => {
      isResizingRef.current = false;
      document.body.style.cursor = "default";
      document.body.style.userSelect = "auto";
    };

    window.addEventListener("mousemove", handleMouseMove);
    window.addEventListener("mouseup", handleMouseUp);
    return () => {
      window.removeEventListener("mousemove", handleMouseMove);
      window.removeEventListener("mouseup", handleMouseUp);
    };
  }, []);

  // Quản lý Alias name cho từng AI Tool
  const [toolAliases] = useState<Record<string, string>>(() => {
    try {
      const saved = localStorage.getItem("nexus_tool_aliases");
      return saved ? JSON.parse(saved) : {};
    } catch {
      return {};
    }
  });

  const wsRef = useRef<WebSocket | null>(null);
  const queueRef = useRef<QueuedItem[]>(queue);
  queueRef.current = queue;

  const isStreamingRef = useRef<boolean>(isStreaming);
  isStreamingRef.current = isStreaming;

  useEffect(() => {
    loadAntigravityProjects();
  }, []);

  const loadAntigravityProjects = () => {
    fetch("/api/antigravity/projects")
      .then((res) => res.json())
      .then((groups: ProjectGroupItem[]) => {
        setProjectGroups(groups);
        if (groups.length > 0 && groups[0].conversations.length > 0 && !activeConversationId) {
          selectConversation(groups[0].conversations[0].id);
        }
      })
      .catch((err) => console.error("Error loading antigravity projects:", err));
  };

  const selectConversation = (id: string) => {
    setActiveConversationId(id);
    for (const g of projectGroups) {
      const c = g.conversations.find((item) => item.id === id);
      if (c) {
        setActiveConversationTitle(c.title);
        break;
      }
    }

    fetch(`/api/antigravity/conversations/${id}/messages`)
      .then((res) => res.json())
      .then((data) => {
        setMessages(
          data.map((m: any) => ({
            role: m.role,
            content: m.content,
            agent: m.agent,
            model: m.model,
            media: m.media,
            steps: m.steps,
            is_running: m.is_running,
          }))
        );
      })
      .catch((err) => console.error("Error loading conversation messages:", err));

    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify({ type: "subscribe", session_id: id }));
    }
  };

  // Tự động đồng bộ Realtime các bước Tool & nội dung từ transcript Antigravity
  useEffect(() => {
    if (!activeConversationId) return;

    let isMounted = true;
    const syncMessages = () => {
      fetch(`/api/antigravity/conversations/${activeConversationId}/messages`)
        .then((res) => res.json())
        .then((data) => {
          if (!isMounted || !Array.isArray(data)) return;
          setMessages(
            data.map((m: any) => ({
              role: m.role,
              content: m.content,
              agent: m.agent,
              model: m.model,
              media: m.media,
              steps: m.steps,
              is_running: m.is_running,
            }))
          );
        })
        .catch(() => {});
    };

    // Chu kỳ sync nhanh khi đang chạy (1.2s), chậm hơn khi idle (4s)
    const lastMsg = messages[messages.length - 1];
    const isWorking = isStreaming || (lastMsg && lastMsg.is_running);
    const intervalTime = isWorking ? 1200 : 4000;

    const timer = setInterval(syncMessages, intervalTime);
    return () => {
      isMounted = false;
      clearInterval(timer);
    };
  }, [activeConversationId, isStreaming, messages.length]);

  const handleNewConversation = () => {
    setActiveConversationId(null);
    setActiveConversationTitle("New Session");
    setMessages([]);
    setQueue([]);
  };

  // Hàm dispatch prompt thật qua WebSocket
  const dispatchPrompt = (
    text: string,
    config: SelectedModelConfig,
    media?: AttachedMedia[]
  ) => {
    const userMedia = media?.map((m) => ({
      mime_type: m.mime_type,
      uri: m.uri,
    }));

    setMessages((prev) => [
      ...prev,
      {
        role: "user",
        content: text,
        media: userMedia,
      },
    ]);
    setIsStreaming(true);
    isStreamingRef.current = true;
    setRunningCommand(null);

    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      wsRef.current.send(
        JSON.stringify({
          type: "prompt",
          session_id: activeConversationId || "",
          content: text,
          media: userMedia,
          agent: config.model.agent,
          model: config.model.id,
          effort: config.effort.toLowerCase(),
          workspace: "/home/chungnh/AI Workspace",
        })
      );
    }
  };

  // Tự động kiểm tra và bốc tin nhắn từ Queue khi task trước đó chạy xong
  const checkAndProcessQueue = () => {
    if (queueRef.current.length > 0) {
      const nextItem = queueRef.current[0];
      setQueue((prev) => prev.slice(1));
      setTimeout(() => {
        dispatchPrompt(
          nextItem.text,
          currentConfig,
          nextItem.media?.map((m) => ({
            uri: m.uri,
            mime_type: m.mime_type,
            url: `/api/media?path=${encodeURIComponent(m.uri)}`,
          }))
        );
      }, 300);
    }
  };

  // WebSocket connection
  useEffect(() => {
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const host = window.location.host;
    const ws = new WebSocket(`${protocol}//${host}/ws`);
    wsRef.current = ws;

    ws.onmessage = (event) => {
      try {
        const evt = JSON.parse(event.data);

        switch (evt.type) {
          case "session_created":
            setActiveConversationId(evt.session_id);
            loadAntigravityProjects();
            break;

          case "task_started":
            setIsStreaming(true);
            setRunningCommand(evt.command || "Processing task...");
            break;

          case "task_finished":
          case "done":
            setIsStreaming(false);
            isStreamingRef.current = false;
            setRunningCommand(null);
            checkAndProcessQueue();
            break;

          case "token":
            setIsStreaming(true);
            setMessages((prev) => {
              const last = prev[prev.length - 1];
              if (last && last.role === "assistant") {
                return [
                  ...prev.slice(0, -1),
                  { ...last, content: last.content + evt.content },
                ];
              } else {
                return [
                  ...prev,
                  {
                    role: "assistant",
                    content: evt.content,
                    agent: currentConfig.model.agent,
                    model: currentConfig.model.name,
                  },
                ];
              }
            });
            break;

          case "diff":
            setMessages((prev) => {
              const last = prev[prev.length - 1];
              if (last && last.role === "assistant") {
                return [
                  ...prev.slice(0, -1),
                  { ...last, diffFile: evt.file, diffPatch: evt.patch },
                ];
              }
              return prev;
            });
            break;

          case "tool_request":
            setRunningCommand(evt.command || `Running ${evt.tool}...`);
            if (evt.requires_approval) {
              setPendingApproval(true);
            }
            setMessages((prev) => {
              const last = prev[prev.length - 1];
              if (last && last.role === "assistant") {
                return [
                  ...prev.slice(0, -1),
                  { ...last, toolCommand: evt.command },
                ];
              }
              return prev;
            });
            break;

          case "approval_resolved":
            setPendingApproval(false);
            break;

          case "tool_result":
            setPendingApproval(false);
            setMessages((prev) => {
              const last = prev[prev.length - 1];
              if (last && last.role === "assistant") {
                return [
                  ...prev.slice(0, -1),
                  { ...last, toolOutput: evt.output },
                ];
              }
              return prev;
            });
            break;

          case "done":
            setIsStreaming(false);
            setRunningCommand(null);
            setPendingApproval(false);
            checkAndProcessQueue();
            break;

          case "error":
            setIsStreaming(false);
            setRunningCommand(null);
            setMessages((prev) => [
              ...prev,
              {
                role: "assistant",
                content: `❌ **Error:** ${evt.message || "An unexpected error occurred"}`,
              },
            ]);
            checkAndProcessQueue();
            break;
        }
      } catch (err) {
        console.error("Parse WS error:", err);
      }
    };

    return () => {
      ws.close();
    };
  }, [currentConfig]);

  // Xử lý khi user gửi tin nhắn từ Input
  const handleSendMessage = (text: string, media?: AttachedMedia[]) => {
    const trimmed = text.trim();
    if (!trimmed && (!media || media.length === 0)) return;

    if (isStreamingRef.current) {
      // Đang có task chạy: Đẩy vào Queue
      const newItem: QueuedItem = {
        id: Math.random().toString(36).substring(7),
        text: trimmed,
        modelId: currentConfig.model.id,
        agent: currentConfig.model.agent,
        media: media?.map((m) => ({ uri: m.uri, mime_type: m.mime_type })),
      };
      setQueue((prev) => [...prev, newItem]);
    } else {
      // Rảnh rỗi: Gửi ngay lập tức
      dispatchPrompt(trimmed, currentConfig, media);
    }
  };

  const handleCancelTask = () => {
    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      wsRef.current.send(
        JSON.stringify({
          type: "cancel",
          session_id: activeConversationId || "",
        })
      );
    }
    setIsStreaming(false);
    isStreamingRef.current = false;
    setRunningCommand(null);
    setPendingApproval(false);
  };

  const handleApprove = () => {
    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      wsRef.current.send(
        JSON.stringify({
          type: "approve",
          session_id: activeConversationId || "",
        })
      );
    }
    setPendingApproval(false);
  };

  const handleReject = () => {
    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      wsRef.current.send(
        JSON.stringify({
          type: "reject",
          session_id: activeConversationId || "",
        })
      );
    }
    setPendingApproval(false);
  };

  // Các thao tác với Queued Item
  const handleRemoveQueue = (id: string) => {
    setQueue((prev) => prev.filter((item) => item.id !== id));
  };

  const handleSendNowQueue = (id: string) => {
    const item = queue.find((q) => q.id === id);
    if (!item) return;
    setQueue((prev) => prev.filter((q) => q.id !== id));
    handleCancelTask();
    setTimeout(() => {
      dispatchPrompt(
        item.text,
        currentConfig,
        item.media?.map((m) => ({
          uri: m.uri,
          mime_type: m.mime_type,
          url: `/api/media?path=${encodeURIComponent(m.uri)}`,
        }))
      );
    }, 200);
  };

  const handleEditQueue = (id: string) => {
    const item = queue.find((q) => q.id === id);
    if (!item) return;
    setQueue((prev) => prev.filter((q) => q.id !== id));
    setEditingText(item.text);
  };

  const handleUpdateConfig = (config: SelectedModelConfig) => {
    setCurrentConfig(config);
    try {
      localStorage.setItem("clara_current_config", JSON.stringify(config));
    } catch (e) {
      console.error("Error saving model config:", e);
    }
  };

  // Đồng bộ với danh sách agent từ backend để đảm bảo không rơi vào agent không khả dụng (inactive)
  useEffect(() => {
    fetch("/api/agents")
      .then((res) => res.json())
      .then((agents: any[]) => {
        if (!Array.isArray(agents)) return;
        const availableMap = new Map<string, boolean>();
        agents.forEach((a) => availableMap.set(a.id, !!a.available));

        // Kiểm tra xem config hiện tại có trỏ vào agent bị inactive không
        setCurrentConfig((prev) => {
          const currentAgent = prev.model.agent;
          const isCurrentAvailable = availableMap.get(currentAgent);

          // Nếu agent hiện tại vẫn available (hoặc là custom/unknown), giữ nguyên
          if (isCurrentAvailable !== false) return prev;

          // Nếu agent hiện tại bị inactive (như claude), tìm agent đầu tiên đang available
          const firstAvailable = agents.find((a) => a.available);
          if (firstAvailable) {
            const nextModel = ALL_MODELS.find((m) => m.agent === firstAvailable.id);
            if (nextModel) {
              const updated = { model: nextModel, effort: prev.effort };
              try {
                localStorage.setItem("clara_current_config", JSON.stringify(updated));
              } catch (_) {}
              return updated;
            }
          }
          return prev;
        });
      })
      .catch(() => {});
  }, []);

  const handleSelectTool = (agentId: string) => {
    const group = PROVIDER_GROUPS.find((g) => {
      if (agentId === "agy") return g.id === "gemini";
      if (agentId === "claude") return g.id === "anthropic";
      if (agentId === "codex") return g.id === "openai";
      return g.id === agentId;
    });

    if (group && group.models.length > 0) {
      handleUpdateConfig({
        model: group.models[0],
        effort: currentConfig.effort || "Medium",
      });
    }
  };

  return (
    <div className="flex h-screen w-screen bg-[#11141a] text-[#cbd5e1] overflow-hidden">
      {/* Sidebar Agent Hub */}
      <Sidebar
        projectGroups={projectGroups}
        activeConversationId={activeConversationId}
        onSelectConversation={selectConversation}
        onNewConversation={handleNewConversation}
      />

      {/* CỘT TRÁI: File Explorer Panel (Duyệt cây thư mục project) */}
      {showFileTree && (
        <FileTreePanel
          currentPath="/home/chungnh/AI Workspace"
          onClose={() => {
            setShowFileTree(false);
            localStorage.setItem("clara_show_file_tree", "false");
          }}
        />
      )}

      {/* CỘT GIỮA: Main Chat & Antigravity Turn Stream */}
      <main className="flex-1 flex flex-col h-full bg-[#11141a] overflow-hidden">
        {/* Top Header với Tool Switcher & Toggle buttons */}
        <header className="h-11 border-b border-[#1d222b] px-4 flex items-center justify-between bg-[#14171e] select-none">
          <div className="flex items-center gap-2.5">
            {/* Nút bật/tắt Cột Trái: Files */}
            <button
              type="button"
              onClick={handleToggleFileTree}
              className={`p-1.5 rounded transition-colors ${
                showFileTree
                  ? "bg-[#1f2636] text-sky-400"
                  : "text-slate-500 hover:text-slate-300 hover:bg-[#1a1f2b]"
              }`}
              title="Toggle File Explorer"
            >
              <FolderTree className="w-3.5 h-3.5" />
            </button>

            {/* Conversation Title chuẩn Antigravity */}
            <span className="text-xs font-normal text-slate-400 truncate max-w-xs">
              {activeConversationTitle}
            </span>
          </div>

          <div className="flex items-center gap-1.5 text-xs text-slate-400">
            {/* 1. Terminal Icon (>_) */}
            <button
              type="button"
              onClick={() => setActiveRightTab((prev) => (prev === "terminal" ? null : "terminal"))}
              className={`p-1.5 rounded-lg transition-colors border cursor-pointer ${
                activeRightTab === "terminal"
                  ? "bg-[#182030] text-sky-400 border-sky-500/30"
                  : "text-slate-400 border-transparent hover:border-[#232a38] hover:bg-[#1a1f2b] hover:text-slate-200"
              }`}
              title="Terminal (>_)"
            >
              <TerminalIcon className="w-3.5 h-3.5 text-sky-400" />
            </button>

            {/* 2. Changes / Git Diff Icon */}
            <button
              type="button"
              onClick={() => setActiveRightTab((prev) => (prev === "changes" ? null : "changes"))}
              className={`p-1.5 rounded-lg transition-colors border cursor-pointer ${
                activeRightTab === "changes"
                  ? "bg-[#182620] text-emerald-300 border-emerald-500/30"
                  : "text-slate-400 border-transparent hover:border-[#232a38] hover:bg-[#1a1f2b] hover:text-slate-200"
              }`}
              title="Changes / Git Diff"
            >
              <GitBranch className="w-3.5 h-3.5 text-emerald-400" />
            </button>

            {/* 3. Browser Icon (Globe) */}
            <button
              type="button"
              onClick={() => setActiveRightTab((prev) => (prev === "browser" ? null : "browser"))}
              className={`p-1.5 rounded-lg transition-colors border cursor-pointer ${
                activeRightTab === "browser"
                  ? "bg-[#272318] text-amber-300 border-amber-500/30"
                  : "text-slate-400 border-transparent hover:border-[#232a38] hover:bg-[#1a1f2b] hover:text-slate-200"
              }`}
              title="Browser"
            >
              <Globe className="w-3.5 h-3.5 text-amber-400" />
            </button>

            {/* 4. Settings */}
            <button
              type="button"
              onClick={() => setActiveRightTab((prev) => (prev === "settings" ? null : "settings"))}
              className={`p-1.5 rounded-lg transition-colors border cursor-pointer ${
                activeRightTab === "settings"
                  ? "bg-[#1f2030] text-slate-200 border-slate-500/30"
                  : "text-slate-400 border-transparent hover:border-[#232a38] hover:bg-[#1a1f2b] hover:text-slate-200"
              }`}
              title="Settings"
            >
              <Settings className="w-3.5 h-3.5" />
            </button>
          </div>
        </header>

        {/* Turn-based stream: căn thẳng hàng 1 cột, ghim câu hỏi ở đỉnh */}
        <TurnStream
          conversationId={activeConversationId}
          messages={messages}
          isStreaming={isStreaming}
        />

        {/* Banner quản lý Task đang chạy ngầm & Nút Approve/Reject lệnh */}
        <TaskBanner
          isRunning={!!runningCommand}
          commandText={runningCommand || undefined}
          requiresApproval={pendingApproval}
          onApprove={handleApprove}
          onReject={handleReject}
        />

        {/* Banner Quản lý Hàng đợi tin nhắn (Queued Messages) */}
        <QueuedMessages
          queue={queue}
          onRemove={handleRemoveQueue}
          onSendNow={handleSendNowQueue}
          onEdit={handleEditQueue}
        />

        {/* Chat input với Model & Effort config */}
        <ChatInput
          onSendMessage={handleSendMessage}
          onCancelTask={handleCancelTask}
          currentConfig={currentConfig}
          onSelectConfig={handleUpdateConfig}
          isRunning={isStreaming}
          initialText={editingText}
          onTextConsumed={() => setEditingText("")}
          onOpenBrowser={() => setActiveRightTab("browser")}
          onOpenChanges={() => setActiveRightTab("changes")}
          messages={messages}
          onSelectAgent={handleSelectTool}
          toolAliases={toolAliases}
        />
      </main>

      {/* CỘT PHẢI (Terminal, Changes, Browser) - Có thể kéo resize chiều rộng */}
      {activeRightTab && (
        <aside
          style={{ width: `${rightPanelWidth}px` }}
          className="relative flex flex-col h-full bg-[#11141a] border-l border-[#1d222b] shrink-0 select-none overflow-hidden animate-in slide-in-from-right duration-150"
        >
          {/* Resize Handle ở mép trái của cột */}
          <div
            onMouseDown={(e) => {
              e.preventDefault();
              isResizingRef.current = true;
              document.body.style.cursor = "col-resize";
              document.body.style.userSelect = "none";
            }}
            className="absolute top-0 bottom-0 left-0 w-1.5 hover:w-2 -ml-0.5 cursor-col-resize hover:bg-sky-500/50 z-30 transition-colors"
            title="Drag to resize width"
          />

          {/* Nội dung tương ứng với tab đang chọn */}
          {activeRightTab === "terminal" && (
            <TerminalPanel
              workDir="/home/chungnh/AI Workspace"
              onClose={() => setActiveRightTab(null)}
            />
          )}

          {activeRightTab === "changes" && (
            <GitDiffPanel
              currentPath="/home/chungnh/AI Workspace"
              onClose={() => setActiveRightTab(null)}
            />
          )}

          {activeRightTab === "browser" && (
            <BrowserPanel
              defaultUrl="http://localhost:8088"
              onClose={() => setActiveRightTab(null)}
            />
          )}
        </aside>
      )}
    </div>
  );
}

export default App;
