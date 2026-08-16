"use client";

import { FormEvent, useState } from "react";
import { STAFF_BASE_PATH } from "../../admin-path";

export default function StaffLoginPage() {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [state, setState] = useState("");

  async function submit(event: FormEvent) {
    event.preventDefault();
    setState("Проверяем служебную учётную запись…");
    const response = await fetch("/api/v1/auth/login", {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email, password, portal: "admin" }),
    });
    if (!response.ok) {
      setState(response.status === 403 ? "Доступ к служебной зоне запрещён." : "Неверная почта или пароль.");
      return;
    }
    location.replace(STAFF_BASE_PATH);
  }

  return (
    <main className="staff-login">
      <section className="staff-login__panel">
        <div className="admin-sidebar__brand">
          <span className="brand-mark">nm</span>
          <div><strong>Naimio Control Center</strong><small>Служебный доступ</small></div>
        </div>
        <h1>Вход для команды платформы</h1>
        <p>Эта зона предназначена только для модераторов и администраторов Naimio.</p>
        <form onSubmit={submit}>
          <label>Email <input type="email" autoComplete="username" required maxLength={320} value={email} onChange={(e) => setEmail(e.target.value)} /></label>
          <label>Пароль <input type="password" autoComplete="current-password" required maxLength={128} value={password} onChange={(e) => setPassword(e.target.value)} /></label>
          <button>Войти в Control Center</button>
        </form>
        <p role="status">{state}</p>
      </section>
    </main>
  );
}
