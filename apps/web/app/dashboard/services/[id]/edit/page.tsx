"use client";

import { use, useEffect, useState } from "react";
import Breadcrumbs from "../../../../breadcrumbs";
import ServiceEditor from "../../service-editor";

type Service={id:string;title:string;slug:string;service_type:string;short_description?:string;description:string;price_type:string;price_from?:{amount_kopecks:number};delivery_days?:number;included_revisions?:number;visibility:string;category?:{id:string;name:string};skills?:{id:string;name:string}[];education_details?:{format:string;duration_minutes?:number;sessions_count?:number;audience_type?:string;group_size_max?:number}};
export default function EditService({params}:{params:Promise<{id:string}>}){
  const{id}=use(params);const[item,setItem]=useState<Service|null>(null);const[failed,setFailed]=useState(false);
  useEffect(()=>{fetch(`/api/v1/me/services/${id}`,{credentials:"same-origin"}).then(response=>response.ok?response.json():Promise.reject()).then(body=>setItem(body.data)).catch(()=>setFailed(true))},[id]);
  if(failed)return <main><div className="empty"><h1>Предложение недоступно</h1><p>Редактировать можно только собственное предложение.</p><a className="button button--quiet" href="/dashboard/services">К моим услугам</a></div></main>;
  if(!item)return <main><div className="skeleton skeleton--title"/><div className="skeleton skeleton--card"/></main>;
  return <main><Breadcrumbs items={[{label:"Главная",href:"/"},{label:"Кабинет",href:"/dashboard"},{label:"Мои услуги",href:"/dashboard/services"},{label:item.title}]}/><header className="page-heading"><div><p className="eyebrow">Редактирование</p><h1>{item.title}</h1><p className="lead">Изменения сохраняются в карточке предложения. Активное предложение при необходимости сначала приостановите.</p></div></header><ServiceEditor existing={item}/></main>
}
