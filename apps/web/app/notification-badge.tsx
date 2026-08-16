"use client";
import { useEffect, useRef, useState } from "react";

type Preview = { id: string; type: string; read_at?: string };

function playNotificationTone() {
  const AudioContextClass = window.AudioContext || (window as typeof window & { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
  if (!AudioContextClass) return;
  try {
    const context = new AudioContextClass();
    const oscillator = context.createOscillator();
    const gain = context.createGain();
    oscillator.type = "sine";
    oscillator.frequency.setValueAtTime(740, context.currentTime);
    oscillator.frequency.exponentialRampToValueAtTime(980, context.currentTime + 0.09);
    gain.gain.setValueAtTime(0.0001, context.currentTime);
    gain.gain.exponentialRampToValueAtTime(0.11, context.currentTime + 0.015);
    gain.gain.exponentialRampToValueAtTime(0.0001, context.currentTime + 0.18);
    oscillator.connect(gain); gain.connect(context.destination);
    oscillator.start(); oscillator.stop(context.currentTime + 0.19);
    oscillator.onended = () => void context.close();
  } catch { /* browser autoplay/security policy: notification remains visual */ }
}

// Initial state is loaded once over HTTP. Every later notification arrives over WebSocket;
// there is no notification polling loop.
export function useUnreadCount(enabled: boolean): number {
  const [items, setItems] = useState<Preview[]>([]);
  const initialized = useRef(false);
  const previousUnread = useRef<Set<string>>(new Set());
  const audioArmed = useRef(false);
  useEffect(() => {
    const arm = () => { audioArmed.current = true; };
    window.addEventListener("pointerdown", arm, { once: true });
    window.addEventListener("keydown", arm, { once: true });
    return () => { window.removeEventListener("pointerdown", arm); window.removeEventListener("keydown", arm); };
  }, []);
  useEffect(() => {
    if (!enabled) { setItems([]); initialized.current = false; previousUnread.current = new Set(); return; }
    let disposed = false;
    let reconnectTimer: number | undefined;
    let socket: WebSocket | null = null;
    const load = async (sound = false) => {
      try {
        const response = await fetch("/api/v1/notifications?limit=20", { credentials: "same-origin", cache: "no-store" });
        if (!response.ok || disposed) return;
        const body = await response.json();
        const next: Preview[] = body?.data ?? [];
        const unread = new Set(next.filter((item) => !item.read_at).map((item) => item.id));
        if (sound && initialized.current && audioArmed.current && Array.from(unread).some((id) => !previousUnread.current.has(id))) playNotificationTone();
        previousUnread.current = unread; initialized.current = true; setItems(next);
      } catch { }
    };
    const connect = () => {
      if (disposed) return;
      const protocol = location.protocol === "https:" ? "wss" : "ws";
      socket = new WebSocket(process.env.NEXT_PUBLIC_WS_URL || `${protocol}://${location.host}/api/v1/ws`);
      socket.onmessage = (event) => {
        try {
          const envelope = JSON.parse(event.data);
          if (envelope.event !== "notification.created" || !envelope.data?.id) return;
          const incoming = envelope.data as Preview;
          setItems((current) => {
            if (current.some((item) => item.id === incoming.id)) return current;
            if (initialized.current && audioArmed.current && !incoming.read_at) playNotificationTone();
            previousUnread.current = new Set([incoming.id, ...Array.from(previousUnread.current)]);
            return [incoming, ...current].slice(0, 20);
          });
        } catch { }
      };
      socket.onclose = () => { if (!disposed) reconnectTimer = window.setTimeout(connect, 1500); };
      socket.onerror = () => socket?.close();
    };
    void load(false);
    connect();
    const clear = () => { setItems([]); initialized.current = false; previousUnread.current = new Set(); };
    window.addEventListener("private-cache-clear", clear);
    const readOne = (event: Event) => { const id=(event as CustomEvent<{id?:string}>).detail?.id; if(id) setItems(current=>current.map(item=>item.id===id?{...item,read_at:new Date().toISOString()}:item)); };
    const readAll = () => setItems(current=>current.map(item=>({...item,read_at:item.read_at||new Date().toISOString()})));
    window.addEventListener("notification-read", readOne);
    window.addEventListener("notifications-read-all", readAll);
    return () => { disposed = true; if (reconnectTimer !== undefined) window.clearTimeout(reconnectTimer); socket?.close(); window.removeEventListener("private-cache-clear", clear); window.removeEventListener("notification-read", readOne); window.removeEventListener("notifications-read-all", readAll); };
  }, [enabled]);
  return items.filter((n) => !n.read_at).length;
}
