"use client";
import { CustomSelect } from "../custom-select";

import { useCallback, useEffect, useMemo, useState } from "react";
import Breadcrumbs from "../breadcrumbs";
import FavoriteButton from "../favorite-button";
import { Avatar, Cover, PresenceLabel } from "../media-components";
import { IconClock, IconRefresh, IconTag, IconWallet } from "../icons";
import { useSiteSettings } from "../site-settings";
import { useInfiniteScroll } from "../use-infinite-scroll";
import ViewToggle, { CatalogView } from "../view-toggle";
import { countLabel } from "../russian-plural";
import { ServiceCardSkeleton, ServicesCatalogSkeleton } from "../skeletons";

type Category = { id: string; name: string };
type Service = { id: string; slug?: string; title: string; short_description?: string; service_type: string; price_type: string; price_from?: { amount_kopecks: number; currency: string }; delivery_days?: number; seller_username?: string; seller_display_name?: string; seller_native_rating?: number; seller_reviews_count?: number; category: { id?: string; name: string }; education_details?: { format?: string; audience_type?: string } };
const typeLabels: Record<string, string> = { PROFESSIONAL_SERVICE: "Услуга", CONSULTATION: "Консультация", EDUCATION: "Обучение", MENTORING: "Наставничество" };
const formatPrice = (item: Service) => { if (item.price_type === "NEGOTIABLE" || !item.price_from) return "Цена по договорённости"; const amount = new Intl.NumberFormat("ru-RU").format(item.price_from.amount_kopecks / 100); return item.price_type === "FROM" ? `от ${amount} ₽` : item.price_type === "HOURLY" ? `${amount} ₽/час` : `${amount} ₽`; };
const priceValue = (item: Service) => item.price_type === "NEGOTIABLE" || !item.price_from ? undefined : item.price_from.amount_kopecks;

