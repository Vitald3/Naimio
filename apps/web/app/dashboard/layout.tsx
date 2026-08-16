import type { Metadata } from "next";
import type { ReactNode } from "react";
import { DashboardNav } from "./dashboard-nav";
import StaffDashboardGuard from "./staff-dashboard-guard";
import DashboardRoleGuard from "./dashboard-role-guard";
export const metadata: Metadata = { robots: { index: false, follow: false } };
export default function Layout({ children }: { children: ReactNode }) {
  return <StaffDashboardGuard><div className="dashboard-shell"><DashboardNav/><div className="dashboard-content"><DashboardRoleGuard>{children}</DashboardRoleGuard></div></div></StaffDashboardGuard>;
}
