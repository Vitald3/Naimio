"use client";
import Breadcrumbs from "../../../../breadcrumbs";

import { use, useEffect, useState } from "react";
import ReviewForm from "../../../../review-form";

type Deal = { id: string; project_id: string; project_title?: string; counterparty_name?: string; status: string; viewer_role: "CUSTOMER" | "FREELANCER" };

export default function ReviewPage({ params }: { params: Promise<{ id: string }> }) {const { id } = use(params);
  const [state, setState] = useState<"loading" | "ready" | "pending" | "none" | "error">("loading");
  const [deal, setDeal] = useState<Deal | null>(null);

  useEffect(() => {
    fetch("/api/v1/me/safe-deals", { credentials: "same-origin" })
      .then(async (r) => {
        if (r.status === 401) {
          location.assign(`/login?next=${encodeURIComponent(location.pathname)}`);
          return null;
        }
        if (!r.ok) throw new Error();
        return r.json();
      })
      .then((body) => {
        if (!body) return;
        const deals: Deal[] = body.data ?? [];
        const forProject = deals.filter((d) => d.project_id === id);
        const completed = forProject.find((d) => d.status === "COMPLETED");
        if (completed) {
          setDeal(completed);
          setState("ready");
        } else if (forProject.length) {
          setState("pending");
        } else {
          setState("none");
        }
      })
      .catch(() => setState("error"));
  }, [id]);

  return (
    <main>
      <Breadcrumbs items={[{label:"Главная",href:"/"},{label:"Кабинет",href:"/dashboard"},{label:"Безопасные сделки",href:"/dashboard/safe-deals"},{label:"Отзыв"}]}/>
      <header>
        <p className="eyebrow">Репутация</p>
        <h1>Отзыв по проекту</h1>
        <p>Отзыв подтверждается завершённой безопасной сделкой и публикуется в профиле второй стороны.</p>
      </header>
      <section>
        {state === "loading" ? (
          <div className="skeleton skeleton--card" />
        ) : state === "error" ? (
          <p role="alert" className="notice notice--error">Не удалось проверить право на отзыв. Обновите страницу.</p>
        ) : state === "pending" ? (
          <p className="notice">Отзыв станет доступен после завершения безопасной сделки по этому проекту. Дождитесь приёмки работы и расчёта.</p>
        ) : state === "none" ? (
          <p className="notice">По этому проекту у вас нет завершённой безопасной сделки, поэтому оставить отзыв нельзя.</p>
        ) : deal ? (
          <ReviewForm projectId={id} role={deal.viewer_role} revieweeName={deal.counterparty_name} />
        ) : null}
      </section>
    </main>
  );
}
