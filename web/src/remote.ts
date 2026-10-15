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

/** Default web portals for agent remote management */
export function getAgentWebUrl(agent: string): string {
  const base = getBaseAgent(agent);
  if (base === "claude") return "https://claude.ai/code";
  if (base === "codex") return "https://chatgpt.com/codex";
  if (base === "agy") return "https://antigravity.google.com";
  return "https://antigravity.google.com";
}

export async function openAgentWeb(agent: string, dir: string, taskId?: string): Promise<void> {
  const fallback = getAgentWebUrl(agent);
  try {
    const r = await fetch("/api/bridge/open", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ agent, workspace_path: dir, task_id: taskId }),
    });
    const d = await r.json().catch(() => ({}));
    const targetUrl = d.url || fallback;
    window.open(targetUrl, "_blank", "noopener");
  } catch {
    window.open(fallback, "_blank", "noopener");
  }
}

