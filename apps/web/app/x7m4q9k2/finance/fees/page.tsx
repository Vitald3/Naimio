"use client";
import { CustomSelect } from "../../../custom-select";
import { FormEvent, useCallback, useEffect, useState } from "react";
import { AdminError, AdminHeader, AdminLoading, AdminTable, formatDate, formatMoney } from "../../admin-ui";
import { createRandomID } from "../../../random-id";
import AdminReasonEditor from "../../admin-reason-editor";

// Two independent, versioned economics tables — platform commission (revenue)
// and provider/Safe-Deal cost — configured here. Every change creates a NEW
// version with confirmation + reason + audit entry and never mutates history,
// so existing Safe Deals (which snapshot their own economics) are untouched.

type FeeRule = {
  version: number;
  commission_basis_points: number;
  minimum_fee_kopecks: number;
  maximum_fee_kopecks?: number | null;
  platform_fee_payer_mode: string;
  platform_customer_share_basis_points: number;
  provider_fee_payer_mode: string;
  provider_customer_share_basis_points: number;
  enabled: boolean;
  effective_from: string;
  created_at: string;
};
type PaymentProviderSetting = { provider: string; enabled: boolean; configured: boolean; environment: string };
type PricingRule = {
  version: number;
  provider: string;
  payment_method: string;
  percent_basis_points: number;
  fixed_fee_kopecks: number;
  minimum_fee_kopecks: number;
  maximum_fee_kopecks?: number | null;
  enabled: boolean;
  effective_from: string;
  created_at: string;
};

const payerLabels: Record<string, string> = { CUSTOMER: "Заказчик", FREELANCER: "Исполнитель", SPLIT: "Пополам", PLATFORM: "Платформа" };
const methodLabels: Record<string, string> = { CARD: "Карта", SBP: "СБП", T_PAY: "T-Pay", SBER_PAY: "SberPay", OTHER: "Другой" };
const providerLabels: Record<string,string> = { yookassa: "ЮKassa", tbank: "Т-Банк", yandex_pay: "Яндекс Пэй", cloudpayments: "CloudPayments", robokassa: "Robokassa", sandbox: "Sandbox" };
const payerOptions = ["CUSTOMER", "FREELANCER", "SPLIT", "PLATFORM"];
const methodOptions = ["CARD", "SBP", "T_PAY", "SBER_PAY", "OTHER"];

// The API returns stable English error codes; the UI renders Russian.
const errorText: Record<string, string> = {
  FORBIDDEN: "Недостаточно прав: экономику платежей меняют только администраторы.",
  UNAUTHENTICATED: "Требуется вход администратора.",
  CONFIRMATION_REQUIRED: "Требуется подтверждение изменения.",
  REASON_REQUIRED: "Укажите причину изменения (не короче 3 символов).",
  VALIDATION_ERROR: "Значения правила не прошли проверку — исправьте и попробуйте снова.",
  NOT_FOUND: "Раздел не найден.",
  INTERNAL_ERROR: "Внутренняя ошибка сервера. Попробуйте позже.",
};
async function financeRequest<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, { credentials: "same-origin", cache: "no-store", ...init, headers: { "Content-Type": "application/json", ...(init?.headers || {}) } });
  if (!response.ok) {
    let code = "";
    try { const body = await response.json(); code = body?.error?.code || ""; } catch {}
    throw new Error(errorText[code] ?? `Не удалось выполнить операцию (HTTP ${response.status}).`);
  }
  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}

// Admin enters human values (percent, rubles); we convert to the integer
// basis-points / kopecks the API stores. This is config entry, not deal money —
// the authoritative per-deal amounts are always computed by the backend.
const toBasisPoints = (percent: string) => Math.round(Number(percent) * 100);
const toKopecks = (rubles: string) => Math.round(Number(rubles) * 100);
const optionalKopecks = (rubles: string) => (rubles.trim() === "" ? null : toKopecks(rubles));
const percentText = (basisPoints: number) => `${new Intl.NumberFormat("ru-RU", { maximumFractionDigits: 2 }).format(basisPoints / 100)}%`;
const payerText = (mode: string, shareBasisPoints: number) => (mode === "SPLIT" ? `${payerLabels[mode] ?? mode} · заказчик ${percentText(shareBasisPoints)}` : payerLabels[mode] ?? mode);
const limitText = (kopecks?: number | null) => (kopecks === null || kopecks === undefined ? "без лимита" : formatMoney(kopecks));

