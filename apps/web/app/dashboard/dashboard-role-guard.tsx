"use client";

import type { ReactNode } from "react";
import { useEffect } from "react";
import { usePathname } from "next/navigation";
import { useAuth } from "../auth-state";
import { AuthBootstrapLoader } from "../auth-loader";

const customerPaths = ["/dashboard/projects", "/dashboard/vacancies", "/dashboard/team"];
const freelancerPaths = ["/dashboard/proposals", "/dashboard/job-applications", "/dashboard/services", "/dashboard/portfolio", "/dashboard/analytics"];

export default function DashboardRoleGuard({ children }: { children: ReactNode }) {
  const pathname = usePathname() || "";
  const { state, user } = useAuth();
  const required = customerPaths.some(path => pathname === path || pathname.startsWith(path + "/")) ? "CUSTOMER" : freelancerPaths.some(path => pathname === path || pathname.startsWith(path + "/")) ? "FREELANCER" : null;
  useEffect(() => {
    if (state === "anonymous") window.location.replace(`/login?next=${encodeURIComponent(pathname || "/dashboard")}`);
  }, [pathname, state]);
  if (state === "loading" || state === "anonymous") return <AuthBootstrapLoader as="div" />;
  if (required && !user?.capabilities?.includes(required)) {
    const customer = required === "CUSTOMER";
    return <main><div className="role-boundary"><p className="eyebrow">Другой режим</p><h1>{customer ? "Раздел заказчика" : "Раздел исполнителя"}</h1><p>{customer ? "Создание проектов и вакансий доступно только в режиме заказчика." : "Услуги, портфолио и отклики доступны только в режиме исполнителя."}</p><a className="button" href="/settings/account">Настроить режимы аккаунта</a></div></main>;
  }
  return children;
}
