"use client";

import { useState } from "react";
import { useAuth } from "../../auth-state";
import { useToast } from "../../toast";

export default function ServiceOrderAction({ serviceID, sellerID, education }: { serviceID: string; sellerID: string; education?: boolean }) {
  const { state, user } = useAuth();
  const { push } = useToast();
  const [busy, setBusy] = useState(false);
  if (user?.id === sellerID) return <div className="service-order-own"><button type="button" className="button service-order-button" disabled>Это ваше предложение</button><small>Для проверки покупки откройте страницу из аккаунта заказчика или выйдите из аккаунта.</small></div>;
  async function start() {
    if (state !== "authenticated") {
      location.assign(`/login?next=${encodeURIComponent(`/services/${serviceID}`)}`);
      return;
    }
    setBusy(true);
    try {
      const response = await fetch("/api/v1/conversations", { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ participant_user_id: sellerID }) });
      const body = await response.json().catch(() => null);
      if (!response.ok || !body?.data?.id) throw new Error(body?.error?.message || "Не удалось открыть диалог");
      sessionStorage.setItem("service-conversation-context", JSON.stringify({ conversation_id: body.data.id, service_id: serviceID, prompt: education ? "Здравствуйте! Хочу записаться на это предложение." : "Здравствуйте! Хочу обсудить заказ этой услуги." }));
      location.assign(`/messages?conversation=${encodeURIComponent(body.data.id)}&service=${encodeURIComponent(serviceID)}`);
    } catch (error) {
      push({ kind: "error", title: education ? "Не удалось записаться" : "Не удалось начать заказ", message: error instanceof Error ? error.message : "Попробуйте ещё раз." });
      setBusy(false);
    }
  }
  return <button type="button" className="button service-order-button" disabled={busy} onClick={start}>{busy ? "Открываем оформление…" : education ? "Записаться на обучение" : "Купить / заказать услугу"}</button>;
}
