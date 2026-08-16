"use client";

import { useEffect, useMemo, useState } from "react";
import Breadcrumbs from "../breadcrumbs";
import ProBadge from "../pro-badge";
import { useAuth } from "../auth-state";
import { PricingPlansSkeleton } from "../skeletons";

type Entitlement = { feature_key: string; kind: "BOOLEAN" | "LIMIT"; enabled: boolean; limit_value?: number; unlimited: boolean };
type Plan = { id: string; code: string; name: string; description: string; tier: string; billing_period: string; currency: string; amount_kopecks: number; entitlements: Entitlement[] };
type Subscription = { id?: string; plan_name: string; status: string; current_period_end: string; cancel_at_period_end?: boolean };
type Mine = { capabilities: { effective_pro: boolean; subscription?: Subscription; features: Record<string, Entitlement> }; history: Subscription[]; provider_connected?: boolean; payment_method_configured?: boolean };
type PaymentAttempt = { id: string; status: string; amount_kopecks: number; currency: string; provider: string; created_at: string; payment_method?: string };
type Checkout = { attempt: PaymentAttempt; subscription: Subscription; confirmation_url?: string };

const labels: Record<string, string> = {
  "profile.pro_badge": "Заметный PRO-бейдж в профиле",
  "profile.analytics": "Расширенная аналитика профиля",
  "search.priority_visibility": "Приоритетная видимость в поиске",
  "portfolio.item_limit": "Кейсов в портфолио",
  "portfolio.media_limit": "Медиа в одном кейсе",
};
const rub = (v: number) => new Intl.NumberFormat("ru-RU").format(v / 100);
const date = (v: string) => new Intl.DateTimeFormat("ru-RU", { day: "numeric", month: "long", year: "numeric" }).format(new Date(v));

async function jsonRequest<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, { cache: "no-store", ...init, headers: { "Content-Type": "application/json", ...(init?.headers ?? {}) } });
  const body = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(body?.error?.message || "Не удалось выполнить операцию");
  return body.data as T;
}

