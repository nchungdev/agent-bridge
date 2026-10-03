import React, { useState, useEffect, useRef } from "react";
import { Terminal as TerminalIcon, X, Maximize2, Minimize2, Trash2, Send } from "lucide-react";

interface TerminalModalProps {
  isOpen: boolean;
  onClose: () => void;
  workDir?: string;
}

export const TerminalModal: React.FC<TerminalModalProps> = ({
  isOpen,
  onClose,
  workDir = "/home/chungnh/AI Workspace",
}) => {
  const [output, setOutput] = useState<string[]>([]);
  const [input, setInput] = useState("");
  const [isMaximized, setIsMaximized] = useState(false);
  const [history, setHistory] = useState<string[]>([]);
  const [historyIdx, setHistoryIdx] = useState(-1);
  const wsRef = useRef<WebSocket | null>(null);
  const terminalEndRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (!isOpen) {
      if (wsRef.current) {
        wsRef.current.close();
        wsRef.current = null;
      }
      return;
    }

    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const host = window.location.host;
    const ws = new WebSocket(`${protocol}//${host}/ws/terminal?dir=${encodeURIComponent(workDir)}`);
    wsRef.current = ws;

    setOutput([
      `\x1b[36mConnected to AI Terminal (${workDir})\x1b[0m`,
      `Type commands or bash scripts below. Press Enter to execute.`,
      `-------------------------------------------------------`,
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
  }, [isOpen, workDir]);

  useEffect(() => {
    if (isOpen) {
      terminalEndRef.current?.scrollIntoView({ behavior: "smooth" });
    }
  }, [output, isOpen]);

  if (!isOpen) return null;

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
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm animate-in fade-in duration-150">
      <div
        className={`bg-[#0f1117] border border-[#232733] rounded-2xl shadow-2xl flex flex-col overflow-hidden transition-all duration-200 ${
          isMaximized ? "w-full h-full rounded-none" : "w-[900px] h-[580px] max-w-full max-h-[90vh]"
        }`}
      >
        {/* Terminal Header */}
        <div className="h-10 px-4 bg-[#151922] border-b border-[#232733] flex items-center justify-between select-none">
          <div className="flex items-center gap-2 text-slate-300 text-xs font-mono">
            <TerminalIcon className="w-3.5 h-3.5 text-sky-400" />
            <span className="font-semibold text-slate-200">Terminal</span>
            <span className="text-slate-500">•</span>
            <span className="text-slate-400 text-[11px] truncate max-w-md">{workDir}</span>
          </div>

          <div className="flex items-center gap-1.5">
            <button
              type="button"
              onClick={() => setOutput([])}
              className="p-1 rounded text-slate-400 hover:text-slate-200 hover:bg-[#202532] transition-colors"
              title="Clear terminal"
            >
              <Trash2 className="w-3.5 h-3.5" />
            </button>
            <button
              type="button"
              onClick={() => setIsMaximized(!isMaximized)}
              className="p-1 rounded text-slate-400 hover:text-slate-200 hover:bg-[#202532] transition-colors"
              title={isMaximized ? "Restore" : "Maximize"}
            >
              {isMaximized ? <Minimize2 className="w-3.5 h-3.5" /> : <Maximize2 className="w-3.5 h-3.5" />}
            </button>
            <button
              type="button"
              onClick={onClose}
              className="p-1 rounded text-slate-400 hover:text-rose-400 hover:bg-[#202532] transition-colors"
              title="Close terminal"
            >
              <X className="w-4 h-4" />
            </button>
          </div>
        </div>

        {/* Terminal Body */}
        <div
          className="flex-1 p-4 font-mono text-[13px] text-slate-200 overflow-y-auto overflow-x-auto bg-[#0a0c10] leading-relaxed whitespace-pre-wrap select-text cursor-text"
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
          className="h-11 px-3 bg-[#13161f] border-t border-[#232733] flex items-center gap-2 font-mono text-[13px]"
        >
          <span className="text-emerald-400 font-bold select-none shrink-0">$</span>
          <input
            ref={inputRef}
            type="text"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Run shell command..."
            className="flex-1 bg-transparent text-slate-100 placeholder-slate-600 focus:outline-none"
            autoFocus
          />
          <button
            type="submit"
            disabled={!input.trim()}
            className="p-1.5 rounded-lg text-slate-400 hover:text-slate-100 hover:bg-[#202633] transition-colors disabled:opacity-40 cursor-pointer"
          >
            <Send className="w-3.5 h-3.5" />
          </button>
        </form>
      </div>
    </div>
  );
};
