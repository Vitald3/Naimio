"use client";
import { CustomSelect } from "../custom-select";

import { useCallback, useEffect, useMemo, useState } from "react";
import Breadcrumbs from "../breadcrumbs";
import { Avatar, Cover, PresenceLabel } from "../media-components";
import FavoriteButton from "../favorite-button";
import { IconBook, IconClock, IconRefresh, IconTag, IconWallet } from "../icons";
import ViewToggle, { CatalogView } from "../view-toggle";
import { countLabel } from "../russian-plural";
import { EducationCatalogSkeleton } from "../skeletons";

type Category = { id: string; name: string };
type Service = {
  id: string;
  slug?: string;
  title: string;
  short_description?: string;
  service_type: string;
  price_type: string;
  price_from?: { amount_kopecks: number };
  category?: { id?: string; name: string };
  education_details?: {
    format: string;
    audience_type?: string;
    duration_minutes?: number;
    sessions_count?: number;
  };
  seller_display_name?: string;
  seller_username?: string;
  seller_native_rating?: number;
  seller_reviews_count?: number;
};
const formatLabels: Record<string, string> = {
  ONLINE: "Онлайн",
  OFFLINE: "Очно",
  ASYNC: "Асинхронно",
  HYBRID: "Смешанно",
};
const audienceLabels: Record<string, string> = {
  INDIVIDUAL: "Индивидуально",
  GROUP: "Группа",
  BOTH: "Индивидуально или группа",
};
const priceText = (v: Service) =>
  v.price_type === "NEGOTIABLE" || !v.price_from
    ? "Цена по договорённости"
    : `${new Intl.NumberFormat("ru-RU").format(v.price_from.amount_kopecks / 100)} ₽`;
const priceValue = (v: Service) =>
  v.price_type === "NEGOTIABLE" || !v.price_from
    ? undefined
    : v.price_from.amount_kopecks;
