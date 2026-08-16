"use client";
import PrettyDateInput from "../pretty-date-input";
import { CustomSelect } from "../custom-select";

import { useCallback, useEffect, useMemo, useState } from "react";
import Breadcrumbs from "../breadcrumbs";
import FavoriteButton from "../favorite-button";
import { IconBriefcase, IconClock, IconRefresh, IconTag, IconWallet } from "../icons";
import { useAuth } from "../auth-state";
import { useSiteSettings } from "../site-settings";
import { useInfiniteScroll } from "../use-infinite-scroll";
import ViewToggle, { CatalogView } from "../view-toggle";
import { ProjectCardSkeleton, ProjectsCatalogSkeleton } from "../skeletons";

type Category = { id: string; slug: string; name: string };
type Project = { id: string; title: string; description: string; budget: { type: string; min_kopecks?: number; max_kopecks?: number; currency: string }; deadline_at?: string; created_at?: string; experience_level?: string; category?: { id?: string; slug?: string; name: string }; skills: Array<{ id: string; name: string }>; proposal_count?: number; customer_display_name?: string };
const experienceLabels: Record<string, string> = { BEGINNER: "Начинающий", INTERMEDIATE: "Средний", ADVANCED: "Опытный", EXPERT: "Эксперт" };
const money = (value?: number) => value === undefined ? "" : `${new Intl.NumberFormat("ru-RU").format(value / 100)} ₽`;
const budgetText = (item: Project) => item.budget.type === "NEGOTIABLE" ? "Бюджет по договорённости" : item.budget.type === "FIXED" ? money(item.budget.min_kopecks) : `${money(item.budget.min_kopecks)} — ${money(item.budget.max_kopecks)}${item.budget.type === "HOURLY" ? " / час" : ""}`;
const budgetValue = (item: Project) => item.budget.type === "NEGOTIABLE" ? undefined : item.budget.max_kopecks ?? item.budget.min_kopecks;

