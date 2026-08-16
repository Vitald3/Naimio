"use client";
import PrettyDateInput from "../../../pretty-date-input";
import { CustomSelect } from "../../../custom-select";

import { FormEvent, use, useCallback, useEffect, useMemo, useState } from "react";
import { track } from "../../../analytics";
import Breadcrumbs from "../../../breadcrumbs";
import { useToast } from "../../../toast";
import ProjectDescriptionEditor from "../../../project-description-editor";
import FormattedText from "../../../formatted-text";

type Ref = { id: string; name: string; slug?: string };
type Budget = { type: "FIXED" | "RANGE" | "HOURLY" | "NEGOTIABLE"; min_kopecks?: number; max_kopecks?: number; currency: string };
type Project = {
  id: string;
  title: string;
  slug: string;
  description: string;
  visibility: "PUBLIC" | "PRIVATE";
  status: string;
  moderation_status?: string;
  moderation_reason?: string;
  source_type: string;
  category?: Ref;
  budget: Budget;
  deadline_at?: string;
  experience_level?: string;
  skills: Array<Ref & { importance?: number }>;
};

const projectStatus: Record<string, string> = { DRAFT: "Черновик", PUBLISHED: "Опубликован", OPEN: "Приём откликов", MATCHING: "Подбор исполнителя", IN_PROGRESS: "В работе", AWAITING_FUNDING: "Ожидает оплаты по Безопасной сделке", COMPLETED: "Завершён", CANCELLED: "Отменён", CANCELED: "Отменён", ARCHIVED: "В архиве" };
const sourceLabels: Record<string, string> = { MANUAL: "Создан вручную", AI_ASSISTED: "Составлен с помощью ИИ", AI_GENERATED: "Сгенерирован ИИ", IMPORTED: "Импортирован" };
const experienceLabels: Record<string, string> = { "": "Не важно", BEGINNER: "Начинающий", INTERMEDIATE: "Средний", ADVANCED: "Опытный", EXPERT: "Эксперт" };
const pillClass = (status: string) => ["COMPLETED", "OPEN", "PUBLISHED", "IN_PROGRESS"].includes(status) ? "status-pill status-pill--positive" : ["CANCELLED", "CANCELED", "ARCHIVED", "FAILED"].includes(status) ? "status-pill status-pill--negative" : status === "DRAFT" ? "status-pill" : "status-pill status-pill--warning";
const toRubles = (value?: number) => value === undefined ? "" : String(Math.round(value / 100));

