import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Terminal as TerminalIcon,
  GitBranch,
  FolderOpen,
  Folder,
  PanelLeftClose,
  PanelLeftOpen,
  Menu,
  Settings,
  Zap,
  Copy,
  Check,
  ChevronDown,
  ChevronRight,
  Plus,
  X,
  Layers,
  Trash2,
  Pencil,
  FolderPlus,
  ArrowUp,
  BookOpen,
  FileEdit,
  Eye,
  Eraser,
  Radio,
  MoreHorizontal,
  Shield,
  ArrowUpCircle,
} from "lucide-react";
import { TerminalPanel } from "./components/TerminalPanel";
import { remoteLaunch, getBaseAgent } from "./remote";
import { RemoteControl } from "./components/RemoteControl";
import { UsageMeter } from "./components/UsageMeter";
import { SettingsModal, type UpdateStatus } from "./components/SettingsModal";

export function isProtectedWorkspacePath(p?: string): boolean {
  if (!p) return true;
  const clean = p.trim().replace(/[/\\]+$/, "");
  if (clean === "" || clean === "/" || clean === "." || clean === "~") return true;
  const parts = clean.split(/[/\\]/).filter(Boolean);
  if (parts.length === 1) return true;
  if (parts.length === 2 && (parts[0] === "Users" || parts[0] === "home")) return true;
  return false;
}

interface Workspace {
  id: string;
  path: string;
  name: string;
}

interface NativeTurn {
  role: string;
  content: string;
}

interface NativeSession {
  agent: string;
  id: string;
  title: string;
  workspace: string;
  updated_at: string;
  last_user: string;
  last_assistant: string;
  turn_count: number;
  turns?: NativeTurn[];
}

interface ToolBinding {
  agent: string;
  native_session_id: string;
  title?: string;
  turn_count?: number;
  last_synced_hash?: string;
  updated_at: string;
}

interface BridgeSession {
  id: string;
  workspace_id?: string;
  workspace?: string;
  title: string;
  status?: string;
  current_agent?: string;
  last_handoff_summary?: string;
  bindings?: Record<string, string>; // agent -> native_session_id
  tools?: ToolBinding[]; // level 3 native sessions per tool
  created_at: string;
  updated_at: string;
}

interface EngineStatus {
  id: string;
  name: string;
  base?: string;
  binary: string;
  installCmd?: string;
  installed: boolean;
  auth_status?: string;
  has_auth?: boolean;
}

/** a terminal tab: an agent CLI (resumed session, fresh session or handoff) or a plain shell */
interface TermTab {
  key: string;
  sessionId: string;
  label: string;
  workDir: string;
  createdAt: number;
  agent?: string;
  launch?: { agent: string; resume?: string; fresh?: boolean; remote?: boolean; name?: string };
  /** remote access was switched on for this running session (after it started) */
  remoteOn?: boolean;
  /** the session this tab was handed off from: it supplies the context for the next switch */
  from?: { agent: string; id: string };
}

const BASE_AGENT_META: Record<string, { label: string; badge: string; dot: string; chip: string }> = {
  agy: { label: "Antigravity", badge: "bg-blue-500/15 text-blue-300 border-blue-500/30", dot: "bg-blue-400", chip: "hover:border-blue-500/50 hover:text-blue-300" },
  claude: { label: "Claude Code", badge: "bg-orange-500/15 text-orange-300 border-orange-500/30", dot: "bg-orange-400", chip: "hover:border-orange-500/50 hover:text-orange-300" },
  codex: { label: "Codex", badge: "bg-emerald-500/15 text-emerald-300 border-emerald-500/30", dot: "bg-emerald-400", chip: "hover:border-emerald-500/50 hover:text-emerald-300" },
};

const AGENT_META: Record<string, { label: string; badge: string; dot: string; chip: string }> = new Proxy(BASE_AGENT_META as any, {
  get(target, prop: string) {
    if (typeof prop !== "string") return undefined;
    if (prop in target) return target[prop];
    const base = getBaseAgent(prop);
    if (base in target) {
      const b = target[base];
      const suffix = prop.replace(/^[a-zA-Z0-9]+[-_]/, "");
      return {
        ...b,
        label: suffix ? `${b.label} (${suffix})` : b.label,
      };
    }
    return {
      label: prop,
      badge: "bg-purple-500/15 text-purple-300 border-purple-500/30",
      dot: "bg-purple-400",
      chip: "hover:border-purple-500/50 hover:text-purple-300",
    };
  },
});

const INITIAL_ENGINES: EngineStatus[] = [
  { id: "agy", name: "Antigravity", binary: "antigravity", installed: false },
  { id: "claude", name: "Claude Code", binary: "claude", installCmd: "npm install -g @anthropic-ai/claude-code", installed: false },
  { id: "codex", name: "Codex", binary: "codex", installCmd: "npm install -g @openai/codex", installed: false },
];

const AGENT_ORDER: Record<string, number> = {
  agy: 1,
  claude: 2,
  codex: 3,
};

function relTime(s?: string): string {
  if (!s) return "";
  const t = Date.parse(s);
  if (Number.isNaN(t) || t < 86400000) return "";
  const m = Math.max(0, Math.round((Date.now() - t) / 60000));
  if (m < 1) return "just now";
  if (m < 60) return `${m}m ago`;
  if (m < 1440) return `${Math.floor(m / 60)}h ago`;
  return `${Math.floor(m / 1440)}d ago`;
}

/** useState that survives reloads (localStorage may be blocked, so every access is guarded) */
function usePersistedState<T>(key: string, initial: T): [T, React.Dispatch<React.SetStateAction<T>>] {
  const [v, setV] = useState<T>(() => {
    try {
      const raw = localStorage.getItem(key);
      return raw == null ? initial : (JSON.parse(raw) as T);
    } catch {
      return initial;
    }
  });
  useEffect(() => {
    try {
      localStorage.setItem(key, JSON.stringify(v));
    } catch {
      /* storage unavailable: the state just won't persist */
    }
  }, [key, v]);
  return [v, setV];
}

/** true while the viewport matches the media query (kept in sync on resize/rotate) */
function useMedia(query: string): boolean {
  const [match, setMatch] = useState(() => window.matchMedia(query).matches);
  useEffect(() => {
    const mq = window.matchMedia(query);
    const on = () => setMatch(mq.matches);
    on();
    mq.addEventListener("change", on);
    return () => mq.removeEventListener("change", on);
  }, [query]);
  return match;
}

