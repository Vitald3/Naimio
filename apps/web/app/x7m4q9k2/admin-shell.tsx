"use client";

import type { ReactNode } from "react";
import { usePathname } from "next/navigation";
import { STAFF_LOGIN_PATH } from "../admin-path";
import { AdminAuthProvider } from "./admin-auth";
import { AdminGuard } from "./admin-guard";
import AdminNav from "./admin-nav";

export default function AdminShell({ children }: { children: ReactNode }) {
  const pathname = usePathname() || "";
  if (pathname === STAFF_LOGIN_PATH) return <>{children}</>;
  return (
    <AdminAuthProvider>
      <AdminGuard>
        <main className="admin-shell">
          <AdminNav />
          <section className="admin-content">{children}</section>
        </main>
      </AdminGuard>
    </AdminAuthProvider>
  );
}
