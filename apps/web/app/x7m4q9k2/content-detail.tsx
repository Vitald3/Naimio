"use client";

import { useCallback, useEffect, useState } from "react";
import { AdminDetailSkeleton, AdminEntityLink, AdminError, AdminHeader, AdminReasonAction, AdminRichText, StatusPill, adminRequest, formatDate } from "./admin-ui";

type ContentItem={id:string;kind:string;title:string;owner_user_id:string;owner_display_name:string;status:string;moderation_status:string;moderation_reason?:string;category_name?:string;created_at:string;updated_at:string};

const kindLabels:Record<string,string>={PROJECT:"Проект",SERVICE:"Услуга",VACANCY:"Вакансия"};

export default function AdminContentDetail({endpoint,kind,id}:{endpoint:"projects"|"services"|"vacancies";kind:"PROJECT"|"SERVICE"|"VACANCY";id:string}){
  const[item,setItem]=useState<ContentItem|null>(null);const[error,setError]=useState("");const[loading,setLoading]=useState(true);
  const load=useCallback(async()=>{setLoading(true);setError("");try{const body=await adminRequest<{data:ContentItem}>(`/api/v1/admin/${endpoint}/${encodeURIComponent(id)}`);setItem(body.data)}catch(e){setError(e instanceof Error?e.message:"Не удалось загрузить объект")}finally{setLoading(false)}},[endpoint,id]);
  useEffect(()=>{void load()},[load]);
  async function moderate(action:"HIDE"|"RESTORE"|"REJECT"|"DELETE",reason:string){await adminRequest(`/api/v1/admin/${endpoint}/${id}/moderation`,{method:"POST",body:JSON.stringify({action,reason})});if(action==="DELETE"){location.href=`/x7m4q9k2/${endpoint}`;return;}await load()}
  if(loading&&!item)return <><AdminHeader title="Загрузка объекта…" description={`${kindLabels[kind]} · административная карточка и модерация`}/><AdminDetailSkeleton/></>;if(error||!item)return <AdminError message={error||"Объект не найден"} onRetry={load}/>;
  return <><AdminHeader title={item.title} description={`${kindLabels[kind]} · административная карточка и модерация`}/><section className="admin-section admin-content-detail">
    <div className="admin-content-detail__meta"><div><span>Статус</span><StatusPill value={item.status}/></div><div><span>Модерация</span><StatusPill value={item.moderation_status}/></div><div><span>Категория</span><strong>{item.category_name||"Не указана"}</strong></div><div><span>Владелец</span><AdminEntityLink type="USER" id={item.owner_user_id} label={item.owner_display_name} newTab={false}/></div><div><span>Создан</span><strong>{formatDate(item.created_at)}</strong></div><div><span>Обновлён</span><strong>{formatDate(item.updated_at)}</strong></div></div>
    {item.moderation_reason?<div className="admin-content-detail__reason"><h2>Последнее решение модерации</h2><AdminRichText value={item.moderation_reason}/></div>:null}
    <div className="admin-row-actions admin-content-detail__actions">{item.moderation_status==="HIDDEN"?<AdminReasonAction label="Восстановить" title={`Восстановить: ${item.title}`} onConfirm={r=>moderate("RESTORE",r)}/>:<AdminReasonAction label="Скрыть" title={`Скрыть: ${item.title}`} onConfirm={r=>moderate("HIDE",r)}/>}<AdminReasonAction label="Отклонить" tone="danger" title={`Отклонить: ${item.title}`} description="Владелец получит уведомление, email и системное сообщение поддержки с причиной и ссылкой на исправление." onConfirm={r=>moderate("REJECT",r)}/><AdminReasonAction label="Удалить" tone="danger" title={`Удалить: ${item.title}`} description="Объект исчезнет с сайта. Причина останется в аудите, владелец получит уведомление, email и сообщение поддержки." onConfirm={r=>moderate("DELETE",r)}/></div>
  </section></>;
}