export default function ProPage() {
  const { state } = useAuth();
  const [plans, setPlans] = useState<Plan[]>([]);
  const [enabled, setEnabled] = useState<boolean | null>(null);
  const [providerConnected, setProviderConnected] = useState(false);
  const [mine, setMine] = useState<Mine | null>(null);
  const [history, setHistory] = useState<PaymentAttempt[]>([]);
  const [busy, setBusy] = useState("");
  const [message, setMessage] = useState("");

  const refreshMine = async () => {
    if (state !== "authenticated") return;
    const [mineData, billingHistory] = await Promise.all([
      jsonRequest<Mine>("/api/v1/me/subscription"),
      jsonRequest<PaymentAttempt[]>("/api/v1/me/pro-billing/history").catch(() => []),
    ]);
    setMine(mineData);
    setHistory(billingHistory);
  };

  useEffect(() => {
    jsonRequest<{ pro_subscriptions_enabled: boolean; provider_connected: boolean; plans: Plan[] }>("/api/v1/monetization")
      .then((data) => {
        setEnabled(!!data.pro_subscriptions_enabled);
        setProviderConnected(!!data.provider_connected);
        setPlans((data.plans ?? []).filter((p) => p.tier === "PRO"));
      })
      .catch(() => setEnabled(false));
  }, []);

  useEffect(() => { void refreshMine().catch(() => undefined); }, [state]); // eslint-disable-line react-hooks/exhaustive-deps

  const active = !!mine?.capabilities.effective_pro;
  const current = mine?.capabilities.subscription;
  const pending = current?.status === "PENDING";
  const pastDue = current?.status === "PAST_DUE";
  const canBuy = state === "authenticated" && providerConnected && !active && !pending && !pastDue;
  const sortedHistory = useMemo(() => [...history].sort((a, b) => Date.parse(b.created_at) - Date.parse(a.created_at)), [history]);

  async function checkout(plan: Plan) {
    if (!canBuy) return;
    setBusy(plan.id); setMessage("");
    try {
      const key = `pro-${crypto.randomUUID()}`;
      const out = await jsonRequest<Checkout>("/api/v1/me/pro-billing/checkout", {
        method: "POST",
        headers: { "Idempotency-Key": key },
        body: JSON.stringify({ plan_id: plan.id }),
      });
      if (out.confirmation_url) {
        window.location.assign(out.confirmation_url);
        return;
      }
      window.location.assign(`/pro/payment-return?attempt_id=${encodeURIComponent(out.attempt.id)}`);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Не удалось начать оплату");
    } finally { setBusy(""); }
  }

  async function recoverPayment() {
    if (!pastDue || !providerConnected) return;
    setBusy("recover"); setMessage("");
    try {
      const key = `pro-recovery-${crypto.randomUUID()}`;
      const out = await jsonRequest<Checkout>("/api/v1/me/pro-billing/recover", {
        method: "POST",
        headers: { "Idempotency-Key": key },
        body: "{}",
      });
      if (out.confirmation_url) {
        window.location.assign(out.confirmation_url);
        return;
      }
      window.location.assign(`/pro/payment-return?attempt_id=${encodeURIComponent(out.attempt.id)}`);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Не удалось повторить оплату");
    } finally { setBusy(""); }
  }

  async function cancelRenewal() {
    setBusy("cancel"); setMessage("");
    try {
      await jsonRequest<Subscription>("/api/v1/me/pro-billing/cancel", { method: "POST", body: "{}" });
      setMessage("Автопродление отключено. PRO останется активным до конца оплаченного периода.");
      await refreshMine();
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Не удалось отключить автопродление");
    } finally { setBusy(""); }
  }

  if (enabled === null) return <main className="pro-page"><Breadcrumbs items={[{ label: "Главная", href: "/" }, { label: "Naimio PRO" }]}/><header className="pro-hero"><div><p className="eyebrow">Больше возможностей для роста</p><div className="pro-hero__title"><h1>Naimio PRO</h1><ProBadge/></div><p className="lead">Больше кейсов, лучше видимость и полезная аналитика. Все основные функции Naimio остаются доступными бесплатно.</p></div></header><PricingPlansSkeleton count={2}/></main>;
  if (!enabled) return <main><Breadcrumbs items={[{label:"Главная",href:"/"},{label:"Naimio PRO"}]}/><div className="empty"><h1>Naimio PRO сейчас недоступен</h1><p>Основные возможности маркетплейса продолжают работать без ограничений.</p><a className="button" href="/freelancers">Перейти в каталог</a></div></main>;

  return <main className="pro-page">
    <Breadcrumbs items={[{ label: "Главная", href: "/" }, { label: "Naimio PRO" }]}/>
    <header className="pro-hero"><div><p className="eyebrow">Больше возможностей для роста</p><div className="pro-hero__title"><h1>Naimio PRO</h1><ProBadge/></div><p className="lead">Больше кейсов, лучше видимость и полезная аналитика. Все основные функции Naimio остаются доступными бесплатно.</p><div className="pro-hero__actions"><a className="button" href="#plans">Смотреть планы</a><a className="button button--quiet" href="#compare">Сравнить возможности</a></div></div><div className="pro-orbit" aria-hidden="true"><span>PRO</span><b>◆</b></div></header>

    {message ? <div className="notice" role="status">{message}</div> : null}

    {active && current ? <section className="pro-current"><div><ProBadge/><h2>PRO активен</h2><p>{current.plan_name} · действует до {date(current.current_period_end)}</p><p>{mine?.payment_method_configured ? "Способ оплаты сохранён у платёжного провайдера для автопродления." : "Сохранённый способ оплаты не настроен."}</p>{current.cancel_at_period_end ? <p>Автопродление отключено.</p> : <button className="button button--quiet" type="button" disabled={busy === "cancel"} onClick={() => void cancelRenewal()}>{busy === "cancel" ? "Сохраняем…" : "Отключить автопродление"}</button>}</div><span className="badge">{current.status || "ACTIVE"}</span></section> : null}
    {!active && pending && current ? <section className="pro-current"><div><h2>Оплата PRO ожидает подтверждения</h2><p>{current.plan_name}. Если вы уже завершили оплату у провайдера, статус обновится после защищённого webhook или сверки.</p></div><span className="badge">Ожидает оплаты</span></section> : null}
    {pastDue && current ? <section className="pro-current"><div><h2>Не удалось продлить PRO</h2><p>Платёж за новый период не подтверждён. До конца уже оплаченного периода доступ сохраняется; автоматический retry остаётся привязан к исходному провайдеру.</p><p>{mine?.payment_method_configured ? "Способ оплаты сохранён у платёжного провайдера." : "Сохранённый способ оплаты недоступен — можно заново пройти защищённую форму провайдера."}</p><div className="pro-hero__actions">{!current.cancel_at_period_end && providerConnected ? <button className="button" type="button" disabled={busy === "recover"} onClick={() => void recoverPayment()}>{busy === "recover" ? "Создаём платёж…" : "Повторить оплату / сменить способ"}</button> : null}{current.cancel_at_period_end ? <p>Автопродление отключено.</p> : <button className="button button--quiet" type="button" disabled={busy === "cancel"} onClick={() => void cancelRenewal()}>{busy === "cancel" ? "Сохраняем…" : "Отключить автопродление"}</button>}</div></div><span className="badge">Требуется оплата</span></section> : null}

    <section id="compare"><div className="section-heading"><div><p className="eyebrow">Честная модель</p><h2>FREE остаётся полноценным</h2></div></div><div className="pro-compare"><article><h3>FREE</h3><ul><li>Профиль и базовое портфолио</li><li>Проекты, услуги и вакансии</li><li>Отклики и сообщения</li><li>Safe Deal и отзывы</li><li>Избранное и настройки</li></ul></article><article className="is-pro"><ProBadge/><ul><li>Всё, что есть в FREE</li><li>Расширенные лимиты портфолио</li><li>Визуальное отличие профиля</li><li>Приоритетная видимость</li><li>Расширенная аналитика</li></ul></article></div></section>

    <section id="plans"><div className="section-heading"><div><p className="eyebrow">Конфигурация планов</p><h2>Выберите период</h2></div><p>Цены и состав преимуществ управляются платформой.</p></div><div className="pro-plans">{plans.map(plan => <article key={plan.id}><span className="badge">{plan.billing_period === "YEAR" ? "На год" : "На месяц"}</span><h3>{plan.name}</h3><p>{plan.description}</p><strong>{rub(plan.amount_kopecks)} ₽ <small>/ {plan.billing_period === "YEAR" ? "год" : "месяц"}</small></strong><ul>{plan.entitlements.filter(v => v.enabled).map(v => <li key={v.feature_key}>{labels[v.feature_key] ?? v.feature_key}{v.kind === "LIMIT" ? `: ${v.unlimited ? "без ограничений" : v.limit_value}` : ""}</li>)}</ul>{state !== "authenticated" ? <a className="button" href="/login?next=/pro">Войти и подключить PRO</a> : active ? <button type="button" disabled>PRO уже активен</button> : pending ? <><button type="button" disabled>Платёж уже создан</button><small>Завершите текущую оплату — второй платёж не будет создан.</small></> : pastDue ? <><button type="button" disabled>Ожидается продление</button><small>Повторное списание выполняется только по существующей подписке и исходному провайдеру.</small></> : providerConnected ? <button type="button" disabled={busy === plan.id} onClick={() => void checkout(plan)}>{busy === plan.id ? "Создаём платёж…" : "Подключить PRO"}</button> : <><button type="button" disabled>Оплата временно недоступна</button><small>Платёжный маршрут не настроен. Списание не выполняется.</small></>}</article>)}</div></section>

    {state === "authenticated" && sortedHistory.length ? <section><div className="section-heading"><div><p className="eyebrow">Биллинг</p><h2>История платежей</h2></div></div><div className="table-wrap"><table><thead><tr><th>Дата</th><th>Сумма</th><th>Статус</th><th>Провайдер</th></tr></thead><tbody>{sortedHistory.map(item => <tr key={item.id}><td>{date(item.created_at)}</td><td>{rub(item.amount_kopecks)} ₽</td><td><span className="badge">{item.status}</span></td><td>{item.provider}</td></tr>)}</tbody></table></div></section> : null}
  </main>;
}
