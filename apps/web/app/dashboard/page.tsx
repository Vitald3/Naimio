"use client";

import { useEffect, useState } from "react";
import Breadcrumbs from "../breadcrumbs";
import { useAuth } from "../auth-state";
import { DashboardOverviewSkeleton } from "../skeletons";

type Item = {
  id: string;
  title?: string;
  status?: string;
  proposal_count?: number;
  moderation_status?: string;
  moderation_reason?: string;
  category?: { name?: string };
  budget?: { type?: string; min_kopecks?: number; max_kopecks?: number };
};

const data = (body: unknown): Item[] => Array.isArray((body as { data?: unknown })?.data) ? (body as { data: Item[] }).data : [];
const projectStatuses: Record<string, string> = { DRAFT: "Черновик", OPEN: "Принимает отклики", IN_PROGRESS: "В работе", AWAITING_FUNDING: "Ожидает оплаты", COMPLETED: "Завершён", CANCELLED: "Отменён" };
const money = (kopecks?: number) => kopecks === undefined ? "" : `${new Intl.NumberFormat("ru-RU").format(kopecks / 100)} ₽`;
function projectBudget(item: Item) {
  const budget = item.budget;
  if (!budget || budget.type === "NEGOTIABLE") return "По договорённости";
  if (budget.min_kopecks !== undefined && budget.max_kopecks !== undefined && budget.min_kopecks !== budget.max_kopecks) return `${money(budget.min_kopecks)} — ${money(budget.max_kopecks)}`;
  return money(budget.min_kopecks ?? budget.max_kopecks);
}