const proposalLabel=(count:number)=>{const mod100=count%100,mod10=count%10;const word=mod100>=11&&mod100<=14?"откликов":mod10===1?"отклик":mod10>=2&&mod10<=4?"отклика":"откликов";return `${count} ${word}`};
const plainDescription = (value: string) => value
  .replace(/<br\s*\/?\s*>/gi, " ")
  .replace(/<\/(?:p|div|h[1-6]|li|ul|ol)>/gi, " ")
  .replace(/<[^>]+>/g, " ")
  .replace(/&nbsp;/gi, " ")
  .replace(/&amp;/gi, "&")
  .replace(/&lt;/gi, "<")
  .replace(/&gt;/gi, ">")
  .replace(/&quot;/gi, '\"')
  .replace(/&#39;|&apos;/gi, "'")
  .replace(/&#(\d+);/g, (_, code: string) => String.fromCodePoint(Number(code)))
  .replace(/&#x([0-9a-f]+);/gi, (_, code: string) => String.fromCodePoint(Number.parseInt(code, 16)))
  .replace(/\s+/g, " ")
  .trim();

export default function ProjectsPage() {
  const { user } = useAuth();
  const { catalog_page_size: pageSize } = useSiteSettings();
  const [query, setQuery] = useState("");
  const [category, setCategory] = useState("");
  const [budgetType, setBudgetType] = useState("");
  const [experience, setExperience] = useState("");
  const [minBudget, setMinBudget] = useState("");
  const [deadline, setDeadline] = useState("");
  const [sort, setSort] = useState("newest");
  const [view, setView] = useState<CatalogView>("grid");
  const [items, setItems] = useState<Project[]>([]);
  const [categories, setCategories] = useState<Category[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [cursor, setCursor] = useState<string | null>(null);
  const [failed, setFailed] = useState(false);
  const [updatedAt, setUpdatedAt] = useState<Date | null>(null);

  useEffect(() => { fetch("/api/v1/categories").then((r) => r.ok ? r.json() : { data: [] }).then((b) => setCategories(b.data ?? [])).catch(() => undefined); }, []);

  const load = useCallback(async (nextCursor?: string, append = false, background = false) => {
    const params = new URLSearchParams();
    if (query.trim()) params.set("q", query.trim());
    if (category) params.set("category", category);
    if (budgetType) params.set("budget_type", budgetType);
    if (experience) params.set("experience_level", experience);
    const minimum = Number(minBudget || 0);
    if (Number.isFinite(minimum) && minimum > 0) params.set("min_budget_kopecks", String(Math.round(minimum * 100)));
    if (deadline) params.set("deadline_before", deadline);
    params.set("limit", String(pageSize));
    if (nextCursor) params.set("cursor", nextCursor);
    if (append) setLoadingMore(true); else if (!background) setLoading(true);
    if (!background) setFailed(false);
    try {
      const response = await fetch(`/api/v1/projects?${params}`, { cache: "no-store" });
      if (!response.ok) throw new Error();
      const body = await response.json();
      const next:Project[]=body.data??[];
      setItems((current) => {
        if (append) return [...current, ...next.filter((item) => !current.some((existing) => existing.id === item.id))];
        if (!background) return next;
        const freshIds = new Set(next.map((item) => item.id));
        return [...next, ...current.filter((item) => !freshIds.has(item.id))];
      });
      if (!background) setCursor(body.page?.next_cursor ?? null);
      setUpdatedAt(new Date());
    } catch { if (!append && !background) { setItems([]); setFailed(true); } }
    finally { if (!background) setLoading(false); if (append) setLoadingMore(false); }
  }, [query, category, budgetType, experience, minBudget, deadline, pageSize]);

  useEffect(() => { const timer = window.setTimeout(() => void load(), 220); return () => window.clearTimeout(timer); }, [load]);
  useEffect(() => { const refresh=()=>{if(document.visibilityState==="visible"&&!loading&&!loadingMore)void load(undefined,false,true)};const timer=window.setInterval(refresh,15000);document.addEventListener("visibilitychange",refresh);return()=>{window.clearInterval(timer);document.removeEventListener("visibilitychange",refresh)}},[loading,loadingMore,load]);
  const loadMore=useCallback(()=>{if(cursor&&!loadingMore)void load(cursor,true)},[cursor,load,loadingMore]);
  const sentinel=useInfiniteScroll(!!cursor,loadingMore,loadMore);

  const filtered = useMemo(() => {
    const list = [...items];
    if (sort === "budget_desc") list.sort((a, b) => (budgetValue(b) ?? -1) - (budgetValue(a) ?? -1));
    else if (sort === "budget_asc") list.sort((a, b) => (budgetValue(a) ?? Number.POSITIVE_INFINITY) - (budgetValue(b) ?? Number.POSITIVE_INFINITY));
    else list.sort((a, b) => new Date(b.created_at ?? 0).getTime() - new Date(a.created_at ?? 0).getTime());
    return list;
  }, [items, sort]);

  return <main>
    <Breadcrumbs items={[{ label: "Главная", href: "/" }, { label: "Проекты" }]} />
    <div className="page-heading"><div><p className="eyebrow">Открытые проекты</p><h1>Проекты</h1><p className="lead">Задачи заказчиков с бюджетом, сроками, навыками и прозрачным контекстом.</p></div>{user?.capabilities?.includes("CUSTOMER") ? <a className="button" href="/create-project">Разместить задачу</a> : null}</div>
    <section className="filters filters--expanded" aria-label="Фильтры проектов">
      <label>Поиск<input type="search" maxLength={120} placeholder="Flutter, дизайн, SEO…" value={query} onChange={(e) => setQuery(e.target.value)} /></label>
      <label>Категория<CustomSelect value={category} onChange={(e) => setCategory(e.target.value)}><option value="">Все категории</option>{categories.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</CustomSelect></label>
      <label>Бюджет<CustomSelect value={budgetType} onChange={(e) => setBudgetType(e.target.value)}><option value="">Любой тип</option><option value="FIXED">Фиксированный</option><option value="RANGE">Диапазон</option><option value="HOURLY">Почасовой</option><option value="NEGOTIABLE">По договорённости</option></CustomSelect></label>
      <label>Опыт<CustomSelect value={experience} onChange={(e) => setExperience(e.target.value)}><option value="">Любой</option>{Object.entries(experienceLabels).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</CustomSelect></label>
      <label>Минимальный бюджет, ₽<input type="number" min="0" step="1000" value={minBudget} onChange={(e) => setMinBudget(e.target.value)} placeholder="Например, 50000" /></label>
      <label>Срок до<PrettyDateInput value={deadline} onChange={setDeadline} ariaLabel="Срок проекта до" /></label>
    </section>
    {loading ? <ProjectsCatalogSkeleton count={6} view={view} /> : failed ? <div className="error">Не удалось загрузить проекты. Попробуйте ещё раз.</div> : filtered.length === 0 ? <div className="empty"><h2>Проекты не найдены</h2><p>Измените фильтры или вернитесь позже — каталог обновляется автоматически.</p></div> : <>
      <div className="list-toolbar"><div><p className="list-count">Загружено: <strong>{filtered.length}</strong></p><small className="live-refresh"><IconRefresh size={14}/>{updatedAt ? `Обновлено ${updatedAt.toLocaleTimeString("ru-RU", { hour: "2-digit", minute: "2-digit" })}` : "Каталог загружен"}</small></div><div className="toolbar-controls"><ViewToggle value={view} onChange={setView}/><label className="list-sort">Сортировка<CustomSelect value={sort} onChange={(e) => setSort(e.target.value)}><option value="newest">Сначала новые</option><option value="budget_desc">Бюджет: больше</option><option value="budget_asc">Бюджет: меньше</option></CustomSelect></label></div></div>
      <ul className={`project-catalog-list project-catalog-list--${view}`}>{filtered.map((item) => <li key={item.id}><article className="list-card project-catalog-card"><div className="card-corner-action"><FavoriteButton type="PROJECT" id={item.id} compact /></div><p className="list-card__eyebrow"><IconTag size={14}/>{item.category?.name ?? "Без категории"}{item.experience_level ? ` · ${experienceLabels[item.experience_level] ?? item.experience_level}` : ""}</p><h2><a href={`/projects/${item.id}`}>{item.title}</a></h2><p className="list-card__desc">{plainDescription(item.description)}</p><div className="project-card-facts"><strong className="catalog-price"><IconWallet size={16}/>{budgetText(item)}</strong>{item.deadline_at ? <span><IconClock size={16}/>до {new Intl.DateTimeFormat("ru-RU").format(new Date(item.deadline_at))}</span> : null}{item.proposal_count !== undefined ? <span><IconBriefcase size={16}/>{proposalLabel(item.proposal_count)}</span> : null}</div>{item.skills?.length ? <div className="chip-row">{item.skills.slice(0, 6).map((skill) => <span className="chip" key={skill.id}>{skill.name}</span>)}</div> : null}<a className="card-link" href={`/projects/${item.id}`}>Открыть проект →</a></article></li>)}</ul>
      <div ref={sentinel} className="infinite-loader" aria-live="polite">{loadingMore?<><span className="spinner"/><span>Загружаем ещё проекты…</span><div className="infinite-loader__skeletons"><ProjectCardSkeleton/><ProjectCardSkeleton/></div></>:cursor?<span>Прокрутите ниже, чтобы увидеть ещё</span>:<span>Вы посмотрели все проекты</span>}</div>
    </>}
  </main>;
}
