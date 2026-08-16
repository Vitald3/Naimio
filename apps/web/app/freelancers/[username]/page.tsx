import type { Metadata } from "next";
import { Avatar, PresenceLabel } from "../../media-components";
import { notFound } from "next/navigation";
import { canonical, jsonLD, missingMetadata, publicMetadata, summary } from "../../seo";
import ReviewsSection, { type NativeTrust, type Review } from "../../reviews-section";
import Breadcrumbs from "../../breadcrumbs";
import FavoriteButton from "../../favorite-button";
import PortfolioGallery from "./portfolio-gallery";
import { russianPlural } from "../../russian-plural";
import ProBadge from "../../pro-badge";
import EngagementTracker from "../../engagement-tracker";

type Category = { id: string; slug: string; name: string; is_primary: boolean };
type Skill = { id: string; slug: string; name: string; level?: string; is_featured: boolean };
type Language = { code: string; level: string };
type PortfolioItem = {
  id: string;
  title: string;
  description?: string;
  external_url?: string;
  price_min_kopecks?: number;
  price_max_kopecks?: number;
  completed_on?: string;
  categories: Category[];
  skills: Skill[];
  media: Array<{id:string}>;
};
type Profile = {
  id: string;
  username: string;
  display_name: string;
  professional_title?: string;
  bio?: string;
  location_text?: string;
  country_code?: string;
  experience_years?: number;
  hourly_rate_kopecks?: number;
  minimum_order_kopecks?: number;
  availability: string;
  categories: Category[];
  skills: Skill[];
  custom_skills?: string[];
  languages: Language[];
  effective_pro?: boolean;
};
type ExternalReputation = { platform: string; display_name: string; profile_url: string; verified: true; rating?: number; reviews_count?: number; completed_orders_count?: number; account_since?: string; verified_at?: string };

const formatRubles = (kopecks: number) => new Intl.NumberFormat("ru-RU").format(kopecks / 100);
const availabilityLabels:Record<string,string>={AVAILABLE:"Доступен для новых задач",PARTIALLY_BUSY:"Готов обсуждать новые задачи",BUSY:"Сейчас занят",UNAVAILABLE:"Временно недоступен"};
const languageLabels:Record<string,string>={NATIVE:"Родной",FLUENT:"Свободно",ADVANCED:"Продвинутый",INTERMEDIATE:"Средний",BASIC:"Базовый"};

async function loadProfile(username: string): Promise<Profile | null> {
  const baseURL = process.env.API_BASE_URL ?? "http://localhost:8080";
  const response = await fetch(`${baseURL}/api/v1/profiles/${encodeURIComponent(username)}`, { next: { revalidate: 120 } });
  if (response.status === 404) return null;
  if (!response.ok) throw new Error("profile request failed");
  const body = await response.json();
  return body.data ?? null;
}

async function loadPortfolio(username: string): Promise<PortfolioItem[]> {
  const baseURL = process.env.API_BASE_URL ?? "http://localhost:8080";
  const response = await fetch(`${baseURL}/api/v1/profiles/${encodeURIComponent(username)}/portfolio?limit=20`, { cache: "no-store" });
  if (response.status === 404) return [];
  if (!response.ok) throw new Error("portfolio request failed");
  const body = await response.json();
  return body.data ?? [];
}

async function loadExternalReputation(username: string): Promise<ExternalReputation[]> {
  const baseURL = process.env.API_BASE_URL ?? "http://localhost:8080";
  const response = await fetch(`${baseURL}/api/v1/profiles/${encodeURIComponent(username)}/external-reputations`, { next: { revalidate: 120 } });
  if (response.status === 404) return [];
  if (!response.ok) throw new Error("external reputation request failed");
  const body = await response.json();
  return body.data ?? [];
}
async function loadReviews(username: string): Promise<{items:Review[];trust:NativeTrust;nextCursor:string|null}> { const baseURL=process.env.API_BASE_URL??"http://localhost:8080";const response=await fetch(`${baseURL}/api/v1/profiles/${encodeURIComponent(username)}/reviews?limit=20`,{next:{revalidate:120}});if(response.status===404)return {items:[],trust:{reviews_count:0,completed_projects_count:0},nextCursor:null};if(!response.ok)throw new Error("reviews request failed");const body=await response.json();return {items:body.data??[],trust:body.trust??{reviews_count:0,completed_projects_count:0},nextCursor:body.page?.next_cursor??null} }

function formatPrice(item: PortfolioItem): string | null {
  if (item.price_min_kopecks === undefined && item.price_max_kopecks === undefined) return null;
  if (item.price_min_kopecks !== undefined && item.price_max_kopecks !== undefined) {
    return `${formatRubles(item.price_min_kopecks)}–${formatRubles(item.price_max_kopecks)} ₽`;
  }
  if (item.price_min_kopecks !== undefined) return `от ${formatRubles(item.price_min_kopecks)} ₽`;
  return `до ${formatRubles(item.price_max_kopecks!)} ₽`;
}

export async function generateMetadata({params}:{params:Promise<{username:string}>}):Promise<Metadata>{const {username}=await params;const profile=await loadProfile(username);if(!profile)return missingMetadata("Профиль не найден");const description=summary(profile.bio,`${profile.display_name}${profile.professional_title?` — ${profile.professional_title}`:""}. Навыки, портфолио и подтверждённая репутация.`);return publicMetadata(`${profile.display_name}${profile.professional_title?` — ${profile.professional_title}`:""}`,description,`/freelancers/${profile.username}`)}

