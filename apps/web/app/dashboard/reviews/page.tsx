"use client";
import Breadcrumbs from "../../breadcrumbs";

import { useEffect, useState } from "react";
import { IconStar } from "../../icons";

type Given = {
  id: string;
  project_id: string;
  project_title?: string;
  reviewer_role: "CUSTOMER" | "FREELANCER" | string;
  rating_overall: number;
  would_work_again?: boolean;
  text?: string;
  dimensions: Record<string, number>;
  created_at: string;
};

const dimensionLabels: Record<string, string> = {
  QUALITY: "Качество работы",
  DEADLINE: "Соблюдение сроков",
  COMMUNICATION: "Коммуникация",
  BUDGET_ACCURACY: "Точность бюджета",
  BRIEF_QUALITY: "Качество брифа",
  PAYMENT_BEHAVIOR: "Платёжная дисциплина",
  REASONABLE_REVISIONS: "Разумные правки",
};
const dateFmt = new Intl.DateTimeFormat("ru-RU", { day: "numeric", month: "long", year: "numeric" });

function Stars({ value }: { value: number }) {
  return (
    <span className="review-stars" aria-label={`Оценка ${value} из 5`} title={`${value} из 5`}>
      {[1, 2, 3, 4, 5].map((n) => (
        <IconStar key={n} size={15} className={n <= value ? "is-filled" : "is-empty"} />
      ))}
    </span>
  );
}

export default function MyReviewsPage() {
  const [items, setItems] = useState<Given[]>([]);
  const [cursor, setCursor] = useState<string | null>(null);
  const [state, setState] = useState<"loading" | "ready" | "error">("loading");
  const [more, setMore] = useState(false);

  async function fetchPage(nextCursor?: string | null) {
    const qs = nextCursor ? `?limit=20&cursor=${encodeURIComponent(nextCursor)}` : "?limit=20";
    const res = await fetch(`/api/v1/me/reviews/given${qs}`, { credentials: "same-origin", cache: "no-store" });
    if (res.status === 401) {
      location.assign(`/login?next=${encodeURIComponent(location.pathname)}`);
      return null;
    }
    if (!res.ok) throw new Error();
    return res.json();
  }

  useEffect(() => {
    fetchPage()
      .then((body) => {
        if (!body) return;
        setItems(body.data ?? []);
        setCursor(body.page?.next_cursor ?? null);
        setState("ready");
      })
      .catch(() => setState("error"));
  }, []);

  async function loadMore() {
    if (!cursor || more) return;
    setMore(true);
    try {
      const body = await fetchPage(cursor);
      if (!body) return;
      setItems((prev) => [...prev, ...(body.data ?? [])]);
      setCursor(body.page?.next_cursor ?? null);
    } catch {
      setState("error");
    } finally {
      setMore(false);
    }
  }

  return (
    <>
      <Breadcrumbs items={[{label:"Главная",href:"/"},{label:"Кабинет",href:"/dashboard"},{label:"Мои отзывы"}]}/>
      <div className="page-heading">
        <div>
          <p className="eyebrow">Репутация</p>
          <h1>Мои отзывы</h1>
          <p>Отзывы, которые вы оставили о заказчиках и исполнителях по завершённым безопасным сделкам.</p>
        </div>
        <a className="button button--quiet" href="/dashboard/safe-deals">
          Безопасные сделки
        </a>
      </div>
      {state === "loading" ? (
        <div className="skeleton skeleton--card" />
      ) : state === "error" ? (
        <div className="error">Не удалось загрузить ваши отзывы. Обновите страницу.</div>
      ) : items.length ? (
        <section className="reviews-section">
          <ul className="reviews-list">
            {items.map((review) => (
              <li key={review.id} className="review-card">
                <div className="review-card__head">
                  <div>
                    <strong className="review-card__author">
                      {review.reviewer_role === "CUSTOMER" ? "Вы оценили исполнителя" : "Вы оценили заказчика"}
                    </strong>
                    {review.project_title ? (
                      <span className="review-card__role">
                        <a href={`/projects/${review.project_id}`}>{review.project_title}</a>
                      </span>
                    ) : null}
                  </div>
                  <Stars value={review.rating_overall} />
                </div>
                {review.text ? <p className="review-card__text">{review.text}</p> : null}
                {Object.keys(review.dimensions || {}).length ? (
                  <ul className="review-card__dimensions">
                    {Object.entries(review.dimensions).map(([key, score]) => (
                      <li key={key}>
                        <span>{dimensionLabels[key] ?? key}</span>
                        <strong>{score}/5</strong>
                      </li>
                    ))}
                  </ul>
                ) : null}
                <div className="review-card__foot">
                  {review.would_work_again !== undefined ? (
                    <span className={review.would_work_again ? "review-chip review-chip--yes" : "review-chip review-chip--no"}>
                      {review.would_work_again ? "Готов работать снова" : "Не готов работать снова"}
                    </span>
                  ) : (
                    <span />
                  )}
                  <div className="review-card__foot-right">
                    <time dateTime={review.created_at}>{dateFmt.format(new Date(review.created_at))}</time>
                  </div>
                </div>
              </li>
            ))}
          </ul>
          {cursor ? (
            <button className="button button--quiet reviews-more" onClick={loadMore} disabled={more}>
              {more ? "Загрузка…" : "Показать ещё"}
            </button>
          ) : null}
        </section>
      ) : (
        <div className="empty">
          <h2>Вы пока не оставляли отзывов</h2>
          <p>Отзыв можно оставить после завершённой безопасной сделки — со страницы сделки или проекта.</p>
          <a className="button button--quiet" href="/dashboard/safe-deals">
            К безопасным сделкам
          </a>
        </div>
      )}
    </>
  );
}
