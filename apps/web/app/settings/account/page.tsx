"use client";

import Breadcrumbs from "../../breadcrumbs";
import { useAuth } from "../../auth-state";
import { IconBriefcase, IconUser } from "../../icons";
import { useState } from "react";

export default function AccountSettingsPage() {
  const { user, state, refresh } = useAuth();
  const [sending, setSending] = useState(false);
  const [activating, setActivating] = useState("");
  const [notice, setNotice] = useState("");
  const verified = user?.email_verified ?? false;

  async function resend() {
    setSending(true); setNotice("");
    const response = await fetch("/api/v1/auth/resend-verification", { method: "POST", credentials: "same-origin" });
    setSending(false);
    setNotice(response.ok ? "Новое письмо отправлено. Ссылка действует 24 часа." : "Не удалось отправить письмо. Попробуйте позже.");
  }
  async function activate(capability: "CUSTOMER" | "FREELANCER") {
    setActivating(capability); setNotice("");
    const response = await fetch(`/api/v1/me/capabilities/${capability}`, { method: "PUT", credentials: "same-origin" });
    if (response.ok) { await refresh(); setNotice(capability === "CUSTOMER" ? "Режим заказчика включён." : "Режим исполнителя включён."); }
    else setNotice("Не удалось включить режим.");
    setActivating("");
  }

  const mode = (capability: "CUSTOMER" | "FREELANCER", title: string, text: string) => {
    const active = user?.capabilities?.includes(capability) ?? false;
    return <article className={`account-mode-card${active ? " is-active" : ""}`}><span className="account-mode-card__icon">{capability === "CUSTOMER" ? <IconBriefcase size={22}/> : <IconUser size={22}/>}</span><div><div className="account-mode-card__title"><h3>{title}</h3><span>{active ? "Активен" : "Не включён"}</span></div><p>{text}</p>{!active ? <button className="button button--quiet button--compact" type="button" disabled={!!activating} onClick={() => void activate(capability)}>{activating === capability ? "Включаем…" : "Включить режим"}</button> : null}</div></article>;
  };

  return <main><Breadcrumbs items={[{label:"Главная",href:"/"},{label:"Кабинет",href:"/dashboard"},{label:"Настройки",href:"/settings/profile"},{label:"Аккаунт"}]}/><div className="page-heading"><div><p className="eyebrow">Настройки</p><h1>Аккаунт</h1><p className="lead">Контактные данные и режимы работы на площадке.</p></div></div><nav className="settings-tabs" aria-label="Разделы настроек"><a href="/settings/profile">Профиль</a><a className="is-active" href="/settings/account" aria-current="page">Аккаунт</a><a href="/settings/notifications">Уведомления</a><a href="/settings/security">Безопасность</a></nav>
    <section className="settings-card"><h2>Email</h2>{state === "loading" ? <div className="skeleton"/> : <><div className="account-fact"><span>Текущий адрес</span><strong>{user?.email ?? "Не указан"}</strong></div><div className="account-fact"><span>Подтверждение</span><strong className={verified ? "status-positive" : "status-warning"}>{verified ? "Подтверждён" : "Не подтверждён"}</strong></div>{!verified ? <div className="email-verification-callout"><div><strong>Подтвердите адрес</strong><p>Это защищает аккаунт и позволяет получать важные уведомления.</p></div><button className="button button--quiet button--compact" type="button" onClick={() => void resend()} disabled={sending}>{sending ? "Отправляем…" : "Отправить письмо ещё раз"}</button></div> : null}</>}</section>
    <section className="settings-card"><h2>Режимы аккаунта</h2><p className="form-hint">Это один аккаунт с общей историей и репутацией, но разделы заказчика и исполнителя не пересекаются. API проверяет нужный режим для каждого действия.</p><div className="account-modes">{mode("CUSTOMER", "Заказчик", "Создание проектов и вакансий, выбор исполнителей и управление командой.")}{mode("FREELANCER", "Исполнитель", "Профессиональный профиль, услуги, портфолио и отклики на проекты.")}</div></section>
    {notice ? <p className="notice" role="status">{notice}</p> : null}
  </main>;
}
