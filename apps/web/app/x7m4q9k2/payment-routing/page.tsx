"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { AdminError, AdminHeader, AdminLoading, AdminTable, StatusPill, adminRequest } from "../admin-ui";
import { useToast } from "../../toast";
import { CustomSelect } from "../../custom-select";

type Route = { domain: string; provider: string; enabled: boolean; configured: boolean; environment: "sandbox" | "production"; updated_at: string };
type ProviderSetting = { provider: string; enabled: boolean; configured: boolean; environment: "sandbox" | "production"; capabilities?: string[]; updated_at: string };
type ConfigField = { key: string; label: string; secret: boolean; required: boolean; configured: boolean; value?: string };
type ProviderConfig = { provider: string; environment: "sandbox" | "production"; configured: boolean; fields: ConfigField[] };

const domains: Record<string, string> = { SAFE_DEAL: "Safe Deal", PRO_SUBSCRIPTION: "PRO-подписки", OTHER_PLATFORM_PAYMENT: "Другие платежи платформы" };
const providers = ["yookassa", "tbank", "yandex_pay", "cloudpayments", "robokassa"];
const providerLabels: Record<string, string> = { yookassa: "ЮKassa", tbank: "Т-Банк", yandex_pay: "Яндекс Пэй", cloudpayments: "CloudPayments", robokassa: "Robokassa" };

function supportsDomain(setting: ProviderSetting | undefined, domain: string) {
  const caps = new Set(setting?.capabilities ?? []);
  if (domain === "SAFE_DEAL") return caps.has("SAFE_DEAL") && caps.has("PAYOUT_CARD");
  if (domain === "PRO_SUBSCRIPTION") return caps.has("ONE_TIME_PAYMENT") && (caps.has("MERCHANT_MANAGED_RECURRING") || caps.has("PROVIDER_MANAGED_SUBSCRIPTION"));
  return caps.has("ONE_TIME_PAYMENT");
}

