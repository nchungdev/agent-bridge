import React, { useCallback, useEffect, useRef, useState } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebglAddon } from "@xterm/addon-webgl";
import "@xterm/xterm/css/xterm.css";
import { Terminal as TerminalIcon, X, Eraser, RotateCw, ArrowDown, ArrowUp, Keyboard } from "lucide-react";

interface TerminalPanelProps {
  workDir: string;
  /** false while another right-hand tab is in front; the shell keeps running */
  visible?: boolean;
  onClose: () => void;
  /** run this agent's CLI in the PTY instead of a bare shell (resume = the CLI's own session id) */
  launch?: { agent: string; account?: string; resume?: string; fresh?: boolean; remote?: boolean; name?: string; title?: string; taskId?: string };
  /** persistent shell id: the server keeps the PTY alive and re-attaches to it (with its screen) on reconnect */
  sessionId?: string;
  title?: string;
  /** hide the built-in header when the parent renders its own tab bar */
  headless?: boolean;
}

type Status = "connecting" | "open" | "closed";

/** A real terminal (xterm.js) attached to a server-side PTY: colours, vim/htop/less, Tab completion, Ctrl+C, resize. */
let cachedHardwareWebgl: boolean | null = null;
function hardwareWebgl(): boolean {
  if (cachedHardwareWebgl !== null) return cachedHardwareWebgl;
  try {
    const forced = new URLSearchParams(window.location.search).get("renderer") || localStorage.getItem("bridge_renderer");
    if (forced === "dom") {
      cachedHardwareWebgl = false;
      return false;
    }
    const canvas = document.createElement("canvas");
    const gl = canvas.getContext("webgl2") || canvas.getContext("webgl");
    if (!gl) {
      cachedHardwareWebgl = false;
      return false;
    }
    const info = (gl as WebGLRenderingContext).getExtension?.("WEBGL_debug_renderer_info");
    const name = info ? String((gl as WebGLRenderingContext).getParameter(info.UNMASKED_RENDERER_WEBGL)) : "";
    cachedHardwareWebgl = !/swiftshader|llvmpipe|software|basic render/i.test(name);
    return cachedHardwareWebgl;
  } catch {
    cachedHardwareWebgl = false;
    return false;
  }
}

