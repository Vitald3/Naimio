"use client";
import { CustomSelect } from "../custom-select";

import { useCallback, useEffect, useMemo, useState } from "react";
import Breadcrumbs from "../breadcrumbs";
import {
  IconBriefcase,
  IconGrid,
  IconList,
  IconMapPin,
  IconRefresh,
} from "../icons";
import { useAuth } from "../auth-state";
import { useSiteSettings } from "../site-settings";
import { useInfiniteScroll } from "../use-infinite-scroll";
import { cityOptions } from "../location-options";
import { VacanciesCatalogSkeleton, VacancyCardSkeleton } from "../skeletons";

type Category = { id: string; name: string };
type Vacancy = {
  id: string;
  slug?: string;
  title: string;
  company?: { name: string };
  category?: { id?: string; name: string };
  employment_type: string;
  salary_min_kopecks?: number;
  salary_max_kopecks?: number;
  remote: boolean;
  location?: string;
  experience_level?: string;
  skills: Array<{ id: string; name: string }>;
  published_at: string;
};
const employment: Record<string, string> = {
  FULL_TIME: "Полная занятость",
  PART_TIME: "Частичная занятость",
  CONTRACT: "Контракт",
  INTERNSHIP: "Стажировка",
};
const experience: Record<string, string> = {
  JUNIOR: "Junior",
  MIDDLE: "Middle",
  SENIOR: "Senior",
  LEAD: "Lead",
  ANY: "Любой уровень",
};
const salary = (v: Vacancy) => {
  const f = (n?: number) =>
    n ? new Intl.NumberFormat("ru-RU").format(n / 100) : "";
  if (!v.salary_min_kopecks && !v.salary_max_kopecks)
    return "Зарплата по договорённости";
  return `${v.salary_min_kopecks ? `от ${f(v.salary_min_kopecks)}` : ""}${v.salary_max_kopecks ? ` до ${f(v.salary_max_kopecks)}` : ""} ₽`;
};
const salaryValue = (v: Vacancy) =>
  v.salary_max_kopecks ?? v.salary_min_kopecks;

