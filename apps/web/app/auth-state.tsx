"use client";

import { usePathname } from "next/navigation";
import { STAFF_BASE_PATH } from "./admin-path";
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react";

export type SessionUser = {
  id: string;
  email: string;
  display_name: string;
  username?: string;
  email_verified?: boolean;
  avatar_media_object_id?: string;
  gender?: "MALE" | "FEMALE" | "UNSPECIFIED";
  roles: string[];
  capabilities: string[];
};
type AuthValue = {
  state: "loading" | "anonymous" | "authenticated";
  user: SessionUser | null;
  refresh: () => Promise<void>;
  logout: () => Promise<void>;
};
const AuthContext = createContext<AuthValue>({
  state: "loading",
  user: null,
  refresh: async () => undefined,
  logout: async () => undefined,
});

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const pathname = usePathname() || "";
  const staffPath = pathname === STAFF_BASE_PATH || pathname.startsWith(STAFF_BASE_PATH + "/");
  const [state, setState] = useState<AuthValue["state"]>(staffPath ? "anonymous" : "loading");
  const [user, setUser] = useState<SessionUser | null>(null);
  const refresh = useCallback(async () => {
    setState("loading");
    try {
      const response = await fetch("/api/v1/auth/session", {
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
    if (staffPath) {
      setUser(null);
      setState("anonymous");
      return;
    }
    void refresh();
  }, [refresh, staffPath]);
  useEffect(() => {
    if (staffPath || state !== "authenticated") return;
    const heartbeat = () => fetch("/api/v1/presence/heartbeat", { method: "POST", credentials: "same-origin" }).catch(() => undefined);
    void heartbeat();
    const timer = window.setInterval(heartbeat, 45000);
    const onVisibility = () => { if (document.visibilityState === "visible") void heartbeat(); };
    document.addEventListener("visibilitychange", onVisibility);
    return () => { window.clearInterval(timer); document.removeEventListener("visibilitychange", onVisibility); };
  }, [staffPath, state]);
  const logout = useCallback(async () => {
    // Clear client-visible identity immediately so a slow network cannot make
    // the first click appear to do nothing. The request is keepalive-enabled so
    // the server can revoke the session and expire the cookie during navigation.
    setUser(null);
    setState("anonymous");
    await fetch("/api/v1/auth/logout", {
      method: "POST",
      credentials: "same-origin",
      keepalive: true,
      cache: "no-store",
      signal: AbortSignal.timeout(900),
    }).catch(() => undefined);
    window.dispatchEvent(new Event("private-cache-clear"));
    try { window.sessionStorage.clear(); } catch {}
    window.location.replace("/");
  }, []);
  const value = useMemo(
    () => ({ state, user, refresh, logout }),
    [state, user, refresh, logout],
  );
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}
export const useAuth = () => useContext(AuthContext);
