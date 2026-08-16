"use client";
import { CustomSelect } from "./custom-select";

import { FormEvent, useEffect, useMemo, useState } from "react";
import { IconStar, IconX } from "./icons";
import { Avatar } from "./media-components";
import { ratingTone } from "./rating";
import { countLabel } from "./russian-plural";
import { useToast } from "./toast";

export type Review = {
  id: string;
  project_id: string;
  project_title?: string;
  reviewer_name?: string;
  reviewer_role: string;
  rating_overall: number;
  would_work_again?: boolean;
  text?: string;
  dimensions: Record<string, number>;
  created_at: string;
};
export type NativeTrust = {
  native_rating?: number;
  reviews_count: number;
  completed_projects_count: number;
  recommendation_rate?: number;
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
const roleLabels: Record<string, string> = { CUSTOMER: "Заказчик", FREELANCER: "Исполнитель" };
const reportReasons: Array<[string, string]> = [
  ["SPAM", "Спам или реклама"],
  ["HARASSMENT_ABUSE", "Оскорбления"],
  ["PERSONAL_DATA", "Персональные данные"],
  ["IRRELEVANT", "Не по теме"],
  ["MANIPULATION_CONFLICT", "Манипуляция или конфликт интересов"],
  ["OTHER", "Другое"],
];
const dateFmt = new Intl.DateTimeFormat("ru-RU", { day: "numeric", month: "long", year: "numeric" });

function Stars({ value }: { value: number }) {
  return (
    <span className={`review-stars ${ratingTone(value)}`} aria-label={`Оценка ${value} из 5`} title={`${value} из 5`}>
      {[1, 2, 3, 4, 5].map((n) => (
        <IconStar key={n} size={15} className={n <= value ? "is-filled" : "is-empty"} />
      ))}
    </span>
  );
}

function ReportControl({ id }: { id: string }) {
  const { push } = useToast();
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState("");
  const [state, setState] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    if (!open) return;
    const close = (event: KeyboardEvent) => { if (event.key === "Escape" && !busy) setOpen(false); };
    document.addEventListener("keydown", close);
    document.body.classList.add("modal-open");
    return () => { document.removeEventListener("keydown", close); document.body.classList.remove("modal-open"); };
  }, [open, busy]);
  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!reason) return;
    setBusy(true);
    setState("");
    try {
      const res = await fetch(`/api/v1/reviews/${id}/reports`, {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ reason_code: reason, description: "" }),
      });
      if (res.status === 401) setState("Войдите, чтобы отправить жалобу.");
      else if (res.status === 409) setState("Вы уже отправляли такую жалобу.");
      else if (res.status === 204) {
        setReason("");
        setOpen(false);
        push({ kind: "success", title: "Жалоба отправлена", message: "Модератор проверит отзыв. Сам отзыв пока остаётся видимым." });
      }
      else setState("Не удалось отправить жалобу.");
    } catch {
      setState("Не удалось отправить жалобу.");
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="review-report">
      <button type="button" className="review-report__trigger" onClick={() => { setState(""); setOpen(true); }}>Пожаловаться</button>
      {open ? <div className="modal-backdrop" role="presentation" onMouseDown={(event) => { if (event.currentTarget === event.target && !busy) setOpen(false); }}>
      <section className="review-report__modal" role="dialog" aria-modal="true" aria-labelledby={`report-${id}-title`}>
        <button type="button" className="modal-close" onClick={() => setOpen(false)} disabled={busy} aria-label="Закрыть"><IconX size={20}/></button>
        <p className="eyebrow">Модерация</p>
        <h2 id={`report-${id}-title`}>Пожаловаться на отзыв</h2>
        <p>Выберите причину. Жалоба не удаляет отзыв автоматически — её проверит модератор.</p>
      <form onSubmit={submit} className="review-report__form">
        <label>
          Причина
          <CustomSelect value={reason} onChange={(e) => setReason(e.target.value)} required>
            <option value="">Выберите причину</option>
            {reportReasons.map(([value, label]) => (
              <option key={value} value={value}>
                {label}
              </option>
            ))}
          </CustomSelect>
        </label>
        <div className="modal-actions"><button type="button" className="button button--quiet" onClick={() => setOpen(false)} disabled={busy}>Отмена</button><button disabled={busy || !reason}>{busy ? "Отправляем…" : "Отправить жалобу"}</button></div>
        {state ? (
          <p role="status" className="review-report__state">
            {state}
          </p>
        ) : null}
      </form>
      </section></div> : null}
    </div>
  );
}

