"use client";
import { CustomSelect } from "../../custom-select";
import { FormEvent, useCallback, useEffect, useState } from "react";
import { AdminError, AdminHeader, AdminLoading, AdminTable, StatusPill, adminRequest, formatDate } from "../admin-ui";

type Rule = {
  id: string;
  code: string;
  event_type: string;
  beneficiary: "INVITER" | "INVITED";
  reward_type: string;
  reward_value: number;
  reward_unit: string;
  enabled: boolean;
  updated_at?: string;
};

export default function ReferralRulesPage() {
  const [items, setItems] = useState<Rule[]>([]);
  const [code, setCode] = useState("");
  const [value, setValue] = useState(1);
  const [beneficiary, setBeneficiary] = useState<"INVITER" | "INVITED">("INVITER");
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const body = await adminRequest<{ data: Rule[] }>("/api/v1/admin/referral-rules");
      setItems(body.data ?? []);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Не удалось загрузить правила");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  async function create(event: FormEvent) {
    event.preventDefault();
    setMessage("");
    setError("");
    try {
      await adminRequest("/api/v1/admin/referral-rules", {
        method: "POST",
        body: JSON.stringify({
          code,
          event_type: "INVITE_ACCEPTED",
          beneficiary,
          reward_type: "BONUS",
          reward_value: value,
          reward_unit: "CREDITS",
          max_uses: null,
          starts_at: null,
          ends_at: null,
          enabled: true,
          config: { valid_days: 90 },
        }),
      });
      setCode("");
      setValue(1);
      setMessage("Правило создано. Изменение записано в аудит.");
      await load();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Не удалось создать правило");
    }
  }

  async function toggle(rule: Rule) {
    setMessage("");
    setError("");
    try {
      await adminRequest(`/api/v1/admin/referral-rules/${rule.id}`, {
        method: "PATCH",
        body: JSON.stringify({
          code: rule.code,
          event_type: rule.event_type,
          beneficiary: rule.beneficiary,
          reward_type: rule.reward_type,
          reward_value: rule.reward_value,
          reward_unit: rule.reward_unit,
          max_uses: null,
          starts_at: null,
          ends_at: null,
          enabled: !rule.enabled,
          config: { valid_days: 90 },
        }),
      });
      setMessage(rule.enabled ? "Правило отключено." : "Правило включено.");
      await load();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Не удалось изменить правило");
    }
  }

  return <>
    <AdminHeader title="Реферальные правила" description="Промо-правила не создают денежный баланс или выплаты и используются только для маркетинговых льгот." />
    <section className="admin-panel">
      <p className="eyebrow">Новое правило</p>
      <form className="admin-inline-form" onSubmit={create}>
        <label>Код
          <input required maxLength={100} pattern="[A-Z0-9_]+" data-pattern-message="Используйте заглавные латинские буквы, цифры и подчёркивания, например WELCOME_INVITER." placeholder="WELCOME_INVITER" value={code} onChange={(e) => setCode(e.target.value.toUpperCase())} />
        </label>
        <label>Получатель
          <CustomSelect value={beneficiary} onChange={(e) => setBeneficiary(e.target.value as "INVITER" | "INVITED")}>
            <option value="INVITER">Пригласивший</option>
            <option value="INVITED">Приглашённый</option>
          </CustomSelect>
        </label>
        <label>Промо-кредиты
          <input type="number" min={1} max={1000000} value={value} onChange={(e) => setValue(Number(e.target.value))} />
        </label>
        <button>Создать правило</button>
      </form>
      {message ? <p className="notice" role="status">{message}</p> : null}
    </section>
    {error ? <AdminError message={error} onRetry={load} /> : loading ? <AdminLoading /> :
      <AdminTable columns={["Правило", "Триггер", "Получатель", "Награда", "Статус", "Действие"]} empty={!items.length}>
        {items.map((rule) => <tr key={rule.id}>
          <td><strong>{rule.code}</strong>{rule.updated_at ? <small>{formatDate(rule.updated_at)}</small> : null}</td>
          <td>{rule.event_type}</td>
          <td>{rule.beneficiary === "INVITER" ? "Пригласивший" : "Приглашённый"}</td>
          <td><strong>{rule.reward_value} {rule.reward_unit}</strong><small>{rule.reward_type}</small></td>
          <td><StatusPill value={rule.enabled ? "ACTIVE" : "DISABLED"} /></td>
          <td><button className="button button--quiet button--compact" type="button" onClick={() => toggle(rule)}>{rule.enabled ? "Отключить" : "Включить"}</button></td>
        </tr>)}
      </AdminTable>}
  </>;
}
