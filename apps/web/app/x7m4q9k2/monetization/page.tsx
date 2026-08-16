"use client";
import { FormEvent, useCallback, useEffect, useState } from "react";
import { CustomSelect } from "../../custom-select";
import { useToast } from "../../toast";
import {
  AdminError,
  AdminHeader,
  AdminLoading,
  AdminReasonAction,
  AdminTable,
  StatusPill,
  adminRequest,
  formatDate,
  formatMoney,
} from "../admin-ui";
type Entitlement = {
  feature_key: string;
  kind: string;
  enabled: boolean;
  limit_value?: number;
  unlimited: boolean;
  config: Record<string, unknown>;
};
type Plan = {
  id: string;
  code: string;
  name: string;
  description: string;
  tier: string;
  billing_period: string;
  currency: string;
  amount_kopecks: number;
  active: boolean;
  display_order: number;
  entitlements: Entitlement[];
};
type Subscription = {
  id: string;
  user_id: string;
  user_name: string;
  plan_name: string;
  status: string;
  starts_at: string;
  current_period_end: string;
};
type Data = {
  overview: {
    pro_system_enabled: boolean;
    active_count: number;
    new_30_days: number;
    expiring_7_days: number;
    provider_connected: boolean;
  };
  plans: Plan[];
  subscriptions: Subscription[];
};
type Flag = { key: string; enabled: boolean; config: Record<string, unknown> };
type PaymentRoute = { domain: string; provider: string; enabled: boolean; configured: boolean; environment: string };
type PaymentProvider = { provider: string; enabled: boolean; configured: boolean; environment: string; capabilities?: string[] };
const providerLabels: Record<string,string> = { yookassa: "ЮKassa", tbank: "Т-Банк", yandex_pay: "Яндекс Пэй", cloudpayments: "CloudPayments", robokassa: "Robokassa" };
const supportsPro = (provider: PaymentProvider) => { const caps = new Set(provider.capabilities ?? []); return caps.has("ONE_TIME_PAYMENT") && (caps.has("MERCHANT_MANAGED_RECURRING") || caps.has("PROVIDER_MANAGED_SUBSCRIPTION")); };
export default function MonetizationPage() {
  const { push } = useToast();
  const [data, setData] = useState<Data | null>(null),
    [error, setError] = useState(""),
    [loading, setLoading] = useState(true);
  const [flags, setFlags] = useState<Flag[]>([]);
  const [paymentRoutes, setPaymentRoutes] = useState<PaymentRoute[]>([]);
  const [paymentProviders, setPaymentProviders] = useState<PaymentProvider[]>([]);
  const [providerBusy, setProviderBusy] = useState(false);
  const load = useCallback(() => {
    setLoading(true);
    Promise.all([
      adminRequest<{ data: Data }>("/api/v1/admin/monetization"),
      adminRequest<{ data: Flag[] }>("/api/v1/admin/feature-flags"),
      adminRequest<{ data: PaymentRoute[] }>("/api/v1/admin/payment-routing"),
      adminRequest<{ data: PaymentProvider[] }>("/api/v1/admin/payment-routing/providers"),
    ])
      .then(([m, f, routes, providers]) => {
        setData(m.data);
        setFlags(f.data);
        setPaymentRoutes(routes.data ?? []);
        setPaymentProviders(providers.data ?? []);
        setError("");
      })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, []);
  useEffect(load, [load]);
  const flag = flags.find((v) => v.key === "pro_subscriptions_enabled");
  const proRoute = paymentRoutes.find((route) => route.domain === "PRO_SUBSCRIPTION");
  const proProvider = proRoute ? paymentProviders.find((provider) => provider.provider === proRoute.provider) : undefined;
  const proReady = Boolean(proRoute?.enabled && proRoute?.configured && proProvider?.enabled && proProvider?.configured && proProvider && supportsPro(proProvider));
  async function setProProvider(provider: string) {
    setProviderBusy(true);
    try {
      await adminRequest("/api/v1/admin/payment-routing/routes/PRO_SUBSCRIPTION", { method: "PUT", body: JSON.stringify({ provider }) });
      push({ kind: "success", title: "Провайдер PRO изменён", message: `Новые подписки и продления будут направляться через ${providerLabels[provider] ?? provider}.` });
      load();
    } catch (e) {
      push({ kind: "error", title: "Не удалось изменить провайдера PRO", message: e instanceof Error ? e.message : "Проверьте конфигурацию провайдера" });
    } finally { setProviderBusy(false); }
  }
  async function toggle(reason: string) {
    if (!flag) return;
    await adminRequest(`/api/v1/admin/feature-flags/${flag.key}`, {
      method: "PATCH",
      body: JSON.stringify({
        enabled: !flag.enabled,
        config: flag.config || {},
        reason,
      }),
    });
    push({
      kind: "success",
      title: flag.enabled ? "PRO выключен" : "PRO включён",
      message:
        "Записи подписок сохранены; изменение влияет только на продуктовые поверхности и привилегии.",
    });
    load();
  }
  async function transition(
    id: string,
    action: "cancel" | "expire",
    reason: string,
  ) {
    await adminRequest(
      `/api/v1/admin/monetization/subscriptions/${id}/${action}`,
      { method: "POST", body: JSON.stringify({ reason }) },
    );
    load();
  }
  return (
    <>
      <AdminHeader
        title="Монетизация / PRO"
        description="Планы, права доступа и жизненный цикл подписок. Safe Deal остаётся отдельным доменом."
        actions={
          flag ? (
            <AdminReasonAction
              label={flag.enabled ? "Выключить PRO" : "Включить PRO"}
              tone={flag.enabled ? "danger" : "default"}
              title={`${flag.enabled ? "Выключить" : "Включить"} PRO-подписки`}
              description={
                flag.enabled
                  ? "Покупка и PRO-привилегии исчезнут, но все записи сохранятся."
                  : "Действующие подписки снова получат свои привилегии."
              }
              onConfirm={toggle}
            />
          ) : null
        }
      />
      {loading ? (
        <AdminLoading />
      ) : error ? (
        <AdminError message={error} onRetry={load} />
      ) : data ? (
        <>
          <div className="admin-metrics">
            <article>
              <span>Активных PRO</span>
              <strong>{data.overview.active_count}</strong>
            </article>
            <article>
              <span>Новых за 30 дней</span>
              <strong>{data.overview.new_30_days}</strong>
            </article>
            <article>
              <span>Истекают за 7 дней</span>
              <strong>{data.overview.expiring_7_days}</strong>
            </article>
            <article>
              <span>Платёжный провайдер</span>
              <strong>{proReady ? (providerLabels[proRoute?.provider ?? ""] ?? proRoute?.provider) : "Не подключён"}</strong>
              <small>{proRoute ? `${proRoute.environment === "production" ? "Production" : "Sandbox"} · ${proReady ? "готов" : "маршрут не готов"}` : "маршрут PRO не задан"}</small>
            </article>
          </div>
          <section className="admin-section">
            <h2>Провайдер PRO-платежей</h2>
            <p>Этот маршрут определяет PSP для новых покупок PRO и автоматических продлений. Уже созданные платежи остаются закреплены за исходным провайдером.</p>
            <div className="form-grid">
              <label className="field"><span>Текущий провайдер</span><CustomSelect value={proRoute?.provider ?? ""} disabled={providerBusy} onChange={(e) => void setProProvider(e.target.value)}>
                <option value="" disabled>Выберите провайдера</option>
                {paymentProviders.filter((provider) => provider.enabled && provider.configured && supportsPro(provider)).map((provider) => <option key={provider.provider} value={provider.provider}>{providerLabels[provider.provider] ?? provider.provider} · {provider.environment === "production" ? "Production" : "Sandbox"}</option>)}
              </CustomSelect></label>
            </div>
            {!paymentProviders.some((provider) => provider.enabled && provider.configured && supportsPro(provider)) ? <p className="admin-warning">Сначала настройте и включите хотя бы один PSP с поддержкой разовых и повторных платежей в разделе «Платёжные провайдеры».</p> : null}
          </section>
          <section className="admin-section">
            <h2>Планы и преимущества</h2>
            <div className="monetization-plans">
              {data.plans.map((plan) => (
                <PlanEditor key={plan.id} plan={plan} saved={load} />
              ))}
            </div>
          </section>
          <GrantForm
            plans={data.plans.filter((p) => p.tier === "PRO" && p.active)}
            saved={load}
          />
          <section className="admin-section">
            <h2>Подписки</h2>
            <AdminTable
              columns={["Пользователь", "План", "Статус", "Период", "Действия"]}
              empty={!data.subscriptions.length}
            >
              {data.subscriptions.map((s) => (
                <tr key={s.id}>
                  <td>
                    <strong>{s.user_name}</strong>
                    <small>{s.user_id}</small>
                  </td>
                  <td>{s.plan_name}</td>
                  <td>
                    <StatusPill value={s.status} />
                  </td>
                  <td>
                    <small>
                      {formatDate(s.starts_at)} —{" "}
                      {formatDate(s.current_period_end)}
                    </small>
                  </td>
                  <td>
                    <div className="inline-actions">
                      {!["CANCELED", "EXPIRED"].includes(s.status) ? (
                        <>
                          <AdminReasonAction
                            label="Отменить"
                            tone="danger"
                            title="Отменить подписку"
                            onConfirm={(r) => transition(s.id, "cancel", r)}
                          />
                          <AdminReasonAction
                            label="Истечь сейчас"
                            tone="danger"
                            title="Завершить подписку"
                            onConfirm={(r) => transition(s.id, "expire", r)}
                          />
                        </>
                      ) : (
                        <small>История сохранена</small>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
            </AdminTable>
          </section>
        </>
      ) : null}
    </>
  );
}
function PlanEditor({ plan, saved }: { plan: Plan; saved: () => void }) {
  const [value, setValue] = useState(plan),
    [saving, setSaving] = useState(false),
    [reason, setReason] = useState("Обновление конфигурации плана");
  async function save(e: FormEvent) {
    e.preventDefault();
    if (!reason.trim()) return;
    setSaving(true);
    try {
      await adminRequest(`/api/v1/admin/monetization/plans/${plan.id}`, {
        method: "PATCH",
        headers: { "X-Admin-Reason": encodeURIComponent(reason.trim()) },
        body: JSON.stringify(value),
      });
      await Promise.all(
        value.entitlements.map((entitlement) =>
          adminRequest(
            `/api/v1/admin/monetization/plans/${plan.id}/entitlements`,
            {
              method: "PUT",
              body: JSON.stringify({ entitlement, reason: reason.trim() }),
            },
          ),
        ),
      );
      saved();
    } finally {
      setSaving(false);
    }
  }
  return (
    <form className="monetization-plan" onSubmit={save}>
      <div>
        <StatusPill value={value.active ? "ACTIVE" : "DISABLED"} />
        <small>
          {value.code} · {value.billing_period}
        </small>
      </div>
      <label>
        Название
        <input
          maxLength={120}
          value={value.name}
          onChange={(e) => setValue({ ...value, name: e.target.value })}
        />
      </label>
      <label>
        Описание
        <textarea
          maxLength={1000}
          rows={2}
          value={value.description}
          onChange={(e) => setValue({ ...value, description: e.target.value })}
        />
      </label>
      <div className="field-row">
        <label>
          Цена, ₽
          <input
            type="number"
            min={0}
            disabled={value.tier === "FREE"}
            value={value.amount_kopecks / 100}
            onChange={(e) =>
              setValue({
                ...value,
                amount_kopecks: Math.round(Number(e.target.value) * 100),
              })
            }
          />
        </label>
        <label>
          Порядок
          <input
            type="number"
            min={0}
            value={value.display_order}
            onChange={(e) =>
              setValue({ ...value, display_order: Number(e.target.value) })
            }
          />
        </label>
      </div>
      <label className="checkbox-row">
        <input
          type="checkbox"
          checked={value.active}
          onChange={(e) => setValue({ ...value, active: e.target.checked })}
        />
        Активен
      </label>
      <label>
        Причина изменения для аудита
        <input type="text" value={reason} onChange={(e) => setReason(e.target.value)} placeholder="Кратко укажите причину для аудита"/>
      </label>
      <ul>
        {value.entitlements.map((v, index) => (
          <li className="entitlement-editor" key={v.feature_key}>
            <code title={v.feature_key}>{v.feature_key}</code>
            <label className="checkbox-row">
              <input
                type="checkbox"
                checked={v.enabled}
                onChange={(e) => {
                  const entitlements = [...value.entitlements];
                  entitlements[index] = { ...v, enabled: e.target.checked };
                  setValue({ ...value, entitlements });
                }}
              />
              Включено
            </label>
            {v.kind === "LIMIT" ? (
              <>
                <label className="checkbox-row">
                  <input
                    type="checkbox"
                    checked={v.unlimited}
                    onChange={(e) => {
                      const entitlements = [...value.entitlements];
                      entitlements[index] = {
                        ...v,
                        unlimited: e.target.checked,
                      };
                      setValue({ ...value, entitlements });
                    }}
                  />
                  Без лимита
                </label>
                <input
                  aria-label={`Лимит ${v.feature_key}`}
                  type="number"
                  min={0}
                  disabled={v.unlimited}
                  value={v.limit_value ?? 0}
                  onChange={(e) => {
                    const entitlements = [...value.entitlements];
                    entitlements[index] = {
                      ...v,
                      limit_value: Number(e.target.value),
                    };
                    setValue({ ...value, entitlements });
                  }}
                />
              </>
            ) : null}
          </li>
        ))}
      </ul>
      <button disabled={saving}>
        {saving
          ? "Сохраняем…"
          : `Сохранить · ${formatMoney(value.amount_kopecks)}`}
      </button>
    </form>
  );
}
function GrantForm({ plans, saved }: { plans: Plan[]; saved: () => void }) {
  const [user, setUser] = useState(""),
    [plan, setPlan] = useState(plans[0]?.id ?? ""),
    [days, setDays] = useState(30),
    [reason, setReason] = useState("Тестовый доступ по решению администратора"),
    [busy, setBusy] = useState(false);
  useEffect(() => {
    if (!plan && plans[0]) setPlan(plans[0].id);
  }, [plan, plans]);
  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    try {
      const start = new Date(),
        end = new Date(start.getTime() + days * 86400000);
      await adminRequest("/api/v1/admin/monetization/subscriptions", {
        method: "POST",
        body: JSON.stringify({
          user_id: user,
          plan_id: plan,
          starts_at: start.toISOString(),
          ends_at: end.toISOString(),
          reason,
        }),
      });
      setUser("");
      saved();
    } finally {
      setBusy(false);
    }
  }
  return (
    <section className="admin-section">
      <h2>Выдать PRO вручную</h2>
      <p>Только для администраторов. Действие и причина попадут в аудит.</p>
      <form className="admin-inline-form" onSubmit={submit}>
        <label>
          ID пользователя
          <input
            required
            value={user}
            onChange={(e) => setUser(e.target.value)}
            placeholder="UUID пользователя"
          />
        </label>
        <label>
          План
          <CustomSelect value={plan} onChange={(e) => setPlan(e.target.value)}>
            {plans.map((p) => (
              <option value={p.id} key={p.id}>
                {p.name}
              </option>
            ))}
          </CustomSelect>
        </label>
        <label>
          Дней
          <input
            required
            type="number"
            min={1}
            max={730}
            value={days}
            onChange={(e) => setDays(Number(e.target.value))}
          />
        </label>
        <label>
          Причина
          <input type="text" value={reason} onChange={(e) => setReason(e.target.value)} placeholder="Кратко укажите причину для аудита"/>
        </label>
        <button disabled={busy}>{busy ? "Выдаём…" : "Выдать PRO"}</button>
      </form>
    </section>
  );
}