export default function ReviewsSection({
  username,
  initial,
  trust,
  initialNextCursor,
  subject,
  sectionLabel,
  emptyProfileLabel,
}: {
  username: string;
  initial: Review[];
  trust: NativeTrust;
  initialNextCursor: string | null;
  subject: "freelancer" | "customer";
  sectionLabel?: string;
  emptyProfileLabel?: string;
}) {
  const [items, setItems] = useState<Review[]>(initial);
  const [cursor, setCursor] = useState<string | null>(initialNextCursor);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const distribution = useMemo(() => {
    const counts = [0, 0, 0, 0, 0];
    for (const r of items) {
      const idx = Math.min(5, Math.max(1, r.rating_overall)) - 1;
      counts[idx] += 1;
    }
    return counts;
  }, [items]);
  const shown = items.length;

  async function loadMore() {
    if (!cursor || loading) return;
    setLoading(true);
    setError("");
    try {
      const res = await fetch(
        `/api/v1/profiles/${encodeURIComponent(username)}/reviews?limit=20&cursor=${encodeURIComponent(cursor)}`,
        { credentials: "same-origin", cache: "no-store" },
      );
      if (!res.ok) throw new Error();
      const body = await res.json();
      setItems((prev) => [...prev, ...(body.data ?? [])]);
      setCursor(body.page?.next_cursor ?? null);
    } catch {
      setError("Не удалось загрузить ещё отзывы.");
    } finally {
      setLoading(false);
    }
  }

  const heading = sectionLabel ?? (subject === "customer" ? "Отзывы исполнителей" : "Отзывы на платформе");

  if (!trust.reviews_count) {
    return (
      <section className="reviews-section">
        <h2>{heading}</h2>
        {emptyProfileLabel ? <p className="reviews-new-label">{emptyProfileLabel}</p> : null}
        <p className="reviews-empty">
          Пока нет отзывов. Отзывы появляются только после завершённых безопасных сделок — их нельзя оставить
          вручную, поэтому каждый отзыв подтверждён реальным проектом.
        </p>
      </section>
    );
  }

  return (
    <section className="reviews-section">
      <h2>{heading}</h2>
      <div className="reviews-summary">
        <div className="reviews-score">
          <strong>{trust.native_rating?.toFixed(1) ?? "—"}</strong>
          <Stars value={Math.round(trust.native_rating ?? 0)} />
          <small>{countLabel(trust.reviews_count, ["отзыв", "отзыва", "отзывов"])} на платформе</small>
        </div>
        <ul className="reviews-facts">
          <li>
            <span>Завершённых проектов</span>
            <strong>{trust.completed_projects_count}</strong>
          </li>
          {trust.recommendation_rate !== undefined && trust.recommendation_rate !== null ? (
            <li>
              <span>Рекомендуют к повторной работе</span>
              <strong>{Math.round(trust.recommendation_rate)}%</strong>
            </li>
          ) : null}
        </ul>
      </div>
      {shown ? (
        <div className="reviews-distribution" aria-label="Распределение оценок среди показанных отзывов">
          {[5, 4, 3, 2, 1].map((star) => {
            const count = distribution[star - 1];
            const pct = shown ? Math.round((count / shown) * 100) : 0;
            return (
              <div className="reviews-bar" key={star}>
                <span className="reviews-bar__label">{star}★</span>
                <span className="reviews-bar__track">
                  <span className="reviews-bar__fill" style={{ width: `${pct}%` }} />
                </span>
                <span className="reviews-bar__count">{count}</span>
              </div>
            );
          })}
        </div>
      ) : null}
      <ul className="reviews-list">
        {items.map((review) => (
          <li key={review.id} className="review-card">
            <div className="review-card__head">
              <div className="review-card__author-row"><Avatar name={review.reviewer_name || "Участник платформы"} id={review.id} size="sm"/><div>
                <strong className="review-card__author">{review.reviewer_name || "Участник платформы"}</strong>
                <span className="review-card__role">{roleLabels[review.reviewer_role] ?? review.reviewer_role}</span></div>
              </div>
              <Stars value={review.rating_overall} />
            </div>
            {review.project_title ? (
              <p className="review-card__project">Проект: {review.project_title}</p>
            ) : null}
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
                <ReportControl id={review.id} />
              </div>
            </div>
          </li>
        ))}
      </ul>
      {error ? <p role="alert" className="notice notice--error">{error}</p> : null}
      {cursor ? (
        <button className="button button--quiet reviews-more" onClick={loadMore} disabled={loading}>
          {loading ? "Загрузка…" : "Показать ещё отзывы"}
        </button>
      ) : null}
    </section>
  );
}
