import React, { useCallback, useEffect, useRef, useState } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";
import { Terminal as TerminalIcon, X, Eraser, RotateCw } from "lucide-react";

interface TerminalPanelProps {
  workDir: string;
  /** false while another right-hand tab is in front; the shell keeps running */
  visible?: boolean;
  onClose: () => void;
  /** run this agent's CLI in the PTY instead of a bare shell (resume = the CLI's own session id) */
  launch?: { agent: string; resume?: string };
  /** persistent shell id: the server keeps the PTY alive and re-attaches to it (with its screen) on reconnect */
  sessionId?: string;
  title?: string;
  /** hide the built-in header when the parent renders its own tab bar */
  headless?: boolean;
}

type Status = "connecting" | "open" | "closed";

/** A real terminal (xterm.js) attached to a server-side PTY: colours, vim/htop/less, Tab completion, Ctrl+C, resize. */
export const TerminalPanel: React.FC<TerminalPanelProps> = ({ workDir, visible = true, onClose, launch, sessionId, title = "Terminal", headless = false }) => {
  const hostRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<Terminal | null>(null);
  const fitRef = useRef<FitAddon | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const [status, setStatus] = useState<Status>("connecting");
  const [reason, setReason] = useState("");

  const sendResize = useCallback(() => {
    const ws = wsRef.current;
    const term = termRef.current;
    if (ws && ws.readyState === WebSocket.OPEN && term) {
      ws.send(JSON.stringify({ type: "resize", cols: term.cols, rows: term.rows }));
    }
  }, []);

  const fit = useCallback(() => {
    const host = hostRef.current;
    if (!host || host.clientWidth === 0 || host.clientHeight === 0) return; // hidden tab
    try {
      fitRef.current?.fit();
    } catch {
      /* not laid out yet */
    }
    sendResize();
  }, [sendResize]);

  const connect = useCallback(() => {
    const term = termRef.current;
    if (!term) return;
    wsRef.current?.close();
    setStatus("connecting");
    setReason("");
    const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
    const q = new URLSearchParams({ dir: workDir });
    if (sessionId) q.set("id", sessionId);
    if (launch) {
      q.set("agent", launch.agent);
      if (launch.resume) q.set("resume", launch.resume);
    }
    const ws = new WebSocket(`${proto}//${window.location.host}/ws/terminal?${q}`);
    ws.binaryType = "arraybuffer";
    wsRef.current = ws;
    ws.onopen = () => {
      setStatus("open");
      fit();
      term.focus();
    };
    ws.onmessage = (ev) => {
      if (typeof ev.data === "string") term.write(ev.data);
      else term.write(new Uint8Array(ev.data as ArrayBuffer));
    };
    ws.onclose = (ev) => {
      if (wsRef.current !== ws) return; // replaced by a newer connection
      setStatus("closed");
      setReason(ev.reason || (ev.code === 1006 ? "connection lost" : "session ended"));
    };
  }, [workDir, fit, launch, sessionId]);

  // create the terminal once per panel
  useEffect(() => {
    const host = hostRef.current;
    if (!host) return;
    const term = new Terminal({
      cursorBlink: true,
      fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, "Liberation Mono", monospace',
      fontSize: 12.5,
      lineHeight: 1.25,
      scrollback: 5000,
      theme: { background: "#0a0c10", foreground: "#e2e8f0", cursor: "#38bdf8", selectionBackground: "#2b3a55" },
    });
    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    term.open(host);
    termRef.current = term;
    fitRef.current = fitAddon;

    term.onData((d) => {
      const ws = wsRef.current;
      if (ws && ws.readyState === WebSocket.OPEN) ws.send(new TextEncoder().encode(d)); // raw bytes to the PTY
    });
    // copy the selection with Cmd+C / Ctrl+Shift+C (plain Ctrl+C stays an interrupt)
    term.attachCustomKeyEventHandler((e) => {
      if (e.type === "keydown" && e.key.toLowerCase() === "c" && (e.metaKey || (e.ctrlKey && e.shiftKey)) && term.hasSelection()) {
        navigator.clipboard?.writeText(term.getSelection());
        return false;
      }
      // let the browser raise its paste event for Ctrl/Cmd+V and Ctrl+Shift+V instead of sending ^V
      if (e.type === "keydown" && e.key.toLowerCase() === "v" && (e.metaKey || e.ctrlKey)) return false;
      return true;
    });

    // paste: text goes through term.paste (bracketed paste, so editors/agents get it as one block);
    // an image is uploaded and its file path is pasted, which agent CLIs accept as an attachment
    const onPaste = async (e: ClipboardEvent) => {
      const items = Array.from(e.clipboardData?.items ?? []);
      const img = items.find((i) => i.kind === "file" && i.type.startsWith("image/"));
      const text = e.clipboardData?.getData("text/plain");
      e.preventDefault();
      e.stopPropagation();
      if (img) {
        const file = img.getAsFile();
        if (!file) return;
        try {
          const r = await fetch("/api/terminal/upload", { method: "POST", headers: { "Content-Type": file.type }, body: file });
          if (!r.ok) throw new Error(await r.text());
          const { path } = await r.json();
          term.paste(`${path} `);
        } catch (err) {
          term.write(`\r\n\x1b[31m[paste image failed: ${String(err).trim()}]\x1b[0m\r\n`);
        }
      } else if (text) {
        term.paste(text);
      }
    };
    host.addEventListener("paste", onPaste, true);

    const ro = new ResizeObserver(() => fit());
    ro.observe(host);
    fit();
    connect();

    return () => {
      host.removeEventListener("paste", onPaste, true);
      ro.disconnect();
      wsRef.current?.close();
      wsRef.current = null;
      term.dispose();
      termRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // coming back to the tab: refit and focus
  useEffect(() => {
    if (visible) {
      const t = window.setTimeout(() => {
        fit();
        termRef.current?.focus();
      }, 30);
      return () => window.clearTimeout(t);
    }
  }, [visible, fit]);

  return (
    <div className="flex flex-col h-full w-full bg-[#0a0c10] text-xs select-none overflow-hidden">
      {!headless && <div className="h-11 px-3 border-b border-[#1d222b] flex items-center justify-between bg-[#14171e] shrink-0">
        <div className="flex items-center gap-2 min-w-0">
          <TerminalIcon className="w-3.5 h-3.5 text-sky-400 shrink-0" />
          <span className="font-semibold text-slate-200">{title}</span>
          <span className={`w-1.5 h-1.5 rounded-full shrink-0 ${status === "open" ? "bg-emerald-400" : status === "connecting" ? "bg-amber-400 animate-pulse" : "bg-rose-500"}`} title={status} />
          <span className="text-slate-400 text-[10.5px] truncate max-w-[180px]" title={workDir}>{workDir}</span>
        </div>
        <div className="flex items-center gap-1 shrink-0">
          <button type="button" onClick={() => termRef.current?.clear()} className="p-1 rounded text-slate-400 hover:text-slate-200 hover:bg-[#1f2533] transition-colors cursor-pointer" title="Clear screen">
            <Eraser className="w-3.5 h-3.5" />
          </button>
          <button type="button" onClick={connect} className="p-1 rounded text-slate-400 hover:text-slate-200 hover:bg-[#1f2533] transition-colors cursor-pointer" title={launch ? "Restart this agent" : "Start a new shell session"}>
            <RotateCw className="w-3.5 h-3.5" />
          </button>
          <button type="button" onClick={onClose} className="p-1 rounded text-slate-400 hover:text-rose-400 hover:bg-[#1f2533] transition-colors cursor-pointer" title="Close terminal (ends the shell)">
            <X className="w-3.5 h-3.5" />
          </button>
        </div>
      </div>}

      <div className="relative flex-1 min-h-0">
        <div ref={hostRef} className="absolute inset-0 p-2" onClick={() => termRef.current?.focus()} />
        {status === "closed" && (
          <div className="absolute inset-x-0 bottom-0 flex items-center justify-between gap-3 border-t border-rose-900/50 bg-[#1c1416]/95 px-3 py-2 text-[11.5px] text-rose-200">
            <span>{launch ? "Agent session ended" : "Shell disconnected"}{reason ? ` — ${reason}` : ""}</span>
            <button type="button" onClick={connect} className="rounded bg-rose-700/70 px-2.5 py-1 text-white hover:bg-rose-600 cursor-pointer">Reconnect</button>
          </div>
        )}
      </div>
    </div>
  );
};
