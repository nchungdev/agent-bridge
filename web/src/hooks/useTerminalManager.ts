import { useEffect, useMemo, useState } from "react";
import type { BridgeSession, TermTab } from "../types/bridge";
import { AGENT_META } from "../constants/agentMeta";
import { remoteLaunch } from "../remote";

export function useTerminalManager(
  workspace: string,
  sessions: BridgeSession[],
  onSyncHandoff: (ws: string, taskId: string, toAgent: string, fromAgent?: string) => void,
  onRefreshWorkspace: (ws: string, quiet?: boolean) => void,
  onShowToast: (msg: string) => void
) {
  const [tabs, setTabs] = useState<TermTab[]>([]);
  const [activeKey, setActiveKey] = useState<string | null>(null);
  const [selectedSessionId, setSelectedSessionId] = useState<string | null>(null);

  // Re-attach to running sessions from server
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
        } catch {}
        setActiveKey((cur) => cur ?? (list.some((x) => x.id === last) ? last : list[0].id));
      })
      .catch(() => {});
  }, []);

  useEffect(() => {
    try {
      if (activeKey) localStorage.setItem("bridge_active_tab", activeKey);
    } catch {}
  }, [activeKey]);

  const activeTab = tabs.find((t) => t.key === activeKey) || null;

  const activeSessionId = useMemo(() => {
    if (activeTab?.sessionId) return activeTab.sessionId;
    if (selectedSessionId) return selectedSessionId;
    if (tabs.length > 0) return tabs[0].sessionId;
    if (sessions.length > 0) return sessions[0].id;
    return null;
  }, [activeTab, selectedSessionId, tabs, sessions]);

  const currentSessionTabs = useMemo(() => {
    if (!activeSessionId) return [];
    return tabs.filter((t) => t.sessionId === activeSessionId);
  }, [tabs, activeSessionId]);

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

  const killSession = (key: string) => {
    fetch(`/api/terminal/sessions/${key}`, { method: "DELETE" }).catch(() => {});
  };

  const openTab = (t: TermTab) => {
    setTabs((prev) => [...prev, t]);
    setActiveKey(t.key);
    setSelectedSessionId(t.sessionId);
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

  const openSession = (s: BridgeSession) => {
    setSelectedSessionId(s.id);
    const sessTabs = tabs.filter((t) => t.sessionId === s.id);
    if (sessTabs.length > 0) {
      const match = sessTabs.find((t) => t.agent === s.current_agent) || sessTabs[0];
      setActiveKey(match.key);
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
      launch: {
        ...(boundNativeId ? { agent, resume: boundNativeId } : { agent, fresh: true }),
        ...remoteLaunch(agent, workspace),
      },
    });
  };

  const openAgentInSession = async (to: string, targetTaskId?: string) => {
    const taskId = targetTaskId || activeSessionId;
    if (!taskId) return;
    setSelectedSessionId(taskId);

    const existing = tabs.find((t) => t.sessionId === taskId && t.agent === to);
    if (existing) {
      setActiveKey(existing.key);
      if (activeTab?.agent && activeTab.agent !== to) {
        onSyncHandoff(workspace, taskId, to, activeTab.agent);
      }
      return;
    }

    const currentTask = sessions.find((s) => s.id === taskId);
    const boundNativeId = currentTask?.bindings?.[to];
    const safeId = taskId.slice(0, 18).replace(/[^a-zA-Z0-9_-]/g, "");
    const workDir = activeTab?.workDir || workspace;

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
        onSyncHandoff(workDir, taskId, to, activeTab.agent);
      }
      return;
    }

    const fromAgent = activeTab?.agent;
    const fromNativeId = fromAgent ? currentTask?.bindings?.[fromAgent] : undefined;

    if (fromAgent && fromNativeId) {
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
      } catch {}
    }

    if (!boundNativeId) {
      fetch("/api/bridge/sessions/bind", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          bridge_session_id: taskId,
          agent: to,
          native_session_id: "",
        }),
      })
        .then(() => onRefreshWorkspace(workDir, true))
        .catch(() => {});
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

  const triggerInput = async (text: string, label?: string) => {
    if (!activeKey) return;
    try {
      await fetch(`/api/terminal/sessions/${encodeURIComponent(activeKey)}/input`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ input: text }),
      });
      if (label) {
        onShowToast(label);
      }
    } catch (err) {
      console.error("Trigger input error:", err);
    }
  };

  const setRemoteOn = (key: string) => {
    setTabs((prev) => prev.map((t) => (t.key === key ? { ...t, remoteOn: true } : t)));
  };

  return {
    tabs,
    setTabs,
    activeKey,
    setActiveKey,
    activeTab,
    activeSessionId,
    selectedSessionId,
    setSelectedSessionId,
    currentSessionTabs,
    sessionOpenAgents,
    openTab,
    closeTab,
    killSession,
    openSession,
    openAgentInSession,
    openShellInSession,
    triggerInput,
    setRemoteOn,
  };
}
