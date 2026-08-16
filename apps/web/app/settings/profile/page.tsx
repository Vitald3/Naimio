"use client";
import { CustomSelect } from "../../custom-select";

import { FormEvent, useEffect, useState } from "react";
import Breadcrumbs from "../../breadcrumbs";
import { useAuth } from "../../auth-state";
import { AuthBootstrapLoader } from "../../auth-loader";
import { useToast } from "../../toast";
import { Avatar } from "../../media-components";
import FileTypeBadge from "../../file-type-badge";
import { cityOptions, countryOptions } from "../../location-options";
import { avatarFor } from "../../media";

type Category = { id: string; name: string };
type Skill = { id: string; name: string };
type Profile = {
  professional_title?: string;
  bio?: string;
  location_text?: string;
  country_code?: string;
  experience_years?: number;
  hourly_rate_kopecks?: number;
  minimum_order_kopecks?: number;
  availability?: string;
  profile_visibility?: string;
  categories?: Array<{ id: string }>;
  skills?: Array<{ id: string }>;
  custom_skills?: string[];
  languages?: Array<{ code: string; level: string }>;
};

export default function ProfessionalProfileSettings() {
  const { user, state, refresh } = useAuth();
  const { push } = useToast();
  const [displayName, setDisplayName] = useState("");
  const [profile, setProfile] = useState<Profile>({
    availability: "AVAILABLE",
    profile_visibility: "PUBLIC",
  });
  const [categories, setCategories] = useState<Category[]>([]);
  const [skills, setSkills] = useState<Skill[]>([]);
  const [categoryIDs, setCategoryIDs] = useState<string[]>([]);
  const [skillIDs, setSkillIDs] = useState<string[]>([]);
  const [customSkills, setCustomSkills] = useState<string[]>([]);
  const [skillDraft, setSkillDraft] = useState("");
  const [saving, setSaving] = useState(false);
  const [avatarUploading, setAvatarUploading] = useState(false);
  const [avatarFile, setAvatarFile] = useState<File | null>(null);
  const [avatarRevision, setAvatarRevision] = useState(0);
  const [pendingAvatarID, setPendingAvatarID] = useState<string | undefined>();
  const [avatarPreviewURL, setAvatarPreviewURL] = useState("");

  useEffect(() => () => {
    if (avatarPreviewURL.startsWith("blob:")) URL.revokeObjectURL(avatarPreviewURL);
  }, [avatarPreviewURL]);

  useEffect(() => { if (user?.display_name) setDisplayName(user.display_name); }, [user?.display_name]);

  useEffect(() => {
    Promise.all([
      fetch("/api/v1/categories", { cache: "no-store" }).then((r) =>
        r.ok ? r.json() : { data: [] },
      ),
      fetch("/api/v1/skills?limit=100", { cache: "no-store" }).then((r) =>
        r.ok ? r.json() : { data: [] },
      ),
      fetch("/api/v1/me/professional-profile", {
        credentials: "same-origin",
        cache: "no-store",
      }).then((r) => (r.ok ? r.json() : null)),
    ])
      .then(([cats, sk, current]) => {
        setCategories(cats.data ?? []);
        setSkills(sk.data ?? []);
        if (current?.data) {
          setProfile(current.data);
          setCategoryIDs(
            (current.data.categories ?? []).map((v: { id: string }) => v.id),
          );
          setSkillIDs(
            (current.data.skills ?? []).map((v: { id: string }) => v.id),
          );
          setCustomSkills(current.data.custom_skills ?? []);
        }
      })
      .catch(() =>
        push({ kind: "error", title: "Не удалось загрузить профиль" }),
      );
  }, [push]);

  async function changeAvatar(file: File) {
    if (!file.type.startsWith("image/") || file.size > 5 * 1024 * 1024) {
      push({ kind: "error", title: "Выберите JPG, PNG или WebP до 5 МБ" });
      return;
    }
    setAvatarUploading(true);
    setAvatarFile(file);
    try {
      const presign = await fetch("/api/v1/uploads/presign", {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          purpose: "AVATAR",
          filename: file.name,
          mime_type: file.type,
          size_bytes: file.size,
        }),
      });
      const prepared = await presign.json().catch(() => null);
      if (!presign.ok)
        throw new Error(
          prepared?.error?.message || "Не удалось подготовить загрузку",
        );
      const data = prepared.data;
      const put = await fetch(data.upload_url, {
        method: "PUT",
        headers: data.headers,
        body: file,
      });
      if (!put.ok) throw new Error("Не удалось загрузить изображение");
      const complete = await fetch(
        `/api/v1/uploads/${data.media_id}/complete`,
        { method: "POST", credentials: "same-origin" },
      );
      if (!complete.ok) throw new Error("Не удалось завершить загрузку");
      let clean = false;
      for (let attempt = 0; attempt < 15; attempt++) {
        const response = await fetch(`/api/v1/uploads/${data.media_id}`, {
          credentials: "same-origin",
          cache: "no-store",
        });
        const result = await response.json();
        if (result.data?.scan_status === "CLEAN") {
          clean = true;
          break;
        }
        if (["FAILED", "INFECTED"].includes(result.data?.scan_status))
          throw new Error("Изображение отклонено проверкой безопасности");
        await new Promise((resolve) => setTimeout(resolve, 700));
      }
      if (!clean) throw new Error("Проверка изображения ещё не завершена");
      if (avatarPreviewURL.startsWith("blob:")) URL.revokeObjectURL(avatarPreviewURL);
      setAvatarPreviewURL(URL.createObjectURL(file));
      setPendingAvatarID(data.media_id);
      push({
        kind: "info",
        title: "Изображение загружено",
        message: "Предпросмотр обновлён. Нажмите «Сохранить профиль», чтобы применить аватар.",
      });
    } catch (error) {
      setAvatarFile(null);
      push({
        kind: "error",
        title: "Не удалось обновить аватар",
        message: error instanceof Error ? error.message : "Попробуйте ещё раз.",
      });
    } finally {
      setAvatarUploading(false);
    }
  }

  function removeAvatar() {
    if (avatarPreviewURL.startsWith("blob:")) URL.revokeObjectURL(avatarPreviewURL);
    setAvatarPreviewURL(avatarFor(`${user?.id || "profile"}-empty`));
    setAvatarFile(null);
    setPendingAvatarID("");
    push({ kind: "info", title: "Аватар будет удалён после сохранения профиля" });
  }

  async function save(event: FormEvent) {
    event.preventDefault();
    setSaving(true);
    try {
      const body = {
        professional_title: profile.professional_title ?? "",
        bio: profile.bio ?? "",
        location_text: profile.location_text ?? "",
        country_code: (profile.country_code ?? "").toUpperCase(),
        experience_years: profile.experience_years ?? null,
        hourly_rate_kopecks: profile.hourly_rate_kopecks ?? null,
        minimum_order_kopecks: profile.minimum_order_kopecks ?? null,
        availability: profile.availability || "AVAILABLE",
        profile_visibility: profile.profile_visibility || "PUBLIC",
        categories: categoryIDs.map((id, index) => ({
          id,
          is_primary: index === 0,
        })),
        skills: skillIDs.map((id, index) => ({
          id,
          is_featured: index < 5,
        })),
        custom_skills: customSkills,
        languages: profile.languages ?? [],
      };
      const update = await fetch("/api/v1/me/professional-profile", {
        method: "PATCH",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
      if (!update.ok)
        throw new Error(
          (await update.json().catch(() => null))?.error?.message ||
            "Не удалось сохранить профиль",
        );
      const accountPatch: Record<string, string> = { display_name: displayName.trim() };
      if (pendingAvatarID !== undefined) accountPatch.avatar_media_object_id = pendingAvatarID;
      const accountUpdate = await fetch("/api/v1/me", {
        method: "PATCH", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify(accountPatch),
      });
      if (!accountUpdate.ok) throw new Error("Профессиональные данные сохранены, но имя или аватар применить не удалось");
      await refresh();
      setPendingAvatarID(undefined);
      setAvatarRevision((value) => value + 1);
      push({
        kind: "success",
        title: "Профессиональный профиль сохранён",
        message: "Изменения уже доступны в каталоге.",
      });
    } catch (error) {
      push({
        kind: "error",
        title: "Не удалось сохранить профиль",
        message: error instanceof Error ? error.message : undefined,
      });
    } finally {
      setSaving(false);
    }
  }
  const toggle = (
    items: string[],
    set: (v: string[]) => void,
    id: string,
    limit: number,
  ) =>
    set(
      items.includes(id)
        ? items.filter((v) => v !== id)
        : items.length < limit
          ? [...items, id]
          : items,
    );
  const addSkill = () => {
    const value = skillDraft.trim().replace(/\s+/g, " ");
    if (!value || skillIDs.length + customSkills.length >= 50) return;
    const known = skills.find(skill => skill.name.localeCompare(value, "ru", { sensitivity: "accent" }) === 0);
    if (known) {
      if (!skillIDs.includes(known.id)) setSkillIDs([...skillIDs, known.id]);
    } else if (value.length >= 2 && !customSkills.some(skill => skill.toLocaleLowerCase("ru") === value.toLocaleLowerCase("ru"))) {
      setCustomSkills([...customSkills, value]);
    }
    setSkillDraft("");
  };
  if (state === "loading") return <AuthBootstrapLoader />;
  if (!user?.capabilities?.includes("FREELANCER")) return <main><div className="role-boundary"><p className="eyebrow">Режим исполнителя</p><h1>Профессиональный профиль недоступен</h1><p>Включите режим исполнителя, чтобы создать профиль, портфолио и услуги.</p><a className="button" href="/settings/account">Настроить режимы аккаунта</a></div></main>;
  return (
    <main>
      <Breadcrumbs
        items={[
          { label: "Главная", href: "/" },
          { label: "Кабинет", href: "/dashboard" },
          { label: "Профессиональный профиль" },
        ]}
      />
      <div className="page-heading">
        <div>
          <p className="eyebrow">Профиль специалиста</p>
          <h1>Профессиональный профиль</h1>
          <p className="lead">
            Именно эти данные видят заказчики в каталоге специалистов. Заполните
            специализацию, опыт, ставку, категории и навыки.
          </p>
        </div>
        {user?.username ? (
          <a
            className="button button--quiet"
            href={`/freelancers/${user.username}`}
          >
            Посмотреть публичный профиль
          </a>
        ) : null}
      </div>
      <form className="profile-settings-form" onSubmit={save}>
        <section className="profile-avatar-editor">
          <Avatar
            key={avatarRevision}
            name={user?.display_name || "Пользователь"}
            id={user?.id}
            size="lg"
            src={avatarPreviewURL || (user?.id ? `/api/v1/avatars/${encodeURIComponent(user.id)}?v=${avatarRevision}` : undefined)}
          />
          <div>
            <strong>Аватар профиля</strong>
            <p>
              JPG, PNG или WebP до 5 МБ. Квадратное изображение лучше выглядит в
              карточках.
            </p>
            <div className="profile-avatar-actions">
              <label className="button button--quiet file-button">
                {avatarUploading ? "Загружаем…" : "Выбрать изображение"}
                <input
                  type="file"
                  accept="image/jpeg,image/png,image/webp"
                  disabled={avatarUploading}
                  onChange={(event) => {
                    const file = event.target.files?.[0];
                    if (file) void changeAvatar(file);
                    event.currentTarget.value = "";
                  }}
                />
              </label>
              {user?.avatar_media_object_id || pendingAvatarID ? (
                <button
                  type="button"
                  className="button button--quiet"
                  onClick={removeAvatar}
                >
                  Удалить
                </button>
              ) : null}
            </div>
          </div>
          {avatarFile ? (
            <div className="selected-file">
              <FileTypeBadge
                name={avatarFile.name}
                mimeType={avatarFile.type}
              />
              <span>
                <strong>{avatarFile.name}</strong>
                <small>
                  {Math.max(1, Math.round(avatarFile.size / 1024))} КБ
                </small>
              </span>
            </div>
          ) : null}
        </section>
        <label>
          Имя в профиле
          <input required minLength={1} maxLength={120} value={displayName} onChange={(event) => setDisplayName(event.target.value)} placeholder="Как к вам обращаться заказчикам"/>
          <small className="form-hint">Отображается в профиле, каталоге, отзывах и сообщениях.</small>
        </label>
        <label>
          Профессиональный заголовок
          <input
            required
            minLength={3}
            maxLength={160}
            value={profile.professional_title ?? ""}
            onChange={(e) =>
              setProfile({ ...profile, professional_title: e.target.value })
            }
            placeholder="Например, Senior Flutter разработчик"
          />
        </label>
        <label>
          О себе
          <textarea
            required
            minLength={30}
            maxLength={5000}
            value={profile.bio ?? ""}
            onChange={(e) => setProfile({ ...profile, bio: e.target.value })}
            placeholder="Опишите опыт, специализацию и тип задач, которые берёте в работу."
          />
        </label>
        <div className="field-row">
          <label>
            Город / локация
            <input
              list="profile-city-options"
              autoComplete="address-level2"
              maxLength={160}
              value={profile.location_text ?? ""}
              onChange={(e) =>
                setProfile({ ...profile, location_text: e.target.value })
              }
            />
            <datalist id="profile-city-options">{cityOptions.map(city => <option value={city} key={city}/>)}</datalist>
          </label>
          <label>
            Страна
            <input
              list="profile-country-options"
              autoComplete="country"
              maxLength={2}
              data-pattern-message="Введите двухбуквенный код страны, например RU или KZ."
              pattern="[A-Za-z]{2}"
              value={profile.country_code ?? ""}
              onChange={(e) =>
                setProfile({ ...profile, country_code: e.target.value })
              }
            />
            <datalist id="profile-country-options">{countryOptions.map(([code, name]) => <option value={code} label={name} key={code}/>)}</datalist>
            <small className="form-hint">Начните вводить название или код страны</small>
          </label>
        </div>
        <div className="field-row">
          <label>
            Опыт, лет
            <input
              type="number"
              min="0"
              max="80"
              value={profile.experience_years ?? ""}
              onChange={(e) =>
                setProfile({
                  ...profile,
                  experience_years: e.target.value
                    ? Number(e.target.value)
                    : undefined,
                })
              }
            />
          </label>
          <label>
            Ставка, ₽/час
            <input
              type="number"
              min="0"
              step="100"
              value={
                profile.hourly_rate_kopecks
                  ? profile.hourly_rate_kopecks / 100
                  : ""
              }
              onChange={(e) =>
                setProfile({
                  ...profile,
                  hourly_rate_kopecks: e.target.value
                    ? Math.round(Number(e.target.value) * 100)
                    : undefined,
                })
              }
            />
          </label>
        </div>
        <div className="field-row">
          <label>
            Доступность
            <CustomSelect
              value={profile.availability || "AVAILABLE"}
              onChange={(e) =>
                setProfile({ ...profile, availability: e.target.value })
              }
            >
              <option value="AVAILABLE">Доступен</option>
              <option value="PARTIALLY_BUSY">Частично занят</option>
              <option value="BUSY">Занят</option>
              <option value="UNAVAILABLE">Недоступен</option>
            </CustomSelect>
          </label>
          <label>
            Видимость
            <CustomSelect
              value={profile.profile_visibility || "PUBLIC"}
              onChange={(e) =>
                setProfile({ ...profile, profile_visibility: e.target.value })
              }
            >
              <option value="PUBLIC">Публичный профиль</option>
              <option value="PRIVATE">Скрытый профиль</option>
            </CustomSelect>
          </label>
        </div>
        <fieldset>
          <legend>
            Категории <small>до 10, первая — основная</small>
          </legend>
          <div className="choice-grid">
            {categories.map((c) => (
              <label className="choice-chip" key={c.id}>
                <input
                  type="checkbox"
                  checked={categoryIDs.includes(c.id)}
                  onChange={() => toggle(categoryIDs, setCategoryIDs, c.id, 10)}
                />
                <span>{c.name}</span>
              </label>
            ))}
          </div>
        </fieldset>
        <fieldset>
          <legend>
            Навыки <small>до 50</small>
          </legend>
          <div className="autocomplete-add"><input list="profile-skill-options" value={skillDraft} maxLength={80} onChange={event => setSkillDraft(event.target.value)} onKeyDown={event => { if (event.key === "Enter") { event.preventDefault(); addSkill(); } }} placeholder="Например, Go, Figma или свой навык"/><datalist id="profile-skill-options">{skills.filter(skill => !skillIDs.includes(skill.id)).map(skill => <option value={skill.name} key={skill.id}/>)}</datalist><button type="button" className="button button--quiet" onClick={addSkill} disabled={!skillDraft.trim()}>Добавить</button></div>
          <p className="form-hint">Выберите подсказку или введите собственный навык и нажмите Enter.</p>
          <div className="selected-tags" aria-label="Выбранные навыки">
            {skillIDs.map(id => { const skill = skills.find(item => item.id === id); return skill ? <button type="button" className="selected-tag" key={id} onClick={() => setSkillIDs(skillIDs.filter(value => value !== id))}>{skill.name}<span aria-hidden="true">×</span></button> : null; })}
            {customSkills.map(skill => <button type="button" className="selected-tag is-custom" key={skill} onClick={() => setCustomSkills(customSkills.filter(value => value !== skill))}>{skill}<small>свой</small><span aria-hidden="true">×</span></button>)}
          </div>
        </fieldset>
        <button disabled={saving}>
          {saving ? "Сохраняем…" : "Сохранить профиль"}
        </button>
      </form>
    </main>
  );
}
