import type { Metadata } from "next";
import type { ReactNode } from "react";
import AdminShell from "./admin-shell";

export const metadata: Metadata = {
  title: "Naimio Control Center",
  robots: { index: false, follow: false, nocache: true },
};

export default function Layout({ children }: { children: ReactNode }) {
  return <AdminShell>{children}</AdminShell>;
}