export default function VacanciesPage() {
  const { user } = useAuth();
  const { catalog_page_size: pageSize } = useSiteSettings();
  const [q, setQ] = useState("");
  const [category, setCategory] = useState("");
  const [type, setType] = useState("");
  const [remote, setRemote] = useState("");
  const [location, setLocation] = useState("");
  const [level, setLevel] = useState("");
  const [minSalary, setMinSalary] = useState("");
  const [sort, setSort] = useState("newest");
  const [view, setView] = useState<"list" | "grid">("list");
  const [items, setItems] = useState<Vacancy[]>([]);
  const [categories, setCategories] = useState<Category[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [cursor, setCursor] = useState<string | null>(null);
  const [failed, setFailed] = useState(false);
  const [updatedAt, setUpdatedAt] = useState<Date | null>(null);
  useEffect(() => {
    fetch("/api/v1/categories")
      .then((r) => (r.ok ? r.json() : { data: [] }))
      .then((b) => setCategories(b.data ?? []))
      .catch(() => undefined);
    const stored = localStorage.getItem("vacancy-view");
    if (stored === "grid" || stored === "list") setView(stored);
  }, []);
  function chooseView(next: "list" | "grid") {
    setView(next);
    localStorage.setItem("vacancy-view", next);
  }
  const load = useCallback(
    async (nextCursor?: string, append = false, background = false) => {
      const p = new URLSearchParams();
      if (q.trim()) p.set("q", q.trim());
      if (category) p.set("category", category);
      if (type) p.set("employment_type", type);
      if (remote) p.set("remote", remote);
      if (location.trim()) p.set("location", location.trim());
      if (level) p.set("experience", level);
      if (minSalary && Number(minSalary) >= 0)
        p.set(
          "min_salary_kopecks",
          String(Math.round(Number(minSalary) * 100)),
        );
      p.set("sort", q ? "RELEVANCE" : "NEWEST");
      p.set("limit", String(pageSize));
      if (nextCursor) p.set("cursor", nextCursor);
      if (append) setLoadingMore(true); else if (!background) setLoading(true);
      if (!background) setFailed(false);
      try {
        const r = await fetch(`/api/v1/vacancies?${p}`, { cache: "no-store" });
        if (!r.ok) throw new Error();
        const b = await r.json();
        const next:Vacancy[]=b.data??[];
        setItems((current) => {
          if (append) return [...current, ...next.filter((item) => !current.some((existing) => existing.id === item.id))];
          if (!background) return next;
          const freshIds = new Set(next.map((item) => item.id));
          return [...next, ...current.filter((item) => !freshIds.has(item.id))];
        });
        if (!background) setCursor(b.page?.next_cursor ?? null);
        setUpdatedAt(new Date());
      } catch {
        if (!append && !background) {
          setItems([]);
          setFailed(true);
        }
      } finally {
        if (!background) setLoading(false);
        if (append) setLoadingMore(false);
      }
    },
    [q, category, type, remote, location, level, minSalary, pageSize],
  );
  useEffect(() => {
    const t = window.setTimeout(() => void load(), 220);
    return () => clearTimeout(t);
  }, [load]);
  useEffect(() => { const refresh=()=>{if(document.visibilityState==="visible"&&!loading&&!loadingMore)void load(undefined,false,true)};const timer=window.setInterval(refresh,15000);document.addEventListener("visibilitychange",refresh);return()=>{window.clearInterval(timer);document.removeEventListener("visibilitychange",refresh)}},[loading,loadingMore,load]);
  const loadMore=useCallback(()=>{if(cursor&&!loadingMore)void load(cursor,true)},[cursor,load,loadingMore]);
  const sentinel=useInfiniteScroll(!!cursor,loadingMore,loadMore);
  const sorted = useMemo(() => {
    const list = [...items];
    if (sort === "salary_desc")
      list.sort((a, b) => (salaryValue(b) ?? -1) - (salaryValue(a) ?? -1));
    else
      list.sort(
        (a, b) =>
          new Date(b.published_at ?? 0).getTime() -
          new Date(a.published_at ?? 0).getTime(),
      );
    return list;
  }, [items, sort]);
  const customer = user?.capabilities?.includes("CUSTOMER");
  const freelancer = user?.capabilities?.includes("FREELANCER");
  return (
    <main>
      <Breadcrumbs
        items={[{ label: "Главная", href: "/" }, { label: "Вакансии" }]}
      />
      <div className="page-heading">
        <div>
          <p className="eyebrow">Работа и найм</p>
          <h1>Вакансии</h1>
          <p className="lead">
            Постоянная работа и долгосрочное сотрудничество — отдельно от
            проектных заказов.
          </p>
        </div>
        {customer ? (
          <a className="button button--quiet" href="/dashboard/vacancies">
            Разместить вакансию
          </a>
        ) : null}
      </div>
      <section
        className="filters filters--expanded"
        aria-label="Фильтры вакансий"
      >
        <label>
          Поиск
          <input
            list="vacancy-city-options"
            type="search"
            maxLength={120}
            placeholder="Backend-разработчик"
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
        </label>
        <label>
          Категория
          <CustomSelect
            value={category}
            onChange={(e) => setCategory(e.target.value)}
          >
            <option value="">Все категории</option>
            {categories.map((c) => (
              <option value={c.id} key={c.id}>
                {c.name}
              </option>
            ))}
          </CustomSelect>
        </label>
        <label>
          Занятость
          <CustomSelect value={type} onChange={(e) => setType(e.target.value)}>
            <option value="">Любая</option>
            {Object.entries(employment).map(([v, l]) => (
              <option key={v} value={v}>
                {l}
              </option>
            ))}
          </CustomSelect>
        </label>
        <label>
          Формат
          <CustomSelect
            value={remote}
            onChange={(e) => setRemote(e.target.value)}
          >
            <option value="">Любой</option>
            <option value="true">Удалённо</option>
            <option value="false">На месте</option>
          </CustomSelect>
        </label>
        <label>
          Опыт
          <CustomSelect
            value={level}
            onChange={(e) => setLevel(e.target.value)}
          >
            <option value="">Любой</option>
            {Object.entries(experience).map(([v, l]) => (
              <option key={v} value={v}>
                {l}
              </option>
            ))}
          </CustomSelect>
        </label>
        <label>
          Город
          <input
            maxLength={160}
            value={location}
            onChange={(e) => setLocation(e.target.value)}
            placeholder="Москва"
          />
          <datalist id="vacancy-city-options">{cityOptions.map(city=><option value={city} key={city}/>)}</datalist>
        </label>
        <label>
          Зарплата от, ₽
          <input
            type="number"
            min="0"
            step="5000"
            value={minSalary}
            onChange={(e) => setMinSalary(e.target.value)}
            placeholder="150000"
          />
        </label>
      </section>
      {loading ? (
        <VacanciesCatalogSkeleton count={6} view={view} />
      ) : failed ? (
        <div className="error">Не удалось загрузить вакансии.</div>
      ) : sorted.length === 0 ? (
        <div className="empty">
          <h2>Вакансии не найдены</h2>
          <p>
            Измените фильтры или вернитесь позже — список обновляется
            автоматически.
          </p>
        </div>
      ) : (
        <>
          <div className="list-toolbar">
            <div>
              <p className="list-count">
                Загружено: <strong>{sorted.length}</strong>
              </p>
              <small className="live-refresh">
                <IconRefresh size={14} />
                {updatedAt
                    ? `Обновлено ${updatedAt.toLocaleTimeString("ru-RU", { hour: "2-digit", minute: "2-digit" })}`
                    : "Каталог загружен"}
              </small>
            </div>
            <div className="toolbar-controls">
              <div className="view-toggle" role="group" aria-label="Вид списка">
                <button
                  type="button"
                  className={view === "list" ? "is-active" : ""}
                  onClick={() => chooseView("list")}
                  aria-label="Список"
                  aria-pressed={view === "list"}
                >
                  <IconList size={18} />
                </button>
                <button
                  type="button"
                  className={view === "grid" ? "is-active" : ""}
                  onClick={() => chooseView("grid")}
                  aria-label="Плитка"
                  aria-pressed={view === "grid"}
                >
                  <IconGrid size={18} />
                </button>
              </div>
              <label className="list-sort">
                Сортировка
                <CustomSelect
                  value={sort}
                  onChange={(e) => setSort(e.target.value)}
                >
                  <option value="newest">Сначала новые</option>
                  <option value="salary_desc">Зарплата: больше</option>
                </CustomSelect>
              </label>
            </div>
          </div>
          <ul className={`vacancy-results vacancy-results--${view}`}>
            {sorted.map((v) => (
              <li key={v.id}>
                <VacancyCard vacancy={v} view={view} />
              </li>
            ))}
          </ul>
          <div ref={sentinel} className="infinite-loader" aria-live="polite">{loadingMore?<><span className="spinner"/><span>Загружаем ещё вакансии…</span><div className="infinite-loader__skeletons"><VacancyCardSkeleton view={view}/><VacancyCardSkeleton view={view}/></div></>:cursor?<span>Прокрутите ниже, чтобы увидеть ещё</span>:<span>Вы посмотрели все вакансии</span>}</div>
        </>
      )}
      <div className="muted-links">
        {customer ? <a href="/dashboard/vacancies">Мои вакансии</a> : null}
        {freelancer ? (
          <a href="/dashboard/job-applications">Мои отклики</a>
        ) : null}
      </div>
    </main>
  );
}
function VacancyCard({
  vacancy: v,
  view,
}: {
  vacancy: Vacancy;
  view: "list" | "grid";
}) {
  return (
    <article className={`vacancy-card vacancy-card--${view}`}>
      <div className="vacancy-card__logo" aria-hidden="true">
        <IconBriefcase size={23} />
        <span>{(v.company?.name ?? "Naimio").slice(0, 2).toUpperCase()}</span>
      </div>
      <div className="vacancy-card__body">
        <div className="vacancy-card__topline">
          <span>{v.company?.name ?? "Прямой работодатель"}</span>
          <span>{employment[v.employment_type] ?? v.employment_type}</span>
          {v.category?.name ? <span>{v.category.name}</span> : null}
        </div>
        <h2>
          <a href={`/vacancies/${v.slug || v.id}`}>{v.title}</a>
        </h2>
        <div className="vacancy-card__facts">
          <span>
            <IconMapPin size={16} />
            {v.remote ? "Удалённо" : v.location || "На месте"}
          </span>
          {v.experience_level ? (
            <span>
              <IconBriefcase size={16} />
              {experience[v.experience_level] ?? v.experience_level}
            </span>
          ) : null}
        </div>
        {v.skills.length ? (
          <div className="vacancy-card__skills">
            {v.skills.slice(0, view === "grid" ? 4 : 6).map((skill) => (
              <span className="chip" key={skill.id}>
                {skill.name}
              </span>
            ))}
          </div>
        ) : null}
      </div>
      <aside className="vacancy-card__aside">
        <strong>{salary(v)}</strong>
        <small>
          {v.remote ? "Удалённая работа" : v.location || "Работа на месте"}
        </small>
        <a
          className="button button--quiet button--compact"
          href={`/vacancies/${v.slug || v.id}`}
        >
          Смотреть вакансию
        </a>
      </aside>
    </article>
  );
}
