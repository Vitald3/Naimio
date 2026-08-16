import type { Metadata } from "next";
import { notFound } from "next/navigation";
import ProposalForm from "../../project/[id]/proposal-form";
import ShareButton from "../../project/[id]/share-button";
import { canonical, jsonLD, missingMetadata, publicMetadata, summary } from "../../seo";
import Breadcrumbs from "../../breadcrumbs";
import FavoriteButton from "../../favorite-button";
import FormattedText from "../../formatted-text";
import { IconImage } from "../../icons";
import { countLabel } from "../../russian-plural";

export const dynamic = "force-dynamic";

type Project = {
  id: string;
  title: string;
  description: string;
  budget: { type: string; min_kopecks?: number; max_kopecks?: number };
  deadline_at?: string;
  experience_level?: string;
  customer_display_name?: string;
  customer_username?: string;
  proposal_count: number;
  status?: string;
  category?: { name: string };
  skills: Array<{ id: string; name: string }>;
  media?: Array<{ id: string; mime_type: string; size_bytes: number; original_filename?: string }>;
};
type CustomerTrust = { native_rating?: number; reviews_count: number; completed_projects_count: number; recommendation_rate?: number };

const experienceLabels: Record<string, string> = { BEGINNER: "Начинающий", INTERMEDIATE: "Средний", ADVANCED: "Опытный", EXPERT: "Эксперт" };
const proposalLabel=(count:number)=>{const mod100=count%100,mod10=count%10;const word=mod100>=11&&mod100<=14?"откликов":mod10===1?"отклик":mod10>=2&&mod10<=4?"отклика":"откликов";return `${count} ${word}`};
// A project page stays reachable through its whole lifecycle, so the proposal
// form is shown only while the project is actually accepting responses. In any
// later state the customer sees a clear reason instead of a form or a 404.
const closedNotice: Record<string, string> = {
  AWAITING_FUNDING: "Исполнитель уже выбран — проект ожидает оплаты по безопасной сделке. Отклики больше не принимаются.",
  IN_PROGRESS: "Проект уже в работе с выбранным исполнителем. Отклики больше не принимаются.",
  COMPLETED: "Проект завершён. Отклики больше не принимаются.",
  CANCELLED: "Проект отменён заказчиком. Отклики не принимаются.",
  ARCHIVED: "Проект перенесён в архив. Отклики не принимаются.",
};
const money = (value?: number) => value === undefined ? "" : `${new Intl.NumberFormat("ru-RU").format(value / 100)} ₽`;
const budget = (item: Project) => item.budget.type === "NEGOTIABLE" ? "По договорённости" : item.budget.type === "FIXED" ? money(item.budget.min_kopecks) : `${money(item.budget.min_kopecks)} — ${money(item.budget.max_kopecks)}${item.budget.type === "HOURLY" ? " / час" : ""}`;

async function loadProject(id: string): Promise<Project | null> {
  const baseURL = process.env.API_BASE_URL ?? "http://localhost:8080";
  const response = await fetch(`${baseURL}/api/v1/projects/${encodeURIComponent(id)}`, { cache: "no-store" });
  if (response.status === 404) return null;
  if (!response.ok) throw new Error("project request failed");
  const body = await response.json(); return body.data ?? null;
}

async function loadCustomerTrust(username?: string): Promise<CustomerTrust | null> {
  if (!username) return null;
  const baseURL = process.env.API_BASE_URL ?? "http://localhost:8080";
  const response = await fetch(`${baseURL}/api/v1/profiles/${encodeURIComponent(username)}/reviews?limit=1`, { next: { revalidate: 120 } });
  if (!response.ok) return null;
  const body = await response.json();
  return body.trust ?? null;
}

export async function generateMetadata({params}:{params:Promise<{id:string}>}):Promise<Metadata>{const {id}=await params;const project=await loadProject(id);if(!project)return missingMetadata("Проект не найден");return publicMetadata(project.title,summary(project.description,"Открытый публичный проект."),`/projects/${project.id}`)}

export default async function ProjectPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const project = await loadProject(id);
  if (!project) notFound();
  const customerTrust = await loadCustomerTrust(project.customer_username);
  const customerHref = project.customer_username
    ? `/customers/${encodeURIComponent(project.customer_username)}${project.customer_display_name ? `?name=${encodeURIComponent(project.customer_display_name)}` : ""}`
    : null;
  const schema={"@context":"https://schema.org","@type":"CreativeWork",name:project.title,description:summary(project.description,project.title,500),url:canonical(`/projects/${project.id}`),keywords:project.skills?.map(skill=>skill.name),about:project.category?.name};
  return <main>
    <Breadcrumbs items={[{label:"Главная",href:"/"},{label:"Проекты",href:"/projects"},{label:project.title}]}/>
    <header className="project-detail-header"><div className="detail-title-row"><p className="eyebrow">{project.category?.name ?? "Без категории"}</p><FavoriteButton type="PROJECT" id={project.id}/></div><h1>{project.title}</h1>{project.customer_display_name ? <p>Заказчик: {project.customer_display_name}</p> : null}<div className="inline-actions"><ShareButton title={project.title}/></div></header>
    <section><h2>Описание</h2><FormattedText value={project.description}/></section>
    {project.media?.length ? <section className="project-detail-attachments"><h2>ТЗ и материалы</h2><ul>{project.media.map((file,index)=><li key={file.id}><IconImage size={18}/><div><strong>{file.original_filename||`Материал ${index+1}`}</strong><small>{file.mime_type} · {file.size_bytes < 1048576 ? `${Math.max(1,Math.round(file.size_bytes/1024))} КБ` : `${(file.size_bytes/1048576).toFixed(1)} МБ`}</small></div><a className="button button--quiet button--compact" href={`/api/v1/projects/${project.id}/attachments/${file.id}`}>Скачать</a></li>)}</ul></section> : null}
    {project.skills?.length ? <section><h2>Необходимые навыки</h2><ul>{project.skills.map((skill) => <li key={skill.id}>{skill.name}</li>)}</ul></section> : null}
    <section><h2>Условия</h2><p>Бюджет: <strong className="detail-price">{budget(project)}</strong></p>{project.deadline_at ? <p>Срок: до {new Intl.DateTimeFormat("ru-RU").format(new Date(project.deadline_at))}</p> : null}{project.experience_level ? <p>Уровень: {experienceLabels[project.experience_level] ?? project.experience_level}</p> : null}<p>Отклики: {proposalLabel(project.proposal_count)}</p></section>
    {customerHref ? <section className="customer-trust"><h2>Репутация заказчика</h2>{customerTrust && customerTrust.reviews_count ? <p>{customerTrust.native_rating?.toFixed(1)} · {countLabel(customerTrust.reviews_count,["отзыв","отзыва","отзывов"])} от исполнителей · Завершённых проектов: {customerTrust.completed_projects_count}{customerTrust.recommendation_rate!==undefined&&customerTrust.recommendation_rate!==null?` · ${Math.round(customerTrust.recommendation_rate)}% рекомендуют`:""}</p> : <p>Пока нет отзывов исполнителей о заказчике.</p>}<p><a className="admin-primary-link" href={customerHref}>Смотреть отзывы о заказчике →</a></p></section> : null}
    {project.status && project.status !== "OPEN" && project.status !== "MATCHING"
      ? <section className="notice">{closedNotice[project.status] ?? "Проект больше не принимает отклики."}</section>
      : <ProposalForm projectId={project.id}/>}
    <script type="application/ld+json" dangerouslySetInnerHTML={{__html:jsonLD(schema)}}/>
  </main>;
}
