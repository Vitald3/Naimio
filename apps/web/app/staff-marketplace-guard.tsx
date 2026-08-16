"use client";

import type { ReactNode } from "react";
import { useEffect } from "react";
import { usePathname } from "next/navigation";
import { useAuth } from "./auth-state";
import { STAFF_BASE_PATH, isStaffRoles } from "./admin-path";

const MARKETPLACE_PRIVATE_PREFIXES = [
  "/dashboard", "/favorites", "/messages", "/notifications", "/settings", "/create-project"
];

export default function StaffMarketplaceGuard({ children }: { children: ReactNode }) {
  const pathname = usePathname() || "";
  const { state, user } = useAuth();
  const staff = isStaffRoles(user?.roles);
  const privateMarketplaceRoute = MARKETPLACE_PRIVATE_PREFIXES.some((prefix) => pathname === prefix || pathname.startsWith(prefix + "/"));
  useEffect(() => {
    if (state === "authenticated" && staff && privateMarketplaceRoute) location.replace(STAFF_BASE_PATH);
  }, [state, staff, privateMarketplaceRoute]);
  if (state === "authenticated" && staff && privateMarketplaceRoute) return <main><section className="empty"><h1>Переход в служебную зону…</h1></section></main>;
  return <>{children}</>;
}
