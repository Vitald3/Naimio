"use client";
import Breadcrumbs from "../../breadcrumbs";

import { useCallback, useEffect, useState } from "react";
import { useAuth } from "../../auth-state";

type Proposal = {
  id: string;
  project_id: string;
  project_title?: string;
  message: string;
  price_kopecks?: number;
  delivery_days?: number;
  status: string;
  submitted_at: string;
};
const statusLabels: Record<string, string> = {
  PENDING: "На рассмотрении",
  SHORTLISTED: "В шорт-листе",
  ACCEPTED: "Принят",
  REJECTED: "Отклонён",
  WITHDRAWN: "Отозван",
};
const money = (value?: number) =>
  value
    ? `${new Intl.NumberFormat("ru-RU").format(value / 100)} ₽`
    : "Цена не указана";

export default function MyProposals() {
  const { state: authState, user } = useAuth();
  const [items, setItems] = useState<Proposal[]>([]);
  const [state, setState] = useState("loading");
  const load = useCallback(
    () =>
      fetch("/api/v1/me/proposals?limit=50", { credentials: "same-origin" })
        .then((r) => (r.ok ? r.json() : Promise.reject()))
        .then((b) => {
          setItems(b.data ?? []);
          setState("ready");
        })
        .catch(() => setState("error")),
    [],
  );
  useEffect(() => {
    if (
      authState === "authenticated" &&
      user?.capabilities?.includes("FREELANCER")
    )
      load();
    else if (authState === "authenticated") setState("ineligible");
  }, [authState, user?.id, user?.capabilities, load]);
  async function withdraw(id: string) {
    if (!confirm("Отозвать отклик?")) return;
    const r = await fetch(`/api/v1/me/proposals/${id}/withdraw`, {
      method: "POST",
      credentials: "same-origin",
    });
    if (r.ok) load();
    else setState("error");
  }
  if (authState === "loading" || state === "loading")
    return <div className="skeleton" />;
  if (state === "ineligible") return (<div className="notice"><h1>Раздел для специалистов</h1><p>«Мои отклики» относятся только к предложениям специалиста по проектам. У заказчика этот пункт скрыт.</p></div>);
  if (state === "error")
    return (
      <div className="error">
        Не удалось загрузить отклики. Попробуйте ещё раз.
      </div>
    );
  return (
    <main>
      <Breadcrumbs items={[{label:"Главная",href:"/"},{label:"Кабинет",href:"/dashboard"},{label:"Мои отклики"}]}/>
      <div className="page-heading">
        <div>
          <p className="eyebrow">Поиск работы</p>
          <h1>Мои отклики</h1>
          <p>Предложения по проектам и их текущие статусы.</p>
        </div>
        <a className="button" href="/projects">
          Найти проект
        </a>
      </div>
      {items.length ? (
        <ul className="response-card-grid">
          {items.map((item) => (
            <li key={item.id}>
              <article>
                <span className="badge">
                  {statusLabels[item.status] ?? item.status}
                </span>
                <h2>
                  <a href={`/projects/${item.project_id}`}>{item.project_title || "Открыть проект"}</a>
                </h2>
                <p>{item.message}</p>
                <p>
                  <strong>{money(item.price_kopecks)}</strong>
                  {item.delivery_days ? ` · ${item.delivery_days} дн.` : ""}
                </p>
                <p>
                  Отправлен{" "}
                  {new Intl.DateTimeFormat("ru-RU").format(
                    new Date(item.submitted_at),
                  )}
                </p>
                {["PENDING", "SHORTLISTED"].includes(item.status) ? (
                  <button type="button" onClick={() => withdraw(item.id)}>
                    Отозвать отклик
                  </button>
                ) : null}
              </article>
            </li>
          ))}
        </ul>
      ) : (
        <div className="empty">
          <h2>Откликов пока нет</h2>
          <p>
            Выберите подходящий открытый проект и отправьте заказчику
            предложение.
          </p>
          <a className="button button--quiet" href="/projects">
            Смотреть проекты
          </a>
        </div>
      )}
    </main>
  );
}
