"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import Breadcrumbs from "../../breadcrumbs";
import { Avatar, PresenceLabel } from "../../media-components";
import { IconSearch, IconStar } from "../../icons";
import { useToast } from "../../toast";
import { countLabel } from "../../russian-plural";

type Member = {
  freelancer_user_id: string;
  username?: string;
  display_name: string;
  availability: string;
  professional_title?: string;
  native_rating?: number;
  reviews_count?: number;
  label?: string;
  notes?: string;
  last_project_id?: string;
  last_project_title?: string;
};
type Candidate={id?:string;user_id?:string;username:string;display_name:string;professional_title?:string;availability?:string;native_rating?:number;reviews_count?:number};
const availabilityLabels:Record<string,string>={AVAILABLE:"Доступен к работе",PARTIALLY_BUSY:"Частично занят",BUSY:"Сейчас занят",UNAVAILABLE:"Временно недоступен"};

export default function TeamPage() {
  const [items, setItems] = useState<Member[]>([]);
  const [id, setID] = useState("");
  const [personQuery,setPersonQuery]=useState("");
  const [candidates,setCandidates]=useState<Candidate[]>([]);
  const [searching,setSearching]=useState(false);
  const [label, setLabel] = useState("");
  const [notes, setNotes] = useState("");
  const [saving, setSaving] = useState(false);
  const { push } = useToast();

  const load = useCallback(async () => {
    const response = await fetch("/api/v1/me/customer-team", { credentials: "same-origin" });
    if (!response.ok) { setItems([]); return; }
    const body = await response.json();
    setItems(body.data ?? []);
  }, []);
  useEffect(() => { void load(); }, [load]);
  useEffect(()=>{
    const query=personQuery.trim();
    if(id||query.length<2){setCandidates([]);setSearching(false);return;}
    const timer=setTimeout(()=>{setSearching(true);fetch(`/api/v1/freelancers?q=${encodeURIComponent(query)}&limit=8`,{cache:"no-store"}).then((response)=>response.ok?response.json():Promise.reject()).then((body)=>setCandidates(body.data??[])).catch(()=>setCandidates([])).finally(()=>setSearching(false));},250);
    return()=>clearTimeout(timer);
  },[personQuery,id]);

  function choose(candidate:Candidate){const candidateID=candidate.id||candidate.user_id;if(!candidateID){push({kind:"error",title:"Не удалось определить пользователя"});return}setID(candidateID);setPersonQuery(candidate.display_name||candidate.username);setCandidates([]);}
  function resetEditor(){setID("");setPersonQuery("");setLabel("");setNotes("");setCandidates([]);}

  async function save(event: FormEvent) {
    event.preventDefault();
    if(!id){push({kind:"error",title:"Выберите специалиста из списка",message:"Начните вводить имя или специализацию и выберите найденного пользователя."});return;}
    setSaving(true);
    const response = await fetch(`/api/v1/me/customer-team/${encodeURIComponent(id)}`, {
      method: "PUT", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ label, notes }),
    });
    setSaving(false);
    if (!response.ok) { push({ kind: "error", title: "Не удалось сохранить специалиста" }); return; }
    resetEditor(); await load();
    push({ kind: "success", title: "Моя команда обновлена" });
  }

  async function remove(freelancerID: string) {
    if (!confirm("Удалить специалиста из команды?")) return;
    const response = await fetch(`/api/v1/me/customer-team/${freelancerID}`, { method: "DELETE", credentials: "same-origin" });
    if (response.ok) { await load(); push({ kind: "success", title: "Специалист удалён из команды" }); }
    else push({ kind: "error", title: "Не удалось удалить специалиста" });
  }

  async function startConversation(freelancerID:string){
    const response=await fetch("/api/v1/conversations",{method:"POST",credentials:"same-origin",headers:{"Content-Type":"application/json"},body:JSON.stringify({participant_user_id:freelancerID})});
    const body=await response.json().catch(()=>null);
    if(!response.ok||!body?.data?.id){push({kind:"error",title:"Не удалось открыть диалог",message:"Попробуйте ещё раз или откройте раздел сообщений."});return;}
    location.assign(`/messages?conversation=${encodeURIComponent(body.data.id)}`);
  }

  return <main>
    <Breadcrumbs items={[{ label: "Главная", href: "/" }, { label: "Кабинет", href: "/dashboard" }, { label: "Моя команда" }]} />
    <header className="page-heading"><div><p className="eyebrow">Постоянные исполнители</p><h1>Моя команда</h1><p className="lead">Добавление выполняется отдельно от избранного: «Моя команда» — это список постоянных исполнителей, а не подписки и не лёгкие закладки.</p></div></header>
    <form onSubmit={save} className="team-editor"><div className="team-person-picker"><label htmlFor="team-person-search">Специалист</label><div className="team-person-picker__input"><IconSearch size={18}/><input id="team-person-search" autoComplete="off" value={personQuery} onChange={(event)=>{setPersonQuery(event.target.value);setID("");}} placeholder="Имя, профессия или навык" role="combobox" aria-autocomplete="list" aria-expanded={candidates.length>0} aria-controls="team-person-options"/>{id?<button type="button" className="button button--quiet button--compact" onClick={()=>{setID("");setPersonQuery("")}}>Изменить</button>:null}</div>{searching?<small className="form-hint">Ищем специалистов…</small>:null}{!id&&candidates.length?<ul id="team-person-options" className="team-autocomplete" role="listbox">{candidates.map((candidate)=><li key={candidate.id||candidate.user_id||candidate.username}><button type="button" role="option" aria-selected="false" onClick={()=>choose(candidate)}><Avatar name={candidate.display_name} id={candidate.username} size="sm"/><span><strong>{candidate.display_name}</strong><small>{candidate.professional_title||candidate.username}{candidate.native_rating?` · ★ ${candidate.native_rating.toFixed(1)} (${candidate.reviews_count??0})`:""}</small></span></button></li>)}</ul>:null}{!id&&personQuery.trim().length>=2&&!searching&&!candidates.length?<small className="form-hint">Ничего не найдено. Попробуйте имя, профессию или навык.</small>:null}{id?<p className="selection-summary">Выбран специалист: <strong>{personQuery}</strong></p>:null}</div><label>Метка<input maxLength={120} value={label} onChange={(event) => setLabel(event.target.value)} placeholder="Например, Flutter / основной" /></label><label>Заметки<textarea maxLength={2000} value={notes} onChange={(event) => setNotes(event.target.value)} placeholder="Внутренняя заметка для себя" /></label><button disabled={saving}>{saving ? "Сохраняем…" : "Добавить или обновить"}</button></form>
    {items.length ? <ul className="team-grid">{items.map((item) => <li key={item.freelancer_user_id}><article className="team-card">
      <div className="team-card__identity"><Avatar name={item.display_name} id={item.username || item.freelancer_user_id} size="lg" /><div><h2>{item.username ? <a href={`/freelancers/${item.username}`}>{item.display_name}</a> : item.display_name}</h2><p>{item.professional_title || "Профессиональный специалист"}</p><div className="team-card__facts"><span className="rating-pill"><IconStar size={14}/>{item.native_rating ? `${item.native_rating.toFixed(1)} · ${countLabel(item.reviews_count ?? 0,["отзыв","отзыва","отзывов"])}` : "Новый профиль"}</span><span>{availabilityLabels[item.availability]||"Доступность уточняется"}</span><PresenceLabel id={item.username || item.freelancer_user_id}/>{item.label ? <span>{item.label}</span> : null}</div></div></div>
      {item.notes ? <p className="team-card__notes">{item.notes}</p> : null}{item.last_project_id ? <p>Последний проект: <a href={`/projects/${item.last_project_id}`}>{item.last_project_title}</a></p> : null}
      <div className="team-card__links"><a href={`/dashboard/projects/new?freelancer=${item.freelancer_user_id}`}>Новый проект</a><a href={`/dashboard/invites?freelancer=${item.freelancer_user_id}`}>Пригласить</a><button type="button" className="team-card__text-action" onClick={()=>void startConversation(item.freelancer_user_id)}>Написать</button></div>
      <div className="team-card__actions"><button type="button" className="button button--quiet" onClick={() => { setID(item.freelancer_user_id); setPersonQuery(item.display_name); setLabel(item.label ?? ""); setNotes(item.notes ?? ""); window.scrollTo({top:0,behavior:"smooth"}); }}>Изменить метку</button><button type="button" className="button button--quiet button--danger" onClick={() => void remove(item.freelancer_user_id)}>Удалить из команды</button></div>
    </article></li>)}</ul> : <div className="empty"><h2>Команда пока пуста</h2><p>Добавляйте сюда специалистов, с которыми хотите работать снова.</p><a className="button button--quiet" href="/freelancers">Найти специалиста</a></div>}
  </main>;
}
