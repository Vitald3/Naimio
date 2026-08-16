"use client";
import { CustomSelect } from "../../custom-select";
import Breadcrumbs from "../../breadcrumbs";

import { FormEvent, useCallback, useEffect, useState } from "react";
import { useToast } from "../../toast";
import { IconArrowRight, IconCheck, IconShield } from "../../icons";

type Reputation = {
  id: string;
  platform: string;
  display_name: string;
  profile_url: string;
  external_username?: string;
  verification_status: "UNVERIFIED" | "PENDING" | "VERIFIED" | "REJECTED" | "EXPIRED";
  verified_at?: string;
};

const platforms = ["KWORK", "FL_RU", "GITHUB", "BEHANCE", "DRIBBBLE", "HABR_CAREER", "OTHER"] as const;
const platformLabels:Record<(typeof platforms)[number],string>={KWORK:"Kwork",FL_RU:"FL.ru",GITHUB:"GitHub",BEHANCE:"Behance",DRIBBBLE:"Dribbble",HABR_CAREER:"Хабр Карьера",OTHER:"Другая площадка"};
const stateLabels: Record<Reputation["verification_status"], string> = {
  UNVERIFIED: "Не подтверждён",
  PENDING: "На проверке",
  VERIFIED: "Подтверждён",
  REJECTED: "Отклонён",
  EXPIRED: "Срок проверки истёк",
};

export default function ReputationSettingsPage() {
  const { push } = useToast();
  const [items, setItems] = useState<Reputation[]>([]);
  const [platform, setPlatform] = useState<(typeof platforms)[number]>("GITHUB");
  const [profileURL, setProfileURL] = useState("");
  const [error, setError] = useState("");
  const [challenge, setChallenge] = useState("");

  const load = useCallback(async () => {
    const response = await fetch("/api/v1/me/external-reputations", { credentials: "same-origin" });
    if (!response.ok) throw new Error("Не удалось загрузить внешнюю репутацию");
    const body = await response.json();
    setItems(body.data ?? []);
  }, []);

  useEffect(() => { load().catch((reason) => setError(reason.message)); }, [load]);

  async function add(event: FormEvent) {
    event.preventDefault(); setError(""); setChallenge("");
    const response = await fetch("/api/v1/me/external-reputations", { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ platform, profile_url: profileURL }) });
    if (!response.ok) { setError("Проверьте адрес профиля и попробуйте снова."); return; }
    setProfileURL(""); await load();
    push({ kind: "success", title: "Внешний профиль добавлен", message: "Теперь выберите способ подтверждения." });
  }

  async function remove(id: string) {
    const response = await fetch(`/api/v1/me/external-reputations/${id}`, { method: "DELETE", credentials: "same-origin" });
    if (!response.ok) { setError("Не удалось удалить профиль."); return; }
    await load();
    push({ kind: "success", title: "Внешний профиль удалён" });
  }

  async function verify(id: string, method: "PROFILE_CODE" | "MANUAL") {
    setError(""); setChallenge("");
    const response = await fetch(`/api/v1/me/external-reputations/${id}/verification`, { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ method, evidence: method === "MANUAL" ? { context: "manual review requested" } : {} }) });
    if (!response.ok) { setError("Не удалось начать проверку."); return; }
    const body = await response.json();
    if (body.data?.code) setChallenge(`Разместите код во внешнем профиле: ${body.data.code}`);
    else setChallenge("Заявка отправлена на ручную проверку.");
    await load();
  }

  return <main className="reputation-settings">
    <Breadcrumbs items={[{label:"Главная",href:"/"},{label:"Кабинет",href:"/dashboard"},{label:"Репутация"}]}/>
    <header className="page-heading"><div><p className="eyebrow">Доверие и опыт</p><h1>Внешняя репутация</h1><p className="lead">Добавьте профиль на другой профессиональной площадке. Внешние оценки показываются отдельно от рейтинга Naimio.</p></div><span className="reputation-settings__shield"><IconShield size={30}/></span></header>
    <form onSubmit={add} className="reputation-add-form">
      <div className="reputation-add-form__heading"><span><IconArrowRight size={20}/></span><div><h2>Добавить внешний профиль</h2><p>Мы не копируем отзывы автоматически и не просим пароль от другой площадки.</p></div></div>
      <label>Платформа <CustomSelect value={platform} onChange={(event) => setPlatform(event.target.value as typeof platform)}>{platforms.map((value) => <option key={value} value={value}>{platformLabels[value]}</option>)}</CustomSelect></label>
      <label>Ссылка на профиль <input type="url" required maxLength={2048} value={profileURL} onChange={(event) => setProfileURL(event.target.value)} /></label>
      <button type="submit">Добавить профиль</button>
    </form>
    {error ? <p role="alert" className="notice notice--error">{error}</p> : null}
    {challenge ? <p role="status" className="notice notice--info">{challenge}</p> : null}
    {items.length ? <section className="reputation-owned"><div className="reputation-owned__heading"><h2>Добавленные профили</h2><span>{items.length}</span></div><ul className="reputation-owned__list">{items.map((item) => <li key={item.id} className={`reputation-owned-card reputation-owned-card--${item.verification_status.toLowerCase()}`}>
      <div className="reputation-owned-card__identity"><span className="reputation-platform-mark">{item.display_name.slice(0,2).toUpperCase()}</span><div><strong>{item.display_name}</strong><a href={item.profile_url} target="_blank" rel="noopener noreferrer">Открыть профиль <IconArrowRight size={14}/></a></div></div>
      <span className="reputation-owned-card__status">{item.verification_status === "VERIFIED" ? <IconCheck size={15}/> : null}{stateLabels[item.verification_status]}</span>
      <div className="reputation-owned-card__actions">{item.verification_status !== "PENDING" && item.verification_status !== "VERIFIED" ? <><button className="button button--quiet button--compact" type="button" onClick={() => verify(item.id, "PROFILE_CODE")}>Проверить кодом</button><button className="button button--quiet button--compact" type="button" onClick={() => verify(item.id, "MANUAL")}>Ручная проверка</button></> : null}<button className="button button--danger button--compact" type="button" onClick={() => remove(item.id)}>Удалить</button></div>
    </li>)}</ul></section> : <div className="empty"><h2>Внешние профили ещё не добавлены</h2><p>Добавьте первую площадку с помощью формы выше.</p></div>}
  </main>;
}
