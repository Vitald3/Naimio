"use client";

import { FormEvent, useEffect, useState } from "react";
import { useAuth } from "../../auth-state";
import { useToast } from "../../toast";

async function apiMessage(response: Response, fallback: string) {
  try {
    const body = await response.json();
    return body?.error?.message || fallback;
  } catch {
    return fallback;
  }
}

export default function ApplicationForm({ vacancyId }: { vacancyId: string }) {
  const { state: authState, user } = useAuth();
  const { push } = useToast();
  const [message, setMessage] = useState("");
  const [sending, setSending] = useState(false);
  const [existing, setExisting] = useState<{cover_message?:string;status:string}|null>(null);
  const [checking, setChecking] = useState(true);

  useEffect(()=>{
    if(authState!=="authenticated"||!user?.capabilities?.includes("FREELANCER")){setChecking(false);return}
    fetch("/api/v1/me/job-applications?limit=50",{credentials:"same-origin",cache:"no-store"})
      .then(r=>r.ok?r.json():Promise.reject()).then(body=>setExisting((body.data??[]).find((item:{job_id:string})=>item.job_id===vacancyId)??null))
      .catch(()=>undefined).finally(()=>setChecking(false));
  },[authState,user?.id,user?.capabilities,vacancyId]);

  if (authState === "loading" || checking) return <div className="skeleton" aria-label="Проверяем возможность отклика" />;
  if (authState === "authenticated" && !user?.capabilities?.includes("FREELANCER")) return null;
  if (authState === "anonymous") {
    return <section className="notice"><h2>Хотите откликнуться?</h2><p>Войдите как специалист, чтобы отправить работодателю отклик.</p><a className="button" href={`/login?next=${encodeURIComponent(`/vacancies/${vacancyId}`)}`}>Войти</a></section>;
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (sending) return;
    setSending(true);
    try {
      const response = await fetch(`/api/v1/vacancies/${vacancyId}/applications`, {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ cover_message: message.trim() }),
      });
      if (!response.ok) {
        const fallback = response.status === 409 ? "Вы уже откликались на эту вакансию." : "Не удалось отправить отклик.";
        throw new Error(await apiMessage(response, fallback));
      }
      const body=await response.json().catch(()=>null);
      setExisting(body?.data??{cover_message:message.trim(),status:"SUBMITTED"});
      setMessage("");
      push({ kind: "success", title: "Отклик отправлен", message: "Статус появится в разделе «Мои отклики»." });
    } catch (error) {
      push({ kind: "error", title: "Отклик не отправлен", message: error instanceof Error ? error.message : "Попробуйте ещё раз." });
    } finally {
      setSending(false);
    }
  }

  if(existing)return <section className="response-summary"><div className="response-summary__head"><div><p className="eyebrow">Ваш отклик</p><h2>Вы уже откликнулись</h2></div><span className="badge">{["NEW","SUBMITTED"].includes(existing.status)?"На рассмотрении":existing.status}</span></div><p className="response-summary__message">{existing.cover_message||"Сопроводительное сообщение не добавлено."}</p><a className="button button--quiet" href="/dashboard/job-applications">Открыть мои отклики</a></section>;
  return <section><h2>Откликнуться</h2><form onSubmit={submit}><label>Сопроводительное сообщение<textarea maxLength={5000} rows={6} value={message} onChange={(event) => setMessage(event.target.value)} placeholder="Коротко расскажите, почему вам подходит эта вакансия" /></label><button type="submit" disabled={sending}>{sending ? "Отправляем…" : "Откликнуться"}</button></form><p className="form-hint">Это отклик на вакансию, а не предложение по проекту.</p></section>;
}