export default async function ProfilePage({ params }: { params: Promise<{ username: string }> }) {
  const { username } = await params;
  const [profile, portfolio, externalReputation, native] = await Promise.all([loadProfile(username), loadPortfolio(username), loadExternalReputation(username), loadReviews(username)]);
  if (!profile) notFound();

  const schema={"@context":"https://schema.org","@type":"Person",name:profile.display_name,url:canonical(`/freelancers/${profile.username}`),jobTitle:profile.professional_title||undefined,description:summary(profile.bio,profile.professional_title||"Публичный профиль специалиста"),knowsAbout:profile.skills?.map(skill=>skill.name)};
  return <main>
    <EngagementTracker eventType="PROFILE_VIEW" subjectUserID={profile.id}/>
    <Breadcrumbs items={[{label:"Главная",href:"/"},{label:"Специалисты",href:"/freelancers"},{label:profile.display_name}]}/>
    <header className="profile-media-header profile-media-header--favorite">
      <Avatar name={profile.display_name} id={profile.username} size="lg"/>
      <div className="profile-media-header__body"><div className="profile-title-row"><h1>{profile.display_name}</h1>{profile.effective_pro?<ProBadge/>:null}</div>
      {profile.professional_title ? <p>{profile.professional_title}</p> : null}<PresenceLabel id={profile.username}/>
      <p>{availabilityLabels[profile.availability]??"Статус доступности уточняется"}{profile.location_text ? ` · ${profile.location_text}` : ""}{profile.country_code ? ` · ${profile.country_code}` : ""}</p>
      {profile.experience_years !== undefined ? <p>Опыт: {profile.experience_years} лет</p> : null}
      {profile.hourly_rate_kopecks !== undefined ? <p>Ставка: {formatRubles(profile.hourly_rate_kopecks)} ₽/час</p> : null}
      {profile.minimum_order_kopecks !== undefined ? <p>Минимальный заказ: {formatRubles(profile.minimum_order_kopecks)} ₽</p> : null}</div>
      <div className="profile-header-favorite"><FavoriteButton type="FREELANCER" id={profile.id}/></div>
    </header>
    {profile.bio ? <section><h2>О специалисте</h2><p>{profile.bio}</p></section> : null}
    {portfolio.length ? <section>
      <h2>Портфолио</h2>
      <PortfolioGallery username={profile.username} subjectUserID={profile.id} items={portfolio.map(item=>({...item,price:formatPrice(item)}))}/>
    </section> : null}
    {profile.categories?.length ? <section><h2>Направления</h2><div className="profile-taxonomy">{profile.categories.map((category) => <span className={category.is_primary?"profile-taxonomy__item is-primary":"profile-taxonomy__item"} key={category.id}>{category.name}{category.is_primary ? <small>Основное</small> : null}</span>)}</div></section> : null}
    {profile.skills?.length ? <section><h2>Навыки</h2><div className="profile-taxonomy">{profile.skills.map((skill) => <span className="profile-taxonomy__item" key={skill.id}>{skill.name}{skill.level ? <small>{skill.level}</small> : null}</span>)}</div></section> : null}
    {profile.custom_skills?.length ? <section className="profile-custom-skills"><h2>Дополнительные навыки</h2><div className="profile-taxonomy">{profile.custom_skills.map(skill => <span className="profile-taxonomy__item is-custom" key={skill}>{skill}<small>Указано специалистом</small></span>)}</div></section> : null}
    {profile.languages?.length ? <section><h2>Языки</h2><ul>{profile.languages.map((language) => <li key={language.code}>{language.code.toUpperCase()} · {languageLabels[language.level]??language.level}</li>)}</ul></section> : null}
    <ReviewsSection username={profile.username} initial={native.items} trust={native.trust} initialNextCursor={native.nextCursor} subject="freelancer" sectionLabel="Рейтинг на платформе" emptyProfileLabel="Новый исполнитель"/>
    {externalReputation.length ? <section className="external-reputation"><div className="external-reputation__heading"><div><p className="eyebrow">Проверено площадкой</p><h2>Подтверждённая внешняя репутация</h2></div><span className="verified-seal" aria-label="Данные подтверждены">✓</span></div><p className="reviews-external-note">Внешняя репутация показана отдельно и не влияет на рейтинг Naimio.</p><div className="external-reputation__grid">{externalReputation.map((item) => <article className="external-reputation-card" key={`${item.platform}:${item.profile_url}`}><div className="external-reputation-card__top"><span className="external-reputation-card__platform">{item.display_name}</span><span className="verified-badge">✓ Подтверждено</span></div><div className="external-reputation-card__metrics">{item.rating !== undefined ? <span><strong>{item.rating.toFixed(1)}</strong><small>рейтинг</small></span> : null}{item.reviews_count !== undefined ? <span><strong>{item.reviews_count}</strong><small>{russianPlural(item.reviews_count,"отзыв","отзыва","отзывов")}</small></span> : null}{item.completed_orders_count !== undefined ? <span><strong>{item.completed_orders_count}</strong><small>заказов</small></span> : null}</div>{item.account_since ? <p>Профиль на площадке с {item.account_since}</p> : null}<a className="card-link" href={item.profile_url} target="_blank" rel="noopener noreferrer">Открыть подтверждённый профиль →</a></article>)}</div></section> : null}
    <script type="application/ld+json" dangerouslySetInnerHTML={{__html:jsonLD(schema)}}/>
  </main>;
}
