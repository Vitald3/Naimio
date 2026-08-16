"use client";

import { useEffect, useMemo, useState } from "react";
import { useSearchParams } from "next/navigation";
import Breadcrumbs from "../../breadcrumbs";

type Attempt = { id: string; status: string; provider: string; amount_kopecks: number; currency: string };
const terminal = new Set(["SUCCEEDED", "FAILED", "CANCELED", "REFUNDED"]);

export default function ProPaymentReturnPage() {
  const params = useSearchParams();
  const attemptID = useMemo(() => (params.get("attempt_id") || "").trim(), [params]);
  const [attempt, setAttempt] = useState<Attempt | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!attemptID) { setError("Не найден идентификатор платежа."); return; }
    let stopped = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let tries = 0;
    const poll = async () => {
      try {
        const response = await fetch(`/api/v1/me/pro-billing/status?attempt_id=${encodeURIComponent(attemptID)}`, { cache: "no-store" });
        const body = await response.json().catch(() => ({}));
        if (!response.ok) throw new Error(body?.error?.message || "Не удалось проверить платёж");
        if (stopped) return;
        const value = body.data as Attempt;
        setAttempt(value);
        if (!terminal.has(value.status) && tries++ < 20) timer = setTimeout(poll, 1500);
      } catch (e) { if (!stopped) setError(e instanceof Error ? e.message : "Не удалось проверить платёж"); }
    };
    void poll();
    return () => { stopped = true; if (timer) clearTimeout(timer); };
  }, [attemptID]);

  const success = attempt?.status === "SUCCEEDED";
  const failed = attempt && terminal.has(attempt.status) && !success;
  return <main><Breadcrumbs items={[{label:"Главная",href:"/"},{label:"Naimio PRO",href:"/pro"},{label:"Результат оплаты"}]}/><section className="empty"><h1>{success ? "PRO подключён" : failed ? "Оплата не завершена" : "Проверяем оплату"}</h1>{error ? <p>{error}</p> : success ? <p>Платёж подтверждён сервером. Доступ PRO активирован.</p> : failed ? <p>Провайдер не подтвердил успешную оплату. Повторного списания автоматически не будет.</p> : <p>Получаем подтверждение от платёжного провайдера. Обычно это занимает несколько секунд.</p>}<div className="inline-actions"><a className="button" href="/pro">Вернуться в Naimio PRO</a><a className="button button--quiet" href="/dashboard">В личный кабинет</a></div></section></main>;
}
