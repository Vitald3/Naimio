"use client";

import { use, useState } from "react";
import { useToast } from "../../../toast";

export default function SandboxPaymentPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const { push } = useToast();
  const [busy, setBusy] = useState(false);

  async function confirmPayment() {
    if (busy) return;
    setBusy(true);
    try {
      const response = await fetch(`/api/v1/dev/sandbox/payments/${encodeURIComponent(id)}`, {
        method: "POST",
        credentials: "same-origin",
      });
      const body = await response.json().catch(() => null);
      if (!response.ok || !body?.data?.deal_id) {
        throw new Error("Не удалось подтвердить тестовую оплату.");
      }
      push({ kind: "success", title: "Тестовая оплата подтверждена", message: "Средства зарезервированы по Безопасной сделке." });
      location.assign(`/dashboard/safe-deals/${encodeURIComponent(body.data.deal_id)}`);
    } catch (error) {
      push({ kind: "error", title: "Оплата не подтверждена", message: error instanceof Error ? error.message : "Попробуйте ещё раз." });
      setBusy(false);
    }
  }

  return <main className="auth-shell">
    <section className="auth-card">
      <p className="eyebrow">Debug · тестовый платёж</p>
      <h1>Подтверждение оплаты</h1>
      <p>Это локальный эмулятор платёжного провайдера. Реальные деньги не списываются.</p>
      <button disabled={busy} onClick={() => void confirmPayment()}>{busy ? "Подтверждаем…" : "Оплатить тестово"}</button>
    </section>
  </main>;
}
