import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { canonical, jsonLD, missingMetadata, publicMetadata, summary } from "../../seo";
import { Avatar, Cover, PresenceLabel } from "../../media-components";
import Breadcrumbs from "../../breadcrumbs";
import FavoriteButton from "../../favorite-button";
import ServiceOrderAction from "./service-order-action";
import { countLabel } from "../../russian-plural";
import EngagementTracker from "../../engagement-tracker";

export const dynamic = "force-dynamic";

type Service = {
  id: string;
  seller_user_id?: string;
  title: string;
  description: string;
  short_description?: string;
  service_type: string;
  price_type: string;
  price_from?: { amount_kopecks: number; currency: string };
  delivery_days?: number;
  included_revisions?: number;
  seller_username?: string;
  seller_display_name?: string;
  seller_native_rating?: number;
  seller_reviews_count?: number;
  category: { name: string };
  skills: Array<{ id: string; name: string }>;
  education_details?: {
    format: string;
    duration_minutes?: number;
    sessions_count?: number;
    audience_type?: string;
    group_size_max?: number;
  };
};

const typeLabels: Record<string, string> = { PROFESSIONAL_SERVICE: "Услуга", CONSULTATION: "Консультация", EDUCATION: "Обучение", MENTORING: "Наставничество" };
const formatLabels: Record<string, string> = { ONLINE: "Онлайн", OFFLINE: "Очно", ASYNC: "Асинхронно", HYBRID: "Смешанно" };
const audienceLabels: Record<string, string> = { INDIVIDUAL: "Индивидуально", GROUP: "Группа" };
const formatPrice = (item: Service) => {
  if (item.price_type === "NEGOTIABLE" || !item.price_from) return "По договорённости";
  const amount = new Intl.NumberFormat("ru-RU").format(item.price_from.amount_kopecks / 100);
  if (item.price_type === "FROM") return `от ${amount} ₽`;
  if (item.price_type === "HOURLY") return `${amount} ₽/час`;
  return `${amount} ₽`;
};

async function loadService(id: string): Promise<Service | null> {
  const baseURL = process.env.API_BASE_URL ?? "http://localhost:8080";
  const response = await fetch(`${baseURL}/api/v1/services/${encodeURIComponent(id)}`, { cache: "no-store" });
  if (response.status === 404) return null;
  if (!response.ok) throw new Error("service request failed");
  const body = await response.json();
  return body.data ?? null;
}

export async function generateMetadata({params}:{params:Promise<{id:string}>}):Promise<Metadata>{const {id}=await params;const service=await loadService(id);if(!service)return missingMetadata("Услуга не найдена");return publicMetadata(service.title,summary(service.short_description||service.description,"Публичная услуга специалиста."),`/services/${service.id}`)}

export default async function ServicePage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const service = await loadService(id);
  if (!service) notFound();
  const education = service.education_details;
  const schema={"@context":"https://schema.org","@type":"Service",name:service.title,description:summary(service.description,service.title,500),url:canonical(`/services/${service.id}`),serviceType:typeLabels[service.service_type]??service.service_type,provider:service.seller_username?{"@type":"Person",name:service.seller_display_name||service.seller_username,url:canonical(`/freelancers/${service.seller_username}`)}:undefined,offers:service.price_from?{"@type":"Offer",priceCurrency:"RUB",price:service.price_from.amount_kopecks/100}:undefined};
  return <main>
    {service.seller_user_id ? <EngagementTracker eventType="SERVICE_VIEW" subjectUserID={service.seller_user_id} entityID={service.id}/> : null}
    <Breadcrumbs items={[{label:"Главная",href:"/"},{label:"Услуги",href:"/services"},{label:service.title}]}/>
    <header className="service-detail-hero"><Cover id={service.id} title={service.title} type={service.service_type}/><div><div className="detail-title-row"><p className="eyebrow">{typeLabels[service.service_type] ?? service.service_type} · {service.category.name}</p><FavoriteButton type="SERVICE" id={service.id}/></div><h1>{service.title}</h1>{service.short_description ? <p className="lead">{service.short_description}</p> : null}</div></header>
    <section><h2>Описание</h2><p>{service.description}</p></section>
    {service.skills?.length ? <section><h2>Навыки</h2><ul>{service.skills.map((skill) => <li key={skill.id}>{skill.name}</li>)}</ul></section> : null}
    <section className="service-purchase-panel"><div><p className="eyebrow">Оформление</p><h2>{education ? "Записаться на обучение" : "Заказать услугу"}</h2><p>Стоимость: <strong className="detail-price">{formatPrice(service)}</strong></p>{service.delivery_days ? <p>Срок: {service.delivery_days} дн.</p> : null}{service.included_revisions !== undefined ? <p>Включено правок: {service.included_revisions}</p> : null}</div>{service.seller_user_id?<ServiceOrderAction serviceID={service.id} sellerID={service.seller_user_id} education={!!education}/>:<p className="notice notice--error">Оформление временно недоступно: у предложения не указан исполнитель.</p>}<p className="form-hint">Нажмите кнопку, согласуйте детали с исполнителем в чате, затем оформите проект и безопасную сделку. Исполнителю откликаться на собственную услугу не нужно — заказ начинает покупатель.</p></section>
    {education ? <section><h2>Формат</h2><p>{formatLabels[education.format] ?? education.format}</p>{education.duration_minutes ? <p>Продолжительность: {education.duration_minutes} мин.</p> : null}{education.sessions_count ? <p>Количество занятий: {education.sessions_count}</p> : null}{education.audience_type ? <p>Аудитория: {audienceLabels[education.audience_type] ?? education.audience_type}</p> : null}{education.group_size_max ? <p>До {education.group_size_max} участников</p> : null}</section> : null}
    {service.seller_username ? <section className="seller-panel"><Avatar name={service.seller_display_name || service.seller_username} id={service.seller_username} size="lg"/><div><h2>Исполнитель</h2><p><a href={`/freelancers/${service.seller_username}`}><strong>{service.seller_display_name || service.seller_username}</strong></a></p><p className="card-meta">{service.seller_native_rating ? `★ ${service.seller_native_rating.toFixed(1)} · ${countLabel(service.seller_reviews_count ?? 0,["отзыв","отзыва","отзывов"])}` : "Новый профиль без отзывов"}</p><PresenceLabel id={service.seller_username}/><p><a className="button button--secondary" href={`/freelancers/${service.seller_username}`}>Открыть профиль</a></p></div></section> : null}
    <script type="application/ld+json" dangerouslySetInnerHTML={{__html:jsonLD(schema)}}/>
  </main>;
}
