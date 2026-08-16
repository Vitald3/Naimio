"use client";

import { FormEvent, useState } from "react";
import { IconStar } from "./icons";

type Role = "CUSTOMER" | "FREELANCER";

// Набор оценочных критериев зависит от роли автора отзыва и совпадает с валидацией на бэкенде.
const DIMENSIONS: Record<Role, Array<[string, string]>> = {
  CUSTOMER: [
    ["QUALITY", "Качество работы"],
    ["DEADLINE", "Соблюдение сроков"],
    ["COMMUNICATION", "Коммуникация"],
    ["BUDGET_ACCURACY", "Точность бюджета"],
  ],
  FREELANCER: [
    ["BRIEF_QUALITY", "Качество брифа"],
    ["COMMUNICATION", "Коммуникация"],
    ["PAYMENT_BEHAVIOR", "Платёжная дисциплина"],
    ["REASONABLE_REVISIONS", "Разумные правки"],
  ],
};

function StarInput({ value, onChange, label }: { value: number; onChange: (v: number) => void; label: string }) {
  return (
    <div className="star-input" role="radiogroup" aria-label={label}>
      {[1, 2, 3, 4, 5].map((n) => (
        <button
          type="button"
          key={n}
          role="radio"
          aria-checked={n === value}
          aria-label={`${n} из 5`}
          className={n <= value ? "star-input__star is-on" : "star-input__star"}
          onClick={() => onChange(n)}
        >
          <IconStar size={22} className={n <= value ? "is-filled" : "is-empty"} />
        </button>
      ))}
    </div>
  );
}

export default function ReviewForm({
  projectId,
  role,
  revieweeName,
  onSuccess,
}: {
  projectId: string;
  role: Role;
  revieweeName?: string;
  onSuccess?: () => void;
}) {
  const dims = DIMENSIONS[role];
  const [overall, setOverall] = useState(5);
  const [scores, setScores] = useState<Record<string, number>>(() => Object.fromEntries(dims.map(([k]) => [k, 5])));
  const [wouldWorkAgain, setWouldWorkAgain] = useState(true);
  const [text, setText] = useState("");
  const [status, setStatus] = useState<"idle" | "sending" | "done">("idle");
  const [error, setError] = useState("");

  const heading = role === "CUSTOMER" ? "Оцените работу исполнителя" : "Оцените сотрудничество с заказчиком";
  const target = revieweeName ? revieweeName : role === "CUSTOMER" ? "исполнителя" : "заказчика";
  const againLabel = role === "CUSTOMER" ? "Готовы снова работать с этим исполнителем?" : "Готовы снова работать с этим заказчиком?";

  async function submit(event: FormEvent) {
    event.preventDefault();
    setStatus("sending");
    setError("");
    try {
      const res = await fetch(`/api/v1/projects/${encodeURIComponent(projectId)}/reviews`, {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          rating_overall: overall,
          would_work_again: wouldWorkAgain,
          text: text.trim(),
          dimensions: scores,
        }),
      });
      if (res.status === 201) {
        setStatus("done");
        onSuccess?.();
        return;
      }
      if (res.status === 401) {
        location.assign(`/login?next=${encodeURIComponent(location.pathname)}`);
        return;
      }
      if (res.status === 409) setError("Вы уже оставили отзыв по этому проекту — второй отзыв оставить нельзя.");
      else if (res.status === 422)
        setError("Отзыв недоступен: он появляется только после завершённой безопасной сделки по этому проекту.");
      else setError("Не удалось опубликовать отзыв. Попробуйте позже.");
      setStatus("idle");
    } catch {
      setError("Не удалось опубликовать отзыв. Проверьте соединение.");
      setStatus("idle");
    }
  }

  if (status === "done") {
    return (
      <div className="review-form-done" role="status">
        <strong>Спасибо! Ваш отзыв опубликован.</strong>
        <p>Он появится в публичном профиле {target} и учтён в рейтинге на платформе.</p>
      </div>
    );
  }

  return (
    <form className="review-form" onSubmit={submit}>
      <div className="review-form__intro">
        <h3>{heading}</h3>
        <p>Отзыв виден публично и подтверждён завершённой безопасной сделкой. Оставить его можно только один раз.</p>
      </div>
      <div className="review-form__row">
        <span className="review-form__label">Общая оценка</span>
        <StarInput value={overall} onChange={setOverall} label="Общая оценка" />
      </div>
      <div className="review-form__dimensions">
        {dims.map(([key, label]) => (
          <div className="review-form__row" key={key}>
            <span className="review-form__label">{label}</span>
            <StarInput value={scores[key]} onChange={(v) => setScores((s) => ({ ...s, [key]: v }))} label={label} />
          </div>
        ))}
      </div>
      <div className="review-form__again">
        <span className="review-form__label">{againLabel}</span>
        <div className="review-form__toggle">
          <button type="button" aria-pressed={wouldWorkAgain} className={wouldWorkAgain ? "is-on" : ""} onClick={() => setWouldWorkAgain(true)}>
            Да
          </button>
          <button type="button" aria-pressed={!wouldWorkAgain} className={!wouldWorkAgain ? "is-on" : ""} onClick={() => setWouldWorkAgain(false)}>
            Нет
          </button>
        </div>
      </div>
      <label>
        Комментарий (необязательно)
        <textarea maxLength={5000} value={text} onChange={(e) => setText(e.target.value)} placeholder="Расскажите, как прошло сотрудничество" />
      </label>
      {error ? <p role="alert" className="notice notice--error">{error}</p> : null}
      <button type="submit" disabled={status === "sending"}>
        {status === "sending" ? "Публикуем…" : "Опубликовать отзыв"}
      </button>
    </form>
  );
}
