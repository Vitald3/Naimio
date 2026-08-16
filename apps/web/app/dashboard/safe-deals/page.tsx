"use client";
import Breadcrumbs from "../../breadcrumbs";
import { useEffect, useState } from "react";

type Deal={id:string;project_id:string;project_title?:string;counterparty_name?:string;gross_amount_kopecks:number;freelancer_amount_kopecks:number;status:string;viewer_role:string};
const money=(n:number)=>new Intl.NumberFormat("ru-RU").format(n/100)+" ₽";
const statusText:Record<string,string>={AWAITING_FUNDING:"Ожидает финансирования",FUNDED:"Финансирование подтверждено",IN_PROGRESS:"В работе",SUBMITTED:"Работа передана",REVISION_REQUESTED:"На доработке",DISPUTED:"Открыт спор",ACCEPTED:"Работа принята",RELEASE_PENDING:"Расчёт выполняется",COMPLETED:"Завершена",CANCEL_PENDING:"Отмена выполняется",CANCELED:"Отменена",REFUND_PENDING:"Возврат выполняется",REFUNDED:"Возврат выполнен",FAILED:"Ошибка"};
export default function SafeDealsPage(){
  const[items,setItems]=useState<Deal[]>([]);const[state,setState]=useState("loading");
  useEffect(()=>{fetch("/api/v1/me/safe-deals",{credentials:"same-origin"}).then(async r=>{if(r.status===401){location.assign("/login?next=/dashboard/safe-deals");return null}if(!r.ok)throw new Error();return r.json()}).then(b=>{setItems(b?.data??[]);setState("ready")}).catch(()=>setState("error"))},[]);
  return <main>
    <Breadcrumbs items={[{label:"Главная",href:"/"},{label:"Кабинет",href:"/dashboard"},{label:"Безопасные сделки"}]}/>
    <div className="page-heading"><div><p className="eyebrow">Контроль выполнения</p><h1>Безопасные сделки</h1><p className="lead">Финансирование, передача результата, доработки и споры проходят через серверный workflow. Финансовый статус подтверждает подключённый провайдер.</p></div></div>
    {state==="loading"?<div className="catalog-skeleton"><div className="skeleton"/><div className="skeleton"/><div className="skeleton"/></div>:state==="error"?<div className="error">Не удалось загрузить сделки. Обновите страницу.</div>:items.length?<div className="deal-list">{items.map(d=><a className="deal-card" key={d.id} href={`/dashboard/safe-deals/${d.id}`}><div><span className={`status-pill status-pill--${["COMPLETED","ACCEPTED","FUNDED","RELEASE_PENDING"].includes(d.status)?"positive":["DISPUTED","FAILED","CANCELED","REFUNDED"].includes(d.status)?"negative":"warning"}`}>{statusText[d.status]??d.status}</span><h2>{d.project_title||"Сделка по проекту"}</h2><small>{d.counterparty_name?`${d.viewer_role==="CUSTOMER"?"Исполнитель":"Заказчик"}: ${d.counterparty_name}`:"Открыть детали сделки"}</small></div><div className="deal-card__money"><span>{d.viewer_role==="CUSTOMER"?"Сумма сделки":"К получению"}</span><strong>{money(d.viewer_role==="CUSTOMER"?d.gross_amount_kopecks:d.freelancer_amount_kopecks)}</strong><span>Открыть →</span></div></a>)}</div>:<div className="empty"><h2>Сделок пока нет</h2><p>После принятия оплачиваемого отклика здесь появится сделка и её этапы.</p><a className="button" href="/projects">Найти проекты</a></div>}
  </main>;
}
