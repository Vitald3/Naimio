"use client";

import type { ReactNode } from "react";
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import { createRandomID } from "./random-id";

type ToastKind = "success" | "error" | "warning" | "info";

type ToastItem = {
  id: string;
  kind: ToastKind;
  title: string;
  message?: string;
};

type ToastContextValue = {
  push: (input: Omit<ToastItem, "id"> & { timeoutMs?: number }) => void;
};

const ToastContext = createContext<ToastContextValue | null>(null);

export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([]);
  const timers = useRef<Record<string, ReturnType<typeof setTimeout>>>({});

  const remove = useCallback((id: string) => {
    const handle = timers.current[id];
    if (handle) {
      clearTimeout(handle);
      delete timers.current[id];
    }
    setItems((current) => current.filter((item) => item.id !== id));
  }, []);

  const push = useCallback(
    ({ timeoutMs = 3600, ...input }: Omit<ToastItem, "id"> & { timeoutMs?: number }) => {
      const id = createRandomID();
      setItems((current) => [...current, { id, ...input }]);
      timers.current[id] = setTimeout(() => remove(id), timeoutMs);
    },
    [remove],
  );

  useEffect(() => {
    const handler = (event: Event) => {
      const detail = (event as CustomEvent<{ message: string; tone?: ToastKind }>).detail;
      if (detail?.message) push({ kind: detail.tone || "success", title: detail.message });
    };
    window.addEventListener("naimio:toast", handler);
    return () => window.removeEventListener("naimio:toast", handler);
  }, [push]);

  const value = useMemo(() => ({ push }), [push]);

  return (
    <ToastContext.Provider value={value}>
      {children}
      <div className="toast-stack" aria-live="polite" aria-atomic="true">
        {items.map((item) => (
          <div key={item.id} className={`toast toast--${item.kind}`} role={item.kind === "error" ? "alert" : "status"}>
            <span className="toast__icon" aria-hidden="true">{item.kind === "success" ? "✓" : item.kind === "error" ? "!" : item.kind === "warning" ? "!" : "i"}</span>
            <div className="toast__content">
              <strong>{item.title}</strong>
              {item.message ? <p>{item.message}</p> : null}
            </div>
            <button type="button" className="toast__close" aria-label="Закрыть уведомление" onClick={() => remove(item.id)}>
              ×
            </button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast() {
  const context = useContext(ToastContext);
  if (!context) throw new Error("useToast must be used inside ToastProvider");
  return context;
}

export function notify(message: string, tone: ToastKind = "success") {
  if (typeof window !== "undefined") window.dispatchEvent(new CustomEvent("naimio:toast", { detail: { message, tone } }));
}