export default function Dashboard() {
  const { user } = useAuth();
  const freelancer = !!user?.capabilities?.includes("FREELANCER");
  const customer = !!user?.capabilities?.includes("CUSTOMER");
  const dual = freelancer && customer;
  const [projects, setProjects] = useState<Item[]>([]);
  const [services, setServices] = useState<Item[]>([]);
  const [deals, setDeals] = useState<Item[]>([]);
  const [proposals, setProposals] = useState<Item[]>([]);
  const [state, setState] = useState<"loading" | "ready" | "error">("loading");

  useEffect(() => {
    const requests = [
      fetch("/api/v1/me/projects", { credentials: "same-origin" }),
      fetch("/api/v1/me/services", { credentials: "same-origin" }),
      fetch("/api/v1/me/safe-deals", { credentials: "same-origin" }),
      freelancer?fetch("/api/v1/me/proposals", { credentials: "same-origin" }) : Promise.resolve(new Response(JSON.stringify({ data: [] }), { status: 200, headers: { "Content-Type": "application/json" } })),
    ];
    Promise.all(requests)
      .then(async (responses) => {
        if (responses.some((response) => response.status === 401)) { location.assign("/login?next=/dashboard"); return []; }
        return Promise.all(responses.map((response) => response.ok ? response.json() : { data: [] }));
      })
      .then((bodies) => {
        if (bodies.length !== 4) return;
        const [projectBody, serviceBody, dealBody, proposalBody] = bodies;
        setProjects(data(projectBody)); setServices(data(serviceBody)); setDeals(data(dealBody)); setProposals(data(proposalBody)); setState("ready");
      })
      .catch(() => setState("error"));
  }, [freelancer]);

  if (state === "loading") return <main><Breadcrumbs items={[{ label: "Главная", href: "/" }, { label: "Кабинет" }]}/><DashboardOverviewSkeleton/></main>;
  if (state === "error") return <main><div className="error">Не удалось загрузить личный кабинет. Обновите страницу или попробуйте позже.</div></main>;
  const firstName = user?.display_name ? `, ${user.display_name.split(" ")[0]}` : "";

  return <main>
    <Breadcrumbs items={[{ label: "Главная", href: "/" }, { label: "Кабинет" }]}/>
    <div className="page-heading"><div><p className="eyebrow">{freelancer && !customer ? "Кабинет специалиста" : customer && !freelancer ? "Кабинет заказчика" : "Личный кабинет"}</p><h1>Добро пожаловать{firstName}</h1><p>{dual ? "Управляйте проектами заказчика, откликами специалиста, услугами и безопасными сделками в одном кабинете." : freelancer ? "Следите за своими предложениями, услугами и безопасными сделками." : "Управляйте проектами, приглашениями и безопасными сделками. Отклики исполнителей открываются внутри конкретного проекта."}</p></div><a className="button" href={customer ? "/dashboard/projects/new" : "/projects"}>{customer ? "Разместить задачу" : "Найти проекты"}</a></div>
    <section><h2>{freelancer ? "Ваша активность" : "Ваши задачи"}</h2><ul className="dash-cards">
      {freelancer ? <><li><article className="dash-card"><p className="eyebrow">Мои отклики</p><strong>{proposals.length}</strong><p>Ваши предложения заказчикам по проектам.</p><a className="card-link" href="/dashboard/proposals">Открыть →</a></article></li><li><article className="dash-card"><p className="eyebrow">Услуги</p><strong>{services.length}</strong><p>Ваши предложения в каталоге.</p><a className="card-link" href="/dashboard/services">Управлять →</a></article></li></> : null}
      {customer ? <li><article className="dash-card"><p className="eyebrow">Проекты</p><strong>{projects.length}</strong><p>Черновики, опубликованные и текущие задачи. Входящие отклики находятся внутри каждого проекта.</p><a className="card-link" href="/dashboard/projects/new">Создать проект →</a></article></li> : null}
      <li><article className="dash-card"><p className="eyebrow">Safe Deal</p><strong>{deals.length}</strong><p>Финансирование, выполнение и приёмка работы.</p><a className="card-link" href="/dashboard/safe-deals">Открыть сделки →</a></article></li>
    </ul></section>
    {customer && projects.length ? <section><div className="section-heading-row"><div><p className="eyebrow">Управление</p><h2>Ваши проекты</h2></div><a className="button button--quiet" href="/dashboard/projects/new">Новый проект</a></div><ul className="record-list project-management-list">{projects.map((project) => { const rejected=project.moderation_status==="HIDDEN"&&Boolean(project.moderation_reason); return <li className="record" key={project.id}><div className="record__head"><strong><a className="admin-primary-link" href={`/dashboard/projects/${project.id}`}>{project.title || "Проект"}</a></strong><span className={`badge${rejected?" badge--danger":""}`}>{rejected?"Отклонён модерацией":(projectStatuses[project.status || ""] || "Статус уточняется")}</span></div><p className="record__body">{project.category?.name || "Без категории"} · {projectBudget(project)} · {project.proposal_count ?? 0} откликов</p><div className="inline-actions"><a className="button button--compact" href={`/dashboard/projects/${project.id}`}>{rejected?"Исправить":"Открыть"}</a>{!rejected&&["OPEN","MATCHING","IN_PROGRESS"].includes(project.status||"")?<><a className="button button--quiet button--compact" href={`/dashboard/projects/${project.id}/proposals`}>Отклики</a><a className="button button--quiet button--compact" href={`/dashboard/projects/${project.id}/recommendations`}>Подбор</a></>:null}</div></li>})}</ul></section> : null}
    <section className="dash-split"><div><h2>{freelancer ? "Профессиональный профиль" : "Отзывы и доверие"}</h2><p>{freelancer ? "Заполните специализацию, портфолио и подтверждённую внешнюю репутацию." : "Следите за отзывами о сотрудничестве и своей репутацией заказчика."}</p><a className="button" href={freelancer ? "/settings/profile" : "/dashboard/reviews"}>{freelancer ? "Редактировать профиль" : "Мои отзывы"}</a></div><div><h2>{freelancer ? "Услуги и вакансии" : "Постоянная команда"}</h2><p>{freelancer ? "Опубликуйте готовые услуги и откликайтесь на вакансии работодателей." : "Сохраняйте проверенных специалистов и приглашайте их в новые проекты."}</p><a className="button button--quiet" href={freelancer ? "/dashboard/services" : "/dashboard/team"}>{freelancer ? "Мои услуги" : "Открыть «Мою команду»"}</a></div></section>
  </main>;
}
