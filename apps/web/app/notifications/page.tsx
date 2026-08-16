"use client";
import Breadcrumbs from "../breadcrumbs";
import {useCallback,useEffect,useState} from "react";
import type React from "react";
import { notificationPresentation } from "../notification-presentation";
import FormattedText from "../formatted-text";
import { NotificationsListSkeleton } from "../skeletons";
type Notification={id:string;type:string;entity_type?:string;entity_id?:string;payload:Record<string,string>;read_at?:string;created_at:string};
// Every event the platform can raise has a clear Russian title and a short hint,
// so a notification never renders as a raw machine code like project_status_changed.
const timeFmt=(v:string)=>new Date(v).toLocaleString("ru-RU",{day:"2-digit",month:"long",hour:"2-digit",minute:"2-digit"});
const notificationHref=(item:Notification)=>{
  if(item.payload?.edit_url)return item.payload.edit_url;
  if(item.type==="proposal_received"&&(item.payload?.project_id||item.entity_id))return `/dashboard/projects/${item.payload?.project_id||item.entity_id}/proposals`;
  if(item.payload?.conversation_id)return `/messages?conversation=${encodeURIComponent(item.payload.conversation_id)}`;
  if(item.type==="invite_accepted")return "/dashboard/invites";
  if(item.type==="new_review_received")return "/dashboard/reviews";
  if(item.type==="safe_deal_update"&&item.payload?.entity_id)return `/dashboard/safe-deals/${item.payload.entity_id}`;
  if(item.payload?.project_id)return `/projects/${item.payload.project_id}`;
  if(item.payload?.job_id)return `/vacancies/${item.payload.job_id}`;
  if(item.payload?.service_id)return `/services/${item.payload.service_id}`;
  if(item.type==="new_project_available")return "/projects";
  if(item.type==="new_vacancy_available")return "/vacancies";
  if(item.type==="new_service_available")return "/services";
  if(item.type==="moderation_update"&&item.entity_id)return item.entity_type==="PROJECT"?`/dashboard/projects/${item.entity_id}`:item.entity_type==="SERVICE"?"/dashboard/services":item.entity_type==="VACANCY"?"/dashboard/vacancies":item.entity_type==="REVIEW"?"/dashboard/reviews":"/dashboard";
  return item.payload?.entity_id?(item.type.includes("review")?"/dashboard/reviews":item.type.includes("deal")?`/dashboard/safe-deals/${item.payload.entity_id}`:item.type.includes("invite")?"/dashboard/invites":"/dashboard"):null;
};
export default function NotificationsPage(){
  const[items,setItems]=useState<Notification[]>([]);
  const[error,setError]=useState("");
  const[ready,setReady]=useState(false);
  const load=useCallback(async()=>{const r=await fetch("/api/v1/notifications",{credentials:"same-origin"});if(!r.ok)throw new Error("Не удалось загрузить уведомления");setItems((await r.json()).data??[]);setReady(true)},[]);
  useEffect(()=>{let disposed=false;let reconnectTimer:number|undefined;let socket:WebSocket|null=null;const initial=()=>load().catch(e=>{if(!disposed){setError(e.message);setReady(true)}});const connect=()=>{if(disposed)return;const protocol=location.protocol==="https:"?"wss":"ws";socket=new WebSocket(process.env.NEXT_PUBLIC_WS_URL||`${protocol}://${location.host}/api/v1/ws`);socket.onmessage=(event)=>{try{const envelope=JSON.parse(event.data);if(envelope.event!=="notification.created"||!envelope.data?.id)return;const incoming=envelope.data as Notification;setItems(current=>current.some(item=>item.id===incoming.id)?current:[incoming,...current]);setError("");setReady(true)}catch{}};socket.onclose=()=>{if(!disposed)reconnectTimer=window.setTimeout(connect,1500)};socket.onerror=()=>socket?.close()};void initial();connect();return()=>{disposed=true;if(reconnectTimer!==undefined)window.clearTimeout(reconnectTimer);socket?.close()}},[load]);
  async function read(id:string){
    const response=await fetch(`/api/v1/notifications/${id}/read`,{method:"POST",credentials:"same-origin"});
    if(response.ok){const at=new Date().toISOString();setItems(current=>current.map(item=>item.id===id?{...item,read_at:item.read_at||at}:item));window.dispatchEvent(new CustomEvent("notification-read",{detail:{id}}));}
  }
  async function openNotification(event:React.MouseEvent<HTMLAnchorElement>,item:Notification,href:string){event.preventDefault();if(!item.read_at)await read(item.id);location.assign(href)}
  async function all(){const response=await fetch("/api/v1/notifications/read-all",{method:"POST",credentials:"same-origin"});if(response.ok){const at=new Date().toISOString();setItems(current=>current.map(item=>({...item,read_at:item.read_at||at})));window.dispatchEvent(new Event("notifications-read-all"));}}
  const unread=items.filter(i=>!i.read_at).length;
  return <main>
    <Breadcrumbs items={[{label:"Главная",href:"/"},{label:"Кабинет",href:"/dashboard"},{label:"Уведомления"}]}/>
    <header className="page-heading"><div><h1>Уведомления</h1><p className="card-meta">{unread?`Непрочитанных: ${unread}`:"Все уведомления прочитаны."}</p></div>{unread?<button type="button" className="button button--quiet" onClick={all}>Отметить все прочитанными</button>:null}</header>
    {error?<p role="alert" className="notice notice--error">{error}</p>:null}
    {!ready && !error ? <NotificationsListSkeleton count={5} /> : null}
    {ready&&!error&&items.length===0?<p className="empty">Новых уведомлений нет.</p>:null}
    {items.length?<ul className="notif-list">{items.map(item=>{const meta=notificationPresentation(item.type);return <li key={item.id} className={item.read_at?"notif-item":"notif-item notif-item--unread"}>
      <div className="notif-item__body"><strong>{item.type==="moderation_update"?(item.payload?.action||meta.title):meta.title}</strong><span className="notif-item__hint">{item.type==="moderation_update"?(item.payload?.title?`Материал: ${item.payload.title}`:meta.hint):meta.hint}</span>{item.type==="moderation_update"&&(item.payload?.reason_html||item.payload?.reason_text)?<div className="notif-item__reason"><b>Причина:</b><FormattedText value={item.payload.reason_html||item.payload.reason_text}/></div>:null}<time className="notif-item__time">{timeFmt(item.created_at)}</time></div>
      <div className="notif-item__actions">{notificationHref(item)?<a className="button button--quiet" href={notificationHref(item)!} onClick={(event)=>void openNotification(event,item,notificationHref(item)!)}>{item.type==="moderation_update"&&item.payload?.edit_url?"Исправить":"Открыть"}</a>:null}{!item.read_at?<button type="button" className="button button--quiet" onClick={()=>read(item.id)}>Прочитано</button>:null}</div>
    </li>;})}</ul>:null}
  </main>;
}
