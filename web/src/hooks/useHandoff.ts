import { useState } from "react";
import type { TermTab } from "../types/bridge";

export function useHandoff(
  workspace: string,
  activeSessionId: string | null,
  activeTab: TermTab | null,
  syncHandoff: (ws: string, taskId: string, toAgent: string, fromAgent?: string) => void,
  triggerInput: (text: string, label?: string) => void
) {
  const [handoffModalOpen, setHandoffModalOpen] = useState(false);
  const [handoffContent, setHandoffContent] = useState("");
  const [handoffPath, setHandoffPath] = useState("");
  const [handoffUpdatedAt, setHandoffUpdatedAt] = useState("");

  const triggerHandoff = (action: "read" | "write") => {
    if (!activeTab) return;
    if (action === "read") {
      if (activeSessionId && activeTab.agent) syncHandoff(workspace, activeSessionId, activeTab.agent);
      triggerInput("Đọc .agent/handoff.md và tiếp tục công việc dang dở.\n", `Triggered: Read Handoff in ${activeTab.label}`);
    } else {
      triggerInput("Tóm tắt tiến độ hiện tại, các thay đổi đã làm và việc cần làm tiếp theo vào .agent/handoff.md để bàn giao.\n", `Triggered: Write Handoff in ${activeTab.label}`);
    }
  };

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
    } catch {}
  };

  return {
    handoffModalOpen,
    setHandoffModalOpen,
    handoffContent,
    handoffPath,
    handoffUpdatedAt,
    triggerHandoff,
    loadHandoffContent,
  };
}
