"use client";
import { useEffect, useState } from "react";
import Breadcrumbs from "../../breadcrumbs";
import { useAuth } from "../../auth-state";

type Application={id:string;job_id:string;job_title:string;cover_message?:string;status:string;created_at:string};
const applicationStatus:Record<string,string>={NEW:"На рассмотрении",SUBMITTED:"На рассмотрении",VIEWED:"Просмотрен",SHORTLISTED:"В шорт-листе",REJECTED:"Отклонён",ACCEPTED:"Принят",HIRED:"Принят"};
const dateFmt=(v:string)=>new Date(v).toLocaleDateString("ru-RU",{day:"2-digit",month:"long",year:"numeric"});
export default function MyApplications(){
  const {state:authState,user}=useAuth();const[items,setItems]=useState<Application[]>([]);const[error,setError]=useState("");const[ready,setReady]=useState(false);
  useEffect(()=>{if(authState!=="authenticated")return;if(!user?.capabilities?.includes("FREELANCER")){setReady(true);return}fetch("/api/v1/me/job-applications",{credentials:"same-origin"}).then(r=>r.ok?r.json():Promise.reject()).then(b=>{setItems(b.data??[]);setReady(true)}).catch(()=>{setError("Не удалось загрузить отклики.");setReady(true)});},[authState,user?.id,user?.capabilities]);
  if(authState==="loading"||!ready)return <div className="skeleton"/>;
  if(!user?.capabilities?.includes("FREELANCER"))return <div className="notice"><h1>Раздел для специалистов</h1><p>Отклики на вакансии появляются только в кабинете специалиста. У заказчика этого раздела в навигации нет.</p></div>;
  return <main><Breadcrumbs items={[{label:"Главная",href:"/"},{label:"Кабинет",href:"/dashboard"},{label:"Мои отклики на вакансии"}]}/><header className="page-heading"><div><p className="eyebrow">Поиск работы</p><h1>Мои отклики на вакансии</h1><p className="card-meta">Отклики на вакансии и их текущий статус.</p></div></header>{error?<p role="alert" className="notice notice--error">{error}</p>:items.length===0?<div className="empty"><h2>Вы пока не откликались на вакансии</h2><p>Найдите подходящую вакансию и отправьте работодателю сопроводительное сообщение.</p><a className="button button--quiet" href="/vacancies">Смотреть вакансии</a></div>:<ul className="record-list response-card-grid">{items.map(a=><li key={a.id} className="record"><div className="record__head"><strong><a className="admin-primary-link" href={`/vacancies/${a.job_id}`}>{a.job_title}</a></strong><span className="badge">{applicationStatus[a.status]??a.status}</span></div><p className="response-card__message">{a.cover_message||"Сопроводительное сообщение не добавлено."}</p><p className="card-meta">Отклик отправлен: {dateFmt(a.created_at)}</p></li>)}</ul>}</main>;
}
