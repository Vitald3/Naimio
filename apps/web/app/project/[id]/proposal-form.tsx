"use client";
import { FormEvent, useEffect, useState } from "react";
import { useAuth } from "../../auth-state";
import { useToast } from "../../toast";
type Alloc = { total_kopecks: number; customer_kopecks: number; freelancer_kopecks: number; platform_kopecks: number };
type Quote = { work_amount_kopecks: number; platform_fee: Alloc; provider_fee: Alloc; freelancer_payout_kopecks: number };
type ExistingProposal = { id:string; project_id:string; message:string; price_kopecks?:number; delivery_days?:number; status:string };
const money = (n: number) => new Intl.NumberFormat("ru-RU", { maximumFractionDigits: 2 }).format(n / 100) + " ₽";
export default function ProposalForm({ projectId }: { projectId: string }) {
  const { state: authState, user } = useAuth();
  const { push } = useToast();
  const [message, setMessage] = useState("");
  const [price, setPrice] = useState("");
  const [days, setDays] = useState("");
  const [state, setState] = useState("");
  const [existing, setExisting] = useState<ExistingProposal | null>(null);
  const [checking, setChecking] = useState(true);
  const [quote, setQuote] = useState<Quote | null>(null);
  // Live net-payout preview. The amount comes verbatim from the authoritative
  // backend quote (CalculateDealQuote); the UI never computes money itself.
  useEffect(() => {
    const value = Number(price);
    if (!price || !Number.isFinite(value) || value <= 0) { setQuote(null); return; }
    const timer = setTimeout(() => {
      fetch("/api/v1/me/safe-deals/quote", { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ work_amount_kopecks: Math.round(value * 100), payment_method: "CARD" }) })
        .then((r) => (r.ok ? r.json() : null)).then((b) => setQuote(b?.data ?? null)).catch(() => setQuote(null));
    }, 400);
    return () => clearTimeout(timer);
  }, [price]);
  useEffect(() => {
    if (authState !== "authenticated" || !user?.capabilities?.includes("FREELANCER")) { setChecking(false); return; }
    fetch("/api/v1/me/proposals?limit=50", { credentials:"same-origin", cache:"no-store" })
      .then(r=>r.ok?r.json():Promise.reject())
      .then(body=>setExisting((body.data??[]).find((item:ExistingProposal)=>item.project_id===projectId)??null))
      .catch(()=>undefined)
      .finally(()=>setChecking(false));
  }, [authState,user?.id,user?.capabilities,projectId]);
  async function submit(event: FormEvent) {
    event.preventDefault();
    setState("Отправляем…");
    const response = await fetch(
      `/api/v1/projects/${encodeURIComponent(projectId)}/proposals`,
      {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          message,
          price_kopecks: price ? Math.round(Number(price) * 100) : null,
          currency: "RUB",
          delivery_days: days ? Number(days) : null,
        }),
      },
    );
    if (response.ok) {
      setState("");
      setMessage("");
      setPrice("");
      setDays("");
      setQuote(null);
      const body = await response.json().catch(()=>null);
      setExisting(body?.data ?? { id:"", project_id:projectId, message, price_kopecks:price?Math.round(Number(price)*100):undefined, delivery_days:days?Number(days):undefined, status:"PENDING" });
      push({ kind: "success", title: "Отклик отправлен", message: "Заказчик увидит предложение в проекте." });
      return;
    }
    let detail = "Не удалось отправить отклик.";
    try {
      const problem = await response.json();
      if (response.status === 409) detail = "Вы уже отправляли отклик на этот проект.";
      else if (response.status === 422) detail = "Проверьте цену, срок и текст предложения.";
      else if (response.status === 401) detail = "Сессия завершилась. Войдите снова.";
      else if (problem?.error?.message) detail = problem.error.message;
    } catch {}
    setState(detail);
    push({ kind: "error", title: "Отклик не отправлен", message: detail });
  }
  if (authState === "loading" || checking)
    return <section><div className="skeleton" aria-label="Проверяем возможность отклика" /></section>;
  if (authState === "anonymous")
    return <section className="notice"><h2>Хотите откликнуться?</h2><p>Войдите как специалист, чтобы отправить заказчику предложение.</p><a className="button" href={`/login?next=${encodeURIComponent(`/projects/${projectId}`)}`}>Войти</a></section>;
  if (!user?.capabilities?.includes("FREELANCER")) return null;
  if (existing) return <section className="response-summary"><div className="response-summary__head"><div><p className="eyebrow">Ваш отклик</p><h2>Предложение уже отправлено</h2></div><span className="badge">{existing.status === "PENDING" ? "На рассмотрении" : existing.status}</span></div><p className="response-summary__message">{existing.message}</p><div className="response-summary__facts"><span><strong>{existing.price_kopecks ? money(existing.price_kopecks) : "Цена не указана"}</strong><small>Стоимость</small></span><span><strong>{existing.delivery_days ? `${existing.delivery_days} дн.` : "Не указан"}</strong><small>Срок</small></span></div><a className="button button--quiet" href="/dashboard/proposals">Открыть мои отклики</a></section>;
  return (
    <section>
      <h2>Откликнуться</h2>
      <form onSubmit={submit}>
        <label>Сообщение<textarea required maxLength={5000} value={message} onChange={(e) => setMessage(e.target.value)} /></label>
        <div className="field-row">
          <label>Цена, ₽<input type="number" min="1" value={price} onChange={(e) => setPrice(e.target.value)} /></label>
          <label>Срок, дней<input type="number" min="1" max="3650" value={days} onChange={(e) => setDays(e.target.value)} /></label>
        </div>
        {quote ? <div className="quote-preview">
          <p className="quote-preview__title">Расчёт выплаты и комиссий</p>
          <div className="quote-preview__row"><span>Стоимость проекта</span><span>{money(quote.work_amount_kopecks)}</span></div>
          <div className="quote-preview__row"><span>Комиссия сервиса</span><span>{money(quote.platform_fee.total_kopecks)}</span></div>
          <div className="quote-preview__row"><span>Комиссия платёжного провайдера</span><span>{money(quote.provider_fee.total_kopecks)}</span></div>
          {quote.platform_fee.freelancer_kopecks > 0 ? <div className="quote-preview__row"><span>Удержание комиссии сервиса из вашей выплаты</span><span>−{money(quote.platform_fee.freelancer_kopecks)}</span></div> : null}
          {quote.provider_fee.freelancer_kopecks > 0 ? <div className="quote-preview__row"><span>Удержание комиссии провайдера из вашей выплаты</span><span>−{money(quote.provider_fee.freelancer_kopecks)}</span></div> : null}
          <div className="quote-preview__row quote-preview__row--total"><span>Вы получите</span><strong>{money(quote.freelancer_payout_kopecks)}</strong></div>
          <p className="quote-preview__note">Комиссии показаны полностью. Если часть комиссии оплачивает заказчик или платформа, она не уменьшает вашу выплату. Итоговые условия фиксируются в момент принятия отклика.</p>
        </div> : null}
        <button type="submit">Отправить отклик</button>
        {state ? <p role="status" className="notice">{state}</p> : null}
      </form>
    </section>
  );
}
