"use client";
import { CustomSelect } from "../../custom-select";
import Breadcrumbs from "../../breadcrumbs";
import{FormEvent,useEffect,useState}from"react";
type Reward={id:string;rule_code:string;reward_type:string;amount:number;unit:string;created_at:string};
// Promo rewards are platform perks, not a cash balance. Render each with a
// human title and a clear value + unit so the block never shows raw codes.
const rewardTitles:Record<string,string>={
  INVITE_ACCEPTED:"Приглашённый пользователь зарегистрировался",
  FIRST_DEAL:"Первая безопасная сделка приглашённого",
  REFERRAL_MILESTONE:"Достигнута цель реферальной программы",
};
const unitLabel=(unit:string,amount:number)=>{
  if(unit==="PERCENT")return `−${amount}% комиссии`;
  if(unit==="KOPECKS")return `${new Intl.NumberFormat("ru-RU").format(amount/100)} ₽ бонуса`;
  if(unit==="DEALS")return `${amount} сделок без комиссии`;
  return `${amount} ${unit}`;
};
const dateFmt=(v:string)=>new Date(v).toLocaleDateString("ru-RU",{day:"2-digit",month:"long",year:"numeric"});
export default function InvitesPage(){
  const[type,setType]=useState("CUSTOMER");const[projectID,setProjectID]=useState("");const[email,setEmail]=useState("");const[link,setLink]=useState("");const[rewards,setRewards]=useState<Reward[]>([]);const[state,setState]=useState("");
  useEffect(()=>{fetch("/api/v1/me/referrals",{credentials:"same-origin"}).then(r=>r.ok?r.json():null).then(b=>setRewards(b?.data?.rewards??[])).catch(()=>undefined)},[]);
  async function create(e:FormEvent){e.preventDefault();setState("Создаём ссылку…");const response=await fetch("/api/v1/me/invites",{method:"POST",credentials:"same-origin",headers:{"Content-Type":"application/json"},body:JSON.stringify({type,project_id:projectID||"",intended_email:email||""})});const body=await response.json();if(!response.ok){setState("Не удалось создать приглашение");return}setLink(body.data.url);setState("Ссылка готова")}
  async function copy(){await navigator.clipboard.writeText(link);setState("Ссылка скопирована")}
  return <main>
    <Breadcrumbs items={[{label:"Главная",href:"/"},{label:"Кабинет",href:"/dashboard"},{label:"Приглашения"}]}/>
    <header className="page-heading"><div><h1>Приглашения и промо-награды</h1><p className="card-meta">Пригласите коллег по персональной ссылке и получайте промо-льготы за их активность.</p></div></header>
    <section className="deal-panel"><h2>Новое приглашение</h2>
      <form className="stacked-form" onSubmit={create}>
        <label>Кого пригласить <CustomSelect value={type} onChange={e=>setType(e.target.value)}><option value="CUSTOMER">Заказчика</option><option value="FREELANCER">Исполнителя</option><option value="PROJECT">Исполнителя в проект</option></CustomSelect></label>
        {type!=="CUSTOMER"?<label>ID проекта <input required value={projectID} onChange={e=>setProjectID(e.target.value)}/></label>:<label>Контекст проекта, если есть <input value={projectID} onChange={e=>setProjectID(e.target.value)}/></label>}
        <label>Email получателя (не показывается публично) <input type="email" maxLength={320} value={email} onChange={e=>setEmail(e.target.value)}/></label>
        <button>Создать ссылку</button>
      </form>
      {link?<div className="invite-link"><a href={link}>{link}</a><div className="inline-actions"><button type="button" className="button button--quiet" onClick={copy}>Копировать</button>{typeof navigator!=="undefined"&&"share"in navigator?<button type="button" className="button button--quiet" onClick={()=>navigator.share({url:link})}>Поделиться</button>:null}</div></div>:null}
      {state?<p role="status" className="notice">{state}</p>:null}
    </section>
    <section className="deal-panel"><h2>Промо-награды</h2><p className="card-meta">Это промо-льготы за развитие платформы, а не денежный баланс к выводу.</p>
      {rewards.length?<ul className="reward-list">{rewards.map(item=><li key={item.id} className="reward"><div className="reward__head"><strong>{rewardTitles[item.rule_code]??item.rule_code}</strong><span className="badge badge--success">{unitLabel(item.unit,item.amount)}</span></div><span className="reward__date">Начислено {dateFmt(item.created_at)}</span></li>)}</ul>:<p className="empty">Начислений пока нет. Приглашайте коллег — льготы появятся здесь.</p>}
    </section>
  </main>;
}
