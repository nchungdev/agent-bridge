import { useState, useEffect, useMemo, useRef } from "react";
import { Sidebar, type ProjectGroupItem } from "./components/Sidebar";
import { TurnStream } from "./components/TurnStream";
import type { MessageItem } from "./components/ChatStream";
import { ChatInput, type AttachedMedia } from "./components/ChatInput";
import { TaskBanner } from "./components/TaskBanner";
import { QueuedMessages, type QueuedItem } from "./components/QueuedMessages";
import { FileTreePanel } from "./components/FileTreePanel";
import { GitDiffPanel } from "./components/GitDiffPanel";
import { type SelectedModelConfig } from "./components/ModelSelector";
import { ALL_MODELS, PROVIDER_GROUPS, type ModelDefinition } from "./lib/models";
import { TerminalPanel } from "./components/TerminalPanel";
import { BrowserPanel } from "./components/BrowserPanel";
import { GitBranch, FolderTree, Terminal as TerminalIcon, Globe, Settings, AlertTriangle, X } from "lucide-react";
import { useHub } from "./v2/useHub";
import { eventsToMessages } from "./v2/convert";
import { fold, type EngineInfo } from "./v2/types";
import { LoginPanel } from "./v2/LoginPanel";
import type { HubConv, ConvAction } from "./components/HubConversations";

const WORKSPACE = "/home/chungnh/AI Workspace";