export default function OwnerProjectPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const { push } = useToast();
  const [item, setItem] = useState<Project | null>(null);
  const [categories, setCategories] = useState<Ref[]>([]);
  const [allSkills, setAllSkills] = useState<Ref[]>([]);
  const [selectedSkills, setSelectedSkills] = useState<string[]>([]);
  const [skillQuery, setSkillQuery] = useState("");
  const [budgetMin, setBudgetMin] = useState("");
  const [budgetMax, setBudgetMax] = useState("");
  const [state, setState] = useState("");
  const [saving, setSaving] = useState(false);

  const load = useCallback(() => fetch(`/api/v1/me/projects/${id}`, { credentials: "same-origin", cache: "no-store" })
    .then((response) => response.ok ? response.json() : Promise.reject())
    .then((body) => {
      const project = body.data as Project;
      setItem(project);
      setSelectedSkills(project.skills?.map((skill) => skill.id) ?? []);
      setBudgetMin(toRubles(project.budget?.min_kopecks));
      setBudgetMax(toRubles(project.budget?.max_kopecks));
    })
    .catch(() => setState("Проект недоступен")), [id]);

  useEffect(() => { void load(); }, [load]);
  useEffect(() => {
    Promise.all([fetch("/api/v1/categories"), fetch("/api/v1/skills?limit=100")])
      .then(async ([categoryResponse, skillResponse]) => Promise.all([
        categoryResponse.ok ? categoryResponse.json() : { data: [] },
        skillResponse.ok ? skillResponse.json() : { data: [] },
      ]))
      .then(([categoryBody, skillBody]) => { setCategories(categoryBody.data ?? []); setAllSkills(skillBody.data ?? []); })
      .catch(() => undefined);
  }, []);

  const filteredSkills = useMemo(() => allSkills.filter((skill) => !skillQuery || skill.name.toLowerCase().includes(skillQuery.toLowerCase())).slice(0, 30), [allSkills, skillQuery]);

  function toggleSkill(skillID: string) {
    setSelectedSkills((current) => current.includes(skillID) ? current.filter((value) => value !== skillID) : current.length < 30 ? [...current, skillID] : current);
  }

  function budgetPayload(): Budget {
    if (!item || item.budget.type === "NEGOTIABLE") return { type: "NEGOTIABLE", currency: "RUB" };
    const min = Math.round(Number(budgetMin) * 100);
    if (item.budget.type === "FIXED") return { type: "FIXED", min_kopecks: min, currency: "RUB" };
    return { type: item.budget.type, min_kopecks: min, max_kopecks: Math.round(Number(budgetMax) * 100), currency: "RUB" };
  }

  async function save(event: FormEvent) {
    event.preventDefault();
    if (!item) return;
    if (!item.category?.id) { push({ kind: "error", title: "Выберите категорию" }); return; }
    if (item.budget.type !== "NEGOTIABLE" && (!budgetMin || Number(budgetMin) <= 0)) { push({ kind: "error", title: "Укажите бюджет" }); return; }
    if (["RANGE", "HOURLY"].includes(item.budget.type) && (!budgetMax || Number(budgetMax) < Number(budgetMin))) { push({ kind: "error", title: "Проверьте диапазон бюджета", message: "Максимальная сумма должна быть не меньше минимальной." }); return; }
    setSaving(true);
    const response = await fetch(`/api/v1/me/projects/${item.id}`, {
      method: "PATCH", credentials: "same-origin", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        category_id: item.category.id,
        title: item.title.trim(),
        slug: /^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(item.slug) ? item.slug : `project-${item.id.slice(0, 8)}`,
        description: item.description.trim(),
        budget: budgetPayload(),
        deadline_at: item.deadline_at || null,
        experience_level: item.experience_level || "",
        visibility: item.visibility,
        skill_ids: selectedSkills,
      }),
    });
    setSaving(false);
    if (!response.ok) { const body = await response.json().catch(() => null); push({ kind: "error", title: "Не удалось сохранить проект", message: body?.error?.message ?? "Проверьте обязательные поля." }); return; }
    push({ kind: "success", title: "Черновик сохранён" });
    await load();
  }

  async function publish() {
    if (!item) return;
    const response = await fetch(`/api/v1/me/projects/${id}/publish`, { method: "POST", credentials: "same-origin" });
    if (!response.ok) { const body = await response.json().catch(() => null); push({ kind: "error", title: "Проект не опубликован", message: body?.error?.message ?? "Заполните категорию, бюджет и обязательные поля." }); return; }
    track("PROJECT_PUBLISHED", { source: item.source_type || "manual" });
    push({ kind: "success", title: "Проект опубликован" });
    await load();
  }

  async function makePublic() {
    if (!item) return;
    const response = await fetch(`/api/v1/me/projects/${item.id}/make-public`, { method: "POST", credentials: "same-origin" });
    if (!response.ok) {
      const body = await response.json().catch(() => null);
      push({ kind: "error", title: "Не удалось опубликовать проект в каталоге", message: body?.error?.message ?? "Попробуйте ещё раз." });
      return;
    }
    push({ kind: "success", title: "Проект появился в общем каталоге" });
    await load();
  }

  if (!item) return <main><div className="skeleton skeleton--title"/><div className="skeleton skeleton--card"/><p role="status">{state || "Загружаем проект…"}</p></main>;

  const navigation = <>
    <Breadcrumbs items={[{ label: "Главная", href: "/" }, { label: "Кабинет", href: "/dashboard" }, { label: item.title }]} />
    <div className="context-nav" aria-label="Разделы проекта">{["OPEN","MATCHING","IN_PROGRESS"].includes(item.status) && item.moderation_status !== "HIDDEN" ? <><a href={`/dashboard/projects/${item.id}/proposals`}>Отклики</a><a href={`/dashboard/projects/${item.id}/recommendations`}>Рекомендованные специалисты</a></> : null}<a href="/dashboard/safe-deals">Безопасные сделки</a></div>
  </>;

  if (item.status === "DRAFT") {
    const rejected = item.moderation_status === "HIDDEN" && Boolean(item.moderation_reason);
    return <main>{navigation}
      <header className="page-heading"><div><p className="eyebrow">{sourceLabels[item.source_type] ?? "Проект"}</p><h1>{rejected ? "Исправление проекта" : "Черновик проекта"}</h1><p className="card-meta">{rejected ? "Проект отклонён модерацией. Исправьте замечания и отправьте его повторно." : "Все параметры доступны вручную. Публикация происходит только после вашей проверки."}</p></div><span className={rejected ? "status-pill status-pill--negative" : pillClass(item.status)}>{rejected ? "Отклонён модерацией" : (projectStatus[item.status] ?? item.status)}</span></header>
      {rejected ? <section className="notice notice--danger"><strong>Причина отклонения</strong><FormattedText value={item.moderation_reason ?? ""}/></section> : null}
      <form className="project-editor" onSubmit={save}>
        <section className="form-section"><div className="form-section__heading"><span>01</span><div><h2>Задача</h2><p>Название и подробное описание результата. Адрес страницы управляется Naimio автоматически.</p></div></div>
          <label>Название<input required minLength={3} maxLength={200} value={item.title} onChange={(event) => setItem({ ...item, title: event.target.value })}/></label>
          <div className="editor-field"><span className="editor-field__label">Описание и ТЗ</span><ProjectDescriptionEditor value={item.description} onChange={(description)=>setItem({...item,description})}/></div>
        </section>
        <section className="form-section"><div className="form-section__heading"><span>02</span><div><h2>Категория и навыки</h2><p>Помогают показывать проект релевантным специалистам.</p></div></div>
          <label>Категория<CustomSelect required value={item.category?.id ?? ""} onChange={(event) => { const category = categories.find((value) => value.id === event.target.value); setItem({ ...item, category }); }}><option value="">Выберите категорию</option>{categories.map((category) => <option key={category.id} value={category.id}>{category.name}</option>)}</CustomSelect></label>
          <label>Найти навык<input type="search" value={skillQuery} onChange={(event) => setSkillQuery(event.target.value)} placeholder="Flutter, Go, Figma…"/></label>
          <div className="skill-picker" aria-label="Навыки">{filteredSkills.map((skill) => <button type="button" className={selectedSkills.includes(skill.id) ? "skill-option is-selected" : "skill-option"} aria-pressed={selectedSkills.includes(skill.id)} key={skill.id} onClick={() => toggleSkill(skill.id)}>{skill.name}</button>)}</div>
        </section>
        <section className="form-section"><div className="form-section__heading"><span>03</span><div><h2>Бюджет и сроки</h2><p>Их можно менять до публикации.</p></div></div>
          <div className="field-row"><label>Тип бюджета<CustomSelect value={item.budget.type} onChange={(event) => setItem({ ...item, budget: { type: event.target.value as Budget["type"], currency: "RUB" } })}><option value="NEGOTIABLE">По договорённости</option><option value="FIXED">Фиксированная сумма</option><option value="RANGE">Диапазон</option><option value="HOURLY">Почасовой диапазон</option></CustomSelect></label><label>Уровень специалиста<CustomSelect value={item.experience_level ?? ""} onChange={(event) => setItem({ ...item, experience_level: event.target.value })}>{Object.entries(experienceLabels).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</CustomSelect></label></div>
          {item.budget.type !== "NEGOTIABLE" ? <div className="field-row"><label>{item.budget.type === "FIXED" ? "Сумма, ₽" : "От, ₽"}<input required type="number" min="1" step="1" value={budgetMin} onChange={(event) => setBudgetMin(event.target.value)}/></label>{["RANGE", "HOURLY"].includes(item.budget.type) ? <label>До, ₽<input required type="number" min="1" step="1" value={budgetMax} onChange={(event) => setBudgetMax(event.target.value)}/></label> : null}</div> : null}
          <div className="field-row"><label>Желаемый срок<PrettyDateInput value={item.deadline_at?.slice(0, 10) ?? ""} onChange={(value) => setItem({ ...item, deadline_at: value ? new Date(`${value}T23:59:59`).toISOString() : undefined })} ariaLabel="Желаемый срок"/></label><label>Видимость<CustomSelect value={item.visibility} onChange={(event) => setItem({ ...item, visibility: event.target.value as Project["visibility"] })}><option value="PRIVATE">Приватный до публикации</option><option value="PUBLIC">Публичный после публикации</option></CustomSelect></label></div>
        </section>
        <div className="project-editor__footer"><div><strong>Перед публикацией всё можно проверить.</strong><p>После публикации проект появится в каталоге и начнёт принимать отклики специалистов.</p></div><div className="inline-actions"><button disabled={saving}>{saving ? "Сохраняем…" : "Сохранить"}</button><button type="button" className="button button--light" disabled={saving} onClick={publish}>Опубликовать</button></div></div>
      </form>
    </main>;
  }

  const moderationRejected = item.moderation_status === "HIDDEN" && Boolean(item.moderation_reason);
  const displayStatus = moderationRejected ? "Отклонён модерацией" : (projectStatus[item.status] ?? item.status);
  return <main>{navigation}
    <header className="page-heading"><div><p className="eyebrow">{sourceLabels[item.source_type] ?? "Проект"}</p><h1>{item.title}</h1><p className="card-meta">Статус: {displayStatus} · {item.visibility === "PUBLIC" ? "Публичный" : "Приватный"}</p></div><span className={moderationRejected ? "status-pill status-pill--negative" : pillClass(item.status)}>{displayStatus}</span></header>
    {moderationRejected ? <section className="notice notice--danger"><strong>Проект не прошёл модерацию</strong><FormattedText value={item.moderation_reason ?? ""}/><p>Исправьте проект и опубликуйте его повторно. До этого отклики и Smart Match недоступны.</p></section> : null}
    <section className="deal-panel"><h2>Описание</h2><FormattedText value={item.description}/></section>
    {!moderationRejected && ["OPEN","MATCHING","IN_PROGRESS"].includes(item.status) ? <section className="deal-panel"><h2>Управление проектом</h2><div className="inline-actions"><a className="button button--quiet" href={`/dashboard/projects/${item.id}/proposals`}>Отклики исполнителей</a><a className="button button--quiet" href={`/dashboard/projects/${item.id}/recommendations`}>Smart Match</a>{item.visibility === "PUBLIC" ? <a className="button button--quiet" href={`/projects/${item.id}`}>Публичная страница</a> : <button type="button" className="button button--quiet" onClick={makePublic}>Опубликовать в каталоге</button>}</div>{item.visibility !== "PUBLIC" ? <p className="card-meta">Сейчас проект приватный: он не отображается в общем списке и у него нет публичной страницы.</p> : null}</section> : null}
  </main>;
}
