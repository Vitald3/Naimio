"use client";

import { FormEvent, useState } from "react";
import { IconBriefcase, IconUser } from "../icons";
import { track } from "../analytics";
import { CustomSelect } from "../custom-select";

type AccountType = "CUSTOMER" | "FREELANCER";

export default function RegisterPage() {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [name, setName] = useState("");
  const [accountType, setAccountType] = useState<AccountType | "">("");
  const [gender, setGender] = useState<"MALE" | "FEMALE" | "">("");
  const [experienceYears, setExperienceYears] = useState("");
  const [hourlyRate, setHourlyRate] = useState("");
  const [availability, setAvailability] = useState("AVAILABLE");
  const [state, setState] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!accountType) { setState("Выберите, как вы планируете пользоваться площадкой."); return; }
    if (!gender) { setState("Укажите пол — он используется для стандартного аватара профиля."); return; }
    if (accountType === "FREELANCER" && (experienceYears === "" || hourlyRate === "")) { setState("Для профиля исполнителя укажите опыт и ставку за час."); return; }
    setBusy(true);
    setState("Создаём аккаунт…");
    const response = await fetch("/api/v1/auth/register", {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email, password, display_name: name, account_type: accountType, gender, ...(accountType === "FREELANCER" ? { experience_years: experienceYears === "" ? null : Number(experienceYears), hourly_rate_kopecks: hourlyRate === "" ? null : Math.round(Number(hourlyRate) * 100), availability } : {}) }),
    });
    if (!response.ok) {
      setBusy(false);
      setState(response.status === 409 ? "Аккаунт с этим email уже существует. Попробуйте войти или восстановить доступ." : "Проверьте данные и попробуйте снова.");
      return;
    }
    track("REGISTRATION_COMPLETED", { account_type: accountType.toLowerCase() });
    const next = new URLSearchParams(location.search).get("next");
    const safeNext = next?.startsWith("/") && !next.startsWith("//") ? next : "/dashboard";
    const nextLocation = `/verify-email?sent=1&next=${encodeURIComponent(safeNext)}`;
    location.assign(nextLocation);
  }

  return <main className="auth-page"><div className="auth-card auth-card--wide"><p className="eyebrow">Новый аккаунт</p><h1>Как вы хотите начать?</h1><p className="lead">Выберите основной режим. Позже в настройках можно включить второй — аккаунт и история останутся общими.</p>
    <form onSubmit={submit}>
      <fieldset className="account-type-picker"><legend>Моя основная роль</legend>
        <label className={accountType === "CUSTOMER" ? "account-type-card is-selected" : "account-type-card"}><input type="radio" name="account_type" value="CUSTOMER" checked={accountType === "CUSTOMER"} onChange={() => setAccountType("CUSTOMER")}/><span className="account-type-card__icon"><IconBriefcase size={24}/></span><span><strong>Я заказчик</strong><small>Ищу специалистов, создаю проекты и вакансии</small></span></label>
        <label className={accountType === "FREELANCER" ? "account-type-card is-selected" : "account-type-card"}><input type="radio" name="account_type" value="FREELANCER" checked={accountType === "FREELANCER"} onChange={() => setAccountType("FREELANCER")}/><span className="account-type-card__icon"><IconUser size={24}/></span><span><strong>Я исполнитель</strong><small>Оформляю профиль, предлагаю услуги и откликаюсь</small></span></label>
      </fieldset>
      <label>Имя <input required autoComplete="name" maxLength={120} value={name} onChange={event => setName(event.target.value)}/></label>
      <label>Пол <CustomSelect required value={gender} onChange={(event) => setGender(event.target.value as "MALE" | "FEMALE")}><option value="">Выберите</option><option value="MALE">Мужской</option><option value="FEMALE">Женский</option></CustomSelect><small className="form-hint">Используется для стандартной иконки профиля. Свой аватар можно загрузить позже.</small></label>
      {accountType === "FREELANCER" ? <div className="registration-professional-fields"><label>Опыт, лет<input required type="number" min="0" max="80" step="1" value={experienceYears} onChange={event=>setExperienceYears(event.target.value)} placeholder="Например, 3"/></label><label>Ставка, ₽/час<input required type="number" min="0" max="1000000" step="100" value={hourlyRate} onChange={event=>setHourlyRate(event.target.value)} placeholder="Например, 2500"/></label><label>Доступность<CustomSelect value={availability} onChange={event=>setAvailability(event.target.value)}><option value="AVAILABLE">Доступен</option><option value="PARTIALLY_BUSY">Частично занят</option><option value="BUSY">Занят</option><option value="UNAVAILABLE">Недоступен</option></CustomSelect></label></div> : null}
      <label>Email <input type="email" autoComplete="email" required maxLength={320} value={email} onChange={event => setEmail(event.target.value)}/></label>
      <label>Пароль <input type="password" autoComplete="new-password" required minLength={10} maxLength={128} value={password} onChange={event => setPassword(event.target.value)}/><small className="form-hint">Не менее 10 символов</small></label>
      <button disabled={busy}>{busy ? "Создаём аккаунт…" : "Создать аккаунт"}</button>
    </form>
    {state ? <p role={state.includes("Создаём") ? "status" : "alert"} className="auth-state">{state}</p> : null}
    <p className="auth-alternative">Уже есть аккаунт? <a href="/login">Войти</a></p>
  </div></main>;
}
