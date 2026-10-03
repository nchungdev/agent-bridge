import type { MessageItem, ToolStepItem } from "../components/ChatStream";
import type { HubEvent } from "./types";

// Names the v1 TurnStream groups on (command / explore / edit); engines use their own tool names.
function stepName(tool: string): string {
  if (tool === "Bash") return "run_command";
  return tool;
}

/** Fold the hub's engine-agnostic event log into the message shape the v1 chat view renders. */
export function eventsToMessages(events: HubEvent[]): MessageItem[] {
  const out: MessageItem[] = [];
  let cur: MessageItem | null = null;
  const steps = new Map<string, ToolStepItem>();

  const ensure = (e: HubEvent): MessageItem => {
    if (!cur) {
      cur = { role: "assistant", content: "", agent: e.engine, model: e.model, steps: [], is_running: true };
      out.push(cur);
    }
    if (e.engine) cur.agent = e.engine;
    if (e.model) cur.model = e.model;
    return cur;
  };

  for (const e of events) {
    switch (e.type) {
      case "user_message":
        if (cur) (cur as MessageItem).is_running = false;
        cur = null;
        out.push({ role: "user", content: e.text ?? "" });
        break;
      case "text_delta":
        ensure(e).content += e.text ?? "";
        break;
      case "tool_call": {
        if (!e.tool) break;
        const a = e.tool.args ?? {};
        const command = typeof a.command === "string" ? a.command : undefined;
        const path = (a.file_path ?? a.path ?? a.pattern) as string | undefined;
        const step: ToolStepItem = {
          name: stepName(e.tool.name),
          action: e.tool.name,
          summary: command ?? path ?? e.tool.name,
          command,
          cwd: typeof a.cwd === "string" ? a.cwd : undefined,
          path,
          status: "running",
        };
        ensure(e).steps!.push(step);
        steps.set(e.tool.id, step);
        break;
      }
      case "tool_result": {
        const st = e.tool && steps.get(e.tool.id);
        if (st && e.tool) {
          st.output = e.tool.output;
          st.status = "done";
        }
        break;
      }
      case "diff":
        if (e.diff) {
          const m = ensure(e);
          m.diffFile = e.diff.file;
          m.diffPatch = e.diff.patch;
        }
        break;
      case "error":
        if (e.err && e.err.kind !== "cancelled") {
          const m = ensure(e);
          m.content += `${m.content ? "\n\n" : ""}❌ **Error:** ${e.err.message}`;
          m.is_running = false;
          cur = null;
        }
        break;
      case "turn_done":
        if (cur) (cur as MessageItem).is_running = false;
        cur = null;
        break;
    }
  }
  return out;
}
