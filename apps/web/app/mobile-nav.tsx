"use client";
import { usePathname } from "next/navigation";
import { useAuth } from "./auth-state";
import { STAFF_BASE_PATH, isStaffRoles } from "./admin-path";

export default function MobileNav(){
  const pathname=usePathname()||"";
  const{state,user}=useAuth();
  const staff=isStaffRoles(user?.roles);
  if(pathname===STAFF_BASE_PATH||pathname.startsWith(STAFF_BASE_PATH+"/")||staff)return null;
  return <nav className="bottom-nav" aria-label="Быстрая навигация"><a href="/">Главная</a><a href="/freelancers">Поиск</a><a href={state==="authenticated"?"/dashboard/projects/new":"/create-project"}>Создать</a>{state==="authenticated"?<><a href="/messages">Сообщения</a><a href="/dashboard">Кабинет</a></>:<><a href="/login">Войти</a><a href="/register">Регистрация</a></>}</nav>
}
