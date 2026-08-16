"use client";
import type { ReactNode } from "react";
import { STAFF_LOGIN_PATH,isStaffRoles } from "../admin-path";
import { useAdminAuth } from "./admin-auth";
import { AuthBootstrapLoader } from "../auth-loader";

export function AdminGuard({children}:{children:ReactNode}){
 const {state,user}=useAdminAuth();
 if(state==="loading")return <AuthBootstrapLoader/>;
 if(state==="anonymous")return <main><div className="empty"><h1>Требуется служебный вход</h1><p>Операционная зона Naimio использует отдельную служебную сессию и не зависит от входа на маркетплейсе.</p><a className="button" href={STAFF_LOGIN_PATH}>Служебный вход</a></div></main>;
 if(!isStaffRoles(user?.roles))return <main><div className="empty"><h1>Страница не найдена</h1><p>Запрошенный ресурс недоступен.</p><a className="button button--quiet" href={STAFF_LOGIN_PATH}>Служебный вход</a></div></main>;
 return children;
}
