"use client";

import { useEffect, useState } from "react";
import Breadcrumbs from "../../breadcrumbs";

type Metrics = {
  period_days: number;
  advanced_unlocked: boolean;
  pro_system_enabled: boolean;
  profile_views?: number;
  portfolio_views?: number;
  service_views?: number;
  proposals_sent: number;
  job_applications_sent: number;
  profile_to_proposal_rate?: number;
  locked_advanced_metrics?: string[];
};

const number = (value: number | undefined) => new Intl.NumberFormat("ru-RU").format(value ?? 0);
const percent = (value: number | undefined) => value === undefined ? "—" : new Intl.NumberFormat("ru-RU", { style: "percent", maximumFractionDigits: 1 }).format(value);

export default function AnalyticsPage() {
  const [metrics, setMetrics] = useState<Metrics | null>(null);
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    fetch("/api/v1/me/analytics", { credentials: "same-origin", cache: "no-store" })
      .then(async response => {
        if (!response.ok) throw new Error();
        const body = await response.json();
        setMetrics(body.data ?? null);
      })
      .catch(() => setFailed(true));
  }, []);
  if (!metrics && !failed) return <main><div className="catalog-skeleton"><div className="skeleton"/><div className="skeleton"/></div></main>;
  if (failed || !metrics) return <main><div className="error">Не удалось загрузить аналитику. Обновите страницу или попробуйте позже.</div></main>;
  const basic = [
    ["Отклики на проекты", number(metrics.proposals_sent)],
    ["Отклики на вакансии", number(metrics.job_applications_sent)],
  ];
  const advanced = [
    ["Просмотры профиля", number(metrics.profile_views)],
    ["Просмотры портфолио", number(metrics.portfolio_views)],
    ["Просмотры услуг", number(metrics.service_views)],
    ["Конверсия профиля в отклик", percent(metrics.profile_to_proposal_rate)],
  ];
  return <main>
    <Breadcrumbs items={[{ label: "Главная", href: "/" }, { label: "Кабинет", href: "/dashboard" }, { label: "Аналитика профиля" }]}/>
    <div className="page-heading"><div><p className="eyebrow">Профиль исполнителя</p><h1>Аналитика</h1><p className="lead">Фактические показатели за последние {metrics.period_days} дней. Повторные просмотры одного объекта за день не учитываются.</p></div></div>
    <section><h2>Основная активность</h2><ul className="dash-cards">{basic.map(([label, value]) => <li key={label}><article className="dash-card"><p className="eyebrow">{label}</p><strong>{value}</strong><p>За последние {metrics.period_days} дней.</p></article></li>)}</ul></section>
    <section><div className="section-heading-row"><div><p className="eyebrow">Naimio PRO</p><h2>Расширенная аналитика</h2></div>{metrics.advanced_unlocked ? <span className="pro-badge">◆ PRO</span> : null}</div>
      {metrics.advanced_unlocked ? <ul className="dash-cards">{advanced.map(([label, value]) => <li key={label}><article className="dash-card"><p className="eyebrow">{label}</p><strong>{value}</strong><p>Только реальные просмотры и действия.</p></article></li>)}</ul> : <div className="notice"><h3>Расширенная аналитика доступна в PRO</h3><p>Просмотры профиля, портфолио и услуг, а также конверсия в отклик откроются при активной PRO-подписке.</p>{metrics.pro_system_enabled ? <a className="button" href="/pro">Подробнее о PRO</a> : null}</div>}
    </section>
  </main>;
}