export default function PaymentRoutingPage() {
  const { push } = useToast();
  const [routes, setRoutes] = useState<Route[] | null>(null);
  const [providerSettings, setProviderSettings] = useState<ProviderSetting[] | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");
  const [editing, setEditing] = useState<string>("");
  const [config, setConfig] = useState<ProviderConfig | null>(null);
  const [values, setValues] = useState<Record<string, string>>({});

  const load = useCallback(() => {
    setError("");
    Promise.all([
      adminRequest<{ data: Route[] }>("/api/v1/admin/payment-routing"),
      adminRequest<{ data: ProviderSetting[] }>("/api/v1/admin/payment-routing/providers"),
    ]).then(([routeResponse, providerResponse]) => {
      setRoutes(routeResponse.data);
      setProviderSettings(providerResponse.data);
    }).catch(e => setError(e.message));
  }, []);
  useEffect(load, [load]);

  async function openConfig(provider: string) {
    setBusy(`config:${provider}`);
    try {
      const response = await adminRequest<{ data: ProviderConfig }>(`/api/v1/admin/payment-routing/providers/${provider}/config`);
      setEditing(provider);
      setConfig(response.data);
      setValues(Object.fromEntries(response.data.fields.map(field => [field.key, field.value ?? ""])));
      window.requestAnimationFrame(() => document.getElementById("provider-config")?.scrollIntoView({ behavior: "smooth", block: "start" }));
    } catch (e) {
      push({ kind: "error", title: "Не удалось открыть конфигурацию", message: e instanceof Error ? e.message : "Попробуйте ещё раз" });
    } finally { setBusy(""); }
  }

  async function saveConfig() {
    if (!editing || !config) return;
    setBusy(`save:${editing}`);
    try {
      const response = await adminRequest<{ data: ProviderConfig }>(`/api/v1/admin/payment-routing/providers/${editing}/config`, {
        method: "PUT",
        body: JSON.stringify({ environment: config.environment, values }),
      });
      setConfig(response.data);
      setValues(Object.fromEntries(response.data.fields.map(field => [field.key, field.value ?? ""])));
      push({ kind: "success", title: "Конфигурация сохранена", message: `${providerLabels[editing]} готов к включению. Секреты зашифрованы и обратно не отображаются.` });
      load();
    } catch (e) {
      push({ kind: "error", title: "Конфигурация не сохранена", message: e instanceof Error ? e.message : "Проверьте обязательные поля" });
    } finally { setBusy(""); }
  }

  async function setEnabled(provider: string, enabled: boolean) {
    setBusy(provider);
    try {
      await adminRequest(`/api/v1/admin/payment-routing/providers/${provider}`, { method: "PATCH", body: JSON.stringify({ enabled }) });
      push({ kind: "success", title: enabled ? "Провайдер включён" : "Провайдер выключен", message: "Новые операции учитывают это изменение; уже созданные продолжают работу у исходного провайдера." });
      load();
    } catch (e) {
      push({ kind: "error", title: "Не удалось изменить провайдера", message: e instanceof Error ? e.message : "Попробуйте ещё раз" });
    } finally { setBusy(""); }
  }

  async function setRoute(domain: string, provider: string) {
    setBusy(domain);
    try {
      await adminRequest(`/api/v1/admin/payment-routing/routes/${domain}`, { method: "PUT", body: JSON.stringify({ provider }) });
      push({ kind: "success", title: "Маршрут обновлён", message: "Изменение влияет только на новые платежные операции." });
      load();
    } catch (e) {
      push({ kind: "error", title: "Маршрут недоступен", message: e instanceof Error ? e.message : "Проверьте конфигурацию и включение провайдера" });
    } finally { setBusy(""); }
  }

  const activeSetting = useMemo(() => providerSettings?.find(item => item.provider === editing), [providerSettings, editing]);

  return <>
    <AdminHeader title="Платёжные провайдеры" description="Credentials, режим Sandbox/Production, включение и маршрутизация управляются здесь. Секретные значения хранятся зашифрованно и никогда не выводятся обратно." />
    {error ? <AdminError message={error} onRetry={load} /> : !routes || !providerSettings ? <AdminLoading /> : <>
      <section className="admin-section">
        <h2>Маршруты операций</h2>
        <p>Здесь выбирается текущий PSP для <strong>новых</strong> операций каждого домена: Safe Deal, PRO-подписок и остальных платежей платформы. Включить можно несколько провайдеров, но для каждого домена используется один выбранный маршрут. Уже созданные платежи остаются закреплены за исходным PSP.</p>
        <AdminTable columns={["Домен", "Провайдер", "Режим", "Подключение"]} empty={!routes.length}>
          {routes.map(route => <tr key={route.domain}>
            <td><strong>{domains[route.domain] ?? route.domain}</strong></td>
            <td><CustomSelect value={route.provider} disabled={busy === route.domain} aria-label={`Провайдер для ${domains[route.domain] ?? route.domain}`} onChange={e => void setRoute(route.domain, e.target.value)}>
              {providers.filter(p => p === route.provider || supportsDomain(providerSettings.find(item => item.provider === p), route.domain)).map(p => {
                const setting = providerSettings.find(item => item.provider === p);
                return <option key={p} value={p} disabled={!setting?.configured || !setting.enabled}>{providerLabels[p] ?? p}{!setting?.configured ? " — не настроен" : !setting.enabled ? " — выключен" : ""}</option>;
              })}
            </CustomSelect></td>
            <td><StatusPill value={route.environment === "production" ? "PRODUCTION" : "SANDBOX"} /></td>
            <td><StatusPill value={route.enabled && route.configured ? "READY" : route.configured ? "DISABLED" : "NOT_CONFIGURED"} /></td>
          </tr>)}
        </AdminTable>
      </section>

      <section className="admin-section">
        <h2>Доступность провайдеров</h2>
        <p>Для каждого PSP сначала заполните credentials и выберите режим. После сохранения провайдер можно включить и назначить маршруту.</p>
        <div className="inline-actions">
          {providers.map(provider => {
            const setting = providerSettings.find(item => item.provider === provider);
            const enabled = setting?.enabled ?? false;
            const configured = setting?.configured ?? false;
            return <article className="admin-kpi" key={provider}>
              <span>{providerLabels[provider] ?? provider}</span>
              <strong>{configured ? "Настроен" : "Нет конфигурации"}</strong>
              <small>Режим: <b>{setting?.environment === "production" ? "PRODUCTION" : "SANDBOX"}</b></small>
              <button className="button button--quiet" disabled={busy !== ""} onClick={() => void openConfig(provider)}>{configured ? "Изменить настройки" : "Настроить"}</button>
              {configured ? <button className="button button--quiet" disabled={busy !== ""} onClick={() => void setEnabled(provider, !enabled)}>{enabled ? "Выключить" : "Включить"}</button> : null}
            </article>;
          })}
        </div>
      </section>

      {editing && config ? <section className="admin-section" id="provider-config">
        <h2>{providerLabels[editing]} — конфигурация</h2>
        <p>Секретные поля после сохранения очищаются в форме. Пустое секретное поле означает «оставить сохранённое значение без изменений».</p>
        <div className="form-grid">
          <label className="field"><span>Режим работы</span><CustomSelect value={config.environment} onChange={e => setConfig({ ...config, environment: e.target.value as "sandbox" | "production" })}>
            <option value="sandbox">Sandbox — тестовые платежи</option>
            <option value="production">Production — реальные платежи</option>
          </CustomSelect></label>
          {config.fields.map(field => <label className="field" key={field.key}>
            <span>{field.label}{field.required ? " *" : ""}{field.secret && field.configured ? " — сохранено" : ""}</span>
            {field.key.includes("pem") ? <textarea rows={5} autoComplete="off" value={values[field.key] ?? ""} placeholder={field.secret && field.configured ? "Оставьте пустым, чтобы не менять" : ""} onChange={e => setValues(v => ({ ...v, [field.key]: e.target.value }))} /> : <input type={field.secret ? "password" : "text"} autoComplete="off" value={values[field.key] ?? ""} placeholder={field.secret && field.configured ? "Оставьте пустым, чтобы не менять" : ""} onChange={e => setValues(v => ({ ...v, [field.key]: e.target.value }))} />}
          </label>)}
        </div>
        {config.environment === "production" ? <p className="admin-warning"><strong>Production:</strong> после включения новые операции этого маршрута могут создавать реальные списания. Сначала проверьте sandbox.</p> : null}
        <div className="inline-actions">
          <button className="button" disabled={busy !== ""} onClick={() => void saveConfig()}>{busy === `save:${editing}` ? "Сохраняем…" : "Сохранить конфигурацию"}</button>
          <button className="button button--quiet" disabled={busy !== ""} onClick={() => { setEditing(""); setConfig(null); }}>Закрыть</button>
          {activeSetting ? <StatusPill value={activeSetting.enabled ? "ENABLED" : "DISABLED"} /> : null}
        </div>
      </section> : null}
    </>}
  </>;
}
