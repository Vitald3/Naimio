"use client";
import {useEffect, useState} from "react";
import {attribution,track} from "../analytics";
import Breadcrumbs from "../breadcrumbs";
import {useAuth} from "../auth-state";

type Candidate = {id:string; name:string; slug:string; confidence:number};
type Brief = {title:string; summary?:string; scope?:string; requirements?:string[]; questions?:string[]; assumptions?:string[]; category_candidates?:Candidate[]; skills?:Candidate[]; budget?:{min_kopecks:number;max_kopecks:number;currency:string;confidence:string};duration_days?:{min:number;max:number}};

export default function CreateProjectPage() {
  const {state:authState}=useAuth();
  const manualHref=authState==="authenticated"?"/dashboard/projects/new":`/login?next=${encodeURIComponent("/dashboard/projects/new")}`;
  const [text,setText]=useState(""); const [token,setToken]=useState(""); const [brief,setBrief]=useState<Brief|null>(null); const [status,setStatus]=useState("");
  useEffect(()=>{const query=new URLSearchParams(location.search).get("draft");const initial=sessionStorage.getItem("guest-project-input");if(initial)setText(initial);if(query){setToken(query);fetch(`/api/v1/project-drafts/${query}`,{credentials:"same-origin"}).then(r=>r.ok?r.json():null).then(body=>{if(body?.data?.raw_input?.text)setText(body.data.raw_input.text);if(body?.data?.normalized_data?.title)setBrief(body.data.normalized_data)})}},[]);
  async function draft(source_type:string){if(token)return token;const response=await fetch("/api/v1/project-drafts",{method:"POST",credentials:"same-origin",headers:{"Content-Type":"application/json"},body:JSON.stringify({source_type,raw_input:{text,attribution:attribution()}})});if(!response.ok)throw new Error();const body=await response.json();setToken(body.draft_token);history.replaceState(null,"",`/create-project?draft=${body.draft_token}`);return body.draft_token as string}
  async function analyze(event:{preventDefault():void},kind:"brief"|"import"){event.preventDefault();setStatus("Анализируем задачу и формируем ориентировочную оценку…");try{const draftToken=await draft(kind==="import"?"IMPORT":"AI_BRIEF");const path=kind==="import"?"project-import":"project-brief";const payload=kind==="import"?{draft_token:draftToken,materials:[{name:"Вставленный материал",text}]}:{draft_token:draftToken,text};const response=await fetch(`/api/v1/ai/${path}`,{method:"POST",credentials:"same-origin",headers:{"Content-Type":"application/json"},body:JSON.stringify(payload)});const body=await response.json();if(!response.ok)throw new Error();setBrief(body.data);setStatus("Готово. Все поля можно изменить перед публикацией.");track("GUEST_PROJECT_ANALYSIS_COMPLETED",{draft_source:kind})}catch{setStatus("AI-помощник недоступен. Текст сохранён — продолжите вручную.")}}
  async function continueToProject(){await fetch(`/api/v1/project-drafts/${token}`,{method:"PATCH",credentials:"same-origin",headers:{"Content-Type":"application/json"},body:JSON.stringify({raw_input:{text,attribution:attribution()},normalized_data:brief})});const response=await fetch(`/api/v1/project-drafts/${token}/claim`,{method:"POST",credentials:"same-origin"});if(response.status===401){location.assign(`/register?next=${encodeURIComponent(`/create-project?draft=${token}`)}`);return}if(response.ok){track("PROJECT_DRAFT_CLAIMED",{draft_source:"guest"});location.assign(`/dashboard/projects/new?draft=${token}`)}else setStatus("Не удалось сохранить черновик. Попробуйте снова.")}
  return (
    <main>
      <Breadcrumbs items={[{label:"Главная",href:"/"},{label:"Разместить задачу"}]}/>
      <header>
        <p className="eyebrow">Новый проект</p>
        <h1>Опишите задачу — бриф соберём вместе</h1>
        <p className="lead">Выберите удобный путь: заполните полноценную форму самостоятельно или дайте AI собрать редактируемый черновик. Категория, навыки, бюджет и сроки всегда остаются под вашим контролем.</p><div className="mode-choice"><a className="button button--quiet" href={manualHref}>Заполнить вручную</a><span>или используйте AI-помощника ниже</span></div>
      </header>
      <div className="compose-grid">
        <form onSubmit={event=>analyze(event,"brief")}>
          <label>Описание или материал
            <textarea required minLength={3} maxLength={100000} value={text} onChange={event=>setText(event.target.value)} placeholder="Например: нужно разработать лендинг с формой заявки и интеграцией с CRM…"/>
          </label>
          <div className="inline-actions">
            <button>Продолжить</button>
            <button type="button" className="button button--quiet" onClick={event=>analyze(event,"import")}>Импортировать материал</button>
          </div>
          <p role="status">{status}</p>
        </form>
        <aside className="compose-aside">
          <p className="eyebrow">Как это устроено</p>
          <ol className="compose-steps">
            <li><span>01</span><div><strong>Опишите результат</strong><p>Текст или материалы — помощник соберёт черновик брифа.</p></div></li>
            <li><span>02</span><div><strong>Проверьте детали</strong><p>Категория, навыки и ориентир цены — всё можно изменить.</p></div></li>
            <li><span>03</span><div><strong>Получите предложения</strong><p>Специалисты откликнутся, а работа идёт через Safe Deal.</p></div></li>
          </ol>
          <p className="compose-note">Внешняя репутация и отзывы платформы показываются отдельно. Файлы и ссылки не загружаются автоматически.</p>
        </aside>
      </div>
      {brief && (
        <section>
          <p className="eyebrow">Черновик брифа</p>
          <h2><input className="brief-title" aria-label="Название проекта" value={brief.title} onChange={event=>setBrief({...brief,title:event.target.value})}/></h2>
          <label>Нормализованное описание
            <textarea value={brief.summary||brief.scope||""} onChange={event=>setBrief({...brief,summary:event.target.value})}/>
          </label>
          <p>Вероятная категория: {brief.category_candidates?.[0]?.name||"Нужно выбрать вручную"}</p>
          <p>Навыки: {brief.skills?.map(item=>item.name).join(", ")||"Нужно выбрать вручную"}</p>
          {brief.budget&&brief.duration_days&&<p>Ориентировочная оценка: {(brief.budget.min_kopecks/100).toLocaleString("ru-RU")}–{(brief.budget.max_kopecks/100).toLocaleString("ru-RU")} ₽ · {brief.duration_days.min}–{brief.duration_days.max} дней</p>}
          {(brief.questions?.length||0)>0&&<><h3>Нужно уточнить</h3><ul>{brief.questions?.map(item=><li key={item}>{item}</li>)}</ul></>}
          <p><small>Оценка ориентировочная; финальные предложения делают специалисты.</small></p>
          <button onClick={continueToProject}>Получить предложения</button>
        </section>
      )}
    </main>
  );
}
