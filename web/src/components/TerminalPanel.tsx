import React, { useState, useEffect, useRef } from "react";
import { Terminal as TerminalIcon, X, Trash2, Send } from "lucide-react";

interface TerminalPanelProps {
  workDir?: string;
  onClose: () => void;
}

export const TerminalPanel: React.FC<TerminalPanelProps> = ({
  workDir = "/home/chungnh/AI Workspace",
  onClose,
}) => {
  const [output, setOutput] = useState<string[]>([]);
  const [input, setInput] = useState("");
  const [history, setHistory] = useState<string[]>([]);
  const [historyIdx, setHistoryIdx] = useState(-1);
  const wsRef = useRef<WebSocket | null>(null);
  const terminalEndRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const host = window.location.host;
    const ws = new WebSocket(`${protocol}//${host}/ws/terminal?dir=${encodeURIComponent(workDir)}`);
    wsRef.current = ws;

    setOutput([
      `\x1b[36mConnected to Terminal (${workDir})\x1b[0m`,
      `Type bash commands below and press Enter.`,
      `---------------------------------------`,
    ]);

    ws.onmessage = (event) => {
      setOutput((prev) => [...prev, event.data]);
    };

    ws.onclose = () => {
      setOutput((prev) => [...prev, `\n\x1b[31m[Session disconnected]\x1b[0m`]);
    };

    setTimeout(() => {
      inputRef.current?.focus();
    }, 100);

    return () => {
      ws.close();
    };
  }, [workDir]);

  useEffect(() => {
    terminalEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [output]);

  const handleSend = (e?: React.FormEvent) => {
    e?.preventDefault();
    if (!input.trim()) return;

    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      wsRef.current.send(input + "\n");
    }

    setHistory((prev) => [...prev, input]);
    setHistoryIdx(-1);
    setInput("");
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "ArrowUp") {
      e.preventDefault();
      if (history.length === 0) return;
      const nextIdx = historyIdx === -1 ? history.length - 1 : Math.max(0, historyIdx - 1);
      setHistoryIdx(nextIdx);
      setInput(history[nextIdx]);
    } else if (e.key === "ArrowDown") {
      e.preventDefault();
      if (historyIdx === -1) return;
      const nextIdx = historyIdx + 1;
      if (nextIdx >= history.length) {
        setHistoryIdx(-1);
        setInput("");
      } else {
        setHistoryIdx(nextIdx);
        setInput(history[nextIdx]);
      }
    }
  };

  const cleanAnsi = (text: string) => {
    return text.replace(/\x1b\[[0-9;]*[a-zA-Z]|\x1b\].*?\x07|\x1b\[.*?[mGKHJP]/g, "");
  };

  return (
    <div className="flex flex-col h-full w-full bg-[#0a0c10] text-xs font-mono select-none overflow-hidden">
      {/* Header */}
      <div className="h-11 px-3 border-b border-[#1d222b] flex items-center justify-between bg-[#14171e] shrink-0">
        <div className="flex items-center gap-2 min-w-0">
          <TerminalIcon className="w-3.5 h-3.5 text-sky-400 shrink-0" />
          <span className="font-semibold text-slate-200">Terminal</span>
          <span className="text-slate-500">•</span>
          <span className="text-slate-400 text-[10.5px] truncate max-w-[180px]">{workDir}</span>
        </div>

        <div className="flex items-center gap-1 shrink-0">
          <button
            type="button"
            onClick={() => setOutput([])}
            className="p-1 rounded text-slate-400 hover:text-slate-200 hover:bg-[#1f2533] transition-colors"
            title="Clear terminal"
          >
            <Trash2 className="w-3.5 h-3.5" />
          </button>
          <button
            type="button"
            onClick={onClose}
            className="p-1 rounded text-slate-400 hover:text-rose-400 hover:bg-[#1f2533] transition-colors"
            title="Close terminal"
          >
            <X className="w-3.5 h-3.5" />
          </button>
        </div>
      </div>

      {/* Terminal Output Stream */}
      <div
        className="flex-1 p-3 font-mono text-[12.5px] text-slate-200 overflow-y-auto overflow-x-auto bg-[#0a0c10] leading-relaxed whitespace-pre-wrap select-text cursor-text"
        onClick={() => inputRef.current?.focus()}
      >
        {output.map((line, idx) => (
          <div key={idx} className="break-all">
            {cleanAnsi(line)}
          </div>
        ))}
        <div ref={terminalEndRef} />
      </div>

      {/* Terminal Input Bar */}
      <form
        onSubmit={handleSend}
        className="h-10 px-2.5 bg-[#12151c] border-t border-[#1d222b] flex items-center gap-1.5 shrink-0"
      >
        <span className="text-emerald-400 font-bold select-none text-[12px]">$</span>
        <input
          ref={inputRef}
          type="text"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="Run command..."
          className="flex-1 bg-transparent text-slate-100 placeholder-slate-600 focus:outline-none text-[12px] font-mono"
        />
        <button
          type="submit"
          disabled={!input.trim()}
          className="p-1 rounded text-slate-400 hover:text-slate-100 hover:bg-[#1f2533] transition-colors disabled:opacity-30 cursor-pointer"
        >
          <Send className="w-3 h-3" />
        </button>
      </form>
    </div>
  );
};
