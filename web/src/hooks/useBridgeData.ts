import { useCallback, useEffect, useState } from "react";
import type { BridgeSession, EngineStatus, NativeSession, UpdateStatus, Workspace } from "../types/bridge";
import { INITIAL_ENGINES, isProtectedWorkspacePath } from "../constants/agentMeta";

export function useBridgeData() {
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
  const [workspace, setWorkspace] = useState<string>(
    () => localStorage.getItem("bridge_workspace") || ""
  );
  const [sessions, setSessions] = useState<BridgeSession[]>([]);
  const [detail, setDetail] = useState<NativeSession | null>(null);
  const [modifiedFiles, setModifiedFiles] = useState<string[]>([]);
  const [engines, setEngines] = useState<EngineStatus[]>(INITIAL_ENGINES);
  const [loading, setLoading] = useState(false);
  const [updateStatus, setUpdateStatus] = useState<UpdateStatus | null>(null);
  const [syncToast, setSyncToast] = useState<string | null>(null);

  const showToast = useCallback((msg: string, duration = 4000) => {
    setSyncToast(msg);
    setTimeout(() => setSyncToast((prev) => (prev === msg ? null : prev)), duration);
  }, []);

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
      /* fallback */
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
      if (gRes.ok) {
        setModifiedFiles((await gRes.json()).modified_files || []);
      }
    } finally {
      if (!quiet) setLoading(false);
    }
  }, []);

  const loadUpdateStatus = useCallback(async (triggerRemoteCheck = false) => {
    try {
      let endpoint = "/api/admin/update/status";
      let method = "GET";
      if (triggerRemoteCheck) {
        endpoint = "/api/admin/update/check";
        method = "POST";
      }
      const res = await fetch(endpoint, { method });
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
      .then((d: Workspace[]) => {
        const list = d || [];
        setWorkspaces(list);
        setWorkspace((current) => {
          if (current && list.some((w) => w.path === current)) {
            return current;
          }
          const saved = localStorage.getItem("bridge_workspace");
          if (saved && list.some((w) => w.path === saved)) {
            return saved;
          }
          return list.length > 0 ? list[0].path : current;
        });
      })
      .catch(() => {});
    loadEngines();

    const lastCheckStr = localStorage.getItem("bridge_last_remote_update_check");
    const lastCheck = lastCheckStr ? parseInt(lastCheckStr, 10) : 0;
    const now = Date.now();
    const shouldCheckRemote = !lastCheck || now - lastCheck >= 24 * 60 * 60 * 1000;

    if (shouldCheckRemote) {
      loadUpdateStatus(true);
      localStorage.setItem("bridge_last_remote_update_check", String(now));
    } else {
      loadUpdateStatus(false);
    }

    const ut = window.setInterval(() => {
      loadUpdateStatus(true);
      localStorage.setItem("bridge_last_remote_update_check", String(Date.now()));
    }, 24 * 60 * 60 * 1000);

    return () => window.clearInterval(ut);
  }, [loadEngines, loadUpdateStatus]);

  useEffect(() => {
    if (!workspace) return;
    localStorage.setItem("bridge_workspace", workspace);
    loadWorkspace(workspace);
    const t = window.setInterval(() => loadWorkspace(workspace, true), 15000);
    return () => window.clearInterval(t);
  }, [workspace, loadWorkspace]);

  const renameWorkspace = async (id: string, newName: string) => {
    setWorkspaces((prev) => prev.map((w) => (w.id === id ? { ...w, name: newName } : w)));
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

  const deleteWorkspace = async (id: string, name: string, e?: React.MouseEvent) => {
    e?.stopPropagation();
    const ws = workspaces.find((w) => w.id === id);
    if (ws && isProtectedWorkspacePath(ws.path)) {
      alert(`Thư mục "${ws.path}" thuộc hệ thống / người dùng được bảo vệ, không thể gỡ bỏ.`);
      return;
    }
    if (!confirm(`Bạn có chắc muốn gỡ bỏ dự án "${name}" khỏi danh sách? (Dữ liệu trên ổ cứng không bị xoá)`)) return;
    try {
      const res = await fetch(`/api/bridge/workspaces/${id}`, { method: "DELETE" });
      if (!res.ok) {
        const d = await res.json().catch(() => ({}));
        alert(d.error || "Gỡ dự án thất bại");
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

  const createWorkspace = async (path: string, name?: string, createDir = true) => {
    const r = await fetch("/api/bridge/workspaces", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ path, name, create_dir: createDir }),
    });
    if (r.ok) {
      const createdWs: Workspace = await r.json();
      setWorkspaces((prev) => [createdWs, ...prev.filter((w) => w.id !== createdWs.id)]);
      setWorkspace(createdWs.path);
    }
  };

  const renameTask = async (id: string, newTitle: string) => {
    setSessions((prev) => prev.map((s) => (s.id === id ? { ...s, title: newTitle } : s)));
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
          showToast(d.message);
          loadWorkspace(ws, true);
        }
      }
    } catch {
      /* best-effort */
    }
  };

  const createNewSession = async (initialAgent = "agy"): Promise<BridgeSession | null> => {
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
        return newSess;
      }
    } catch (e) {
      console.error("Create session error:", e);
    }
    return null;
  };

  const deleteSession = async (id: string): Promise<void> => {
    try {
      await fetch(`/api/bridge/sessions/${id}`, { method: "DELETE" });
      setSessions((prev) => prev.filter((s) => s.id !== id));
    } catch (err) {
      console.error("Delete session error:", err);
    }
  };

  return {
    workspaces,
    workspace,
    setWorkspace,
    sessions,
    setSessions,
    detail,
    setDetail,
    modifiedFiles,
    engines,
    loading,
    updateStatus,
    syncToast,
    setSyncToast,
    showToast,
    loadEngines,
    loadWorkspace,
    loadUpdateStatus,
    renameWorkspace,
    deleteWorkspace,
    createWorkspace,
    renameTask,
    syncHandoff,
    createNewSession,
    deleteSession,
  };
}
