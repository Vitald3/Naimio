"use client";
import Breadcrumbs from "../../../../breadcrumbs";
import { use, useCallback, useEffect, useRef, useState } from "react";
import { createRandomID } from "../../../../random-id";
import { ProposalsListSkeleton } from "../../../../skeletons";
type Proposal = { id: string; freelancer_display_name?: string; message: string; price_kopecks?: number; delivery_days?: number; status: string; safe_deal_id?: string };
const money = (n: number) => new Intl.NumberFormat("ru-RU", { maximumFractionDigits: 2 }).format(n / 100) + " ₽";
const proposalStatus: Record<string, string> = { PENDING: "На рассмотрении", SHORTLISTED: "В шорт-листе", ACCEPTED: "Принят", REJECTED: "Отклонён", WITHDRAWN: "Отозван" };
export default function ProjectProposals({ params }: { params: Promise<{ id: string }> }) {const { id } = use(params);
  const [items, setItems] = useState<Proposal[]>([]);
  const [loading, setLoading] = useState(true);
  const [notice, setNotice] = useState("");
  const [acceptedDealID, setAcceptedDealID] = useState("");
  type Quote={customer_total_kopecks:number;platform_fee?:{total_kopecks:number};provider_fee?:{total_kopecks:number};fee_rule_version?:number;provider_pricing_version?:number};
  const [quotes, setQuotes] = useState<Record<number, Quote>>({});
  const fetched = useRef<Set<number>>(new Set());
  const load = useCallback(() => {
    setLoading(true);
    fetch(`/api/v1/me/projects/${encodeURIComponent(id)}/proposals`)
      .then((r) => (r.ok ? r.json() : Promise.reject()))
      .then((b) => setItems(b.data ?? []))
      .catch(() => setItems([]))
      .finally(() => setLoading(false));
  }, [id]);
  useEffect(() => { load(); }, [load]);
  // The customer total for each proposed price comes from the authoritative
  // backend quote, so the customer sees the real Safe Deal cost before accepting.
  useEffect(() => {
    items.forEach((item) => {
      const price = item.price_kopecks;
      if (!price || fetched.current.has(price)) return;
      fetched.current.add(price);
      fetch("/api/v1/me/safe-deals/quote", { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ work_amount_kopecks: price, payment_method: "CARD" }) })
        .then((r) => (r.ok ? r.json() : null)).then((b) => { if (b?.data) setQuotes((current) => ({ ...current, [price]: b.data as Quote })); }).catch(() => fetched.current.delete(price));
    });
  }, [items]);
  async function act(proposalID: string, action: string) {
    if (action === "accept") { const proposal=items.find(item=>item.id===proposalID); const quote=proposal?.price_kopecks?quotes[proposal.price_kopecks]:undefined; if(!quote)return; if(!window.confirm(`Выбрать исполнителя и создать Безопасную сделку на ${money(quote.customer_total_kopecks)} с учётом комиссий? Работа начнётся только после финансирования.`))return; }
    const response = await fetch(`/api/v1/me/projects/${encodeURIComponent(id)}/proposals/${encodeURIComponent(proposalID)}/${action}`, { method: "POST", headers: action === "accept" ? { "Idempotency-Key": createRandomID() } : undefined });
    const body = await response.json().catch(() => null);
    if (response.ok && action === "accept") {
      const safeDealID = body?.data?.safe_deal_id;
      if (typeof safeDealID === "string" && safeDealID.length > 0) setAcceptedDealID(safeDealID);
      setNotice("Исполнитель выбран. Безопасная сделка создана и ожидает оплаты.");
      await load();
      return;
    }
    setNotice(response.ok ? "Статус обновлён" : body?.error?.message || "Не удалось обновить статус. Обновите страницу и попробуйте ещё раз.");
    if (response.ok) load();
  }
  return <main>
    <Breadcrumbs items={[{label:"Главная",href:"/"},{label:"Кабинет",href:"/dashboard"},{label:"Проект",href:`/dashboard/projects/${id}`},{label:"Отклики"}]}/>
    <header className="page-heading"><div><h1>Отклики на проект</h1><p className="card-meta">Итог к оплате по безопасной сделке рассчитывает платформа и фиксирует его при принятии отклика.</p></div></header>
    {notice ? <div role="status" className="notice proposal-accepted-notice"><span>{notice}</span>{acceptedDealID ? <a className="button" href={`/dashboard/safe-deals/${encodeURIComponent(acceptedDealID)}`}>Перейти в Безопасную сделку</a> : null}</div> : null}
    {loading ? (
      <ProposalsListSkeleton count={3} />
    ) : items.length ? (
      <ul className="record-list">
        {items.map((item) => (
          <li key={item.id} className="record">
            <div className="record__head">
              <strong>{item.freelancer_display_name || "Исполнитель"}</strong>
              <span className="badge">{proposalStatus[item.status] ?? item.status}</span>
            </div>
            <p className="record__body">{item.message}</p>
            {item.price_kopecks ? (
              <div className="proposal-money">
                <span>Цена работы исполнителя: {money(item.price_kopecks)}</span>
                {quotes[item.price_kopecks] ? (
                  <>
                    <span>Комиссия платформы: {money(quotes[item.price_kopecks].platform_fee?.total_kopecks ?? 0)}</span>
                    <span>Комиссия провайдера: {money(quotes[item.price_kopecks].provider_fee?.total_kopecks ?? 0)}</span>
                    <strong>К оплате с комиссиями: {money(quotes[item.price_kopecks].customer_total_kopecks)}</strong>
                  </>
                ) : (
                  <strong>Рассчитываем комиссии…</strong>
                )}
              </div>
            ) : (
              <p className="card-meta">Для Безопасной сделки исполнитель должен указать цену</p>
            )}
            {item.delivery_days ? <p className="card-meta">Срок: {item.delivery_days} дн.</p> : null}
            {item.status === "PENDING" || item.status === "SHORTLISTED" ? (
              <div className="inline-actions">
                <button className="button button--quiet" onClick={() => act(item.id, "shortlist")}>В шорт-лист</button>
                <button className="button button--quiet" onClick={() => act(item.id, "reject")}>Отклонить</button>
                <button disabled={!item.price_kopecks || !quotes[item.price_kopecks]} onClick={() => act(item.id, "accept")}>Выбрать исполнителя</button>
              </div>
            ) : item.status === "ACCEPTED" && item.safe_deal_id ? (
              <div className="inline-actions">
                <a className="button" href={`/dashboard/safe-deals/${encodeURIComponent(item.safe_deal_id)}`}>Перейти в Безопасную сделку</a>
              </div>
            ) : null}
          </li>
        ))}
      </ul>
    ) : (
      <p className="empty">Откликов пока нет.</p>
    )}
  </main>;
}
