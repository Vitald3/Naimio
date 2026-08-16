"use client";

import { FormEvent, useEffect, useState } from "react";
import Breadcrumbs from "../../breadcrumbs";
import { useToast } from "../../toast";
import { notificationPresentation } from "../../notification-presentation";
import { useAuth } from "../../auth-state";

type Preference = { event_type: string; in_app: boolean; email: boolean };
const labels: Record<string, { title: string; hint: string }> = {
  new_message: { title: "Новые сообщения", hint: "Сообщения в диалогах с заказчиками и исполнителями." },
  proposal_received: { title: "Отклики на проект", hint: "Кто-то откликнулся на ваш проект." },
  project_status_changed: { title: "Изменение статуса проекта", hint: "Проект опубликован, взят в работу или завершён." },
  new_review_received: { title: "Новые отзывы", hint: "Вам оставили отзыв о сотрудничестве." },
  invite_accepted: { title: "Принятые приглашения", hint: "Приглашённый специалист принял ваше приглашение." },
  invited_to_project: { title: "Приглашения в проект", hint: "Вас пригласили поработать над проектом." },
  reward_granted: { title: "Промо-награды", hint: "Начислена промо-льгота по программе развития." },
  new_project_available: { title: "Дайджест проектов", hint: "Периодическая подборка новых заказов для исполнителей." },
  new_vacancy_available: { title: "Дайджест вакансий", hint: "Периодическая подборка новых вакансий." },
  new_service_available: { title: "Дайджест услуг", hint: "Периодическая подборка новых услуг и обучения." },
};
const defaults: Preference[] = [
  { event_type: "new_message", in_app: true, email: true },
  { event_type: "proposal_received", in_app: true, email: true },
  { event_type: "project_status_changed", in_app: true, email: false },
  { event_type: "new_review_received", in_app: true, email: true },
  { event_type: "invite_accepted", in_app: true, email: true },
  { event_type: "invited_to_project", in_app: true, email: true },
  { event_type: "reward_granted", in_app: true, email: true },
  { event_type: "new_project_available", in_app: false, email: false },
  { event_type: "new_vacancy_available", in_app: false, email: false },
  { event_type: "new_service_available", in_app: false, email: false },
];

export default function NotificationSettingsPage() {
  const [items, setItems] = useState(defaults);
  const [saving, setSaving] = useState(false);
  const { push } = useToast();
  const { user } = useAuth();
  const customerOnly = Boolean(user?.capabilities?.includes("CUSTOMER") && !user?.capabilities?.includes("FREELANCER"));

  useEffect(() => {
    fetch("/api/v1/notification-preferences", { credentials: "same-origin" })
      .then((response) => response.ok ? response.json() : Promise.reject())
      .then((body) => { if (body.data?.length) { const saved = new Map((body.data as Preference[]).map((item) => [item.event_type, item])); setItems(defaults.map((item) => saved.get(item.event_type) ?? item)); } })
      .catch(() => push({ kind: "error", title: "Не удалось загрузить настройки", message: "Обновите страницу и попробуйте ещё раз." }));
  }, [push]);

  async function save(event: FormEvent) {
    event.preventDefault();
    setSaving(true);
    try {
      const response = await fetch("/api/v1/notification-preferences", {
        method: "PUT",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ preferences: items }),
      });
      if (!response.ok) throw new Error();
      push({ kind: "success", title: "Настройки сохранены", message: "Новые правила уведомлений уже действуют." });
    } catch {
      push({ kind: "error", title: "Не удалось сохранить настройки", message: "Проверьте соединение и повторите попытку." });
    } finally {
      setSaving(false);
    }
  }

  function change(index: number, key: "in_app" | "email", value: boolean) {
    setItems((current) => current.map((item, itemIndex) => itemIndex === index ? { ...item, [key]: value } : item));
  }

  return <main>
    <Breadcrumbs items={[{ label: "Главная", href: "/" }, { label: "Кабинет", href: "/dashboard" }, { label: "Настройки уведомлений" }]} />
    <header className="page-heading"><div><p className="eyebrow">Настройки</p><h1>Настройки уведомлений</h1><p className="lead">Выберите, о каких событиях присылать уведомления в приложении и на email.</p></div></header>
    <form className="notification-settings-form" onSubmit={save}>
      <div className="settings-table-wrap"><table className="settings-table">
        <thead><tr><th>Событие</th><th>В приложении</th><th>Email</th></tr></thead>
        <tbody>{items.map((item, index) => ({ item, index })).filter(({ item }) => !customerOnly || !["new_project_available", "new_vacancy_available", "new_service_available"].includes(item.event_type)).map(({ item, index }) => { const meta = labels[item.event_type]; return <tr key={item.event_type}>
          <td><div className="settings-event"><strong>{(meta ?? notificationPresentation(item.event_type)).title}</strong><small>{(meta ?? notificationPresentation(item.event_type)).hint}</small></div></td>
          <td><input aria-label={`${meta?.title ?? item.event_type} — в приложении`} type="checkbox" checked={item.in_app} onChange={(event) => change(index, "in_app", event.target.checked)} /></td>
          <td><input aria-label={`${(meta ?? notificationPresentation(item.event_type)).title} — email`} type="checkbox" checked={item.email} onChange={(event) => change(index, "email", event.target.checked)} /></td>
        </tr>; })}</tbody>
      </table></div>
      <div className="inline-actions"><button disabled={saving}>{saving ? "Сохраняем…" : "Сохранить"}</button></div>
    </form>
  </main>;
}