export default function FinanceFeesPage() {
  const [fees, setFees] = useState<FeeRule[]>([]);
  const [pricing, setPricing] = useState<PricingRule[]>([]);
  const [providers, setProviders] = useState<PaymentProviderSetting[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState("");

  // Fee-rule form.
  const [fCommission, setFCommission] = useState("10");
  const [fMin, setFMin] = useState("0");
  const [fMax, setFMax] = useState("");
  const [fPlatformPayer, setFPlatformPayer] = useState("FREELANCER");
  const [fPlatformShare, setFPlatformShare] = useState("50");
  const [fProviderPayer, setFProviderPayer] = useState("FREELANCER");
  const [fProviderShare, setFProviderShare] = useState("50");
  const [fReason, setFReason] = useState("");
  const [fConfirm, setFConfirm] = useState(false);
  const [feeMsg, setFeeMsg] = useState("");
  const [feeErr, setFeeErr] = useState("");
  const [feeBusy, setFeeBusy] = useState(false);

  // Provider-pricing form.
  const [pProvider, setPProvider] = useState("yookassa");
  const [pMethod, setPMethod] = useState("CARD");
  const [pPercent, setPPercent] = useState("2");
  const [pFixed, setPFixed] = useState("0");
  const [pMin, setPMin] = useState("0");
  const [pMax, setPMax] = useState("");
  const [pReason, setPReason] = useState("");
  const [pConfirm, setPConfirm] = useState(false);
  const [priceMsg, setPriceMsg] = useState("");
  const [priceErr, setPriceErr] = useState("");
  const [priceBusy, setPriceBusy] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setLoadError("");
    try {
      const [feeBody, priceBody, providerBody] = await Promise.all([
        financeRequest<{ data: FeeRule[] }>("/api/v1/admin/finance/fees"),
        financeRequest<{ data: PricingRule[] }>("/api/v1/admin/finance/provider-pricing"),
        financeRequest<{ data: PaymentProviderSetting[] }>("/api/v1/admin/payment-routing/providers"),
      ]);
      setFees(feeBody.data ?? []);
      setPricing(priceBody.data ?? []);
      setProviders(providerBody.data ?? []);
      const first = (providerBody.data ?? []).find((provider) => provider.configured)?.provider;
      if (first) setPProvider((current) => current === "yookassa" ? first : current);
    } catch (reason) {
      setLoadError(reason instanceof Error ? reason.message : "Не удалось загрузить экономику платежей.");
    } finally {
      setLoading(false);
    }
  }, []);
  useEffect(() => { load(); }, [load]);

  async function submitFee(event: FormEvent) {
    event.preventDefault();
    setFeeMsg(""); setFeeErr(""); setFeeBusy(true);
    try {
      await financeRequest("/api/v1/admin/finance/fees", {
        method: "POST",
        headers: { "X-Request-ID": createRandomID() },
        body: JSON.stringify({
          commission_basis_points: toBasisPoints(fCommission),
          minimum_fee_kopecks: toKopecks(fMin),
          maximum_fee_kopecks: optionalKopecks(fMax),
          platform_fee_payer_mode: fPlatformPayer,
          platform_customer_share_basis_points: fPlatformPayer === "SPLIT" ? toBasisPoints(fPlatformShare) : 0,
          provider_fee_payer_mode: fProviderPayer,
          provider_customer_share_basis_points: fProviderPayer === "SPLIT" ? toBasisPoints(fProviderShare) : 0,
          confirm: fConfirm,
          reason: fReason.trim(),
        }),
      });
      setFeeMsg("Создана новая версия правил комиссии. Изменение записано в аудит; существующие сделки не затронуты.");
      setFReason(""); setFConfirm(false);
      await load();
    } catch (reason) {
      setFeeErr(reason instanceof Error ? reason.message : "Не удалось сохранить правило.");
    } finally {
      setFeeBusy(false);
    }
  }

  async function submitPricing(event: FormEvent) {
    event.preventDefault();
    setPriceMsg(""); setPriceErr(""); setPriceBusy(true);
    try {
      await financeRequest("/api/v1/admin/finance/provider-pricing", {
        method: "POST",
        headers: { "X-Request-ID": createRandomID() },
        body: JSON.stringify({
          provider: pProvider.trim(),
          payment_method: pMethod,
          percent_basis_points: toBasisPoints(pPercent),
          fixed_fee_kopecks: toKopecks(pFixed),
          minimum_fee_kopecks: toKopecks(pMin),
          maximum_fee_kopecks: optionalKopecks(pMax),
          confirm: pConfirm,
          reason: pReason.trim(),
        }),
      });
      setPriceMsg("Создана новая версия тарифа провайдера. Изменение записано в аудит; существующие сделки не затронуты.");
      setPReason(""); setPConfirm(false);
      await load();
    } catch (reason) {
      setPriceErr(reason instanceof Error ? reason.message : "Не удалось сохранить тариф.");
    } finally {
      setPriceBusy(false);
    }
  }

  return <>
    <AdminHeader title="Экономика и комиссии" description="Комиссия платформы (доход) и себестоимость конкретного платёжного провайдера настраиваются отдельно. Для новых Safe Deal применяется тариф PSP, выбранного текущим маршрутом SAFE_DEAL; существующие сделки сохраняют уже рассчитанную экономику." />

    <section className="admin-panel">
      <p className="eyebrow">Комиссия платформы</p>
      <p className="admin-hint">Ставка комиссии, минимум и максимум, а также кто оплачивает комиссию и стоимость эквайринга. Для режима «Пополам» укажите долю заказчика (например, 50% — поровну; 30% — 30/70).</p>
      <form className="admin-config-form finance-form finance-form--platform" onSubmit={submitFee}>
        <div className="finance-form__row">
          <label>Комиссия, %<input type="number" min="0" max="100" step="0.01" required value={fCommission} onChange={(e) => setFCommission(e.target.value)} /></label>
          <label>Минимум, ₽<input type="number" min="0" step="0.01" required value={fMin} onChange={(e) => setFMin(e.target.value)} /></label>
          <label>Максимум, ₽ (необязательно)<input type="number" min="0" step="0.01" placeholder="без лимита" value={fMax} onChange={(e) => setFMax(e.target.value)} /></label>
        </div>
        <div className="finance-form__row">
          <label>Комиссию платит<CustomSelect value={fPlatformPayer} onChange={(e) => setFPlatformPayer(e.target.value)}>{payerOptions.map((mode) => <option key={mode} value={mode}>{payerLabels[mode]}</option>)}</CustomSelect></label>
          {fPlatformPayer === "SPLIT" ? <label>Доля заказчика в комиссии, %<input type="number" min="0" max="100" step="0.01" value={fPlatformShare} onChange={(e) => setFPlatformShare(e.target.value)} /></label> : <span aria-hidden="true" />}
          <label>Эквайринг платит<CustomSelect value={fProviderPayer} onChange={(e) => setFProviderPayer(e.target.value)}>{payerOptions.map((mode) => <option key={mode} value={mode}>{payerLabels[mode]}</option>)}</CustomSelect></label>
        </div>
        {fProviderPayer === "SPLIT" ? <div className="finance-form__row finance-form__row--single"><label>Доля заказчика в эквайринге, %<input type="number" min="0" max="100" step="0.01" value={fProviderShare} onChange={(e) => setFProviderShare(e.target.value)} /></label></div> : null}
        <label className="finance-form__wide">Причина изменения<AdminReasonEditor value={fReason} onChange={setFReason}/></label>
        <label className="finance-form__confirm"><input type="checkbox" checked={fConfirm} onChange={(e) => setFConfirm(e.target.checked)} /><span>Подтверждаю создание новой версии правил комиссии</span></label>
        <button className="finance-form__wide" disabled={feeBusy || !fConfirm || fReason.trim().length < 3}>{feeBusy ? "Сохраняем…" : "Создать новую версию"}</button>
      </form>
      {feeMsg ? <p className="notice" role="status">{feeMsg}</p> : null}
      {feeErr ? <p className="form-error" role="alert">{feeErr}</p> : null}
    </section>

    {loadError ? <AdminError message={loadError} onRetry={load} /> : loading ? <AdminLoading /> : <>
      <section className="admin-panel">
        <p className="eyebrow">Версии правил комиссии</p>
        <AdminTable columns={["Версия", "Комиссия", "Мин. / Макс.", "Комиссию платит", "Эквайринг платит", "Статус", "Действует с"]} empty={!fees.length}>
          {fees.map((rule) => <tr key={rule.version}>
            <td><strong>v{rule.version}</strong></td>
            <td><strong>{percentText(rule.commission_basis_points)}</strong></td>
            <td>{formatMoney(rule.minimum_fee_kopecks)}<small>{limitText(rule.maximum_fee_kopecks)}</small></td>
            <td>{payerText(rule.platform_fee_payer_mode, rule.platform_customer_share_basis_points)}</td>
            <td>{payerText(rule.provider_fee_payer_mode, rule.provider_customer_share_basis_points)}</td>
            <td><span className={rule.enabled ? "status-pill status-pill--positive" : "status-pill"}>{rule.enabled ? "Активна" : "Архив"}</span></td>
            <td>{formatDate(rule.effective_from)}</td>
          </tr>)}
        </AdminTable>
      </section>

      <section className="admin-panel">
        <p className="eyebrow">Тарифы провайдеров (стоимость эквайринга)</p>
        <p className="admin-hint">Это тарифы именно PSP: процент и фиксированная себестоимость эквайринга. Они не включают и не выбирают провайдера — текущий PSP выбирается в «Платёжных провайдерах». При расчёте новой Safe Deal backend берёт тариф того провайдера, который назначен маршруту SAFE_DEAL, и выбранного способа оплаты. Для старых подключений без тарифа миграция создаёт стартовую оценку 2% для CARD; обязательно замените её на фактический тариф вашего договора с PSP.</p>
        <form className="admin-config-form finance-form finance-form--provider" onSubmit={submitPricing}>
          <div className="finance-form__row">
            <label>Провайдер<CustomSelect required value={pProvider} onChange={(e) => setPProvider(e.target.value)}>{providers.filter((provider) => provider.configured).map((provider) => <option key={provider.provider} value={provider.provider}>{providerLabels[provider.provider] ?? provider.provider}{provider.enabled ? "" : " — выключен"}</option>)}</CustomSelect></label>
            <label>Способ оплаты<CustomSelect value={pMethod} onChange={(e) => setPMethod(e.target.value)}>{methodOptions.map((method) => <option key={method} value={method}>{methodLabels[method]}</option>)}</CustomSelect></label>
            <label>Процент, %<input type="number" min="0" max="100" step="0.01" required value={pPercent} onChange={(e) => setPPercent(e.target.value)} /></label>
          </div>
          <div className="finance-form__row">
            <label>Фиксированная часть, ₽<input type="number" min="0" step="0.01" required value={pFixed} onChange={(e) => setPFixed(e.target.value)} /></label>
            <label>Минимум, ₽<input type="number" min="0" step="0.01" required value={pMin} onChange={(e) => setPMin(e.target.value)} /></label>
            <label>Максимум, ₽ (необязательно)<input type="number" min="0" step="0.01" placeholder="без лимита" value={pMax} onChange={(e) => setPMax(e.target.value)} /></label>
          </div>
          <label className="finance-form__wide">Причина изменения<AdminReasonEditor value={pReason} onChange={setPReason}/></label>
          <label className="finance-form__confirm"><input type="checkbox" checked={pConfirm} onChange={(e) => setPConfirm(e.target.checked)} /><span>Подтверждаю создание новой версии тарифа</span></label>
          <button className="finance-form__wide" disabled={priceBusy || !pConfirm || pReason.trim().length < 3}>{priceBusy ? "Сохраняем…" : "Создать новую версию"}</button>
        </form>
        {priceMsg ? <p className="notice" role="status">{priceMsg}</p> : null}
        {priceErr ? <p className="form-error" role="alert">{priceErr}</p> : null}
        <div style={{ marginTop: 16 }}>
          <AdminTable columns={["Версия", "Провайдер", "Способ", "Процент", "Фикс.", "Мин. / Макс.", "Статус", "Действует с"]} empty={!pricing.length}>
            {pricing.map((rule) => <tr key={`${rule.provider}-${rule.payment_method}-${rule.version}`}>
              <td><strong>v{rule.version}</strong></td>
              <td>{providerLabels[rule.provider] ?? rule.provider}</td>
              <td>{methodLabels[rule.payment_method] ?? rule.payment_method}</td>
              <td>{percentText(rule.percent_basis_points)}</td>
              <td>{formatMoney(rule.fixed_fee_kopecks)}</td>
              <td>{formatMoney(rule.minimum_fee_kopecks)}<small>{limitText(rule.maximum_fee_kopecks)}</small></td>
              <td><span className={rule.enabled ? "status-pill status-pill--positive" : "status-pill"}>{rule.enabled ? "Активен" : "Архив"}</span></td>
              <td>{formatDate(rule.effective_from)}</td>
            </tr>)}
          </AdminTable>
        </div>
      </section>
    </>}
  </>;
}
