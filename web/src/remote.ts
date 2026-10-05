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

/** Agents whose remote mode is a flag on each session. Codex uses a shared daemon instead, see the settings view. */
export const PER_SESSION_REMOTE = ["claude", "agy"];

/** extra launch parameters for a session of this agent in this folder */
export function remoteLaunch(agent: string, dir: string): { remote?: boolean; name?: string } {
  if (!PER_SESSION_REMOTE.includes(agent) || !remoteEnabled(agent)) return {};
  return { remote: true, name: dir.split("/").filter(Boolean).pop() || undefined };
}