export default function ServicesPage() {
  const { catalog_page_size: pageSize } = useSiteSettings();
  const [search, setSearch] = useState("");
  const [category, setCategory] = useState("");
  const [serviceType, setServiceType] = useState("");
  const [priceType, setPriceType] = useState("");
  const [format, setFormat] = useState("");
  const [sort, setSort] = useState("relevance");
  const [view, setView] = useState<CatalogView>("grid");
  const [items, setItems] = useState<Service[]>([]);
  const [categories, setCategories] = useState<Category[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [cursor, setCursor] = useState<string | null>(null);
  const [failed, setFailed] = useState(false);
  const [updatedAt, setUpdatedAt] = useState<Date | null>(null);

  useEffect(() => { fetch("/api/v1/categories").then((r) => r.ok ? r.json() : { data: [] }).then((b) => setCategories(b.data ?? [])).catch(() => undefined); }, []);
  const load = useCallback(async (nextCursor?: string, append = false, background = false) => {
    const query = new URLSearchParams(); if (search.trim()) query.set("q", search.trim()); if (category) query.set("category", category); if (serviceType) query.set("service_type", serviceType); if (priceType) query.set("price_type", priceType); if (format) query.set("format", format);
    query.set("limit", String(pageSize)); if (nextCursor) query.set("cursor", nextCursor);
    if (append) setLoadingMore(true); else if (!background) setLoading(true); if (!background) setFailed(false);
    try { const response = await fetch(`/api/v1/services?${query}`, { cache: "no-store" }); if (!response.ok) throw new Error(); const body = await response.json(); const next:Service[]=body.data??[]; setItems((current) => { if (append) return [...current, ...next.filter((item) => !current.some((existing) => existing.id === item.id))]; if (!background) return next; const freshIds = new Set(next.map((item) => item.id)); return [...next, ...current.filter((item) => !freshIds.has(item.id))]; }); if (!background) setCursor(body.page?.next_cursor??null); setUpdatedAt(new Date()); }
    catch { if (!append && !background) { setItems([]); setFailed(true); } }
    finally { if (!background) setLoading(false); if (append) setLoadingMore(false); }
  }, [search, category, serviceType, priceType, format, pageSize]);
  useEffect(() => { const timer = window.setTimeout(() => void load(), 220); return () => window.clearTimeout(timer); }, [load]);
  useEffect(() => { const refresh=()=>{if(document.visibilityState==="visible"&&!loading&&!loadingMore)void load(undefined,false,true)};const timer=window.setInterval(refresh,15000);document.addEventListener("visibilitychange",refresh);return()=>{window.clearInterval(timer);document.removeEventListener("visibilitychange",refresh)}},[loading,loadingMore,load]);
  const loadMore=useCallback(()=>{if(cursor&&!loadingMore)void load(cursor,true)},[cursor,load,loadingMore]);
  const sentinel=useInfiniteScroll(!!cursor,loadingMore,loadMore);

  const sorted = useMemo(() => { const list = [...items]; if (sort === "price_asc") list.sort((a,b)=>(priceValue(a)??Infinity)-(priceValue(b)??Infinity)); else if(sort === "price_desc") list.sort((a,b)=>(priceValue(b)??-1)-(priceValue(a)??-1)); else if(sort === "delivery") list.sort((a,b)=>(a.delivery_days??Infinity)-(b.delivery_days??Infinity)); return list; }, [items, sort]);

  return <main>
    <Breadcrumbs items={[{ label: "Главная", href: "/" }, { label: "Услуги" }]} />
    <div className="page-heading"><div><p className="eyebrow">Каталог предложений</p><h1>Услуги</h1><p className="lead">Готовые услуги, консультации, обучение и наставничество от специалистов Naimio.</p></div><a className="button button--quiet" href="/dashboard/services/new">Создать услугу</a></div>
    <section className="filters filters--expanded" aria-label="Фильтры услуг"><label>Поиск<input type="search" value={search} maxLength={120} onChange={(e)=>setSearch(e.target.value)} placeholder="Например, UX-аудит или Flutter"/></label><label>Категория<CustomSelect value={category} onChange={(e)=>setCategory(e.target.value)}><option value="">Все категории</option>{categories.map((item)=><option key={item.id} value={item.id}>{item.name}</option>)}</CustomSelect></label><label>Тип<CustomSelect value={serviceType} onChange={(e)=>setServiceType(e.target.value)}><option value="">Все типы</option>{Object.entries(typeLabels).map(([value,label])=><option value={value} key={value}>{label}</option>)}</CustomSelect></label><label>Цена<CustomSelect value={priceType} onChange={(e)=>setPriceType(e.target.value)}><option value="">Любая</option><option value="FIXED">Фиксированная</option><option value="FROM">От указанной</option><option value="HOURLY">Почасовая</option><option value="NEGOTIABLE">По договорённости</option></CustomSelect></label><label>Формат<CustomSelect value={format} onChange={(e)=>setFormat(e.target.value)}><option value="">Любой</option><option value="ONLINE">Онлайн</option><option value="OFFLINE">Очно</option><option value="ASYNC">Асинхронно</option><option value="HYBRID">Смешанно</option></CustomSelect></label></section>
    {loading ? <ServicesCatalogSkeleton count={6} /> : failed ? <div className="error">Не удалось загрузить каталог услуг.</div> : sorted.length === 0 ? <div className="empty"><h2>Услуги не найдены</h2><p>Измените фильтры или вернитесь позже.</p></div> : <><div className="list-toolbar"><div><p className="list-count">Загружено: <strong>{sorted.length}</strong></p><small className="live-refresh"><IconRefresh size={14}/>{updatedAt ? `Обновлено ${updatedAt.toLocaleTimeString("ru-RU",{hour:"2-digit",minute:"2-digit"})}` : "Каталог загружен"}</small></div><div className="toolbar-controls"><ViewToggle value={view} onChange={setView}/><label className="list-sort">Сортировка<CustomSelect value={sort} onChange={(e)=>setSort(e.target.value)}><option value="relevance">По релевантности</option><option value="price_asc">Сначала дешевле</option><option value="price_desc">Сначала дороже</option><option value="delivery">Быстрее выполнят</option></CustomSelect></label></div></div><ul className={`service-catalog-grid service-catalog-grid--${view}`}>{sorted.map((item)=><li key={item.id}><article className="service-catalog-card"><div className="service-catalog-card__cover"><Cover id={item.id} title={item.title} type={item.service_type}/><div className="card-corner-action"><FavoriteButton type="SERVICE" id={item.id} compact/></div></div><div className="service-catalog-card__body"><p className="list-card__eyebrow"><IconTag size={14}/>{typeLabels[item.service_type]??item.service_type} · {item.category.name}</p><h2><a href={`/services/${item.slug||item.id}`}>{item.title}</a></h2>{item.short_description?<p className="list-card__desc">{item.short_description}</p>:null}<div className="project-card-facts"><strong className="catalog-price"><IconWallet size={16}/>{formatPrice(item)}</strong>{item.delivery_days?<span><IconClock size={16}/>{item.delivery_days} дн.</span>:null}</div>{item.seller_username?<div className="card-meta card-meta--person"><Avatar name={item.seller_display_name||item.seller_username} id={item.seller_username} size="sm"/><div><a href={`/freelancers/${item.seller_username}`}><strong>{item.seller_display_name||item.seller_username}</strong></a><span className="seller-trust-line">{item.seller_native_rating ? `★ ${item.seller_native_rating.toFixed(1)} · ${countLabel(item.seller_reviews_count ?? 0,["отзыв","отзыва","отзывов"])}` : "Новый профиль"}</span><PresenceLabel id={item.seller_username}/></div></div>:null}<a className="card-link" href={`/services/${item.slug||item.id}`}>Подробнее →</a></div></article></li>)}</ul><div ref={sentinel} className="infinite-loader" aria-live="polite">{loadingMore?<><span className="spinner"/><span>Загружаем ещё услуги…</span><div className="infinite-loader__skeletons"><ServiceCardSkeleton/><ServiceCardSkeleton/></div></>:cursor?<span>Прокрутите ниже, чтобы увидеть ещё</span>:<span>Вы посмотрели все услуги</span>}</div></>}
  </main>;
}
