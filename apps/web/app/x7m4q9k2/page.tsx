"use client";
import { useCallback, useEffect, useState } from "react";
import { AdminError, AdminHeader, AdminMetricsSkeleton, adminRequest } from "./admin-ui";
import { useAuth } from "../auth-state";

type Metrics = {
  users_total:number; users_new_7d:number; projects_active:number; projects_open:number; pending_reputation:number;
  open_reports:number; open_fraud_signals:number; open_disputes:number; active_safe_deals:number; services_active:number;
  vacancies_published:number; recent_admin_actions:number;
};
const cards: Array<[keyof Metrics,string,string]> = [
  ["users_total","Пользователи","Все активные учётные записи"],
  ["users_new_7d","Новые за 7 дней","Свежие регистрации"],
  ["projects_open","Открытые проекты","Доступны исполнителям"],
  ["projects_active","Активные проекты","В работе и подборе"],
  ["pending_reputation","Проверка репутации","Ожидают модератора"],
  ["open_reports","Жалобы","Требуют решения"],
  ["open_fraud_signals","Fraud-сигналы","Нужна проверка"],
  ["open_disputes","Открытые споры","Safe Deal"],
  ["active_safe_deals","Активные сделки","Не завершены"],
  ["services_active","Активные услуги","В публичном каталоге"],
  ["vacancies_published","Вакансии","Опубликованы"],
  ["recent_admin_actions","Действия за 24 ч","Записаны в аудит"],
];
export default function AdminHome(){
  const {user}=useAuth();
  const isAdmin=user?.roles?.some(role=>role==="ADMIN"||role==="SUPER_ADMIN")??false;
  const [data,setData]=useState<Metrics|null>(null);const[error,setError]=useState("");
  const load=useCallback(()=>{setError("");adminRequest<{data:Metrics}>("/api/v1/admin/dashboard").then(v=>setData(v.data)).catch(e=>setError(e.message))},[]);
  useEffect(load,[load]);
  return <><AdminHeader title="Операционный центр" description="Состояние маркетплейса, очереди модерации и финансово-чувствительные процессы в одном месте."/>
  {error?<AdminError message={error} onRetry={load}/>:!data?<AdminMetricsSkeleton count={12}/>:<>
    <div className="admin-kpi-grid">{cards.map(([key,label,hint])=>{const restricted=key==="users_total"||key==="users_new_7d"||key==="recent_admin_actions";const href=key==="pending_reputation"?"/x7m4q9k2/reputation":key==="open_reports"?"/x7m4q9k2/reports":key==="open_fraud_signals"?"/x7m4q9k2/fraud":key==="open_disputes"?"/x7m4q9k2/disputes":key==="active_safe_deals"?"/x7m4q9k2/safe-deals":key.startsWith("projects")?"/x7m4q9k2/projects":key==="services_active"?"/x7m4q9k2/services":key==="vacancies_published"?"/x7m4q9k2/vacancies":key==="recent_admin_actions"?"/x7m4q9k2/audit":"/x7m4q9k2/users";const body=<><span>{label}</span><strong>{data[key]}</strong><small>{hint}</small></>;return restricted&&!isAdmin?<div className="admin-kpi" key={key}>{body}</div>:<a className="admin-kpi" key={key} href={href}>{body}</a>})}</div>
    <section className="admin-quick-grid"><article><p className="eyebrow">Trust & Safety</p><h2>Очередь доверия</h2><p>Проверяйте внешнюю репутацию, жалобы и fraud-сигналы до того, как они влияют на участников рынка.</p><div className="inline-actions"><a className="button button--quiet" href="/x7m4q9k2/reputation">Репутация</a><a className="button button--quiet" href="/x7m4q9k2/reports">Жалобы</a></div></article><article><p className="eyebrow">Safe Deal</p><h2>Споры и сверка</h2><p>Финансовые состояния не редактируются вручную: решения проходят доменный workflow и аудит.</p><div className="inline-actions"><a className="button button--quiet" href="/x7m4q9k2/safe-deals">Сделки</a><a className="button button--quiet" href="/x7m4q9k2/disputes">Споры</a></div></article></section>
  </>}</>;
}
