"use client";

import { useEffect, useState } from "react";
import { IconCheck } from "../icons";
import { useAuth } from "../auth-state";

export default function VerifyEmailPage() {
  const { refresh } = useAuth();
  const [status, setStatus] = useState<"waiting" | "checking" | "verified" | "error">("waiting");
  const [message, setMessage] = useState("");

  useEffect(() => {
    const token = new URLSearchParams(location.search).get("token");
    if (!token) return;
    setStatus("checking");
    fetch("/api/v1/auth/verify-email", { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ token }) })
      .then(async response => {
        if (!response.ok) throw new Error((await response.json().catch(() => null))?.error?.message || "Ссылка недействительна или устарела.");
        setStatus("verified");
        setMessage("Email подтверждён. Теперь все возможности аккаунта доступны.");
        await refresh();
      })
      .catch(error => { setStatus("error"); setMessage(error instanceof Error ? error.message : "Не удалось подтвердить email."); });
  }, [refresh]);

  const next = typeof window === "undefined" ? "/dashboard" : new URLSearchParams(location.search).get("next") || "/dashboard";
  return <main className="auth-page"><section className="auth-card verify-email-card"><span className={`verify-email-card__icon ${status === "verified" ? "is-verified" : ""}`}>{status === "checking" ? <span className="spinner"/> : <IconCheck size={30}/>}</span><p className="eyebrow">Безопасность аккаунта</p><h1>{status === "verified" ? "Email подтверждён" : status === "error" ? "Не удалось подтвердить" : "Проверьте почту"}</h1><p>{message || "Мы отправили ссылку подтверждения на указанный при регистрации адрес. Она действует 24 часа."}</p>{status === "verified" ? <a className="button" href={next.startsWith("/") && !next.startsWith("//") ? next : "/dashboard"}>Перейти в кабинет</a> : <a className="button button--quiet" href="/settings/account">Управление аккаунтом</a>}</section></main>;
}
