"use client";

import { usePathname } from "next/navigation";
import { useAuth } from "../auth-state";

type Link = [string, string];
type Group = { label: string; links: Link[] };

export function DashboardNav() {
  const pathname = usePathname() || "";
  const { user } = useAuth();
  const customer = user?.capabilities?.includes("CUSTOMER") ?? false;
  const freelancer = user?.capabilities?.includes("FREELANCER") ?? false;
  const groups: Group[] = [
    { label: "", links: [["/dashboard", "Обзор"]] },
    ...(customer ? [{ label: "Заказчик", links: [["/dashboard/projects/new", "Создать проект"], ["/dashboard/vacancies", "Мои вакансии"], ["/dashboard/safe-deals", "Безопасные сделки"], ["/dashboard/invites", "Приглашения"]] as Link[] }] : []),
    ...(freelancer ? [{ label: "Работа", links: [["/dashboard/proposals", "Мои отклики на проекты"], ["/dashboard/job-applications", "Мои отклики на вакансии"], ["/dashboard/services", "Мои услуги"], ["/dashboard/portfolio", "Портфолио"], ["/dashboard/analytics", "Аналитика профиля"], ["/dashboard/payouts", "Реквизиты выплат"], ["/dashboard/safe-deals", "Безопасные сделки"], ["/dashboard/invites", "Приглашения"]] as Link[] }] : []),
    { label: "Общение", links: [["/messages", "Сообщения"], ...(customer ? [["/dashboard/team", "Моя команда"] as Link] : []), ["/favorites", "Избранное"]] },
    { label: "Аккаунт", links: [...(freelancer ? [["/settings/profile", "Профессиональный профиль"] as Link, ["/settings/reputation", "Репутация"] as Link] : []), ["/settings/account", "Аккаунт"], ["/settings/security", "Безопасность"], ["/dashboard/reviews", "Мои отзывы"], ["/settings/notifications", "Уведомления"]] },
  ];
  const isActive = (href: string) => href === "/dashboard" ? pathname === "/dashboard" : pathname === href || pathname.startsWith(href + "/");
  return <aside className="dashboard-nav" aria-label="Личный кабинет">{groups.map((group, index) => <div className="dashboard-nav__group" key={`${group.label}-${index}`}>{group.label ? <p className="dashboard-nav__label">{group.label}</p> : null}{group.links.map(([href, label]) => { const active = isActive(href); return <a key={href} href={href} className={active ? "is-active" : undefined} aria-current={active ? "page" : undefined}>{label}</a>; })}</div>)}</aside>;
}
