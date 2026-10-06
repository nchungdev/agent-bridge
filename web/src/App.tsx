import React, { useMemo, useRef, useState, useEffect } from "react";
import { X, Zap } from "lucide-react";
import { openAgentWeb } from "./remote";
import { usePersistedState } from "./hooks/usePersistedState";
import { useMedia } from "./hooks/useMedia";
import { useBridgeData } from "./hooks/useBridgeData";
import { useTerminalManager } from "./hooks/useTerminalManager";
import { useHandoff } from "./hooks/useHandoff";
import { AppSidebar } from "./components/sidebar/AppSidebar";
import { AppHeader } from "./components/header/AppHeader";
import { SessionTabBar } from "./components/terminal/SessionTabBar";
import { TerminalPanel } from "./components/TerminalPanel";
import { EmptyTaskState } from "./components/terminal/EmptyTaskState";
import { NewProjectModal } from "./components/modals/NewProjectModal";
import { HandoffModal } from "./components/modals/HandoffModal";
import { ContextDrawer } from "./components/modals/ContextDrawer";
import { SettingsModal } from "./components/settings/SettingsModal";

export default function App() {
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
  const isMobile = useMedia("(max-width: 767px)");
  const [sidebarPref, setSidebarPref] = usePersistedState("bridge_sidebar_collapsed", false);
  const [sidebarMobile, setSidebarMobile] = useState(true);
  const sidebarCollapsed = isMobile ? sidebarMobile : sidebarPref;
  const setSidebarCollapsed = isMobile ? setSidebarMobile : setSidebarPref;

  const [treeCollapsed, setTreeCollapsed] = usePersistedState("bridge_tree_collapsed", false);
  const [expandedTasks, setExpandedTasks] = usePersistedState<Record<string, boolean>>("bridge_expanded_tasks", {});

  // Modals state
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [settingsTab, setSettingsTab] = useState<"general" | "agents" | "appearance" | "updates">("general");
  const [newProjectOpen, setNewProjectOpen] = useState(false);
  const [contextOpen, setContextOpen] = useState(false);
  const [copied, setCopied] = useState(false);

  // Core data and terminal hooks
  const {
    workspaces,
    workspace,
    setWorkspace,
    sessions,
    detail,
    setDetail,
    modifiedFiles,
    engines,
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
  } = useBridgeData();

  const {
    tabs,
    activeKey,
    setActiveKey,
    activeTab,
    activeSessionId,
    currentSessionTabs,
    sessionOpenAgents,
    openSession,
    closeTab,
    killSession,
    openAgentInSession,
    openShellInSession,
    triggerInput,
    setRemoteOn,
    restartWithRemote,
  } = useTerminalManager(workspace, sessions, syncHandoff, loadWorkspace, showToast);

  const {
    handoffModalOpen,
    setHandoffModalOpen,
    handoffContent,
    handoffPath,
    handoffUpdatedAt,
    triggerHandoff,
    loadHandoffContent,
  } = useHandoff(workspace, activeSessionId, activeTab, syncHandoff, triggerInput);

  const activeSessionInfo = useMemo(() => {
    if (!activeSessionId) return null;
    return sessions.find((s) => s.id === activeSessionId) || null;
  }, [activeSessionId, sessions]);

  const ctx = useMemo(() => {
    if (!activeSessionInfo) return null;
    const agent = activeTab?.agent || activeSessionInfo.current_agent || "agy";
    return {
      agent,
      id: activeSessionInfo.bindings?.[agent] || "",
      title: activeSessionInfo.title,
      workspace,
    };
  }, [activeSessionInfo, activeTab, workspace]);

  const ctxKey = ctx && ctx.id ? `${ctx.agent}:${ctx.id}` : "";
  useEffect(() => {
    setDetail(null);
    if (!ctx || !ctx.id) return;
    const q = new URLSearchParams({ agent: ctx.agent, id: ctx.id, workspace });
    fetch(`/api/bridge/native-session?${q}`)
      .then((r) => (r.ok ? r.json() : null))
      .then(setDetail)
      .catch(() => {});
  }, [ctxKey, workspace, setDetail]);

  const copyText = (text: string) => {
    navigator.clipboard?.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };

  const handleCreateNewSession = async (agent = "agy") => {
    const s = await createNewSession(agent);
    if (s) openSession(s);
  };

  const handleDeleteSession = async (id: string, e?: React.MouseEvent) => {
    e?.stopPropagation();
    if (!confirm("Are you sure you want to delete this task?")) return;
    await deleteSession(id);
    for (const t of tabs.filter((x) => x.sessionId === id)) {
      killSession(t.key);
    }
  };

  const installed = engines.filter((e) => e.installed);
  const availableSessionAgents = installed.filter((e) => !currentSessionTabs.some((t) => t.agent === e.id));
  const hasSessionShell = currentSessionTabs.some((t) => !t.agent);

  return (
    <div className="flex h-dvh w-screen overflow-hidden bg-[#0b0e14] text-slate-100">
      <AppSidebar
        collapsed={sidebarCollapsed}
        isMobile={isMobile}
        workspaces={workspaces}
        currentWorkspace={workspace}
        treeCollapsed={treeCollapsed}
        sessions={sessions}
        activeSessionId={activeSessionId}
        expandedTasks={expandedTasks}
        sessionOpenAgents={sessionOpenAgents}
        installedEngines={installed}
        updateStatus={updateStatus}
        onSetCollapsed={setSidebarCollapsed}
        onSelectWorkspace={(p) => { setWorkspace(p); setTreeCollapsed(false); if (isMobile) setSidebarCollapsed(true); }}
        onToggleTree={() => setTreeCollapsed((v) => !v)}
        onOpenNewProjectModal={() => setNewProjectOpen(true)}
        onCreateSession={() => handleCreateNewSession()}
        onRenameWorkspace={renameWorkspace}
        onDeleteWorkspace={deleteWorkspace}
        onToggleTaskExpand={(id) => setExpandedTasks((prev) => ({ ...prev, [id]: !prev[id] }))}
        onOpenSession={(s) => { openSession(s); if (isMobile) setSidebarCollapsed(true); }}
        onDeleteSession={handleDeleteSession}
        onRenameSession={renameTask}
        onOpenAgent={(ag, tid) => { openAgentInSession(ag, tid); if (isMobile) setSidebarCollapsed(true); }}
        onOpenSettings={(tab) => { if (tab) setSettingsTab(tab); setSettingsOpen(true); }}
      />

      <div className="flex h-full min-w-0 flex-1 flex-col overflow-hidden">
        <AppHeader
          isMobile={isMobile}
          activeSessionInfo={activeSessionInfo}
          activeTab={activeTab}
          activeSessionId={activeSessionId}
          workspace={workspace}
          copied={copied}
          ctx={ctx}
          onToggleSidebar={() => setSidebarCollapsed(false)}
          onCopySessionId={copyText}
          onTriggerInput={triggerInput}
          onSetRemoteOn={setRemoteOn}
          onRestartRemote={restartWithRemote}
          onOpenAgentWeb={openAgentWeb}
        />

        <div className="min-h-0 flex-1 flex overflow-hidden">
          <div ref={termColRef} className="flex min-w-0 flex-1 flex-col bg-[#0c0e14]">
            <SessionTabBar
              tabs={currentSessionTabs}
              activeKey={activeKey}
              availableSessionAgents={availableSessionAgents}
              hasSessionShell={hasSessionShell}
              activeTab={activeTab}
              isMobile={isMobile}
              wideToolbar={wideToolbar}
              workspace={workspace}
              ctx={ctx}
              onSelectTab={setActiveKey}
              onCloseTab={closeTab}
              onOpenAgentInSession={openAgentInSession}
              onOpenShellInSession={openShellInSession}
              onTriggerHandoff={triggerHandoff}
              onViewHandoff={loadHandoffContent}
              onTriggerInput={triggerInput}
            />

            <div className="relative flex-1 overflow-hidden">
              {tabs.map((t) => (
                <div key={`${t.key}-${t.rev || 0}`} className={`absolute inset-0 ${t.key === activeKey ? "" : "invisible"}`}>
                  <TerminalPanel workDir={t.workDir} launch={t.launch} sessionId={t.key} title={t.label} headless visible={t.key === activeKey} onClose={() => closeTab(t.key)} />
                </div>
              ))}

              {currentSessionTabs.length === 0 && (
                <EmptyTaskState
                  activeSessionTitle={activeSessionInfo?.title}
                  activeSessionId={activeSessionId}
                  installedEngines={installed}
                  onOpenAgent={openAgentInSession}
                  onOpenShell={openShellInSession}
                  onCreateSessionWithAgent={handleCreateNewSession}
                />
              )}
            </div>
          </div>

          <ContextDrawer
            isOpen={contextOpen && !!activeTab?.agent}
            ctx={ctx}
            detail={detail}
            modifiedFiles={modifiedFiles}
            copied={copied}
            onClose={() => setContextOpen(false)}
            onCopySessionId={copyText}
          />
        </div>
      </div>

      {syncToast && (
        <div className="fixed bottom-4 right-4 z-50 flex items-center gap-2.5 rounded-lg border border-indigo-500/50 bg-[#151a28] px-4 py-2.5 text-xs text-indigo-200 shadow-2xl backdrop-blur">
          <Zap className="h-4 w-4 shrink-0 text-amber-400 fill-amber-400" />
          <span className="font-medium">{syncToast}</span>
          <button onClick={() => setSyncToast(null)} className="ml-2 cursor-pointer p-0.5 text-slate-400 hover:text-slate-200">
            <X className="h-3.5 w-3.5" />
          </button>
        </div>
      )}

      <NewProjectModal
        isOpen={newProjectOpen}
        onClose={() => setNewProjectOpen(false)}
        onCreateProject={createWorkspace}
      />

      <HandoffModal
        isOpen={handoffModalOpen}
        content={handoffContent}
        path={handoffPath}
        updatedAt={handoffUpdatedAt}
        hasActiveTab={!!activeTab}
        onClose={() => setHandoffModalOpen(false)}
        onSendToActiveTab={() => triggerHandoff("read")}
      />

      <SettingsModal
        isOpen={settingsOpen}
        onClose={() => setSettingsOpen(false)}
        initialTab={settingsTab}
        engines={engines}
        onRefreshEngines={loadEngines}
        onOpenAgentTerminal={(agentId) => {
          if (activeSessionId) openAgentInSession(agentId);
          else handleCreateNewSession(agentId);
        }}
        updateStatus={updateStatus}
        onRefreshUpdateStatus={loadUpdateStatus}
      />
    </div>
  );
}
export { App };