function isTouchDevice(): boolean {
  if (typeof window === "undefined") return false;
  if (window.matchMedia("(pointer: coarse)").matches) return true;
  const isMobileUA =
    /Android|iPhone|iPad|iPod|Mobile|Tablet/i.test(navigator.userAgent) ||
    (navigator.platform === "MacIntel" && navigator.maxTouchPoints > 1);
  if (isMobileUA && (navigator.maxTouchPoints > 0 || "ontouchstart" in window)) return true;
  if (window.innerWidth <= 768 && ("ontouchstart" in window || navigator.maxTouchPoints > 0)) return true;
  return false;
}

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
      termRef.current?.scrollToBottom();
    } catch {
      /* not laid out yet */
    }
    sendResize();
  }, [sendResize]);

  const retriesRef = useRef(0);
  const ctrlRef = useRef(false);
  // upload progress and errors are shown over the terminal: text written into the terminal itself stays on
  // screen (the program inside does not know about it and never redraws that line)
  const [notice, setNotice] = useState<{ kind: "info" | "error"; text: string } | null>(null);
  const noticeTimer = useRef<number | undefined>(undefined);
  const showNotice = useCallback((kind: "info" | "error", text: string, ttl = 0) => {
    window.clearTimeout(noticeTimer.current);
    setNotice({ kind, text });
    if (ttl > 0) noticeTimer.current = window.setTimeout(() => setNotice(null), ttl);
  }, []);
  const [ctrl, setCtrl] = useState(false);
  // Auto-detect mobile/touch devices: show compose bar & touch keys; desktop uses direct terminal
  const [touch, setTouch] = useState<boolean>(() => isTouchDevice());

  useEffect(() => {
    const mq = window.matchMedia("(pointer: coarse)");
    const updateDevice = () => setTouch(isTouchDevice());
    mq.addEventListener?.("change", updateDevice);
    window.addEventListener("resize", updateDevice);
    return () => {
      mq.removeEventListener?.("change", updateDevice);
      window.removeEventListener("resize", updateDevice);
    };
  }, []);

  useEffect(() => {
    const term = termRef.current;
    if (!term?.textarea) return;
    if (touch) {
      term.textarea.setAttribute("inputmode", "none");
    } else {
      term.textarea.removeAttribute("inputmode");
    }
  }, [touch]);
  // Soft keyboards (Telex/VNI and other IMEs) send "composing" text that xterm's hidden input handles badly,
  // so text can be typed in a normal input and sent as one paste with 0 network lag
  const [compose, setCompose] = useState("");
  const [scrolledUp, setScrolledUp] = useState(false);
  const historyRef = useRef<string[]>([]);
  const historyIdxRef = useRef<number>(-1);
  const composeRef = useRef<HTMLInputElement>(null);
  const bottomBarRef = useRef<HTMLDivElement>(null);
  const [showKeys, setShowKeys] = useState<boolean>(() => {
    return localStorage.getItem("bridge_mobile_show_keys") === "1";
  });
  const toggleKeys = () => {
    setShowKeys((v) => {
      const next = !v;
      localStorage.setItem("bridge_mobile_show_keys", next ? "1" : "0");
      return next;
    });
  };

  const sendRaw = useCallback((d: string) => {
    const ws = wsRef.current;
    if (ws && ws.readyState === WebSocket.OPEN) ws.send(new TextEncoder().encode(d)); // raw bytes to the PTY
  }, []);

  const scrollToBottom = useCallback(() => {
    sendRaw("\x1b");
    termRef.current?.scrollToBottom();
    setScrolledUp(false);
  }, [sendRaw]);

  const sendCompose = () => {
    const text = compose;
    setCompose("");
    if (text) {
      historyRef.current.push(text);
      if (historyRef.current.length > 50) historyRef.current.shift();
      historyIdxRef.current = -1;
      termRef.current?.paste(text);
    }
    sendRaw("\r"); // Enter submits; an empty box just sends Enter
    termRef.current?.scrollToBottom();
    setScrolledUp(false);
    composeRef.current?.focus();
  };
  const legacyCopy = useCallback((text: string) => {
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.style.position = "fixed";
    ta.style.opacity = "0";
    document.body.appendChild(ta);
    ta.select();
    document.execCommand("copy");
    ta.remove();
    termRef.current?.focus();
  }, []);

  const copyText = useCallback((text: string) => {
    if (!text) return;
    if (navigator.clipboard?.writeText) {
      navigator.clipboard.writeText(text)
        .then(() => showNotice("info", "Đã chép vào clipboard", 1500))
        .catch(() => {
          legacyCopy(text);
          showNotice("info", "Đã chép vào clipboard", 1500);
        });
    } else {
      legacyCopy(text);
      showNotice("info", "Đã chép vào clipboard", 1500);
    }
  }, [legacyCopy, showNotice]);

  const copyTmuxSelection = useCallback(() => {
    if (!sessionId) {
      if (termRef.current?.hasSelection()) {
        copyText(termRef.current.getSelection());
      }
      return;
    }
    fetch(`/api/terminal/sessions/${encodeURIComponent(sessionId)}/buffer`)
      .then(async (r) => {
        if (!r.ok) {
          if (termRef.current?.hasSelection()) {
            copyText(termRef.current.getSelection());
            return;
          }
          throw new Error("nothing selected");
        }
        const text = await r.text();
        copyText(text);
      })
      .catch(() => {
        if (termRef.current?.hasSelection()) {
          copyText(termRef.current.getSelection());
        }
      });
  }, [sessionId, copyText]);

  const connectRef = useRef<() => void>(() => {});

  const connect = useCallback(() => {
    const term = termRef.current;
    if (!term) return;
    wsRef.current?.close();
    setStatus("connecting");
    setReason("");
    const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
    const q = new URLSearchParams({ dir: workDir });
    if (sessionId) q.set("id", sessionId);
    if (term.cols && term.rows) {
      q.set("cols", String(term.cols));
      q.set("rows", String(term.rows));
    }
    if (launch) {
      q.set("agent", launch.agent);
      if (launch.account) q.set("account", launch.account);
      if (launch.resume) q.set("resume", launch.resume);
      if (launch.fresh) q.set("new", "1");
      if (launch.remote) q.set("remote", "1");
      if (launch.name) q.set("name", launch.name);
      if (launch.title) q.set("title", launch.title);
      if (launch.taskId) q.set("task_id", launch.taskId);
    }
    const ws = new WebSocket(`${proto}//${window.location.host}/ws/terminal?${q}`);
    ws.binaryType = "arraybuffer";
    wsRef.current = ws;
    ws.onmessage = (ev) => {
      if (typeof ev.data === "string") term.write(ev.data);
      else term.write(new Uint8Array(ev.data as ArrayBuffer));
    };
    ws.onopen = () => {
      retriesRef.current = 0;
      setStatus("open");
      fit();
      term.focus();
    };
    ws.onclose = (ev) => {
      if (wsRef.current !== ws) return; // replaced by a newer connection
      // a dropped connection (server restarting, network blip) re-attaches on its own when the terminal
      // is persistent: the shell/agent keeps running server-side
      if (ev.code !== 1000 && sessionId && retriesRef.current < 30) {
        retriesRef.current += 1;
        setStatus("connecting");
        setReason("reconnecting…");
        window.setTimeout(() => {
          if (wsRef.current === ws && termRef.current) connectRef.current();
        }, 1500);
        return;
      }
      setStatus("closed");
      setReason(ev.reason || (ev.code === 1006 ? "connection lost" : "session ended"));
    };
  }, [workDir, fit, launch, sessionId]);
  connectRef.current = connect;

  // create the terminal once per panel
  useEffect(() => {
    const host = hostRef.current;
    if (!host) return;
    const term = new Terminal({
      cursorBlink: true,
      fontFamily: localStorage.getItem("bridge_term_font") || 'ui-monospace, SFMono-Regular, Menlo, Consolas, "Liberation Mono", monospace',
      fontSize: parseFloat(localStorage.getItem("bridge_term_fontsize") || "12.5"),
      lineHeight: 1.25,
      scrollback: 5000,
      scrollSensitivity: 1,
      fastScrollSensitivity: 3,
      macOptionClickForcesSelection: true,
      theme: { background: "#0a0c10", foreground: "#e2e8f0", cursor: "#38bdf8", selectionBackground: "#2b3a55" },
    });
    const fitAddon = new FitAddon();

    const onSettingsChanged = () => {
      const f = localStorage.getItem("bridge_term_font");
      const s = parseFloat(localStorage.getItem("bridge_term_fontsize") || "12.5");
      if (f) term.options.fontFamily = f;
      if (s) term.options.fontSize = s;
      try { fitAddon.fit(); } catch {}
    };
    window.addEventListener("bridge-terminal-settings-changed", onSettingsChanged);
    term.loadAddon(fitAddon);
    term.open(host);
    if (touch) term.textarea?.setAttribute("inputmode", "none");
    // The DOM renderer rebuilds hundreds of elements per update and drops to ~20 fps while tmux redraws the screen
    // for every scroll step; the WebGL one draws on the GPU. It falls back to DOM when WebGL is unavailable or lost.
    try {
      if (hardwareWebgl()) {
        const gl = new WebglAddon();
        gl.onContextLoss(() => gl.dispose());
        term.loadAddon(gl);
      }
    } catch {
      /* no WebGL: keep the DOM renderer */
    }
    termRef.current = term;
    fitRef.current = fitAddon;

    let pasteIntent: { kind: "text" | "image"; at: number } | null = null;
    term.onData((d) => {
      // the on-screen Ctrl key (touch devices) turns the next letter into a control character
      if (ctrlRef.current && d.length === 1) {
        ctrlRef.current = false;
        setCtrl(false);
        const c = d.toLowerCase().charCodeAt(0);
        if (c >= 97 && c <= 122) d = String.fromCharCode(c - 96);
      }
      sendRaw(d);
    });
    const isMac = /Mac|iPhone|iPad/.test(navigator.platform);
    // tmux (set-clipboard on) reports a finished selection as OSC 52: copy it straight away where the browser allows
    term.parser.registerOscHandler(52, (data) => {
      const b64 = data.split(";")[1];
      if (b64 && b64 !== "?") {
        try {
          const bytes = Uint8Array.from(atob(b64), (c) => c.charCodeAt(0));
          copyText(new TextDecoder().decode(bytes));
        } catch {
          /* malformed payload */
        }
      }
      return true;
    });
    // macOS never raises a paste event for Ctrl+V: read the clipboard ourselves (image, else text)
    const ctrlV = async () => {
      try {
        const items = await navigator.clipboard.read();
        for (const it of items) {
          const type = it.types.find((t) => t.startsWith("image/"));
          if (type) {
            const blob = await it.getType(type);
            await uploadAndPasteImage(new File([blob], `clipboard.${type.split("/")[1] || "png"}`, { type }));
            return;
          }
        }
        for (const it of items) {
          if (it.types.includes("text/plain")) {
            term.paste(await (await it.getType("text/plain")).text());
            return;
          }
        }
      } catch {
        /* no clipboard API (needs HTTPS/localhost) or permission denied: hand the key to the program */
      }
      sendRaw("\x16");
    };
    // Cmd+C (Mac), Ctrl+Shift+C (Win/Linux), or Ctrl+C if text is selected; plain Ctrl+C stays SIGINT
    term.attachCustomKeyEventHandler((e) => {
      if (e.type !== "keydown") return true;
      const key = e.key.toLowerCase();
      const hasSel = term.hasSelection();
      if (key === "c" && (e.metaKey || (e.ctrlKey && (e.shiftKey || hasSel)))) {
        e.preventDefault();
        if (hasSel) copyText(term.getSelection());
        else copyTmuxSelection();
        return false;
      }
      if (key === "v" && (e.metaKey || e.ctrlKey)) {
        // Cmd+V / Ctrl+Shift+V paste text (the browser raises a paste event); Ctrl+V pastes an image
        const plainCtrl = e.ctrlKey && !e.metaKey && !e.shiftKey;
        if (plainCtrl && isMac) {
          e.preventDefault();
          void ctrlV();
        } else {
          pasteIntent = { kind: plainCtrl ? "image" : "text", at: Date.now() };
        }
        return false;
      }
      return true;
    });

    // upload image helper
    const uploadAndPasteImage = async (file: File) => {
      showNotice("info", "Đang tải ảnh lên…");
      try {
        let contentType = file.type;
        if (!contentType) {
          const ext = file.name.split(".").pop()?.toLowerCase();
          if (ext === "jpg" || ext === "jpeg") contentType = "image/jpeg";
          else if (ext === "png") contentType = "image/png";
          else if (ext === "webp") contentType = "image/webp";
          else if (ext === "gif") contentType = "image/gif";
          else if (ext === "bmp") contentType = "image/bmp";
          else if (ext === "svg") contentType = "image/svg+xml";
          else contentType = "image/png";
        }

        const r = await fetch("/api/terminal/upload", {
          method: "POST",
          headers: { "Content-Type": contentType },
          body: file,
        });
        if (!r.ok) {
          const errText = await r.text();
          throw new Error(errText || `HTTP ${r.status}`);
        }
        const { path } = await r.json();
        const safePath = path.includes(" ") ? `"${path}"` : path;
        // bracketed paste (when the app enabled it): Claude/Codex only attach an image path that arrives as a paste
        term.paste(`${safePath} `);
        setNotice(null);
        term.focus();
      } catch (err) {
        showNotice("error", `Không dán được ảnh: ${String(err).trim()}`, 6000);
      }
    };

    // paste: text goes through term.paste (bracketed paste, so editors/agents get it as one block);
    // an image is uploaded and its file path is typed into terminal, which agent CLIs accept as an attachment
    const onPaste = async (e: ClipboardEvent) => {
      // keyboard intent expires quickly; a context-menu paste has none and takes whatever the clipboard holds
      const intent = pasteIntent && Date.now() - pasteIntent.at < 1000 ? pasteIntent.kind : null;
      pasteIntent = null;
      // 1. Detect image file from files list first (file manager copy or direct image paste)
      let imgFile: File | null = null;
      const files = Array.from(e.clipboardData?.files ?? []);
      for (const f of files) {
        if (f.type.startsWith("image/") || /\.(png|jpe?g|gif|webp|bmp|svg|avif|ico)$/i.test(f.name)) {
          imgFile = f;
          break;
        }
      }

      // 2. If not found in files, check items (browser copy image / screenshot)
      if (!imgFile && e.clipboardData?.items) {
        const items = Array.from(e.clipboardData.items);
        for (const item of items) {
          if (item.kind === "file") {
            const f = item.getAsFile();
            if (f && (f.type.startsWith("image/") || /\.(png|jpe?g|gif|webp|bmp|svg|avif|ico)$/i.test(f.name))) {
              imgFile = f;
              break;
            }
          }
        }
      }

      const text = e.clipboardData?.getData("text/plain");
      // a screenshot has no text, so Cmd+V pastes it as an image too; with text present Cmd+V stays text
      if (imgFile && (intent !== "text" || !text)) {
        e.preventDefault();
        e.stopPropagation();
        await uploadAndPasteImage(imgFile);
        return;
      }

      if (text) {
        e.preventDefault();
        e.stopPropagation();
        term.paste(text);
      }
    };

    const onDragOver = (e: DragEvent) => {
      e.preventDefault();
      e.stopPropagation();
    };

    const onDrop = async (e: DragEvent) => {
      e.preventDefault();
      e.stopPropagation();
      const files = Array.from(e.dataTransfer?.files ?? []);
      const imgFile = files.find(
        (f) => f.type.startsWith("image/") || /\.(png|jpe?g|gif|webp|bmp|svg|avif|ico)$/i.test(f.name)
      );
      if (imgFile) {
        await uploadAndPasteImage(imgFile);
      }
    };

    // Touch & wheel scrolling:
    // Both wheel and touch events send SGR WheelUp (64) / WheelDown (65), which tmux processes
    // in 3-line chunks in a single redraw (and automatically exits copy-mode when reaching the bottom).
    // Events are throttled to ~32ms (~30fps) to keep the PTY buffer responsive and prevent scroll lag.
    let touchY = 0;
    let touching = false;
    let touchAccY = 0;
    let touchLastDispatch = 0;
    let cell = { col: 1, row: 1 };
    let frame = 0;
    let velocity = 0; // rows per frame while coasting
    let samples: { t: number; y: number }[] = [];
    let wheelClearTimer: number | undefined;
    const rowPx = () => host.getBoundingClientRect().height / term.rows || 16;

    const flushMomentum = () => {
      frame = 0;
      if (touching || Math.abs(velocity) < 0.25) {
        velocity = 0;
        return;
      }
      const isUp = velocity > 0;
      sendRaw(`\x1b[<${isUp ? 64 : 65};${cell.col};${cell.row}M`);
      if (isUp) setScrolledUp(true);
      velocity *= 0.78; // Quick decay to settle within 150-200ms
      if (Math.abs(velocity) >= 0.25) {
        frame = requestAnimationFrame(flushMomentum);
      } else {
        velocity = 0;
      }
    };

    const onTouchStart = (e: TouchEvent) => {
      if (frame) {
        cancelAnimationFrame(frame);
        frame = 0;
      }
      velocity = 0;
      touchAccY = 0;
      touching = e.touches.length === 1;
      if (touching) {
        touchY = e.touches[0].clientY;
        samples = [{ t: performance.now(), y: touchY }];
      }
    };

    const onTouchMove = (e: TouchEvent) => {
      if (!touching || e.touches.length !== 1 || term.modes.mouseTrackingMode === "none") return;
      e.preventDefault();
      const t = e.touches[0];
      const deltaY = t.clientY - touchY;
      touchY = t.clientY;
      touchAccY += deltaY;

      const rect = host.getBoundingClientRect();
      cell = {
        col: Math.min(term.cols, Math.max(1, Math.floor(((t.clientX - rect.left) / rect.width) * term.cols) + 1)),
        row: Math.min(term.rows, Math.max(1, Math.floor(((t.clientY - rect.top) / rect.height) * term.rows) + 1)),
      };

      const now = performance.now();
      samples.push({ t: now, y: t.clientY });
      samples = samples.filter((s) => now - s.t < 100);

      const threshold = rowPx() * 1.5;
      if (Math.abs(touchAccY) >= threshold && now - touchLastDispatch >= 32) {
        const isUp = touchAccY > 0;
        sendRaw(`\x1b[<${isUp ? 64 : 65};${cell.col};${cell.row}M`);
        if (isUp) setScrolledUp(true);
        touchLastDispatch = now;
        touchAccY = touchAccY > 0 ? Math.max(0, touchAccY - threshold) : Math.min(0, touchAccY + threshold);
      }
    };

    const onTouchEnd = () => {
      if (!touching) return;
      touching = false;
      const last = samples[samples.length - 1];
      const first = samples[0];
      if (last && first && last.t - first.t > 15 && performance.now() - last.t < 80) {
        const pyPerMs = (last.y - first.y) / (last.t - first.t);
        velocity = (pyPerMs * 16) / rowPx();
        if (velocity > 3) velocity = 3;
        if (velocity < -3) velocity = -3;
      }
      if (Math.abs(velocity) >= 0.5) {
        frame = requestAnimationFrame(flushMomentum);
      } else {
        velocity = 0;
      }
    };

    // Wheel & trackpad scrolling:
    let wheelAcc = 0;
    let wheelLastTime = 0;
    term.attachCustomWheelEventHandler((e) => {
      if (term.modes.mouseTrackingMode === "none" || e.ctrlKey || e.metaKey) return true; // browser zoom
      e.preventDefault();

      const rect = host.getBoundingClientRect();
      const col = Math.min(term.cols, Math.max(1, Math.floor(((e.clientX - rect.left) / rect.width) * term.cols) + 1));
      const row = Math.min(term.rows, Math.max(1, Math.floor(((e.clientY - rect.top) / rect.height) * term.rows) + 1));

      const lineH = rowPx();
      const delta = e.deltaMode === 1 ? e.deltaY * lineH : e.deltaMode === 2 ? e.deltaY * host.clientHeight : e.deltaY;
      wheelAcc += delta;

      window.clearTimeout(wheelClearTimer);
      wheelClearTimer = window.setTimeout(() => {
        wheelAcc = 0;
      }, 100);

      const now = performance.now();
      const threshold = lineH * 1.5;

      if (Math.abs(wheelAcc) >= threshold && now - wheelLastTime >= 32) {
        const isUp = wheelAcc < 0;
        sendRaw(`\x1b[<${isUp ? 64 : 65};${col};${row}M`);
        if (isUp) setScrolledUp(true);
        wheelLastTime = now;
        wheelAcc = wheelAcc > 0 ? Math.max(0, wheelAcc - threshold) : Math.min(0, wheelAcc + threshold);
      }
      return false;
    });

    host.addEventListener("touchstart", onTouchStart, { passive: true });
    host.addEventListener("touchmove", onTouchMove, { passive: false });
    host.addEventListener("touchend", onTouchEnd, { passive: true });
    host.addEventListener("touchcancel", onTouchEnd, { passive: true });

    const closeKeyboard = (e: PointerEvent) => {
      const input = composeRef.current;
      if (touch && input && document.activeElement === input && !bottomBarRef.current?.contains(e.target as Node)) input.blur();
    };
    document.addEventListener("pointerdown", closeKeyboard);

    const onContextMenu = (e: MouseEvent) => {
      if (term.hasSelection()) {
        e.preventDefault();
        copyText(term.getSelection());
      }
    };
    host.addEventListener("contextmenu", onContextMenu);

    host.addEventListener("paste", onPaste, true);
    host.addEventListener("dragover", onDragOver, false);
    host.addEventListener("drop", onDrop, false);

    const ro = new ResizeObserver(() => fit());
    ro.observe(host);
    fit();
    connect();

    return () => {
      cancelAnimationFrame(frame);
      window.clearTimeout(wheelClearTimer);
      host.removeEventListener("touchstart", onTouchStart);
      host.removeEventListener("touchmove", onTouchMove);
      host.removeEventListener("touchend", onTouchEnd);
      host.removeEventListener("touchcancel", onTouchEnd);
      document.removeEventListener("pointerdown", closeKeyboard);
      window.removeEventListener("bridge-terminal-settings-changed", onSettingsChanged);
      host.removeEventListener("contextmenu", onContextMenu);
      host.removeEventListener("paste", onPaste, true);
      host.removeEventListener("dragover", onDragOver, false);
      host.removeEventListener("drop", onDrop, false);
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
        termRef.current?.scrollToBottom();
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
        <div ref={hostRef} className="absolute inset-0 px-2 py-1 overflow-hidden [contain:strict] [will-change:transform]" onClick={() => { if (!touch) termRef.current?.focus(); }} />
        {notice && (
          <div
            className={`pointer-events-none absolute right-3 top-3 z-10 flex items-center gap-2 rounded-md border px-2.5 py-1.5 text-[11.5px] shadow-lg ${
              notice.kind === "error" ? "border-rose-500/40 bg-[#2a1519]/95 text-rose-200" : "border-sky-500/40 bg-[#10202b]/95 text-sky-200"
            }`}
          >
            {notice.kind === "info" && <span className="h-3 w-3 animate-spin rounded-full border-2 border-sky-400 border-t-transparent" />}
            {notice.text}
          </div>
        )}
        {scrolledUp && (
          <button
            type="button"
            onClick={scrollToBottom}
            className="absolute bottom-3 right-3 z-20 flex items-center gap-1.5 rounded-full bg-indigo-600/90 hover:bg-indigo-500 text-white px-3 py-1.5 text-[11.5px] font-medium shadow-lg backdrop-blur cursor-pointer transition-all active:scale-95 border border-indigo-400/30"
          >
            <ArrowDown className="w-3.5 h-3.5" />
            <span>Xuống đáy</span>
          </button>
        )}
        {status === "closed" && (
          <div className="absolute inset-x-0 bottom-0 flex items-center justify-between gap-3 border-t border-rose-900/50 bg-[#1c1416]/95 px-3 py-2 text-[11.5px] text-rose-200">
            <span>{launch ? "Agent session ended" : "Shell disconnected"}{reason ? ` — ${reason}` : ""}</span>
            <button type="button" onClick={connect} className="rounded bg-rose-700/70 px-2.5 py-1 text-white hover:bg-rose-600 cursor-pointer">Reconnect</button>
          </div>
        )}
      </div>
      {touch && (
        <div ref={bottomBarRef} className="flex shrink-0 flex-col bg-[#0a0c10] px-2 pt-1 pb-[max(0.4rem,env(safe-area-inset-bottom))]">
          {/* Virtual terminal keys - collapsible */}
          {showKeys && (
            <div className="flex shrink-0 items-center gap-1 overflow-x-auto py-1 mb-1.5 scrollbar-none">
              {(
                [
                  ["Esc", "\x1b"],
                  ["Tab", "\t"],
                  ["Ctrl", null],
                  ["↑", "\x1b[A"],
                  ["↓", "\x1b[B"],
                  ["←", "\x1b[D"],
                  ["→", "\x1b[C"],
                  ["^C", "\x03"],
                  ["/", "/"],
                  ["|", "|"],
                  ["~", "~"],
                  ["-", "-"],
                ] as [string, string | null][]
              ).map(([label, seq]) => (
                <button
                  key={label}
                  type="button"
                  onPointerDown={(e) => e.preventDefault()}
                  onClick={() => {
                    if (seq === null) {
                      ctrlRef.current = !ctrlRef.current;
                      setCtrl(ctrlRef.current);
                    } else sendRaw(seq);
                    if (document.activeElement !== composeRef.current) termRef.current?.focus();
                  }}
                  className={`h-7 min-w-[34px] px-2 shrink-0 rounded-md border text-[11.5px] font-mono transition-colors ${
                    label === "Ctrl" && ctrl
                      ? "border-indigo-500 bg-indigo-600 text-white"
                      : "border-white/5 bg-[#161a24] text-slate-300 hover:bg-[#202636] active:bg-indigo-600 active:text-white"
                  }`}
                >
                  {label}
                </button>
              ))}
            </div>
          )}

          {/* Antigravity-style floating input card */}
          <div className="flex flex-col rounded-2xl border border-white/10 bg-[#141722] p-2 shadow-xl focus-within:border-indigo-500/60 focus-within:ring-1 focus-within:ring-indigo-500/30 transition-all">
            <input
              ref={composeRef}
              value={compose}
              onChange={(e) => setCompose(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "ArrowUp") {
                  const h = historyRef.current;
                  if (h.length > 0) {
                    e.preventDefault();
                    const nextIdx = historyIdxRef.current === -1 ? h.length - 1 : Math.max(0, historyIdxRef.current - 1);
                    historyIdxRef.current = nextIdx;
                    setCompose(h[nextIdx]);
                  }
                  return;
                }
                if (e.key === "ArrowDown") {
                  const h = historyRef.current;
                  if (h.length > 0 && historyIdxRef.current !== -1) {
                    e.preventDefault();
                    const nextIdx = historyIdxRef.current + 1;
                    if (nextIdx >= h.length) {
                      historyIdxRef.current = -1;
                      setCompose("");
                    } else {
                      historyIdxRef.current = nextIdx;
                      setCompose(h[nextIdx]);
                    }
                  }
                  return;
                }
                if (e.key === "Enter" && !e.nativeEvent.isComposing && e.keyCode !== 229) {
                  e.preventDefault();
                  sendCompose();
                }
              }}
              enterKeyHint="send"
              autoCapitalize="off"
              autoCorrect="off"
              spellCheck={false}
              placeholder="Nhập tin nhắn hoặc lệnh shell..."
              className="w-full bg-transparent px-1 py-1 text-[13.5px] text-slate-100 placeholder:text-slate-500 outline-none"
            />

            <div className="flex items-center justify-between pt-1 mt-0.5 border-t border-white/5">
              <div className="flex items-center gap-1.5">
                <button
                  type="button"
                  onClick={toggleKeys}
                  className={`flex h-7 items-center gap-1 rounded-lg px-2 text-[11px] font-medium transition-colors cursor-pointer ${
                    showKeys
                      ? "bg-indigo-600/25 text-indigo-300 border border-indigo-500/40"
                      : "bg-white/[0.04] text-slate-400 hover:text-slate-200 border border-transparent"
                  }`}
                  title="Bật/tắt phím terminal (Esc, Tab, Ctrl, mũi tên)"
                >
                  <Keyboard className="h-3.5 w-3.5" />
                  <span>Keys</span>
                </button>
                {compose && (
                  <button
                    type="button"
                    onClick={() => setCompose("")}
                    className="flex h-7 w-7 items-center justify-center rounded-lg bg-white/[0.04] text-slate-400 hover:text-rose-400 transition-colors cursor-pointer"
                    title="Xóa nội dung"
                  >
                    <X className="h-3 w-3" />
                  </button>
                )}
              </div>

              <button
                type="button"
                onPointerDown={(e) => e.preventDefault()}
                onClick={sendCompose}
                className="flex h-7 w-7 items-center justify-center rounded-full bg-indigo-600 text-white hover:bg-indigo-500 active:scale-90 transition-all cursor-pointer shadow-md shadow-indigo-600/20"
                title="Gửi (Enter)"
              >
                <ArrowUp className="h-3.5 w-3.5" />
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