export default function EducationPage() {
  const [type, setType] = useState("EDUCATION");
  const [q, setQ] = useState("");
  const [category, setCategory] = useState("");
  const [format, setFormat] = useState("");
  const [audience, setAudience] = useState("");
  const [priceType, setPriceType] = useState("");
  const [maxDuration, setMaxDuration] = useState("");
  const [sort, setSort] = useState("relevance");
  const [view, setView] = useState<CatalogView>("grid");
  const [items, setItems] = useState<Service[]>([]);
  const [categories, setCategories] = useState<Category[]>([]);
  const [loading, setLoading] = useState(true);
  const [failed, setFailed] = useState(false);
  const [updatedAt, setUpdatedAt] = useState<Date | null>(null);
  useEffect(() => {
    fetch("/api/v1/categories")
      .then((r) => (r.ok ? r.json() : { data: [] }))
      .then((b) => setCategories(b.data ?? []))
      .catch(() => undefined);
  }, []);
  const load = useCallback(async (background = false) => {
    if (!background) setLoading(true);
    if (!background) setFailed(false);
    const p = new URLSearchParams({ service_type: type });
    if (q.trim()) p.set("q", q.trim());
    if (category) p.set("category", category);
    if (format) p.set("format", format);
    if (audience) p.set("audience", audience);
    if (priceType) p.set("price_type", priceType);
    const minutes = Number(maxDuration || 0);
    if (Number.isFinite(minutes) && minutes > 0)
      p.set("max_duration_minutes", String(Math.round(minutes)));
    try {
      const r = await fetch(`/api/v1/services?${p}`, { cache: "no-store" });
      if (!r.ok) throw new Error();
      const b = await r.json();
      const next: Service[] = b.data ?? [];
      setItems((current) => {
        if (!background) return next;
        const freshIds = new Set(next.map((item) => item.id));
        return [...next, ...current.filter((item) => !freshIds.has(item.id))];
      });
      setUpdatedAt(new Date());
    } catch {
      if (!background) {
        setItems([]);
        setFailed(true);
      }
    } finally {
      if (!background) setLoading(false);
    }
  }, [type, q, category, format, audience, priceType, maxDuration]);
  useEffect(() => {
    const t = setTimeout(() => void load(), 220);
    return () => clearTimeout(t);
  }, [load]);
  useEffect(() => {
    const refresh = () => {
      if (document.visibilityState === "visible" && !loading) void load(true);
    };
    const timer = window.setInterval(refresh, 15000);
    document.addEventListener("visibilitychange", refresh);
    return () => {
      window.clearInterval(timer);
      document.removeEventListener("visibilitychange", refresh);
    };
  }, [load, loading]);
  const sorted = useMemo(() => {
    const list = [...items];
    if (sort === "price_asc")
      list.sort(
        (a, b) => (priceValue(a) ?? Infinity) - (priceValue(b) ?? Infinity),
      );
    else if (sort === "price_desc")
      list.sort((a, b) => (priceValue(b) ?? -1) - (priceValue(a) ?? -1));
    else if (sort === "duration")
      list.sort(
        (a, b) =>
          (a.education_details?.duration_minutes ?? Infinity) -
          (b.education_details?.duration_minutes ?? Infinity),
      );
    return list;
  }, [items, sort]);
  return (
    <main>
      <Breadcrumbs
        items={[
          { label: "Главная", href: "/" },
          { label: "Услуги", href: "/services" },
          { label: "Обучение и наставничество" },
        ]}
      />
      <div className="page-heading">
        <div>
          <p className="eyebrow">Развитие навыков</p>
          <h1>Обучение и наставничество</h1>
          <p className="lead">
            Обучение, консультации и наставничество от практикующих специалистов
            — как услуги экспертов, без LMS и учебной платформы.
          </p>
        </div>
        <a className="button button--quiet" href="/dashboard/services/new">
          Создать предложение
        </a>
      </div>
      <section
        className="filters filters--expanded"
        aria-label="Фильтры обучения"
      >
        <label>
          Поиск
          <input
            type="search"
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="Figma, Go, маркетинг…"
          />
        </label>
        <label>
          Тип
          <CustomSelect value={type} onChange={(e) => setType(e.target.value)}>
            <option value="EDUCATION">Обучение</option>
            <option value="MENTORING">Наставничество</option>
            <option value="CONSULTATION">Консультации</option>
          </CustomSelect>
        </label>
        <label>
          Категория
          <CustomSelect
            value={category}
            onChange={(e) => setCategory(e.target.value)}
          >
            <option value="">Все категории</option>
            {categories.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name}
              </option>
            ))}
          </CustomSelect>
        </label>
        <label>
          Проведение
          <CustomSelect
            value={format}
            onChange={(e) => setFormat(e.target.value)}
          >
            <option value="">Любое</option>
            {Object.entries(formatLabels).map(([v, l]) => (
              <option key={v} value={v}>
                {l}
              </option>
            ))}
          </CustomSelect>
        </label>
        <label>
          Аудитория
          <CustomSelect
            value={audience}
            onChange={(e) => setAudience(e.target.value)}
          >
            <option value="">Любая</option>
            <option value="INDIVIDUAL">Индивидуально</option>
            <option value="GROUP">Группа</option>
          </CustomSelect>
        </label>
        <label>
          Цена
          <CustomSelect
            value={priceType}
            onChange={(e) => setPriceType(e.target.value)}
          >
            <option value="">Любая</option>
            <option value="FIXED">Фиксированная</option>
            <option value="FROM">От указанной</option>
            <option value="HOURLY">Почасовая</option>
            <option value="NEGOTIABLE">По договорённости</option>
          </CustomSelect>
        </label>
        <label>
          Длительность до, минут
          <input
            type="number"
            min="0"
            step="15"
            value={maxDuration}
            onChange={(e) => setMaxDuration(e.target.value)}
            placeholder="120"
          />
        </label>
      </section>
      {loading ? (
        <EducationCatalogSkeleton count={6} />
      ) : failed ? (
        <div className="error">Не удалось загрузить предложения.</div>
      ) : sorted.length === 0 ? (
        <div className="empty">
          <h2>Предложений пока нет</h2>
          <p>Измените фильтры или создайте собственное предложение.</p>
        </div>
      ) : (
        <>
          <div className="list-toolbar">
            <div>
              <p className="list-count">
                Найдено: <strong>{sorted.length}</strong>
              </p>
              <small className="live-refresh"><IconRefresh size={14}/>{updatedAt ? `Обновлено ${updatedAt.toLocaleTimeString("ru-RU", { hour: "2-digit", minute: "2-digit" })}` : "Каталог загружен"}</small>
            </div>
            <div className="toolbar-controls"><ViewToggle value={view} onChange={setView}/><label className="list-sort">
              Сортировка
              <CustomSelect
                value={sort}
                onChange={(e) => setSort(e.target.value)}
              >
                <option value="relevance">По релевантности</option>
                <option value="price_asc">Сначала дешевле</option>
                <option value="price_desc">Сначала дороже</option>
                <option value="duration">По длительности</option>
              </CustomSelect>
            </label></div>
          </div>
          <ul className={`education-grid education-grid--${view}`}>
            {sorted.map((v) => (
              <li key={v.id}>
                <article className="service-catalog-card education-card">
                  <div className="service-catalog-card__cover">
                    <Cover id={v.id} title={v.title} type={v.service_type} />
                    <div className="card-corner-action">
                      <FavoriteButton type="SERVICE" id={v.id} compact />
                    </div>
                  </div>
                  <div className="service-catalog-card__body">
                    <p className="list-card__eyebrow">
                      <IconBook size={15} />
                      {v.service_type === "MENTORING"
                        ? "Наставничество"
                        : v.service_type === "CONSULTATION"
                          ? "Консультация"
                          : "Обучение"}
                      {v.category?.name ? ` · ${v.category.name}` : ""}
                    </p>
                    <h2>
                      <a href={`/services/${v.slug || v.id}`}>{v.title}</a>
                    </h2>
                    {v.short_description ? (
                      <p className="list-card__desc">{v.short_description}</p>
                    ) : null}
                    <div className="project-card-facts">
                      <strong className="catalog-price">
                        <IconWallet size={16} />
                        {priceText(v)}
                      </strong>
                      {v.education_details?.duration_minutes ? (
                        <span>
                          <IconClock size={16} />
                          {v.education_details.duration_minutes} мин.
                        </span>
                      ) : null}
                      {v.education_details?.format ? (
                        <span>
                          <IconTag size={16} />
                          {formatLabels[v.education_details.format] ??
                            v.education_details.format}
                        </span>
                      ) : null}
                    </div>
                    {v.education_details?.audience_type ? (
                      <p className="card-meta">
                        {audienceLabels[v.education_details.audience_type] ??
                          v.education_details.audience_type}
                      </p>
                    ) : null}
                    {v.seller_display_name ? (
                      <div className="card-meta card-meta--person">
                        <Avatar
                          name={v.seller_display_name}
                          id={v.seller_username || v.id}
                          size="sm"
                        />
                        <span>
                          Ведёт: <strong>{v.seller_display_name}</strong>
                          {v.seller_native_rating
                            ? ` · ★ ${v.seller_native_rating.toFixed(1)} · ${countLabel(v.seller_reviews_count ?? 0,["отзыв","отзыва","отзывов"])}`
                            : " · Новый профиль"}
                        </span>
                        <PresenceLabel id={v.seller_username || v.id}/>
                      </div>
                    ) : null}
                    <a
                      className="card-link"
                      href={`/services/${v.slug || v.id}`}
                    >
                      Подробнее →
                    </a>
                  </div>
                </article>
              </li>
            ))}
          </ul>
        </>
      )}
    </main>
  );
}
