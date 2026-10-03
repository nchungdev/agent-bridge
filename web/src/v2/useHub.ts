import { useCallback, useEffect, useRef, useState } from "react";
import type { HubEvent, Snapshot } from "./types";

type Msg =
  | { type: "snapshot"; conv: string; snapshot: Snapshot; events: HubEvent[] }
  | { type: "event"; event: HubEvent }
  | { type: "conv_created"; conv: string }
  | { type: "error"; message: string };

/** One WebSocket to /ws/v2 with auto-reconnect; resubscribes (and resyncs from the store) after a drop. */
export function useHub(conv: string | null, onCreated: (id: string) => void) {
  const [events, setEvents] = useState<HubEvent[]>([]);
  const [snapshot, setSnapshot] = useState<Snapshot | null>(null);
  const [connected, setConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const convRef = useRef(conv);
  const lastSeq = useRef(0);
  const onCreatedRef = useRef(onCreated);
  onCreatedRef.current = onCreated;
  convRef.current = conv;

  const send = useCallback((o: object) => {
    const ws = wsRef.current;
    if (ws && ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify(o));
  }, []);

  useEffect(() => {
    let closed = false;
    let timer: number | undefined;
    const connect = () => {
      const proto = location.protocol === "https:" ? "wss" : "ws";
      const ws = new WebSocket(`${proto}://${location.host}/ws/v2`);
      wsRef.current = ws;
      ws.onopen = () => {
        setConnected(true);
        if (convRef.current) ws.send(JSON.stringify({ type: "subscribe", conv: convRef.current, since: lastSeq.current }));
      };
      ws.onmessage = (m) => {
        const d = JSON.parse(m.data) as Msg;
        if (d.type === "snapshot") {
          setSnapshot(d.snapshot);
          setEvents((prev) => {
            const have = new Set(prev.map((e) => e.seq));
            const add = d.events.filter((e) => !have.has(e.seq));
            return [...prev, ...add];
          });
          for (const e of d.events) if ((e.seq ?? 0) > lastSeq.current) lastSeq.current = e.seq ?? 0;
        } else if (d.type === "event") {
          const e = d.event;
          if (e.conv_id !== convRef.current) return;
          if (e.type === "state_change") {
            setSnapshot((s) => (s ? { ...s, live: { ...s.live, [e.engine ?? ""]: String(e.data?.state ?? "") } } : s));
            return;
          }
          if (e.seq) lastSeq.current = Math.max(lastSeq.current, e.seq);
          setEvents((prev) => [...prev, e]);
        } else if (d.type === "conv_created") {
          onCreatedRef.current(d.conv);
        } else if (d.type === "error") {
          setError(d.message);
        }
      };
      ws.onclose = () => {
        setConnected(false);
        if (!closed) timer = window.setTimeout(connect, 1500);
      };
    };
    connect();
    return () => {
      closed = true;
      window.clearTimeout(timer);
      wsRef.current?.close();
    };
  }, []);

  // switching conversation: reset local view and subscribe
  useEffect(() => {
    setEvents([]);
    setSnapshot(null);
    setError(null);
    lastSeq.current = 0;
    if (conv) send({ type: "subscribe", conv, since: 0 });
  }, [conv, send]);

  return { events, snapshot, connected, error, clearError: () => setError(null), send };
}