export default function App() {
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
  const [workspace, setWorkspace] = useState<string>(() => localStorage.getItem("bridge_workspace") || "/home/chungnh/AI Workspace");
  const [sessions, setSessions] = useState<BridgeSession[]>([]);
  const [detail, setDetail] = useState<NativeSession | null>(null);
  const [modifiedFiles, setModifiedFiles] = useState<string[]>([]);
  const [engines, setEngines] = useState<EngineStatus[]>(INITIAL_ENGINES);
  const [loading, setLoading] = useState(false);
  const [, setBusy] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const [tabs, setTabs] = useState<TermTab[]>([]);
  const [activeKey, setActiveKey] = useState<string | null>(null);
  const [selectedSessionId, setSelectedSessionId] = useState<string | null>(null);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [settingsTab, setSettingsTab] = useState<"general" | "agents" | "appearance" | "updates">("general");
  // width of the terminal column: below 768px the handoff and /clear buttons fold into the More menu.
  // Measured here instead of a CSS container query: containment would make every fixed popup inside the
  // column position itself relative to the column rather than the window.
  const termColRef = useRef<HTMLDivElement>(null);
  const [colWidth, setColWidth] = useState(1024);
  useEffect(() => {
    const el = termColRef.current;
    if (!el) return;
    const ro = new ResizeObserver(([e]) => setColWidth(e.contentRect.width));
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  const wideToolbar = colWidth >= 768;
  const [moreOpen, setMoreOpen] = useState(false);
  const [morePos, setMorePos] = useState({ x: 0, y: 0 });
  const [switchOpen, setSwitchOpen] = useState(false);
  const [switchPos, setSwitchPos] = useState({ x: 0, y: 0 });
  const [contextOpen, setContextOpen] = useState(false);
  const [treeCollapsed, setTreeCollapsed] = usePersistedState("bridge_tree_collapsed", false);
  const [expandedTasks, setExpandedTasks] = usePersistedState<Record<string, boolean>>("bridge_expanded_tasks", {});
  const [syncToast, setSyncToast] = useState<string | null>(null);

  // Rename states
  const [editingTaskId, setEditingTaskId] = useState<string | null>(null);
  const [editTaskTitle, setEditTaskTitle] = useState("");
  const [editingWsId, setEditingWsId] = useState<string | null>(null);
  const [editWsName, setEditWsName] = useState("");

  // New Project Modal & Directory Browser states
  const [newProjectOpen, setNewProjectOpen] = useState(false);
  const [newProjectPath, setNewProjectPath] = useState("");
  const [newProjectName, setNewProjectName] = useState("");
  const [newProjectAutoCreate, setNewProjectAutoCreate] = useState(true);
  const [fsCurrent, setFsCurrent] = useState("");
  const [fsParent, setFsParent] = useState("");
  const [fsDirs, setFsDirs] = useState<string[]>([]);

  // Handoff modal states
  const [handoffModalOpen, setHandoffModalOpen] = useState(false);
  const [handoffContent, setHandoffContent] = useState("");
  const [handoffPath, setHandoffPath] = useState("");
  const [handoffUpdatedAt, setHandoffUpdatedAt] = useState("");
  const [handoffCopied, setHandoffCopied] = useState(false);

  // App update states
  const [updateStatus, setUpdateStatus] = useState<UpdateStatus | null>(null);
  // on a phone the sidebar starts collapsed so the terminal gets the whole screen
  const isMobile = useMedia("(max-width: 767px)");
  const [sidebarPref, setSidebarPref] = usePersistedState("bridge_sidebar_collapsed", false);
  const [sidebarMobile, setSidebarMobile] = useState(true);
  // desktop remembers the user's choice; on a phone the drawer always starts closed and is not remembered
  const sidebarCollapsed = isMobile ? sidebarMobile : sidebarPref;
  const setSidebarCollapsed = isMobile ? setSidebarMobile : setSidebarPref;
  // on a phone the sidebar is a drawer over the terminal: close it once something was picked
  const closeDrawer = () => {
    if (isMobile) setSidebarCollapsed(true);
  };

  const activeTab = tabs.find((t) => t.key === activeKey) || null;

  // Active session ID: derived from active tab, selected session, or first available session
  const activeSessionId = useMemo(() => {
    if (activeTab?.sessionId) return activeTab.sessionId;
    if (selectedSessionId) return selectedSessionId;
    if (tabs.length > 0) return tabs[0].sessionId;
    if (sessions.length > 0) return sessions[0].id;
    return null;
  }, [activeTab, selectedSessionId, tabs, sessions]);

  // Tabs grouped for the current active session
  const currentSessionTabs = useMemo(() => {
    if (!activeSessionId) return [];
    return tabs.filter((t) => t.sessionId === activeSessionId);
  }, [tabs, activeSessionId]);

  // Map of sessionId -> Set of agents currently open in tabs
  const sessionOpenAgents = useMemo(() => {
    const map = new Map<string, Set<string>>();
    for (const t of tabs) {
      if (!map.has(t.sessionId)) {
        map.set(t.sessionId, new Set());
      }
      map.get(t.sessionId)!.add(t.agent || "shell");
    }
    return map;
  }, [tabs]);

  // Active session metadata (title, dir, etc.)
  /** open the web version of the active agent: claude.ai/code, chatgpt.com/codex; Antigravity has an IDE instead */
  const openAgentWeb = async (agent: string, dir: string) => {
    const r = await fetch("/api/bridge/open", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ agent, workspace_path: dir }),
    });
    const d = await r.json().catch(() => ({}));
    if (d.url) window.open(d.url, "_blank", "noopener");
    else if (!d.success) alert("Không mở được bản web của agent này");
  };

  const activeSessionInfo = useMemo(() => {
    if (!activeSessionId) return null;
    const found = sessions.find((s) => s.id === activeSessionId);
    if (found) return found;
    return null;
  }, [activeSessionId, sessions]);

  // Transcript / session context for context drawer
  const ctx = useMemo(() => {
    if (!activeSessionInfo) return null;
    const agent = activeTab?.agent || activeSessionInfo.current_agent || "agy";
    const nativeId = activeSessionInfo.bindings?.[agent] || "";
    return {
      agent,
      id: nativeId,
      title: activeSessionInfo.title,
      workspace: workspace,
    };
  }, [activeSessionInfo, activeTab, workspace]);

  // re-attach to the terminals the server is still running (survives reloads and server restarts)
  useEffect(() => {
    fetch("/api/terminal/sessions")
      .then((r) => r.json())
      .then((list: { id: string; dir: string; agent?: string; resume?: string }[]) => {
        if (!list?.length) return;
        setTabs((prev) => [
          ...prev,
          ...list
            .filter((x) => !prev.some((t) => t.key === x.id))
            .map((x) => {
              let sessId = x.resume || "";
              if (!sessId) {
                const m = x.id.match(/^term-(.+)-(?:agy|claude|codex|sh)$/);
                sessId = m ? m[1] : x.id;
              }
              return {
                key: x.id,
                sessionId: sessId,
                label: x.agent ? AGENT_META[x.agent]?.label || x.agent : "Shell",
                workDir: x.dir,
                createdAt: 0,
                agent: x.agent || undefined,
                launch: x.agent ? { agent: x.agent, resume: x.resume || undefined } : undefined,
              };
            }),
        ]);
        let last: string | null = null;
        try {
          last = localStorage.getItem("bridge_active_tab");
        } catch {
          /* storage unavailable */
        }
        setActiveKey((cur) => cur ?? (list.some((x) => x.id === last) ? last : list[0].id));
      })
      .catch(() => {});
  }, []);

  useEffect(() => {
    try {
      if (activeKey) localStorage.setItem("bridge_active_tab", activeKey);
    } catch {
      /* storage unavailable */
    }
  }, [activeKey]);

  const loadEngines = useCallback(async () => {
    let sourceEngines = INITIAL_ENGINES;
    try {
      const res = await fetch("/api/bridge/engines");
      if (res.ok) {
        const discovered = await res.json();
        if (Array.isArray(discovered) && discovered.length > 0) {
          sourceEngines = discovered;
        }
      }
    } catch {
      /* fallback to INITIAL_ENGINES */
    }

    const next = await Promise.all(
      sourceEngines.map(async (eng) => {
        try {
          const r = await fetch("/api/agents/check", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ binary: eng.binary || eng.id }),
          });
          const d = await r.json();
          return {
            ...eng,
            installed: eng.installed ?? !!d.found,
            has_auth: d.has_auth,
            auth_status: d.auth_status,
          };
        } catch {
          return eng;
        }
      })
    );
    setEngines(next);
  }, []);

  const loadWorkspace = useCallback(async (ws: string, quiet = false) => {
    if (!quiet) setLoading(true);
    try {
      const [sRes, gRes] = await Promise.all([
        fetch(`/api/bridge/sessions?workspace=${encodeURIComponent(ws)}`),
        fetch(`/api/bridge/state?workspace=${encodeURIComponent(ws)}`),
      ]);
      if (sRes.ok) {
        const data: BridgeSession[] = await sRes.json();
        setSessions(data || []);
      }
      if (gRes.ok) setModifiedFiles((await gRes.json()).modified_files || []);
    } finally {
      if (!quiet) setLoading(false);
    }
  }, []);

  const loadUpdateStatus = useCallback(async () => {
    try {
      const res = await fetch("/api/admin/update/status");
      if (res.ok) {
        const data: UpdateStatus = await res.json();
        setUpdateStatus(data);
      }
    } catch {
      /* ignore */
    }
  }, []);

  useEffect(() => {
    fetch("/api/bridge/workspaces")
      .then((r) => r.json())
      .then((d: Workspace[]) => setWorkspaces(d || []))
      .catch(() => {});
    loadEngines();
    loadUpdateStatus();
    const ut = window.setInterval(loadUpdateStatus, 180000); // 3 minutes
    return () => window.clearInterval(ut);
  }, [loadEngines, loadUpdateStatus]);

  useEffect(() => {
    localStorage.setItem("bridge_workspace", workspace);
    loadWorkspace(workspace);
    const t = window.setInterval(() => loadWorkspace(workspace, true), 15000);
    return () => window.clearInterval(t);
  }, [workspace, loadWorkspace]);

  // transcript of the active tab's session (context drawer + what a switch hands over)
  const ctxKey = ctx && ctx.id ? `${ctx.agent}:${ctx.id}` : "";
  useEffect(() => {
    setDetail(null);
    if (!ctx || !ctx.id) return;
    const q = new URLSearchParams({ agent: ctx.agent, id: ctx.id, workspace: workspace });
    fetch(`/api/bridge/native-session?${q}`)
      .then((r) => (r.ok ? r.json() : null))
      .then(setDetail)
      .catch(() => {});
  }, [ctxKey, workspace]);

  // ---------- tabs ----------
  // closing a tab ends its process on the server
  const killSession = (key: string) => fetch(`/api/terminal/sessions/${key}`, { method: "DELETE" }).catch(() => {});

  const openTab = (t: TermTab) => {
    setTabs((prev) => [...prev, t]);
    setActiveKey(t.key);
    setSelectedSessionId(t.sessionId);
    closeDrawer();
  };

  const closeTab = (key: string) => {
    killSession(key);
    setTabs((prev) => {
      const closing = prev.find((t) => t.key === key);
      const next = prev.filter((t) => t.key !== key);
      if (activeKey === key) {
        const sameSess = next.filter((t) => t.sessionId === closing?.sessionId);
        if (sameSess.length > 0) {
          setActiveKey(sameSess[sameSess.length - 1].key);
        } else {
          setActiveKey(next[next.length - 1]?.key ?? null);
        }
      }
      return next;
    });
  };

  /** Click a group/session in sidebar: resume its conversations */
  const openSession = (s: BridgeSession) => {
    setSelectedSessionId(s.id);
    const sessTabs = tabs.filter((t) => t.sessionId === s.id);
    if (sessTabs.length > 0) {
      const match = sessTabs.find((t) => t.agent === s.current_agent) || sessTabs[0];
      setActiveKey(match.key);
      closeDrawer();
      return;
    }
    const agent = s.current_agent || "agy";
    const boundNativeId = s.bindings?.[agent];
    const safeId = s.id.slice(0, 18).replace(/[^a-zA-Z0-9_-]/g, "");
    openTab({
      key: `term-${safeId}-${agent}`,
      sessionId: s.id,
      label: AGENT_META[agent]?.label || agent,
      workDir: workspace,
      createdAt: Date.now(),
      agent: agent,
      launch: { ...(boundNativeId ? { agent, resume: boundNativeId } : { agent, fresh: true }), ...remoteLaunch(agent, workspace) },
    });
  };

  /** Create a brand new universal task session/group */
  const createNewSession = async (initialAgent = "agy") => {
    try {
      const r = await fetch("/api/bridge/sessions", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          workspace: workspace,
          title: "New Task",
          initial_agent: initialAgent,
        }),
      });
      if (r.ok) {
        const newSess: BridgeSession = await r.json();
        setSessions((prev) => [newSess, ...prev.filter((x) => x.id !== newSess.id)]);
        openSession(newSess);
      }
    } catch (e) {
      console.error("Create session error:", e);
    }
  };

  /** Delete a universal task session/group and its bindings */
  const deleteSession = async (id: string, e?: React.MouseEvent) => {
    e?.stopPropagation();
    if (!confirm("Are you sure you want to delete this task?")) return;
    try {
      await fetch(`/api/bridge/sessions/${id}`, { method: "DELETE" });
      setSessions((prev) => prev.filter((s) => s.id !== id));
      const sessTabs = tabs.filter((t) => t.sessionId === id);
      for (const t of sessTabs) {
        killSession(t.key);
      }
      setTabs((prev) => prev.filter((t) => t.sessionId !== id));
      if (selectedSessionId === id) {
        setSelectedSessionId(null);
      }
    } catch (err) {
      console.error("Delete session error:", err);
    }
  };

  /** Rename task title */
  const startRenameTask = (task: BridgeSession, e?: React.MouseEvent) => {
    e?.stopPropagation();
    setEditingTaskId(task.id);
    setEditTaskTitle(task.title || "New Task");
  };

  const saveRenameTask = async (id: string, e?: React.FormEvent) => {
    e?.preventDefault();
    if (!editTaskTitle.trim()) return;
    const newTitle = editTaskTitle.trim();
    setSessions((prev) => prev.map((s) => (s.id === id ? { ...s, title: newTitle } : s)));
    setEditingTaskId(null);
    try {
      await fetch("/api/bridge/sessions/title", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ id, title: newTitle }),
      });
    } catch {
      /* ignore */
    }
  };

  /** Rename project (workspace) */
  const startRenameWorkspace = (ws: Workspace, e?: React.MouseEvent) => {
    e?.stopPropagation();
    setEditingWsId(ws.id);
    setEditWsName(ws.name);
  };

  const saveRenameWorkspace = async (id: string, e?: React.FormEvent) => {
    e?.preventDefault();
    if (!editWsName.trim()) return;
    const newName = editWsName.trim();
    setWorkspaces((prev) => prev.map((w) => (w.id === id ? { ...w, name: newName } : w)));
    setEditingWsId(null);
    try {
      await fetch("/api/bridge/workspaces/name", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ id, name: newName }),
      });
    } catch {
      /* ignore */
    }
  };

  /** Unregister workspace */
  const deleteWorkspace = async (id: string, name: string, e?: React.MouseEvent) => {
    e?.stopPropagation();
    const ws = workspaces.find((w) => w.id === id);
    if (ws && isProtectedWorkspacePath(ws.path)) {
      alert(`Thư mục "${ws.path}" thuộc hệ thống / người dùng được bảo vệ, không thể gỡ bỏ.`);
      return;
    }
    if (!confirm(`Are you sure you want to remove project "${name}" from the list? (Files on disk will not be deleted)`)) return;
    try {
      const res = await fetch(`/api/bridge/workspaces/${id}`, { method: "DELETE" });
      if (!res.ok) {
        const d = await res.json().catch(() => ({}));
        alert(d.error || "Failed to remove project from list");
        return;
      }
      const next = workspaces.filter((w) => w.id !== id);
      setWorkspaces(next);
      if (next.length > 0) {
        setWorkspace(next[0].path);
      }
    } catch {
      /* ignore */
    }
  };

  /** File system directory browser */
  const loadDirectories = async (path?: string) => {
    try {
      const q = path ? `?path=${encodeURIComponent(path)}` : "";
      const res = await fetch(`/api/bridge/fs/directories${q}`);
      if (res.ok) {
        const d = await res.json();
        setFsCurrent(d.current || "");
        setFsParent(d.parent || "");
        setFsDirs(d.directories || []);
      }
    } catch {
      /* ignore */
    }
  };

  const handleSelectFolder = (path: string) => {
    setNewProjectPath(path);
    const base = path.split("/").filter(Boolean).pop() || "project";
    if (!newProjectName) {
      setNewProjectName(base);
    }
  };

  const handleCreateProject = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newProjectPath.trim()) return;
    try {
      const r = await fetch("/api/bridge/workspaces", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          path: newProjectPath.trim(),
          name: newProjectName.trim() || undefined,
          create_dir: newProjectAutoCreate,
        }),
      });
      if (r.ok) {
        const createdWs: Workspace = await r.json();
        setWorkspaces((prev) => [createdWs, ...prev.filter((w) => w.id !== createdWs.id)]);
        setWorkspace(createdWs.path);
        setNewProjectOpen(false);
        setNewProjectPath("");
        setNewProjectName("");
      }
    } catch {
      /* ignore */
    }
  };

  /** Smart handoff sync: checks if targetAgent needs context, skips if identical or same session */
  const syncHandoff = async (ws: string, taskId: string, targetAgent: string, sourceAgent?: string) => {
    try {
      const res = await fetch("/api/bridge/sync-handoff", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          workspace: ws,
          task_id: taskId,
          target_agent: targetAgent,
          source_agent: sourceAgent || "",
        }),
      });
      if (res.ok) {
        const d = await res.json();
        if (d.synced) {
          setSyncToast(d.message);
          setTimeout(() => setSyncToast((prev) => (prev === d.message ? null : prev)), 4500);
          loadWorkspace(ws, true);
        }
      }
    } catch {
      /* best-effort */
    }
  };

  /** Send input directly into the active terminal session */
  const handleTriggerInput = async (text: string, label?: string) => {
    if (!activeKey) return;
    try {
      await fetch(`/api/terminal/sessions/${encodeURIComponent(activeKey)}/input`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ input: text }),
      });
      if (label) {
        setSyncToast(label);
        setTimeout(() => setSyncToast((prev) => (prev === label ? null : prev)), 3000);
      }
    } catch (err) {
      console.error("Trigger input error:", err);
    }
  };

  /** Trigger read or write handoff on the active agent tab */
  const handleTriggerHandoff = (action: "read" | "write") => {
    if (!activeTab) return;
    if (action === "read") {
      if (activeSessionId && activeTab.agent) {
        syncHandoff(workspace, activeSessionId, activeTab.agent);
      }
      handleTriggerInput("Đọc .agent/handoff.md và tiếp tục công việc dang dở.\n", `Triggered: Read Handoff in ${activeTab.label}`);
    } else {
      handleTriggerInput("Tóm tắt tiến độ hiện tại, các thay đổi đã làm và việc cần làm tiếp theo vào .agent/handoff.md để bàn giao.\n", `Triggered: Write Handoff in ${activeTab.label}`);
    }
  };

  /** Load .agent/handoff.md content to view in modal */
  const loadHandoffContent = async () => {
    try {
      const res = await fetch(`/api/bridge/handoff-content?workspace=${encodeURIComponent(workspace)}`);
      if (res.ok) {
        const d = await res.json();
        setHandoffContent(d.content || "");
        setHandoffPath(d.path || "");
        setHandoffUpdatedAt(d.updated_at || "");
        setHandoffModalOpen(true);
      }
    } catch (e) {
      console.error("Load handoff error:", e);
    }
  };

  /** Open or switch to an agent in a task: resume its own conversation if already existing! */
  const openAgentInSession = async (to: string, targetTaskId?: string) => {
    const taskId = targetTaskId || activeSessionId;
    if (!taskId) return;
    setSwitchOpen(false);
    setSelectedSessionId(taskId);

    // 1. If tab is already open in this task, focus it immediately!
    const existing = tabs.find((t) => t.sessionId === taskId && t.agent === to);
    if (existing) {
      setActiveKey(existing.key);
      closeDrawer();
      if (activeTab?.agent && activeTab.agent !== to) {
        syncHandoff(workspace, taskId, to, activeTab.agent);
      }
      return;
    }

    const currentTask = sessions.find((s) => s.id === taskId);
    const boundNativeId = currentTask?.bindings?.[to];
    const safeId = taskId.slice(0, 18).replace(/[^a-zA-Z0-9_-]/g, "");
    const workDir = activeTab?.workDir || workspace;

    // 2. If this agent already has a conversation in this task, resume it directly!
    if (boundNativeId) {
      openTab({
        key: `term-${safeId}-${to}`,
        sessionId: taskId,
        label: AGENT_META[to]?.label || to,
        workDir,
        createdAt: Date.now(),
        agent: to,
        launch: { agent: to, resume: boundNativeId, ...remoteLaunch(to, workDir) },
      });
      if (activeTab?.agent && activeTab.agent !== to) {
        syncHandoff(workDir, taskId, to, activeTab.agent);
      }
      return;
    }

    // 3. Otherwise: tool chưa từng có mặt trong task này -> handoff từ agent active trước đó và tạo fresh
    const fromAgent = activeTab?.agent;
    const fromNativeId = fromAgent ? currentTask?.bindings?.[fromAgent] : undefined;

    if (fromAgent && fromNativeId) {
      setBusy(to);
      try {
        await fetch("/api/bridge/handoff", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            workspace_path: workDir,
            session_id: taskId,
            from_agent: fromAgent,
            from_native_id: fromNativeId,
            to_agent: to,
          }),
        });
      } catch {
        /* best-effort */
      } finally {
        setBusy(null);
      }
    }

    // Ensure agent is immediately recorded in the task's bindings
    if (!boundNativeId) {
      fetch("/api/bridge/sessions/bind", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          bridge_session_id: taskId,
          agent: to,
          native_session_id: "",
        }),
      }).then(() => loadWorkspace(workDir, true)).catch(() => {});
    }

    openTab({
      key: `term-${safeId}-${to}`,
      sessionId: taskId,
      label: AGENT_META[to]?.label || to,
      workDir,
      createdAt: Date.now(),
      agent: to,
      launch: { agent: to, fresh: true, ...remoteLaunch(to, workDir) },
      from: fromAgent && fromNativeId ? { agent: fromAgent, id: fromNativeId } : undefined,
    });
  };

  /** Open or switch to shell in current group */
  const openShellInSession = () => {
    if (!activeSessionId) return;
    const existing = tabs.find((t) => t.sessionId === activeSessionId && !t.agent);
    if (existing) {
      setActiveKey(existing.key);
      return;
    }
    const workDir = activeTab?.workDir || workspace;
    const safeId = activeSessionId.slice(0, 18).replace(/[^a-zA-Z0-9_-]/g, "");
    openTab({
      key: `term-${safeId}-sh`,
      sessionId: activeSessionId,
      label: "Shell",
      workDir,
      createdAt: Date.now(),
    });
  };

  // Auto-detect and bind newly created native conversation IDs into the active group
  useEffect(() => {
    if (!activeSessionId || !activeTab?.agent) return;
    const sess = sessions.find((s) => s.id === activeSessionId);
    if (!sess) return;
    const agent = activeTab.agent;
    if (!sess.bindings || !sess.bindings[agent]) {
      const checkTimer = window.setTimeout(async () => {
        try {
          const res = await fetch(`/api/bridge/native-sessions?workspace=${encodeURIComponent(workspace)}`);
          if (res.ok) {
            const list: NativeSession[] = await res.json();
            const found = list.find((n) => n.agent === agent);
            if (found && found.id) {
              await fetch("/api/bridge/sessions/bind", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({
                  bridge_session_id: activeSessionId,
                  agent: agent,
                  native_session_id: found.id,
                }),
              });
              if ((sess.title === "New Task" || sess.title === "Chủ đề mới" || sess.title === "Phiên làm việc mới") && found.title) {
                await fetch("/api/bridge/sessions/title", {
                  method: "PATCH",
                  headers: { "Content-Type": "application/json" },
                  body: JSON.stringify({ id: activeSessionId, title: found.title }),
                });
              }
              loadWorkspace(workspace, true);
            }
          }
        } catch {
          /* ignore */
        }
      }, 3500);
      return () => window.clearTimeout(checkTimer);
    }
  }, [activeSessionId, activeTab, sessions, workspace, loadWorkspace]);

  const copyText = (text: string) => {
    navigator.clipboard?.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };

  const installed = engines.filter((e) => e.installed);
  const availableSessionAgents = installed.filter((e) => !currentSessionTabs.some((t) => t.agent === e.id));
  const hasSessionShell = currentSessionTabs.some((t) => !t.agent);
  const allTurns = detail?.turns || [];
  const lastTurns = allTurns.slice(-6);

  return (
    <div className="flex h-dvh w-screen overflow-hidden bg-[#0b0e14] text-slate-100">
      {/* ---------- Sidebar: folders & sessions, settings pinned at the bottom ---------- */}
      {!sidebarCollapsed && isMobile && <div className="fixed inset-0 z-30 bg-black/60" onClick={() => setSidebarCollapsed(true)} />}
      {!sidebarCollapsed ? (
        <aside
          className={`flex flex-col border-r border-[#1d222b] bg-[#12151c] text-slate-300 select-none ${
            isMobile ? "fixed inset-y-0 left-0 z-40 w-[85vw] max-w-xs pt-[env(safe-area-inset-top)] shadow-2xl" : "h-full w-72 shrink-0"
          }`}
        >
          <div className="flex items-center justify-between border-b border-[#1c212a] px-3.5 pt-3 pb-2">
            <div className="flex items-center gap-2">
              <Zap className="h-3.5 w-3.5 fill-amber-400 text-amber-400" />
              <span className="text-xs font-semibold tracking-wide text-slate-200">Agent Bridge</span>
            </div>
            <button onClick={() => setSidebarCollapsed(true)} className="cursor-pointer p-1 text-slate-500 hover:text-slate-300" title="Thu gọn">
              <PanelLeftClose className="h-3.5 w-3.5" />
            </button>
          </div>

          <div className="flex items-center justify-between px-3.5 pt-3 pb-1 text-[10px] font-semibold uppercase tracking-wider text-slate-500">
            <span>Projects &amp; Tasks</span>
            <div className="flex items-center gap-1">
              <button
                onClick={() => {
                  loadDirectories();
                  setNewProjectOpen(true);
                }}
                className="flex cursor-pointer items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium text-emerald-400 hover:bg-emerald-950/40 hover:text-emerald-300 transition-colors"
                title="Add or import project"
              >
                <FolderPlus className="h-3 w-3" />
                <span>Project</span>
              </button>
              <button
                onClick={() => createNewSession()}
                className="flex cursor-pointer items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium text-indigo-400 hover:bg-indigo-950/40 hover:text-indigo-300 transition-colors"
                title="Create new task for active project"
              >
                <Plus className="h-3 w-3" />
                <span>Task</span>
              </button>
            </div>
          </div>
          <div className="min-h-0 flex-1 space-y-0.5 overflow-y-auto px-2 pb-2">
            {workspaces.map((ws) => {
              const active = ws.path === workspace;
              const open = active && !treeCollapsed;
              const isEditingWs = editingWsId === ws.id;

              return (
                <div key={ws.id}>
                  {/* Level 1: Project (Folder/Workspace) */}
                  {isEditingWs ? (
                    <form onSubmit={(e) => saveRenameWorkspace(ws.id, e)} className="flex items-center gap-1 py-1 px-1">
                      <input
                        autoFocus
                        value={editWsName}
                        onChange={(e) => setEditWsName(e.target.value)}
                        className="h-6 w-full rounded bg-[#1e2433] px-1.5 font-sans text-xs text-slate-100 outline-none border border-indigo-500"
                        onKeyDown={(e) => {
                          if (e.key === "Escape") setEditingWsId(null);
                        }}
                      />
                      <button type="submit" className="p-0.5 text-emerald-400 hover:text-emerald-300"><Check className="h-3 w-3" /></button>
                      <button type="button" onClick={() => setEditingWsId(null)} className="p-0.5 text-slate-500 hover:text-slate-300"><X className="h-3 w-3" /></button>
                    </form>
                  ) : (
                    <div className="group relative flex w-full items-center justify-between rounded-md px-1.5 py-1.5 text-left text-xs transition-colors hover:bg-white/[0.04]">
                      <div
                        onClick={() => {
                          if (active) setTreeCollapsed((v) => !v);
                          else {
                            setWorkspace(ws.path);
                            setTreeCollapsed(false);
                          }
                        }}
                        className="flex min-w-0 flex-1 cursor-pointer items-center gap-1.5"
                        title={`Folder: ${ws.path}`}
                      >
                        {open ? <ChevronDown className="h-3 w-3 shrink-0 text-slate-500" /> : <ChevronRight className="h-3 w-3 shrink-0 text-slate-500" />}
                        {open ? <FolderOpen className="h-3.5 w-3.5 shrink-0 text-amber-400" /> : <Folder className="h-3.5 w-3.5 shrink-0 text-slate-500" />}
                        <span className={`truncate ${active ? "font-semibold text-slate-100" : "text-slate-400"}`}>{ws.name}</span>
                      </div>
                      <div className="hidden shrink-0 items-center gap-1 group-hover:flex">
                        <button
                          onClick={(e) => startRenameWorkspace(ws, e)}
                          className="p-1 text-slate-500 hover:text-indigo-300 rounded hover:bg-white/[0.06] transition-colors"
                          title="Rename project"
                        >
                          <Pencil className="h-2.5 w-2.5" />
                        </button>
                        {isProtectedWorkspacePath(ws.path) ? (
                          <span
                            className="p-1 text-slate-500 cursor-not-allowed select-none"
                            title="Thư mục hệ thống / tài khoản được bảo vệ"
                          >
                            <Shield className="h-2.5 w-2.5 text-slate-500" />
                          </span>
                        ) : (
                          <button
                            onClick={(e) => deleteWorkspace(ws.id, ws.name, e)}
                            className="p-1 text-slate-500 hover:text-rose-400 rounded hover:bg-white/[0.06] transition-colors"
                            title="Remove project from list"
                          >
                            <Trash2 className="h-2.5 w-2.5" />
                          </button>
                        )}
                      </div>
                    </div>
                  )}

                  {open && (
                    <div className="ml-2 space-y-0.5 pl-1.5 pt-0.5">
                      {sessions.length === 0 && !loading && (
                        <div className="px-2 py-3 text-center">
                          <p className="text-[11px] text-slate-500">No tasks yet.</p>
                          <button
                            onClick={() => createNewSession()}
                            className="mt-2 inline-flex items-center gap-1 rounded bg-white/[0.06] px-2.5 py-1 text-[11px] text-indigo-300 hover:bg-indigo-600 hover:text-white transition-colors"
                          >
                            <Plus className="h-3 w-3" /> New Task
                          </button>
                        </div>
                      )}
                      {sessions.map((s) => {
                        const sActive = activeSessionId === s.id;
                        const openAgs = sessionOpenAgents.get(s.id);
                        const isExpanded = expandedTasks[s.id] ?? sActive;
                        const isEditingTask = editingTaskId === s.id;
                        const boundAgents = Object.keys(s.bindings || {});
                        const rawTools: ToolBinding[] = (s.tools && s.tools.length > 0)
                          ? s.tools
                          : boundAgents.map((ag) => ({
                              agent: ag,
                              native_session_id: s.bindings?.[ag] || "",
                              updated_at: s.updated_at,
                            }));

                        // Fixed, stable order for agent tools
                        const toolsList = [...rawTools].sort((a, b) => {
                          const ordA = AGENT_ORDER[a.agent] ?? 99;
                          const ordB = AGENT_ORDER[b.agent] ?? 99;
                          if (ordA !== ordB) return ordA - ordB;
                          return a.agent.localeCompare(b.agent);
                        });

                        const unassignedAgents = installed.filter((e) => !boundAgents.includes(e.id));

                        return (
                          <div key={s.id} className="space-y-0.5">
                            {/* Level 2: Task */}
                            {isEditingTask ? (
                              <form onSubmit={(e) => saveRenameTask(s.id, e)} className="flex items-center gap-1 py-1 px-1">
                                <input
                                  autoFocus
                                  value={editTaskTitle}
                                  onChange={(e) => setEditTaskTitle(e.target.value)}
                                  className="h-6 w-full rounded bg-[#1e2433] px-1.5 font-sans text-xs text-slate-100 outline-none border border-indigo-500"
                                  onKeyDown={(e) => {
                                    if (e.key === "Escape") setEditingTaskId(null);
                                  }}
                                />
                                <button type="submit" className="p-0.5 text-emerald-400 hover:text-emerald-300"><Check className="h-3 w-3" /></button>
                                <button type="button" onClick={() => setEditingTaskId(null)} className="p-0.5 text-slate-500 hover:text-slate-300"><X className="h-3 w-3" /></button>
                              </form>
                            ) : (
                              <div
                                onClick={() => openSession(s)}
                                className={`group relative flex cursor-pointer items-center justify-between rounded px-1.5 py-1 text-left transition-colors ${
                                  sActive
                                    ? "text-slate-100 font-medium hover:bg-white/[0.03]"
                                    : "text-slate-400 hover:bg-white/[0.03] hover:text-slate-200"
                                }`}
                              >
                                <div className="flex min-w-0 items-center gap-1.5">
                                  <button
                                    onClick={(e) => {
                                      e.stopPropagation();
                                      setExpandedTasks((prev) => ({ ...prev, [s.id]: !isExpanded }));
                                    }}
                                    className="cursor-pointer p-0.5 text-slate-500 hover:text-slate-300"
                                    title={isExpanded ? "Collapse tools" : "Expand tools"}
                                  >
                                    {isExpanded ? (
                                      <ChevronDown className="h-3 w-3 shrink-0" />
                                    ) : (
                                      <ChevronRight className="h-3 w-3 shrink-0" />
                                    )}
                                  </button>
                                  <Layers className={`h-3 w-3 shrink-0 ${sActive ? "text-indigo-400" : "text-slate-500"}`} />
                                  <span className="truncate text-xs">
                                    {s.title || "New Task"}
                                  </span>
                                </div>

                                <div className="flex shrink-0 items-center gap-1 pl-1">
                                  <button
                                    onClick={(e) => startRenameTask(s, e)}
                                    className="hidden rounded p-0.5 text-slate-500 hover:text-indigo-300 group-hover:block transition-colors"
                                    title="Rename task"
                                  >
                                    <Pencil className="h-2.5 w-2.5" />
                                  </button>
                                  <button
                                    onClick={(e) => deleteSession(s.id, e)}
                                    className="hidden rounded p-0.5 text-slate-500 hover:text-rose-400 group-hover:block transition-colors"
                                    title="Delete task"
                                  >
                                    <Trash2 className="h-3 w-3" />
                                  </button>
                                </div>
                              </div>
                            )}

                            {/* Level 3: Native Sessions of Tools under this Task */}
                            {isExpanded && (
                              <div className="ml-3 space-y-0.5 pl-3 py-0.5">
                                {toolsList.length === 0 && (
                                  <div className="px-2 py-1 text-[11px] text-slate-500">
                                    No tools assigned.
                                  </div>
                                )}
                                {toolsList.map((tool) => {
                                  const meta = AGENT_META[tool.agent];
                                  const isTabOpen = openAgs?.has(tool.agent);
                                  const isCurrentActiveTab = activeTab?.agent === tool.agent && sActive;
                                  return (
                                    <div
                                      key={tool.agent}
                                      onClick={() => openAgentInSession(tool.agent, s.id)}
                                      className={`flex cursor-pointer items-center justify-between rounded px-2 py-1 text-xs transition-colors ${
                                        isCurrentActiveTab
                                          ? "text-indigo-300 font-medium bg-white/[0.04]"
                                          : isTabOpen
                                          ? "text-slate-200 hover:bg-white/[0.03]"
                                          : "text-slate-400 hover:bg-white/[0.03] hover:text-slate-300"
                                      }`}
                                      title={`Tool: ${meta?.label || tool.agent}`}
                                    >
                                      <div className="flex min-w-0 items-center gap-1.5">
                                        <span className="truncate text-[11.5px]">{meta?.label || tool.agent}</span>
                                      </div>
                                    </div>
                                  );
                                })}

                                {/* Quick add tool into task (anti-duplicate) */}
                                {unassignedAgents.length > 0 && (
                                  <div className="flex items-center gap-1 pt-1 pl-1 text-[10px] text-slate-500">
                                    <span>+ Assign:</span>
                                    {unassignedAgents.map((e) => (
                                      <button
                                        key={e.id}
                                        onClick={() => openAgentInSession(e.id, s.id)}
                                        className="cursor-pointer rounded px-1.5 py-0.5 text-[10px] text-slate-400 hover:bg-white/[0.06] hover:text-slate-200 transition-colors"
                                        title={`Assign ${e.name} to this task`}
                                      >
                                        {e.name}
                                      </button>
                                    ))}
                                  </div>
                                )}
                              </div>
                            )}
                          </div>
                        );
                      })}
                    </div>
                  )}
                </div>
              );
            })}
          </div>

          {/* pinned at the bottom */}
          <div className="shrink-0 border-t border-[#1d222b] bg-[#101217] p-2.5">
            <button
              onClick={() => {
                setSettingsTab("general");
                setSettingsOpen(true);
              }}
              className="flex w-full cursor-pointer items-center justify-between rounded-lg px-2.5 py-1.5 text-xs text-slate-400 hover:bg-white/[0.04] hover:text-slate-200 transition-colors"
            >
              <div className="flex items-center gap-2">
                <Settings className="h-3.5 w-3.5 text-violet-400" />
                <span>Settings</span>
              </div>
              {updateStatus?.has_update && (
                <span className="flex h-2 w-2 relative">
                  <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-amber-400 opacity-75"></span>
                  <span className="relative inline-flex rounded-full h-2 w-2 bg-amber-500"></span>
                </span>
              )}
            </button>
          </div>
        </aside>
      ) : isMobile ? null : (
        <div className="flex w-11 shrink-0 flex-col items-center justify-between border-r border-[#1d222b] bg-[#12151c] py-3">
          <button onClick={() => setSidebarCollapsed(false)} className="cursor-pointer rounded-lg p-2 text-slate-400 hover:bg-[#1a1e28] hover:text-slate-200" title="Mở rộng">
            <PanelLeftOpen className="h-4 w-4" />
          </button>
          <button
            onClick={() => {
              setSettingsTab("general");
              setSettingsOpen(true);
            }}
            className="relative cursor-pointer rounded-lg p-2 text-slate-400 hover:bg-[#1a1e28] hover:text-slate-200"
            title="Settings"
          >
            <Settings className="h-4 w-4 text-violet-400" />
            {updateStatus?.has_update && (
              <span className="absolute top-1.5 right-1.5 h-2 w-2 rounded-full bg-amber-500" />
            )}
          </button>
        </div>
      )}

      {/* ---------- Main ---------- */}
      <div className="flex h-full min-w-0 flex-1 flex-col">
        <header className="flex h-12 shrink-0 items-center justify-between gap-2 border-b border-[#1d222b] bg-[#101319] px-3 pt-[env(safe-area-inset-top)] sm:gap-4 sm:px-5">
          <div className="flex min-w-0 items-center gap-2.5">
            {isMobile && (
              <button onClick={() => setSidebarCollapsed(false)} className="-ml-1 shrink-0 cursor-pointer rounded-md p-1.5 text-slate-300 hover:bg-[#1a1e28]" title="Menu">
                <Menu className="h-5 w-5" />
              </button>
            )}
            <Layers className="hidden h-4 w-4 shrink-0 text-indigo-400 sm:block" />
            <div className="min-w-0 leading-tight">
              <div className="flex items-center gap-2">
                <span className="truncate text-[13px] font-semibold text-slate-100">
                  {activeSessionInfo?.title || activeTab?.label || "Agent Bridge"}
                </span>
                {activeSessionId && (
                  <button
                    onClick={() => copyText(activeSessionId)}
                    className="flex shrink-0 cursor-pointer items-center gap-1 rounded bg-[#1c222e] px-1.5 py-0.5 font-mono text-[10px] text-slate-400 hover:bg-[#252d3d] hover:text-slate-200"
                    title={`Copy Session ID: ${activeSessionId}`}
                  >
                    {copied ? <Check className="h-2.5 w-2.5 text-emerald-400" /> : <Copy className="h-2.5 w-2.5" />}
                    <span>{activeSessionId.slice(0, 8)}</span>
                  </button>
                )}
              </div>
              <div className="hidden truncate font-mono text-[10.5px] text-slate-500 sm:block" title={activeTab?.workDir || workspace}>
                {activeTab?.workDir || workspace}
              </div>
            </div>
          </div>

          <div className="flex shrink-0 items-center gap-2">
            {updateStatus?.has_update && (
              <button
                onClick={() => {
                  setSettingsTab("updates");
                  setSettingsOpen(true);
                }}
                className="flex cursor-pointer items-center gap-1.5 rounded-full border border-amber-500/40 bg-amber-500/10 px-2.5 py-1 text-[11px] font-medium text-amber-300 hover:bg-amber-500/20 transition-all shadow-sm animate-pulse"
                title={`Có bản cập nhật mới (${updateStatus.commits_behind || 1} commit)`}
              >
                <ArrowUpCircle className="h-3.5 w-3.5 text-amber-400" />
                <span className="hidden sm:inline">Bản cập nhật mới</span>
              </button>
            )}
            {activeTab?.agent && (
              <>
                {!isMobile && <UsageMeter agent={activeTab.agent} nativeId={(ctx?.agent === activeTab.agent ? ctx.id : "") || activeTab.launch?.resume || ""} workspace={activeTab.workDir || workspace} />}
                <RemoteControl
                  key={activeTab.key}
                  agent={activeTab.agent}
                  workDir={activeTab.workDir}
                  on={!!(activeTab.launch?.remote || activeTab.remoteOn)}
                  sendInput={handleTriggerInput}
                  onEnabled={() => setTabs((prev) => prev.map((t) => (t.key === activeTab.key ? { ...t, remoteOn: true } : t)))}
                  onOpenIde={() => openAgentWeb(activeTab.agent!, activeTab.workDir)}
                />
              </>
            )}
          </div>
        </header>

        {/* ---------- Terminal workspace (default) ---------- */}
        <div className="min-h-0 flex-1 flex">
          <div ref={termColRef} className="flex min-w-0 flex-1 flex-col bg-[#0c0e14]">
            {/* Session Tabs Bar: strictly displays tabs of the active session */}
            <div className="flex h-9 shrink-0 items-center gap-1 overflow-x-auto border-b border-[#1d222b] bg-[#101319] px-2">
              {currentSessionTabs.map((t) => {
                const isActive = t.key === activeKey;
                return (
                  <div
                    key={t.key}
                    onClick={() => setActiveKey(t.key)}
                    className={`flex shrink-0 cursor-pointer items-center gap-1.5 rounded-md px-2.5 py-1 text-[12px] transition-colors ${
                      isActive
                        ? "bg-white/[0.08] font-medium text-slate-100 border border-white/10 shadow-sm"
                        : "text-slate-400 hover:bg-white/[0.04] hover:text-slate-200 border border-transparent"
                    }`}
                  >
                    {t.agent ? <span className={`h-1.5 w-1.5 rounded-full ${AGENT_META[t.agent]?.dot}`} /> : <TerminalIcon className="h-3.5 w-3.5 text-sky-400" />}
                    <span title={t.workDir}>{t.label}</span>
                    {(t.launch?.remote || t.remoteOn) && <span title="Remote: điều khiển được từ app web/mobile của agent"><Radio className="h-3 w-3 text-emerald-400" /></span>}
                    <button
                      onClick={(e) => {
                        e.stopPropagation();
                        closeTab(t.key);
                      }}
                      className="cursor-pointer rounded p-0.5 text-slate-500 hover:text-rose-400 transition-colors"
                      title="Close tab (keeps session in task)"
                    >
                      <X className="h-3 w-3" />
                    </button>
                  </div>
                );
              })}

              {/* Add agent or shell to current session */}
              {(availableSessionAgents.length > 0 || !hasSessionShell) && (
                <div className="relative shrink-0">
                  <button
                    onClick={(e) => {
                      const r = e.currentTarget.getBoundingClientRect();
                      setSwitchPos({ x: Math.max(8, Math.min(r.left, window.innerWidth - 232)), y: r.bottom + 4 });
                      setSwitchOpen((v) => !v);
                    }}
                    className="flex cursor-pointer items-center gap-1 rounded-md px-2 py-1 text-[11.5px] text-slate-400 hover:bg-white/[0.04] hover:text-slate-200"
                    title="Add agent or shell to this task"
                  >
                    <Plus className="h-3.5 w-3.5" />
                    <span className="text-[11px]">Add</span>
                  </button>
                  {switchOpen && (
                    <div style={{ left: switchPos.x, top: switchPos.y }} className="fixed z-50 w-56 max-w-[calc(100vw-1.5rem)] rounded-lg border border-[#2c3447] bg-[#161b26] p-1 shadow-xl">
                      <div className="px-2.5 py-1 text-[10.5px] font-semibold uppercase tracking-wider text-slate-500">Open in this task</div>
                      {availableSessionAgents.length === 0 && !hasSessionShell && (
                        <div className="px-3 py-2 text-[11.5px] text-slate-400">All agents and shell are open in this task.</div>
                      )}
                      {availableSessionAgents.map((e) => (
                        <button
                          key={e.id}
                          onClick={() => openAgentInSession(e.id)}
                          className="flex w-full cursor-pointer items-center gap-2 rounded-md px-3 py-2 text-left text-[12px] text-slate-200 hover:bg-[#222a3a]"
                        >
                          <span className={`h-1.5 w-1.5 rounded-full ${AGENT_META[e.id]?.dot}`} />
                          <span>{e.name}</span>
                        </button>
                      ))}
                      {!hasSessionShell && (
                        <button
                          onClick={() => {
                            setSwitchOpen(false);
                            openShellInSession();
                          }}
                          className="flex w-full cursor-pointer items-center gap-2 rounded-md px-3 py-2 text-left text-[12px] text-slate-200 hover:bg-[#222a3a]"
                        >
                          <TerminalIcon className="h-3.5 w-3.5 text-sky-400" />
                          <span>Shell</span>
                        </button>
                      )}
                    </div>
                  )}
                </div>
              )}

              {/* Right side: handoff trigger options for active tab + Clear (the usage meter is in the header, except on phones) */}
              <div className="ml-auto flex shrink-0 items-center gap-1.5 pr-1">
                {isMobile && activeTab?.agent && <UsageMeter agent={activeTab.agent} nativeId={(ctx?.agent === activeTab.agent ? ctx.id : "") || activeTab.launch?.resume || ""} workspace={activeTab.workDir || workspace} />}
                {/* wide enough: the buttons themselves; narrower: the same actions in the More menu */}
                <div className={`${wideToolbar ? "flex" : "hidden"} items-center gap-1.5`}>
                  <div className="flex items-center gap-0.5 rounded-md bg-[#161a24] p-0.5 border border-white/5">
                    <button
                      onClick={() => handleTriggerHandoff("read")}
                      disabled={!activeTab}
                      className="flex cursor-pointer items-center gap-1 rounded px-2 py-0.5 text-[11px] font-medium text-slate-300 hover:bg-white/[0.08] hover:text-indigo-300 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
                      title="Send 'Read .agent/handoff.md and continue' to active tab"
                    >
                      <BookOpen className="h-3 w-3 text-indigo-400" />
                      <span className="hidden sm:inline">Read Handoff</span>
                    </button>
                    <button
                      onClick={() => handleTriggerHandoff("write")}
                      disabled={!activeTab}
                      className="flex cursor-pointer items-center gap-1 rounded px-2 py-0.5 text-[11px] font-medium text-slate-300 hover:bg-white/[0.08] hover:text-emerald-300 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
                      title="Ask active agent to summarize and write into .agent/handoff.md"
                    >
                      <FileEdit className="h-3 w-3 text-emerald-400" />
                      <span className="hidden sm:inline">Write Handoff</span>
                    </button>
                    <button
                      onClick={loadHandoffContent}
                      className="flex cursor-pointer items-center gap-1 rounded px-1.5 py-0.5 text-[11px] text-slate-400 hover:bg-white/[0.08] hover:text-slate-200 transition-colors"
                      title="View current .agent/handoff.md content"
                    >
                      <Eye className="h-3 w-3 text-slate-400" />
                    </button>
                  </div>

                  <button
                    onClick={() => handleTriggerInput("/clear\n", "Sent /clear to active tab")}
                    disabled={!activeTab}
                    className="flex cursor-pointer items-center gap-1 rounded px-2 py-1 text-[11px] text-slate-400 hover:bg-white/[0.04] hover:text-slate-200 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
                    title="Send /clear to active tab"
                  >
                    <Eraser className="h-3 w-3 text-slate-500" />
                    <span>/clear</span>
                  </button>
                </div>

                <div className={wideToolbar ? "hidden" : ""}>
                  <button
                    onClick={(e) => {
                      const r = e.currentTarget.getBoundingClientRect();
                      setMorePos({ x: Math.max(8, Math.min(r.right - 208, window.innerWidth - 216)), y: r.bottom + 4 });
                      setMoreOpen((v) => !v);
                    }}
                    className="flex cursor-pointer items-center rounded-md border border-white/5 bg-[#161a24] px-1.5 py-1 text-slate-300 hover:bg-white/[0.08] hover:text-slate-100"
                    title="Handoff, /clear"
                  >
                    <MoreHorizontal className="h-4 w-4" />
                  </button>
                  {moreOpen && (
                    <>
                      <div className="fixed inset-0 z-40" onClick={() => setMoreOpen(false)} />
                      <div style={{ left: morePos.x, top: morePos.y }} className="fixed z-50 w-52 rounded-lg border border-[#2c3447] bg-[#161b26] p-1 shadow-xl">
                        {(
                          [
                            [BookOpen, "text-indigo-400", "Read Handoff", () => handleTriggerHandoff("read"), !activeTab],
                            [FileEdit, "text-emerald-400", "Write Handoff", () => handleTriggerHandoff("write"), !activeTab],
                            [Eye, "text-slate-400", "Xem handoff.md", () => loadHandoffContent(), false],
                            [Eraser, "text-slate-500", "/clear", () => handleTriggerInput("/clear\n", "Sent /clear to active tab"), !activeTab],
                          ] as const
                        ).map(([Icon, color, label, run, disabled]) => (
                          <button
                            key={label}
                            disabled={disabled}
                            onClick={() => {
                              setMoreOpen(false);
                              run();
                            }}
                            className="flex w-full cursor-pointer items-center gap-2 rounded-md px-3 py-2 text-left text-[12px] text-slate-200 hover:bg-[#222a3a] disabled:cursor-not-allowed disabled:opacity-40"
                          >
                            <Icon className={`h-3.5 w-3.5 ${color}`} />
                            {label}
                          </button>
                        ))}
                      </div>
                    </>
                  )}
                </div>
              </div>
            </div>

            <div className="relative flex-1 overflow-hidden">
              {tabs.map((t) => (
                <div key={t.key} className={`absolute inset-0 ${t.key === activeKey ? "" : "invisible"}`}>
                  <TerminalPanel workDir={t.workDir} launch={t.launch} sessionId={t.key} title={t.label} headless visible={t.key === activeKey} onClose={() => closeTab(t.key)} />
                </div>
              ))}

              {currentSessionTabs.length === 0 && (
                <div className="flex h-full flex-col items-center justify-center gap-5 p-8 text-center">
                  <Layers className="h-8 w-8 text-indigo-400/80" />
                  <div>
                    <div className="text-[14px] font-medium text-slate-200">
                      {activeSessionInfo?.title || "No open tabs in this task"}
                    </div>
                    <div className="mt-1 text-[12px] text-slate-500">
                      {activeSessionId ? `Task ID: ${activeSessionId} · Open an agent below to start working.` : "Select a task from the sidebar or create a new one."}
                    </div>
                  </div>
                  <div className="flex flex-wrap justify-center gap-2">
                    {installed.map((e) => (
                      <button
                        key={e.id}
                        onClick={() => (activeSessionId ? openAgentInSession(e.id) : createNewSession(e.id))}
                        className="flex cursor-pointer items-center gap-2 rounded-lg border border-[#2c3447] bg-[#1b202c] px-3.5 py-2 text-[12px] text-slate-200 hover:bg-[#232a3a]"
                      >
                        <span className={`h-1.5 w-1.5 rounded-full ${AGENT_META[e.id]?.dot}`} />
                        {e.name}
                      </button>
                    ))}
                    <button
                      onClick={() => (activeSessionId ? openShellInSession() : createNewSession("agy"))}
                      className="flex cursor-pointer items-center gap-2 rounded-lg border border-[#2c3447] bg-[#1b202c] px-3.5 py-2 text-[12px] text-slate-200 hover:bg-[#232a3a]"
                    >
                      <TerminalIcon className="h-3.5 w-3.5 text-sky-400" />
                      Shell
                    </button>
                  </div>
                </div>
              )}
            </div>
          </div>

          {/* ---------- Context drawer: what a switch would hand over ---------- */}
          {contextOpen && activeTab?.agent && (
            <aside className="fixed inset-0 z-30 flex w-full flex-col border-l border-[#1d222b] bg-[#101319] pt-[env(safe-area-inset-top)] md:static md:z-auto md:w-80 md:shrink-0 md:pt-0 xl:w-[360px]">
              <div className="flex items-center justify-between border-b border-[#1d222b] px-4 py-2.5">
                <span className="text-[11px] font-semibold uppercase tracking-wider text-slate-500">Context</span>
                <button onClick={() => setContextOpen(false)} className="cursor-pointer p-0.5 text-slate-500 hover:text-slate-300">
                  <X className="h-3.5 w-3.5" />
                </button>
              </div>
              <div className="min-h-0 flex-1 space-y-4 overflow-y-auto p-4">
                {!ctx ? (
                  <div className="text-[12px] leading-relaxed text-slate-500">No session recorded yet. Send your first message to see context here.</div>
                ) : (
                  <>
                    <div>
                      <div className="flex items-center gap-2">
                        <span className={`rounded border px-1.5 py-0.5 text-[10px] font-medium ${AGENT_META[ctx.agent]?.badge}`}>{AGENT_META[ctx.agent]?.label}</span>
                        <button onClick={() => copyText(ctx.id)} className="ml-auto flex cursor-pointer items-center gap-1 font-mono text-[10px] text-slate-500 hover:text-slate-300" title="Copy session id">
                          {copied ? <Check className="h-3 w-3 text-emerald-400" /> : <Copy className="h-3 w-3" />}
                          {ctx.id.slice(0, 8)}
                        </button>
                      </div>
                      <div className="mt-1.5 text-[13px] font-semibold leading-snug text-slate-100">{ctx.title || "Untitled"}</div>
                      <div className="mt-0.5 text-[11px] text-slate-500">
                        {detail?.turn_count ?? 0} messages{detail?.updated_at && relTime(detail.updated_at) && ` · updated ${relTime(detail.updated_at)}`}
                      </div>
                    </div>

                    <div>
                      <div className="mb-2 text-[10.5px] font-semibold uppercase tracking-wider text-indigo-300">{lastTurns.length} recent messages · handoff payload</div>
                      {!detail && <div className="text-[12px] text-slate-500">Reading transcript…</div>}
                      <div className="space-y-2.5">
                        {lastTurns.map((t, i) => (
                          <div key={i} className={`rounded-lg border px-3 py-2 ${t.role === "user" ? "border-[#2a3347] bg-[#171c28]" : "border-[#232a39] bg-[#121620]"}`}>
                            <div className="mb-0.5 text-[10px] font-medium uppercase tracking-wide text-slate-500">{t.role === "user" ? "User" : AGENT_META[ctx.agent]?.label}</div>
                            <div className="line-clamp-6 whitespace-pre-wrap break-words text-[12px] leading-relaxed text-slate-300">{t.content}</div>
                          </div>
                        ))}
                      </div>
                    </div>
                  </>
                )}

                <div>
                  <div className="mb-2 flex items-center gap-1.5 text-[10.5px] font-semibold uppercase tracking-wider text-slate-500">
                    <GitBranch className="h-3.5 w-3.5 text-emerald-400" />
                    Changed Files
                    <span className="ml-auto font-normal normal-case tracking-normal">{modifiedFiles.length}</span>
                  </div>
                  {modifiedFiles.length === 0 ? (
                    <div className="text-[11.5px] text-slate-500">Clean working tree.</div>
                  ) : (
                    <div className="max-h-56 space-y-1 overflow-y-auto">
                      {modifiedFiles.map((f) => (
                        <div key={f} className="truncate rounded bg-[#1a2030] px-2 py-1 font-mono text-[10.5px] text-slate-300" title={f}>
                          {f}
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              </div>
            </aside>
          )}
        </div>
      </div>

      {/* Smart handoff sync notification toast */}
      {syncToast && (
        <div className="fixed bottom-4 right-4 z-50 flex items-center gap-2.5 rounded-lg border border-indigo-500/50 bg-[#151a28] px-4 py-2.5 text-xs text-indigo-200 shadow-2xl backdrop-blur">
          <Zap className="h-4 w-4 shrink-0 text-amber-400 fill-amber-400" />
          <span className="font-medium">{syncToast}</span>
          <button
            onClick={() => setSyncToast(null)}
            className="ml-2 cursor-pointer p-0.5 text-slate-400 hover:text-slate-200"
          >
            <X className="h-3.5 w-3.5" />
          </button>
        </div>
      )}

      {/* Modal: New Project Dialog */}
      {newProjectOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm p-4 animate-in fade-in duration-150">
          <div className="w-full max-w-lg rounded-xl border border-[#2b3548] bg-[#121620] shadow-2xl overflow-hidden flex flex-col text-slate-200">
            {/* Modal Header */}
            <div className="flex items-center justify-between border-b border-[#1f2636] px-5 py-3.5 bg-[#161b27]">
              <div className="flex items-center gap-2">
                <FolderPlus className="h-4 w-4 text-emerald-400" />
                <span className="text-sm font-semibold text-slate-100">Add Project</span>
              </div>
              <button
                onClick={() => setNewProjectOpen(false)}
                className="cursor-pointer p-1 text-slate-500 hover:text-slate-300 rounded"
              >
                <X className="h-4 w-4" />
              </button>
            </div>

            {/* Modal Body */}
            <form onSubmit={handleCreateProject} className="p-5 space-y-4 text-xs">
              <div>
                <label className="block text-[11px] font-semibold uppercase tracking-wider text-slate-400 mb-1.5">
                  Folder Path
                </label>
                <div className="flex items-center gap-2">
                  <input
                    type="text"
                    required
                    placeholder="/home/chungnh/my-project"
                    value={newProjectPath}
                    onChange={(e) => handleSelectFolder(e.target.value)}
                    className="flex-1 rounded-lg border border-[#2c3549] bg-[#0c0e14] px-3 py-2 text-xs font-mono text-slate-100 placeholder-slate-600 outline-none focus:border-indigo-500"
                  />
                  <button
                    type="button"
                    onClick={() => {
                      if (!fsCurrent) loadDirectories(newProjectPath || undefined);
                    }}
                    className="cursor-pointer rounded-lg border border-[#2c3549] bg-[#1a202c] px-3 py-2 text-xs font-medium text-slate-300 hover:bg-[#232b3b]"
                  >
                    Browse
                  </button>
                </div>
              </div>

              {/* Directory Browser Accordion */}
              {fsCurrent && (
                <div className="rounded-lg border border-[#222a3a] bg-[#0c0f16] p-2.5 space-y-2">
                  <div className="flex items-center justify-between text-[11px] text-slate-400 border-b border-[#1c2230] pb-1.5">
                    <span className="truncate font-mono" title={fsCurrent}>📂 {fsCurrent}</span>
                    {fsParent && (
                      <button
                        type="button"
                        onClick={() => loadDirectories(fsParent)}
                        className="flex cursor-pointer items-center gap-1 rounded bg-[#181f2c] px-2 py-0.5 text-[10.5px] text-indigo-300 hover:bg-indigo-900/40"
                      >
                        <ArrowUp className="h-3 w-3" /> Up
                      </button>
                    )}
                  </div>
                  <div className="max-h-36 overflow-y-auto space-y-0.5 pr-1">
                    {fsDirs.length === 0 ? (
                      <div className="py-2 text-center text-[11px] text-slate-500">No subdirectories found.</div>
                    ) : (
                      fsDirs.map((dir) => (
                        <div
                          key={dir}
                          className="flex items-center justify-between rounded px-2 py-1 hover:bg-[#181f2d] group cursor-pointer"
                          onClick={() => {
                            const sub = fsCurrent.endsWith("/") ? `${fsCurrent}${dir}` : `${fsCurrent}/${dir}`;
                            handleSelectFolder(sub);
                            loadDirectories(sub);
                          }}
                        >
                          <span className="truncate font-mono text-[11px] text-slate-300 group-hover:text-indigo-200">
                            📁 {dir}
                          </span>
                          <span className="text-[10px] text-slate-500 opacity-0 group-hover:opacity-100">
                            Select
                          </span>
                        </div>
                      ))
                    )}
                  </div>
                </div>
              )}

              <div>
                <label className="block text-[11px] font-semibold uppercase tracking-wider text-slate-400 mb-1.5">
                  Project Name (Optional)
                </label>
                <input
                  type="text"
                  placeholder="e.g. Backend API, Mobile App..."
                  value={newProjectName}
                  onChange={(e) => setNewProjectName(e.target.value)}
                  className="w-full rounded-lg border border-[#2c3549] bg-[#0c0e14] px-3 py-2 text-xs text-slate-100 placeholder-slate-600 outline-none focus:border-indigo-500"
                />
                <span className="mt-1 block text-[10.5px] text-slate-500">
                  Display name in sidebar (does not change folder path).
                </span>
              </div>

              <div className="flex items-center gap-2 pt-1">
                <input
                  type="checkbox"
                  id="auto_create_dir"
                  checked={newProjectAutoCreate}
                  onChange={(e) => setNewProjectAutoCreate(e.target.checked)}
                  className="rounded border-[#2c3549] bg-[#0c0e14] text-indigo-500 focus:ring-0 cursor-pointer"
                />
                <label htmlFor="auto_create_dir" className="text-[11.5px] text-slate-300 cursor-pointer">
                  Create folder if it does not exist
                </label>
              </div>

              {/* Modal Actions */}
              <div className="flex items-center justify-end gap-2.5 pt-3 border-t border-[#1c2230]">
                <button
                  type="button"
                  onClick={() => setNewProjectOpen(false)}
                  className="cursor-pointer rounded-lg border border-[#2c3549] bg-[#161a24] px-4 py-2 text-xs font-medium text-slate-300 hover:bg-[#202634]"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={!newProjectPath.trim()}
                  className="cursor-pointer rounded-lg bg-indigo-600 px-4 py-2 text-xs font-medium text-white hover:bg-indigo-500 disabled:opacity-50 disabled:cursor-not-allowed shadow"
                >
                  Add Project
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Modal: View .agent/handoff.md */}
      {handoffModalOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm p-4 animate-in fade-in duration-150">
          <div className="w-full max-w-2xl max-h-[85vh] rounded-xl border border-[#2b3548] bg-[#121620] shadow-2xl overflow-hidden flex flex-col text-slate-200">
            {/* Modal Header */}
            <div className="flex items-center justify-between border-b border-[#1f2636] px-5 py-3.5 bg-[#161b27]">
              <div className="flex items-center gap-2">
                <BookOpen className="h-4 w-4 text-indigo-400" />
                <span className="text-sm font-semibold text-slate-100">.agent/handoff.md</span>
                {handoffUpdatedAt && (
                  <span className="text-[11px] text-slate-500 font-mono">
                    · {relTime(handoffUpdatedAt)}
                  </span>
                )}
              </div>
              <div className="flex items-center gap-2">
                <button
                  onClick={() => {
                    navigator.clipboard?.writeText(handoffContent);
                    setHandoffCopied(true);
                    setTimeout(() => setHandoffCopied(false), 1500);
                  }}
                  className="flex cursor-pointer items-center gap-1 rounded bg-white/[0.06] px-2.5 py-1 text-[11px] font-medium text-slate-300 hover:bg-white/[0.1] transition-colors"
                  title="Copy handoff content"
                >
                  {handoffCopied ? <Check className="h-3 w-3 text-emerald-400" /> : <Copy className="h-3 w-3" />}
                  <span>{handoffCopied ? "Copied" : "Copy"}</span>
                </button>
                <button
                  onClick={() => setHandoffModalOpen(false)}
                  className="cursor-pointer p-1 text-slate-500 hover:text-slate-300 rounded"
                >
                  <X className="h-4 w-4" />
                </button>
              </div>
            </div>

            {/* Modal Body */}
            <div className="flex-1 overflow-y-auto p-4 font-mono text-xs leading-relaxed text-slate-300 bg-[#0c0e14] whitespace-pre-wrap select-text">
              {handoffContent ? (
                handoffContent
              ) : (
                <div className="py-8 text-center text-slate-500">
                  File .agent/handoff.md is currently empty or has not been created yet.
                  <div className="mt-2 text-[11px]">
                    Click "Write Handoff" in the tab bar to have an agent summarize and write into it.
                  </div>
                </div>
              )}
            </div>

            {/* Modal Footer */}
            <div className="flex items-center justify-between border-t border-[#1c2230] px-5 py-3 bg-[#161b27]">
              <span className="truncate font-mono text-[10.5px] text-slate-500" title={handoffPath}>
                {handoffPath}
              </span>
              <div className="flex items-center gap-2">
                <button
                  type="button"
                  onClick={() => {
                    setHandoffModalOpen(false);
                    handleTriggerHandoff("read");
                  }}
                  disabled={!handoffContent || !activeTab}
                  className="cursor-pointer rounded-lg bg-indigo-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-indigo-500 disabled:opacity-50 disabled:cursor-not-allowed shadow"
                >
                  Send to Active Tab
                </button>
                <button
                  type="button"
                  onClick={() => setHandoffModalOpen(false)}
                  className="cursor-pointer rounded-lg border border-[#2c3549] bg-[#161a24] px-3.5 py-1.5 text-xs font-medium text-slate-300 hover:bg-[#202634]"
                >
                  Close
                </button>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Modal: Settings (General, Agents, Appearance, Updates) */}
      <SettingsModal
        isOpen={settingsOpen}
        onClose={() => setSettingsOpen(false)}
        initialTab={settingsTab}
        engines={engines}
        onRefreshEngines={loadEngines}
        onOpenAgentTerminal={(agentId) => {
          if (activeSessionId) {
            openAgentInSession(agentId);
          } else {
            createNewSession(agentId);
          }
        }}
        updateStatus={updateStatus}
        onRefreshUpdateStatus={loadUpdateStatus}
      />
    </div>
  );
}
export { App };