export function App() {
  const [projectGroups, setProjectGroups] = useState<ProjectGroupItem[]>([]);
  const [activeConversationId, setActiveConversationId] = useState<string | null>(null);
  const [activeConversationTitle, setActiveConversationTitle] = useState<string>("New Session");
  const [agyMessages, setAgyMessages] = useState<MessageItem[]>([]); // read-only Antigravity history
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

  const queueRef = useRef<QueuedItem[]>(queue);
  queueRef.current = queue;

  const loadAntigravityProjects = () =>
    fetch("/api/antigravity/projects")
      .then((res) => res.json())
      .then((groups: ProjectGroupItem[]) => setProjectGroups(Array.isArray(groups) ? groups : []))
      .catch((err) => console.error("Error loading antigravity projects:", err));

  const [hubConvs, setHubConvs] = useState<HubConv[]>([]);
  const [engines, setEngines] = useState<EngineInfo[]>([]);
  const [loginFor, setLoginFor] = useState<string | null>(null);
  const [permissionMode, setPermissionMode] = useState<string>(() => localStorage.getItem("hub_permission_mode") || "ask");
  const restoredRef = useRef(false);

  const refreshHubConvs = () =>
    fetch("/api/v2/convs").then((r) => r.json()).then((l) => setHubConvs(Array.isArray(l) ? l : [])).catch(() => {});
  const refreshEngines = (force = false) =>
    fetch(`/api/v2/engines${force ? "?refresh=1" : ""}`).then((r) => r.json()).then((l) => Array.isArray(l) && setEngines(l)).catch(() => {});

  useEffect(() => {
    loadAntigravityProjects();
    refreshHubConvs();
    refreshEngines();
  }, []);

  // Reopen the last hub conversation once the list is known.
  useEffect(() => {
    if (restoredRef.current || hubConvs.length === 0) return;
    restoredRef.current = true;
    const linked = /^#\/c\/([\w-]+)/.exec(window.location.hash)?.[1];
    const last = linked ?? localStorage.getItem("hub_last_conv");
    if (last && hubConvs.some((c) => c.id === last)) setActiveConversationId(last);
  }, [hubConvs]);

  const agyConvIds = useMemo(() => new Set(projectGroups.flatMap((g) => g.conversations.map((c) => c.id))), [projectGroups]);
  const isHubConv = activeConversationId === null || !agyConvIds.has(activeConversationId);

  const hub = useHub(isHubConv ? activeConversationId : null, (id) => {
    setActiveConversationId(id);
    localStorage.setItem("hub_last_conv", id);
    refreshHubConvs();
  });
  const hubMessages = useMemo(() => eventsToMessages(hub.events), [hub.events]);
  const messages = isHubConv ? hubMessages : agyMessages;

  const liveStates = Object.values(hub.snapshot?.live ?? {});
  const isStreaming = isHubConv && liveStates.some((s) => s === "starting" || s === "running" || s === "awaiting_approval");
  const pendingApprovals = useMemo(
    () => fold(hub.events).filter((i): i is Extract<ReturnType<typeof fold>[number], { kind: "approval" }> => i.kind === "approval" && !i.resolved),
    [hub.events]
  );
  const pending = isHubConv ? pendingApprovals[0] : undefined;
  const pendingText = pending ? String(pending.args?.command ?? pending.args?.file_path ?? pending.args?.path ?? pending.tool) : undefined;

  // Live model lists (cached on the hub for 24h) replace the built-in list for engines that can enumerate models.
  const liveModels = useMemo(() => {
    const out: Record<string, ModelDefinition[]> = {};
    for (const e of engines) {
      if (!e.capabilities.model_listing || e.models.length < 2) continue;
      out[e.id] = e.models.map((m) => ({
        id: m.id,
        name: m.name,
        tier: m.tier === "thinking" ? "Thinking" : m.tier === "smart" ? "Smart" : m.tier === "fast" ? "Fast" : "Medium",
        agent: e.id,
      }));
    }
    return out;
  }, [engines]);
  const refreshModels = async (): Promise<void> => {
    try {
      const l = await fetch("/api/v2/models/refresh", { method: "POST" }).then((r) => r.json());
      if (Array.isArray(l)) setEngines(l);
    } catch {
      /* keep the current list */
    }
  };

  const activeAgent = currentConfig.model.agent;
  const activeEngine = engines.find((e) => e.id === activeAgent);
  const allowedModes = activeEngine?.capabilities.permission_modes;
  const authBlocked = !!activeEngine?.auth && activeEngine.auth.known && (!activeEngine.auth.installed || !activeEngine.auth.logged_in);

  const applyMode = (m: string) => {
    setPermissionMode(m);
    localStorage.setItem("hub_permission_mode", m);
    if (activeConversationId && isHubConv) hub.send({ type: "set_mode", conv: activeConversationId, mode: m });
  };
  // Keep the permission mode valid for the selected engine (e.g. Antigravity cannot ask mid-task).
  useEffect(() => {
    if (allowedModes && allowedModes.length && !allowedModes.includes(permissionMode)) {
      applyMode(allowedModes.includes("accept-edits") ? "accept-edits" : allowedModes[0]);
    }
  }, [activeAgent, engines]);

  useEffect(() => {
    if (isHubConv && activeConversationId) {
      const c = hubConvs.find((x) => x.id === activeConversationId);
      if (c) setActiveConversationTitle(c.name || "Conversation");
    }
  }, [hubConvs, activeConversationId, isHubConv]);

  const selectConversation = (id: string) => {
    setActiveConversationId(id);
    if (!agyConvIds.has(id)) {
      localStorage.setItem("hub_last_conv", id);
      const c = hubConvs.find((x) => x.id === id);
      setActiveConversationTitle(c?.name || "Conversation");
      return;
    }
    for (const g of projectGroups) {
      const c = g.conversations.find((item) => item.id === id);
      if (c) {
        setActiveConversationTitle(c.title);
        break;
      }
    }
    fetch(`/api/antigravity/conversations/${id}/messages`)
      .then((res) => res.json())
      .then((data) => setAgyMessages(Array.isArray(data) ? data : []))
      .catch((err) => console.error("Error loading conversation messages:", err));
  };

  // Antigravity history is read-only; keep it fresh while it is open.
  useEffect(() => {
    if (!activeConversationId || isHubConv) return;
    let alive = true;
    const timer = setInterval(() => {
      fetch(`/api/antigravity/conversations/${activeConversationId}/messages`)
        .then((res) => res.json())
        .then((data) => alive && Array.isArray(data) && setAgyMessages(data))
        .catch(() => {});
    }, 4000);
    return () => {
      alive = false;
      clearInterval(timer);
    };
  }, [activeConversationId, isHubConv]);

  const handleNewConversation = () => {
    setActiveConversationId(null);
    setActiveConversationTitle("New Session");
    setAgyMessages([]);
    setQueue([]);
    localStorage.removeItem("hub_last_conv");
  };

  // Send a prompt through the v2 hub (an Antigravity history chat continues as a new hub chat).
  const dispatchPrompt = (text: string, config: SelectedModelConfig, media?: AttachedMedia[]) => {
    const userMedia = media?.map((m) => ({ mime_type: m.mime_type, uri: m.uri }));
    let conv = activeConversationId ?? "";
    if (!isHubConv) {
      conv = "";
      setActiveConversationId(null);
      setActiveConversationTitle("New Session");
    }
    hub.send({
      type: "send",
      conv,
      engine: config.model.agent,
      model: config.model.id,
      effort: config.effort.toLowerCase(),
      mode: permissionMode,
      text,
      media: userMedia,
      workspace: WORKSPACE,
    });
  };

  const checkAndProcessQueue = () => {
    if (queueRef.current.length > 0) {
      const nextItem = queueRef.current[0];
      setQueue((prev) => prev.slice(1));
      setTimeout(() => {
        dispatchPrompt(
          nextItem.text,
          currentConfig,
          nextItem.media?.map((m) => ({ uri: m.uri, mime_type: m.mime_type, url: `/api/media?path=${encodeURIComponent(m.uri)}` }))
        );
      }, 300);
    }
  };

  // When a turn finishes, send the next queued message.
  const wasStreaming = useRef(false);
  useEffect(() => {
    if (wasStreaming.current && !isStreaming) checkAndProcessQueue();
    wasStreaming.current = isStreaming;
  }, [isStreaming]);

  const handleSendMessage = (text: string, media?: AttachedMedia[]) => {
    const trimmed = text.trim();
    if (!trimmed && (!media || media.length === 0)) return;
    if (authBlocked) return; // the sign-in banner explains why
    if (isStreaming) {
      setQueue((prev) => [
        ...prev,
        {
          id: Math.random().toString(36).substring(7),
          text: trimmed,
          modelId: currentConfig.model.id,
          agent: currentConfig.model.agent,
          media: media?.map((m) => ({ uri: m.uri, mime_type: m.mime_type })),
        },
      ]);
    } else {
      dispatchPrompt(trimmed, currentConfig, media);
    }
  };

  // "!cmd": run a shell command on the server in this conversation's workspace (no model).
  // Privileged commands wait for an in-app approval (same card as agent permissions).
  const [shellPending, setShellPending] = useState<string | null>(null);
  const runShell = (command: string, confirmed: boolean) => {
    hub.send({
      type: "shell",
      conv: isHubConv ? activeConversationId ?? "" : "",
      engine: currentConfig.model.agent,
      model: currentConfig.model.id,
      effort: currentConfig.effort.toLowerCase(),
      mode: permissionMode,
      text: command,
      confirmed,
      workspace: WORKSPACE,
    });
  };
  const handleShellCommand = (command: string) => {
    if (!command) return;
    const privileged = /(^|[\s;&|(`])(sudo|su|doas|pkexec)([\s;&|)]|$)/.test(command);
    if (privileged) setShellPending(command);
    else runShell(command, false);
  };

  const handleCancelTask = () => {
    if (activeConversationId && isHubConv) hub.send({ type: "cancel", conv: activeConversationId });
  };

  const decide = (allow: boolean, scope: "once" | "session" = "once") => {
    if (!pending || !activeConversationId) return;
    hub.send({ type: "decide", conv: activeConversationId, approval_id: pending.id, allow, scope });
  };
  const handleApprove = () => decide(true);
  const handleReject = () => decide(false);
  const handleApproveSession = () => decide(true, "session");

  const handleRemoveQueue = (id: string) => setQueue((prev) => prev.filter((item) => item.id !== id));

  const handleSendNowQueue = (id: string) => {
    const item = queue.find((q) => q.id === id);
    if (!item) return;
    setQueue((prev) => prev.filter((q) => q.id !== id));
    handleCancelTask();
    setTimeout(() => {
      dispatchPrompt(
        item.text,
        currentConfig,
        item.media?.map((m) => ({ uri: m.uri, mime_type: m.mime_type, url: `/api/media?path=${encodeURIComponent(m.uri)}` }))
      );
    }, 400);
  };

  const handleEditQueue = (id: string) => {
    const item = queue.find((q) => q.id === id);
    if (!item) return;
    setQueue((prev) => prev.filter((q) => q.id !== id));
    setEditingText(item.text);
  };

  // keep the address bar pointing at the open conversation (so "Copy link" / reload work)
  useEffect(() => {
    if (isHubConv && activeConversationId) window.history.replaceState(null, "", `#/c/${activeConversationId}`);
    else if (!activeConversationId && window.location.hash.startsWith("#/c/")) window.history.replaceState(null, "", window.location.pathname);
  }, [activeConversationId, isHubConv]);

  const patchConv = (id: string, body: object) =>
    fetch(`/api/v2/convs/${id}`, { method: "PATCH", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) }).then(() => refreshHubConvs());

  const handleConvAction = async (a: ConvAction) => {
    switch (a.type) {
      case "pin": case "unpin": await patchConv(a.id, { pinned: a.type === "pin" }); break;
      case "unread": await patchConv(a.id, { unread: true }); break;
      case "rename": await patchConv(a.id, { name: a.value }); break;
      case "group": await patchConv(a.id, { group: a.value }); break;
      case "archive": case "unarchive":
        await patchConv(a.id, { archived: a.type === "archive" });
        if (a.type === "archive" && a.id === activeConversationId) handleNewConversation();
        break;
      case "copylink":
        try { await navigator.clipboard.writeText(`${window.location.origin}${window.location.pathname}#/c/${a.id}`); } catch { /* clipboard unavailable */ }
        break;
      case "fork": {
        const r = await fetch(`/api/v2/convs/${a.id}/fork`, { method: "POST" }).then((x) => x.json()).catch(() => null);
        await refreshHubConvs();
        if (r?.id) selectConversation(r.id);
        break;
      }
      case "delete":
        await fetch(`/api/v2/convs/${a.id}`, { method: "DELETE" });
        if (a.id === activeConversationId) handleNewConversation();
        await refreshHubConvs();
        break;
    }
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
    const live = liveModels[agentId];
    if (live && live.length > 0) {
      handleUpdateConfig({ model: live[0], effort: currentConfig.effort || "Medium" });
      return;
    }
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
      {loginFor && (
        <LoginPanel engine={loginFor} onClose={() => setLoginFor(null)} onDone={() => refreshEngines(true)} />
      )}
      {/* Sidebar Agent Hub */}
      <Sidebar
        projectGroups={projectGroups}
        hubConvs={hubConvs}
        onConvAction={handleConvAction}
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
          isRunning={!!pending}
          commandText={pendingText}
          requiresApproval={!!pending}
          onApprove={handleApprove}
          onReject={handleReject}
          onApproveSession={handleApproveSession}
          toolName={pending?.tool}
        />

        {/* Privileged "!cmd" waiting for explicit approval */}
        {shellPending && (
          <TaskBanner
            isRunning
            requiresApproval
            commandText={shellPending}
            onApprove={() => { runShell(shellPending, true); setShellPending(null); }}
            onReject={() => setShellPending(null)}
          />
        )}

        {/* Banner Quản lý Hàng đợi tin nhắn (Queued Messages) */}
        <QueuedMessages
          queue={queue}
          onRemove={handleRemoveQueue}
          onSendNow={handleSendNowQueue}
          onEdit={handleEditQueue}
        />

        {/* Engine not signed in: explain and offer login */}
        {authBlocked && (
          <div className="w-full max-w-4xl mx-auto px-4 mb-2">
            <div className="flex items-center justify-between gap-3 rounded-xl border border-amber-600/40 bg-[#1c1816] px-3.5 py-2 text-xs text-amber-200">
              <span className="flex items-center gap-2 min-w-0">
                <AlertTriangle className="w-4 h-4 shrink-0 text-amber-400" />
                <span className="truncate">
                  <b>{activeAgent}</b> is not signed in on the server{activeEngine?.auth?.login_hint ? <> — <code className="bg-black/40 px-1 rounded">{activeEngine.auth.login_hint}</code></> : null}
                </span>
              </span>
              <span className="flex gap-2 shrink-0">
                {activeEngine?.can_login && (
                  <button onClick={() => setLoginFor(activeAgent)} className="px-2.5 py-1 rounded-lg bg-sky-600 hover:bg-sky-500 text-white font-medium cursor-pointer">Sign in</button>
                )}
                <button onClick={() => refreshEngines(true)} className="px-2.5 py-1 rounded-lg bg-amber-700/60 hover:bg-amber-600 text-white cursor-pointer">Re-check</button>
              </span>
            </div>
          </div>
        )}
        {hub.error && (
          <div className="w-full max-w-4xl mx-auto px-4 mb-2">
            <div className="flex items-center justify-between rounded-xl border border-rose-700/40 bg-[#1c1417] px-3.5 py-2 text-xs text-rose-300">
              <span className="break-all">{hub.error}</span>
              <button onClick={hub.clearError} className="ml-3 shrink-0 text-rose-400 hover:text-rose-200 cursor-pointer" aria-label="Dismiss"><X className="w-3.5 h-3.5" /></button>
            </div>
          </div>
        )}

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
          permissionMode={permissionMode}
          onChangePermissionMode={applyMode}
          onShellCommand={handleShellCommand}
          allowedModes={allowedModes}
          liveModels={liveModels}
          modelsFetchedAt={activeEngine?.models_fetched_at}
          onRefreshModels={refreshModels}
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
