"use client";

import type { ReactNode } from "react";
import { useEffect } from "react";
import { useAuth } from "../auth-state";
import { STAFF_BASE_PATH, isStaffRoles } from "../admin-path";
import { AuthBootstrapLoader } from "../auth-loader";

export default function StaffDashboardGuard({children}:{children:ReactNode}){
  const {state,user}=useAuth();
  const staff=isStaffRoles(user?.roles);
  useEffect(()=>{
    if(state==="anonymous")location.replace(`/login?next=${encodeURIComponent(`${location.pathname}${location.search}`)}`);
    else if(state==="authenticated"&&staff)location.replace(STAFF_BASE_PATH);
  },[state,staff]);
  if(state!=="authenticated")return <AuthBootstrapLoader/>;
  if(state==="authenticated"&&staff)return <section className="empty"><h1>Перенаправляем в служебную зону…</h1></section>;
  return children;
}
