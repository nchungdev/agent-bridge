import { useCallback, useEffect, useRef, useState } from "react";
import type { HubEvent, Snapshot } from "./types";

type Msg =
  | { type: "snapshot"; conv: string; snapshot: Snapshot; events: HubEvent[]; has_more?: boolean; full?: boolean }
  | { type: "event"; event: HubEvent }
  | { type: "conv_created"; conv: string }
  | { type: "error"; message: string };

/** One WebSocket to /ws/v2 with auto-reconnect; resubscribes (and resyncs from the store) after a drop. */
export function useHub(conv: string | null, onCreated: (id: string) => void) {
  const [events, setEvents] = useState<HubEvent[]>([]);
  const [snapshot, setSnapshot] = useState<Snapshot | null>(null);
  const [connected, setConnected] = useState(false);
  const [hasMore, setHasMore] = useState(false);
  const [loadingEarlier, setLoadingEarlier] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const convRef = useRef(conv);
  const lastSeq = useRef(0);
  const createdRef = useRef<string | null>(null);
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
          const incoming = d.events ?? [];
          if (d.full) setHasMore(!!d.has_more); // a catch-up after reconnect says nothing about older history
          setSnapshot(d.snapshot);
          setEvents((prev) => {
            const have = new Set(prev.map((e) => e.seq));
            const add = incoming.filter((e) => !have.has(e.seq));
            return [...prev, ...add];
          });
          for (const e of incoming) if ((e.seq ?? 0) > lastSeq.current) lastSeq.current = e.seq ?? 0;
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
          // the server already subscribed us; keep what arrives while React switches conversation
          convRef.current = d.conv;
          createdRef.current = d.conv;
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
    if (conv && conv === createdRef.current) {
      createdRef.current = null;
      send({ type: "subscribe", conv, since: lastSeq.current });
      return;
    }
    setEvents([]);
    setHasMore(false);
    setSnapshot(null);
    setError(null);
    lastSeq.current = 0;
    if (conv) send({ type: "subscribe", conv, since: 0 });
  }, [conv, send]);

  // page backwards: the events just older than the oldest one we hold
  const loadEarlier = useCallback(async () => {
    const c = convRef.current;
    if (!c || loadingEarlier) return;
    setLoadingEarlier(true);
    try {
      let min = Infinity;
      setEvents((cur) => { for (const e of cur) if ((e.seq ?? 0) > 0 && (e.seq as number) < min) min = e.seq as number; return cur; });
      await Promise.resolve();
      if (!Number.isFinite(min)) return;
      const r = await fetch(`/api/v2/conv/${c}/events?before=${min}`).then((x) => x.json());
      const older: HubEvent[] = r.events ?? [];
      setEvents((cur) => {
        const have = new Set(cur.map((e) => e.seq));
        return [...older.filter((e) => !have.has(e.seq)), ...cur];
      });
      setHasMore(!!r.has_more);
    } catch {
      setError("Could not load earlier messages");
    } finally {
      setLoadingEarlier(false);
    }
  }, [loadingEarlier]);

  return { events, snapshot, connected, error, clearError: () => setError(null), send, hasMore, loadingEarlier, loadEarlier };
}
