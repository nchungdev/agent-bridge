// Remote access (the agents' web / mobile apps): which agents should open their sessions with it on.
// Kept per browser in localStorage; the server only does what the launch parameters ask for.

const key = (agent: string) => `bridge_remote_${agent}`;

export function remoteEnabled(agent: string): boolean {
  try {
    return localStorage.getItem(key(agent)) === "1";
  } catch {
    return false;
  }
}

export function setRemoteEnabled(agent: string, on: boolean): void {
  try {
    if (on) localStorage.setItem(key(agent), "1");
    else localStorage.removeItem(key(agent));
  } catch {
    /* storage unavailable */
  }
}

/** Extracts the base engine type for an agent variant (e.g. claude-me -> claude). */
export function getBaseAgent(agent: string): string {
  if (!agent) return "";
  const a = agent.toLowerCase().trim();
  if (a === "claude" || a.startsWith("claude-") || a.startsWith("claude_")) return "claude";
  if (a === "agy" || a.startsWith("agy-") || a.startsWith("agy_") || a === "antigravity" || a.startsWith("antigravity-") || a.startsWith("antigravity_")) return "agy";
  if (a === "codex" || a.startsWith("codex-") || a.startsWith("codex_")) return "codex";
  return a;
}

/** Agents whose remote mode is a flag on each session. Codex uses a shared daemon instead, see the settings view. */
export const PER_SESSION_REMOTE = ["claude", "agy"];

/** extra launch parameters for a session of this agent in this folder */
export function remoteLaunch(agent: string, dir: string): { remote?: boolean; name?: string } {
  const base = getBaseAgent(agent);
  if (!PER_SESSION_REMOTE.includes(base) || !remoteEnabled(agent)) return {};
  return { remote: true, name: dir.split("/").filter(Boolean).pop() || undefined };
}

export async function openAgentWeb(agent: string, dir: string): Promise<void> {
  const r = await fetch("/api/bridge/open", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ agent, workspace_path: dir }),
  });
  const d = await r.json().catch(() => ({}));
  if (d.url) window.open(d.url, "_blank", "noopener");
  else if (!d.success) alert("Không mở được bản web của agent này");
}
