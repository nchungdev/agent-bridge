import React, { useState } from "react";
import { Radio, RefreshCw } from "lucide-react";

const LABEL: Record<string, string> = { claude: "Claude Code", agy: "Antigravity", codex: "Codex" };

interface Props {
  agent: string;
  workDir: string;
  /** this session was opened with (or switched to) remote access */
  on: boolean;
  /** type a line into the active terminal (it goes to the agent) */
  sendInput: (text: string, toast?: string) => void;
  /** remember that remote access was switched on for this session */
  onEnabled: () => void;
  /** remote access of this session was switched off in place */
  onDisabled: () => void;
  /** the agent's own session id; "" until the CLI has reported it */
  nativeId: string;
  /** restart the agent on the same session with remote access on/off */
  onRestart: (remote: boolean) => void;
  onOpenIde: () => void;
}

/** Header button of an agent tab: one click switches remote access on/off for it. */
export const RemoteControl: React.FC<Props> = ({ agent, workDir, on, sendInput, onEnabled, onDisabled, nativeId }) => {
  const [busy, setBusy] = useState(false);

  const runDaemon = async (action: "enable" | "disable") => {
    setBusy(true);
    try {
      const qs = new URLSearchParams({ dir: workDir || "", session: nativeId || "" });
      const r = await fetch(`/api/bridge/remote/${agent}/${action}?${qs}`, { method: "POST" });
      const d = await r.json();
      if (d.ok) (action === "enable" ? onEnabled : onDisabled)();
    } catch {
      /* ignore: the button state simply does not change */
    } finally {
      setBusy(false);
    }
  };

  const toggle = () => {
    if (agent === "codex") {
      // codex has no per-session flag, only the shared daemon
      runDaemon(on ? "disable" : "enable");
      return;
    }
    if (on) {
      // claude: /remote-control opens its status panel (disconnect lives there)
      sendInput(agent === "agy" ? "/remote-control off\n" : "/remote-control\n", `Remote ${LABEL[agent] || agent}: tắt`);
      if (agent === "agy") onDisabled();
    } else {
      sendInput("/remote-control\n", `Remote ${LABEL[agent] || agent}: bật`);
      onEnabled();
    }
  };

  return (
    <button
      onClick={toggle}
      disabled={busy}
      className={`flex shrink-0 cursor-pointer items-center gap-1.5 rounded-md border px-2.5 py-1 text-[12px] transition-colors disabled:cursor-wait disabled:opacity-60 ${
        on ? "border-emerald-500/40 bg-emerald-500/10 text-emerald-300 hover:bg-emerald-500/15" : "border-[#2c3447] bg-[#1b202c] text-slate-300 hover:bg-[#222838] hover:text-slate-100"
      }`}
      title={on ? `Tắt Remote ${LABEL[agent] || agent}` : `Bật Remote ${LABEL[agent] || agent}`}
    >
      {busy ? <RefreshCw className="h-3.5 w-3.5 animate-spin" /> : <Radio className="h-3.5 w-3.5" />}
      <span className="hidden sm:inline">Remote</span>
      {on && <span className="h-1.5 w-1.5 rounded-full bg-emerald-400" />}
    </button>
  );
};
