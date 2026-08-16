"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react";
import type { SessionUser } from "../auth-state";

type AdminAuthValue = {
  state: "loading" | "anonymous" | "authenticated";
  user: SessionUser | null;
  refresh: () => Promise<void>;
  logout: () => Promise<void>;
};

const AdminAuthContext = createContext<AdminAuthValue>({
  state: "loading",
  user: null,
  refresh: async () => undefined,
  logout: async () => undefined,
});

export function AdminAuthProvider({ children }: { children: React.ReactNode }) {
  const [state, setState] = useState<AdminAuthValue["state"]>("loading");
  const [user, setUser] = useState<SessionUser | null>(null);

  const refresh = useCallback(async () => {
    setState("loading");
    try {
      const response = await fetch("/api/v1/auth/admin-session", {
        credentials: "same-origin",
        cache: "no-store",
      });
      if (!response.ok) {
        setUser(null);
        setState("anonymous");
        return;
      }
      const body = await response.json();
      if (!body?.data) {
        setUser(null);
        setState("anonymous");
        return;
      }
      setUser(body.data);
      setState("authenticated");
    } catch {
      setUser(null);
      setState("anonymous");
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const logout = useCallback(async () => {
    setUser(null);
    setState("anonymous");
    await fetch("/api/v1/auth/admin-logout", {
      method: "POST",
      credentials: "same-origin",
    }).catch(() => undefined);
  }, []);

  const value = useMemo(
    () => ({ state, user, refresh, logout }),
    [state, user, refresh, logout],
  );

  return (
    <AdminAuthContext.Provider value={value}>
      {children}
    </AdminAuthContext.Provider>
  );
}

export const useAdminAuth = () => useContext(AdminAuthContext);
