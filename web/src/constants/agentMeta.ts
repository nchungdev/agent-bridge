import { getBaseAgent } from "../remote";
import type { EngineStatus } from "../types/bridge";

export function isProtectedWorkspacePath(p?: string): boolean {
  if (!p) return true;
  const clean = p.trim().replace(/[/\\]+$/, "");
  if (clean === "" || clean === "/" || clean === "." || clean === "~") return true;
  const parts = clean.split(/[/\\]/).filter(Boolean);
  if (parts.length === 1) return true;
  if (parts.length === 2 && (parts[0] === "Users" || parts[0] === "home")) return true;
  return false;
}

export const BASE_AGENT_META: Record<string, { label: string; badge: string; dot: string; chip: string }> = {
  agy: {
    label: "Antigravity",
    badge: "bg-blue-500/15 text-blue-300 border-blue-500/30",
    dot: "bg-blue-400",
    chip: "hover:border-blue-500/50 hover:text-blue-300",
  },
  claude: {
    label: "Claude Code",
    badge: "bg-orange-500/15 text-orange-300 border-orange-500/30",
    dot: "bg-orange-400",
    chip: "hover:border-orange-500/50 hover:text-orange-300",
  },
  codex: {
    label: "Codex",
    badge: "bg-emerald-500/15 text-emerald-300 border-emerald-500/30",
    dot: "bg-emerald-400",
    chip: "hover:border-emerald-500/50 hover:text-emerald-300",
  },
};

export const AGENT_META: Record<string, { label: string; badge: string; dot: string; chip: string }> = new Proxy(
  BASE_AGENT_META as any,
  {
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
  }
);

export const INITIAL_ENGINES: EngineStatus[] = [
  { id: "agy", name: "Antigravity", binary: "antigravity", installed: false },
  { id: "claude", name: "Claude Code", binary: "claude", installCmd: "npm install -g @anthropic-ai/claude-code", installed: false },
  { id: "codex", name: "Codex", binary: "codex", installCmd: "npm install -g @openai/codex", installed: false },
];

export const AGENT_ORDER: Record<string, number> = {
  agy: 1,
  claude: 2,
  codex: 3,
};

export function relTime(s?: string): string {
  if (!s) return "";
  const d = new Date(s);
  const diff = (Date.now() - d.getTime()) / 1000;
  if (isNaN(diff)) return "";
  if (diff < 60) return "just now";
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
  return `${Math.floor(diff / 86400)}d ago`;
}
