"use client";

import type { ReactNode } from "react";
import { useEffect } from "react";
import { useAuth } from "./auth-state";
import { AuthBootstrapLoader } from "./auth-loader";

export default function AuthGuard({ children }: { children: ReactNode }) {
  const { state } = useAuth();

  useEffect(() => {
    if (state !== "anonymous") return;
    const next = `${location.pathname}${location.search}`;
    location.replace(`/login?next=${encodeURIComponent(next)}`);
  }, [state]);

  if (state !== "authenticated") {
    return <AuthBootstrapLoader />;
  }
  return children;
}

