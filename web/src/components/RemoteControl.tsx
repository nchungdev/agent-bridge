import React, { useCallback, useEffect, useRef, useState } from "react";
import { ExternalLink, Radio, RefreshCw } from "lucide-react";
import { PER_SESSION_REMOTE, remoteEnabled, setRemoteEnabled } from "../remote";

const WEB_URL: Record<string, string> = {
  claude: "https://claude.ai/code",
  codex: "https://chatgpt.com/codex",
};

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
  onOpenIde: () => void;
}

type Out = { ok: boolean; text: string } | null;

/** Header button of an agent tab: switch on remote access for it and open the agent's web app. */
export const RemoteControl: React.FC<Props> = ({ agent, workDir, on, sendInput, onEnabled, onOpenIde }) => {
  const [open, setOpen] = useState(false);
  const [pos, setPos] = useState({ x: 0, y: 0 });
  const [busy, setBusy] = useState<string | null>(null);
  const [out, setOut] = useState<Out>(null);
  const [daemon, setDaemon] = useState<string>(""); // agy reports its daemon status
  const [always, setAlways] = useState(() => remoteEnabled(agent));
  const btn = useRef<HTMLButtonElement>(null);
  const web = WEB_URL[agent];
  const perSession = PER_SESSION_REMOTE.includes(agent);

  useEffect(() => {
    setAlways(remoteEnabled(agent));
    setOut(null);
  }, [agent]);

  const loadStatus = useCallback(() => {
    if (agent !== "agy") return;
    fetch("/api/bridge/remote")
      .then((r) => r.json())
      .then((list: { agent: string; status?: string }[]) => setDaemon(list.find((x) => x.agent === "agy")?.status || ""))
      .catch(() => {});
  }, [agent]);

  useEffect(() => {
    if (open) loadStatus();
  }, [open, loadStatus]);

  const run = async (action: "enable" | "disable" | "pair") => {
    setBusy(action);
    setOut(null);
    try {
      const r = await fetch(`/api/bridge/remote/${agent}/${action}`, { method: "POST" });
      const d = await r.json();
      setOut({ ok: !!d.ok, text: d.output || (d.ok ? "Xong." : "Lệnh không thành công.") });
      if (d.ok && action === "enable") onEnabled();
      loadStatus();
    } catch (e) {
      setOut({ ok: false, text: String(e) });
    } finally {
      setBusy(null);
    }
  };

  const toggleAlways = () => {
    const next = !always;
    setAlways(next);
    setRemoteEnabled(agent, next);
  };

  const item = "flex w-full cursor-pointer items-center gap-2 rounded-md px-3 py-2 text-left text-[12px] text-slate-200 hover:bg-[#222a3a] disabled:cursor-not-allowed disabled:opacity-50";

  return (
    <>
      <button
        ref={btn}
        onClick={() => {
          const r = btn.current?.getBoundingClientRect();
          if (r) setPos({ x: Math.max(8, Math.min(r.right - 304, window.innerWidth - 312)), y: r.bottom + 4 });
          setOpen((v) => !v);
        }}
        className={`flex shrink-0 cursor-pointer items-center gap-1.5 rounded-md border px-2.5 py-1 text-[12px] transition-colors ${
          on ? "border-emerald-500/40 bg-emerald-500/10 text-emerald-300 hover:bg-emerald-500/15" : "border-[#2c3447] bg-[#1b202c] text-slate-300 hover:bg-[#222838] hover:text-slate-100"
        }`}
        title={`Remote control: điều khiển ${LABEL[agent] || agent} từ app web / mobile`}
      >
        <Radio className="h-3.5 w-3.5" />
        <span className="hidden sm:inline">Remote</span>
        {on && <span className="h-1.5 w-1.5 rounded-full bg-emerald-400" />}
      </button>

      {open && (
        <>
          <div className="fixed inset-0 z-40" onClick={() => setOpen(false)} />
          <div style={{ left: pos.x, top: pos.y }} className="fixed z-50 w-[304px] rounded-lg border border-[#2c3447] bg-[#161b26] p-1.5 shadow-xl">
            <div className="px-2.5 pb-1.5 pt-1 text-[10.5px] font-semibold uppercase tracking-wider text-slate-500">
              Remote · {LABEL[agent] || agent}
            </div>

            {agent === "claude" && (
              <>
                <p className="px-2.5 pb-1.5 text-[11.5px] leading-relaxed text-slate-400">
                  Điều khiển phiên này từ claude.ai/code hoặc app Claude trên điện thoại. Claude sẽ in link và mã QR trong terminal.
                </p>
                <button
                  className={item}
                  disabled={on}
                  onClick={() => {
                    sendInput("/remote-control\n", "Đã gửi /remote-control cho Claude");
                    onEnabled();
                    setOpen(false);
                  }}
                >
                  <Radio className="h-3.5 w-3.5 text-emerald-400" />
                  {on ? "Remote đang bật cho phiên này" : "Bật Remote cho phiên này"}
                </button>
              </>
            )}

            {agent === "agy" && (
              <>
                <p className="px-2.5 pb-1.5 text-[11.5px] leading-relaxed text-slate-400">
                  Antigravity chạy một daemon remote cho cả máy.
                  {daemon && <span className="mt-1 block rounded bg-[#10141c] px-2 py-1 font-mono text-[10.5px] text-slate-300">{daemon}</span>}
                </p>
                <button className={item} disabled={busy !== null} onClick={() => run("enable")}>
                  {busy === "enable" ? <RefreshCw className="h-3.5 w-3.5 animate-spin" /> : <Radio className="h-3.5 w-3.5 text-emerald-400" />}
                  Bật daemon Remote
                </button>
                <button className={item} disabled={busy !== null} onClick={() => run("disable")}>
                  {busy === "disable" ? <RefreshCw className="h-3.5 w-3.5 animate-spin" /> : <Radio className="h-3.5 w-3.5 text-slate-500" />}
                  Tắt daemon Remote
                </button>
              </>
            )}

            {agent === "codex" && (
              <>
                <p className="px-2.5 pb-1.5 text-[11.5px] leading-relaxed text-slate-400">
                  Codex dùng một daemon remote chung. Bật daemon, rồi lấy mã ghép nối và nhập vào app Codex (web hoặc điện thoại).
                </p>
                <button className={item} disabled={busy !== null} onClick={() => run("enable")}>
                  {busy === "enable" ? <RefreshCw className="h-3.5 w-3.5 animate-spin" /> : <Radio className="h-3.5 w-3.5 text-emerald-400" />}
                  Bật daemon Remote
                </button>
                <button className={item} disabled={busy !== null} onClick={() => run("pair")}>
                  {busy === "pair" ? <RefreshCw className="h-3.5 w-3.5 animate-spin" /> : <Radio className="h-3.5 w-3.5 text-sky-400" />}
                  Lấy mã ghép nối
                </button>
                <button className={item} disabled={busy !== null} onClick={() => run("disable")}>
                  <Radio className="h-3.5 w-3.5 text-slate-500" />
                  Tắt daemon Remote
                </button>
              </>
            )}

            {out && (
              <pre className={`mx-1 mt-1 max-h-40 overflow-auto whitespace-pre-wrap break-words rounded border px-2 py-1.5 font-mono text-[10.5px] leading-snug ${out.ok ? "border-[#2a3347] bg-[#10141c] text-slate-300" : "border-rose-500/40 bg-[#2a1519] text-rose-200"}`}>
                {out.text}
              </pre>
            )}

            {perSession && (
              <label className="mt-1 flex cursor-pointer items-start gap-2 rounded-md px-3 py-2 text-[12px] text-slate-300 hover:bg-[#222a3a]">
                <input type="checkbox" checked={always} onChange={toggleAlways} className="mt-0.5 accent-emerald-500" />
                <span>
                  Luôn bật Remote cho session {LABEL[agent]} mở mới
                  <span className="block text-[10.5px] text-slate-500">dùng cờ <code>--remote-control</code> khi khởi động</span>
                </span>
              </label>
            )}

            <div className="my-1 h-px bg-[#232a3b]" />
            {web ? (
              <a href={web} target="_blank" rel="noopener noreferrer" className={item} onClick={() => setOpen(false)}>
                <ExternalLink className="h-3.5 w-3.5 text-slate-400" />
                Mở {web.replace("https://", "")}
              </a>
            ) : (
              <button className={item} onClick={() => { onOpenIde(); setOpen(false); }}>
                <ExternalLink className="h-3.5 w-3.5 text-slate-400" />
                Mở Antigravity IDE (trên máy chủ)
              </button>
            )}
            <div className="px-3 pb-1 pt-0.5 text-[10.5px] text-slate-500" title={workDir}>
              Remote đăng ký máy này với tài khoản của bạn.
            </div>
          </div>
        </>
      )}
    </>
  );
};
