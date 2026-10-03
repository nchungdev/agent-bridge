export type EventType =
  | "text_delta" | "user_message" | "tool_call" | "tool_result" | "approval_request"
  | "approval_resolved" | "diff" | "usage" | "error" | "turn_done" | "engine_switch" | "state_change" | "shell";

export interface HubEvent {
  seq?: number;
  conv_id?: string;
  engine?: string;
  model?: string;
  type: EventType;
  time?: string;
  text?: string;
  tool?: { id: string; name: string; args?: Record<string, unknown>; output?: string };
  approval?: { id: string; tool: string; args?: Record<string, unknown>; risk?: string; title?: string };
  diff?: { file: string; patch: string };
  usage?: { input_tokens: number; output_tokens: number; context_tokens?: number; context_window?: number };
  err?: { kind: string; message: string };
  data?: Record<string, unknown>;
}

export interface AuthStatus { installed: boolean; known: boolean; logged_in: boolean; detail?: string; login_hint?: string }

export interface LoginState { flow?: string; running: boolean; finished: boolean; success: boolean; error?: string; urls: string[]; code?: string; needs_code: boolean; output: string }

export interface EngineInfo {
  id: string;
  can_login?: boolean;
  models_fetched_at?: string;
  auth?: AuthStatus;
  capabilities: { model_listing?: boolean; streaming: boolean; resume: boolean; permission_prompts: boolean; plan_mode: boolean; permission_modes?: string[]; mode_requires_restart?: boolean };
  models: { id: string; name: string; tier?: string }[];
}

export interface Snapshot {
  conv: { ActiveEngine: string; Mode: string; Workspace: string } | null;
  live: Record<string, string>;
  pending_approvals: { id: string; tool: string; args?: Record<string, unknown>; risk?: string }[];
}

export interface ConvSummary { id: string; name: string; workspace?: string; updated_at?: string }

export type Item =
  | { kind: "user"; key: string; text: string }
  | { kind: "assistant"; key: string; engine: string; text: string }
  | { kind: "tool"; key: string; name: string; args?: Record<string, unknown>; output?: string }
  | { kind: "approval"; key: string; id: string; tool: string; args?: Record<string, unknown>; risk?: string; resolved?: { allow: boolean; by: string } }
  | { kind: "switch"; key: string; from: string; to: string }
  | { kind: "error"; key: string; message: string; errKind: string };

/** Fold the raw event log into display items. Pure, so it also rebuilds history after a reconnect. */
export function fold(events: HubEvent[]): Item[] {
  const items: Item[] = [];
  const byApproval = new Map<string, Extract<Item, { kind: "approval" }>>();
  const byTool = new Map<string, Extract<Item, { kind: "tool" }>>();
  events.forEach((e, i) => {
    const key = String(e.seq ?? `l${i}`);
    switch (e.type) {
      case "user_message":
        items.push({ kind: "user", key, text: e.text ?? "" });
        break;
      case "text_delta": {
        const last = items[items.length - 1];
        if (last && last.kind === "assistant" && last.engine === (e.engine ?? "")) last.text += e.text ?? "";
        else items.push({ kind: "assistant", key, engine: e.engine ?? "", text: e.text ?? "" });
        break;
      }
      case "tool_call":
        if (e.tool) {
          const it = { kind: "tool" as const, key, name: e.tool.name, args: e.tool.args };
          byTool.set(e.tool.id, it);
          items.push(it);
        }
        break;
      case "tool_result":
        if (e.tool) {
          const it = byTool.get(e.tool.id);
          if (it) it.output = e.tool.output;
        }
        break;
      case "approval_request":
        if (e.approval) {
          const it = { kind: "approval" as const, key, id: e.approval.id, tool: e.approval.tool, args: e.approval.args, risk: e.approval.risk };
          byApproval.set(e.approval.id, it);
          items.push(it);
        }
        break;
      case "approval_resolved": {
        const it = byApproval.get(String(e.data?.id));
        if (it) it.resolved = { allow: Boolean(e.data?.allow), by: String(e.data?.by ?? "") };
        break;
      }
      case "engine_switch":
        items.push({ kind: "switch", key, from: String(e.data?.from ?? ""), to: String(e.data?.to ?? "") });
        break;
      case "error":
        if (e.err && e.err.kind !== "cancelled") items.push({ kind: "error", key, message: e.err.message, errKind: e.err.kind });
        break;
    }
  });
  return items;
}
