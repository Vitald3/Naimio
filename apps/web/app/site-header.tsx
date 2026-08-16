"use client";

import { useUnreadCount } from "./notification-badge";
import { useAuth } from "./auth-state";
import { IconSearch, IconMessage, IconMenu, IconX } from "./icons";
import { Avatar } from "./media-components";
import NaimioLogo from "./logo";
import { STAFF_BASE_PATH, isStaffRoles } from "./admin-path";
import { usePathname } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { useSiteSettings } from "./site-settings";

const publicLinks = [
  ["/freelancers", "Специалисты"],
  ["/services", "Услуги"],
  ["/projects", "Проекты"],
  ["/vacancies", "Вакансии"],
  ["/education", "Обучение"],
] as const;

export default function SiteHeader() {
  const pathname = usePathname() || "";
  const { state, user, logout } = useAuth();
  const staff = isStaffRoles(user?.roles);
  const staffPath = pathname === STAFF_BASE_PATH || pathname.startsWith(STAFF_BASE_PATH + "/");
  const unread = useUnreadCount(state === "authenticated" && !staff && !staffPath);
  const [mobileOpen, setMobileOpen] = useState(false);
  const accountMenu = useRef<HTMLDetailsElement>(null);
  const settings = useSiteSettings();
  const visibleLinks = [...publicLinks, ...(settings.blog_enabled ? [["/blog", "Блог"]] as const : [])];

  useEffect(() => {
    setMobileOpen(false);
  }, [pathname]);
  useEffect(() => {
    const outside = (event: PointerEvent) => {
      if (accountMenu.current?.open && !accountMenu.current.contains(event.target as Node)) accountMenu.current.open = false;
    };
    const escape = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        if (accountMenu.current) accountMenu.current.open = false;
        setMobileOpen(false);
      }
    };
    document.addEventListener("pointerdown", outside);
    document.addEventListener("keydown", escape);
    return () => { document.removeEventListener("pointerdown", outside); document.removeEventListener("keydown", escape); };
  }, []);

  if (staffPath) return null;

  return (
    <header className="site-header">
      <div className="site-header__row">
        <a className="brand" href="/" aria-label="Naimio — главная">
          <NaimioLogo />
        </a>

        <form className="header-search" action="/freelancers" method="get" role="search">
          <span className="header-search__icon" aria-hidden="true"><IconSearch size={18} /></span>
          <input name="q" type="search" maxLength={120} placeholder="Найти специалиста или навык" aria-label="Поиск специалистов" />
        </form>

        <nav className="site-nav" aria-label="Основная навигация">
          {visibleLinks.map(([href, label]) => <a href={href} key={href}>{label}</a>)}
        </nav>

        {state === "authenticated" && staff ? (
          <div className="site-actions site-actions--auth">
            <a className="header-cta" href={STAFF_BASE_PATH}>Control Center</a>
            <button className="button button--quiet button--compact" type="button" onClick={logout}>Выйти</button>
          </div>
        ) : state === "authenticated" ? (
          <div className="site-actions site-actions--auth">
            {user?.capabilities?.includes("CUSTOMER") ? <a className="header-cta" href="/dashboard/projects/new">Разместить задачу</a> : <a className="header-cta header-cta--secondary" href="/projects">Найти проект</a>}
            <a className="header-icon-link" href="/messages" aria-label="Сообщения" title="Сообщения"><IconMessage size={20} /></a>
            <details ref={accountMenu} className="account-menu">
              <summary aria-label="Меню аккаунта">
                <span className="avatar-wrap">
                  <Avatar name={user?.display_name || "Пользователь"} id={user?.id || user?.display_name || "me"} size="sm"/>
                  {unread > 0 ? <span className="notif-badge" aria-label={`Непрочитанных уведомлений: ${unread}`}>{unread > 9 ? "9+" : unread}</span> : null}
                </span>
                <span className="account-menu__name">{user?.display_name}</span>
              </summary>
              <div>
                <a href="/dashboard" onClick={() => { if (accountMenu.current) accountMenu.current.open = false; }}>Личный кабинет</a>
                <a href="/notifications" onClick={() => { if (accountMenu.current) accountMenu.current.open = false; }}>Уведомления{unread ? ` (${unread})` : ""}</a>
                <a href="/settings/reputation" onClick={() => { if (accountMenu.current) accountMenu.current.open = false; }}>Профиль и репутация</a>
                {settings.pro_subscriptions_enabled ? <a href="/pro" onClick={() => { if (accountMenu.current) accountMenu.current.open = false; }}>Naimio PRO</a> : null}
                <button type="button" onClick={logout}>Выйти</button>
              </div>
            </details>
          </div>
        ) : (
          <div className="site-actions site-actions--guest">
            {state === "loading" ? <span className="auth-loading" aria-label="Проверяем сессию" /> : <><a href="/login">Войти</a><a className="register-link" href="/register">Регистрация</a></>}
            <a className="header-cta" href="/create-project">Разместить задачу</a>
          </div>
        )}
        <button type="button" className="mobile-menu-toggle" aria-label={mobileOpen ? "Закрыть меню" : "Открыть меню"} aria-expanded={mobileOpen} aria-controls="mobile-header-menu" onClick={() => setMobileOpen(value => !value)}>{mobileOpen ? <IconX size={22}/> : <IconMenu size={22}/>}</button>
      </div>
      {mobileOpen ? <nav id="mobile-header-menu" className="mobile-header-menu" aria-label="Мобильная навигация">
        {visibleLinks.map(([href, label]) => <a href={href} key={href}>{label}</a>)}
        {settings.pro_subscriptions_enabled ? <a href="/pro">Naimio PRO</a> : null}
        {state === "authenticated" ? <><a href="/messages">Сообщения</a><a href="/favorites">Избранное</a><a href="/dashboard">Личный кабинет</a></> : <><a href="/login">Войти</a><a href="/register">Регистрация</a></>}
      </nav> : null}
    </header>
  );
}
