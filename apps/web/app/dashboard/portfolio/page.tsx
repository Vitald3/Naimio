"use client";

import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import Breadcrumbs from "../../breadcrumbs";
import { CustomSelect } from "../../custom-select";
import { Cover } from "../../media-components";
import { useAuth } from "../../auth-state";
import { AuthBootstrapLoader } from "../../auth-loader";
import { useToast } from "../../toast";
import FileTypeBadge from "../../file-type-badge";
import { createRandomID } from "../../random-id";

type Ref = { id: string; name: string; slug?: string };
type Item = {
  id: string;
  title: string;
  slug: string;
  description?: string;
  external_url?: string;
  visibility: string;
  sort_order: number;
  categories: Ref[];
  skills: Ref[];
  media: Array<{ id: string }>;
};
const empty = {
  title: "",
  description: "",
  external_url: "",
  visibility: "PUBLIC",
  sort_order: "0",
  category_ids: [] as string[],
  skill_ids: [] as string[],
  media_object_ids: [] as string[],
};
const translit: Record<string, string> = {
  а: "a",
  б: "b",
  в: "v",
  г: "g",
  д: "d",
  е: "e",
  ё: "e",
  ж: "zh",
  з: "z",
  и: "i",
  й: "y",
  к: "k",
  л: "l",
  м: "m",
  н: "n",
  о: "o",
  п: "p",
  р: "r",
  с: "s",
  т: "t",
  у: "u",
  ф: "f",
  х: "h",
  ц: "ts",
  ч: "ch",
  ш: "sh",
  щ: "sch",
  ъ: "",
  ы: "y",
  ь: "",
  э: "e",
  ю: "yu",
  я: "ya",
};
const slugFor = (value: string) =>
  value
    .toLowerCase()
    .split("")
    .map((char) => translit[char] ?? char)
    .join("")
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/(^-|-$)/g, "")
    .slice(0, 180) || "portfolio-work";

export default function PortfolioPage() {
  const { user, state } = useAuth();
  const { push } = useToast();
  const [items, setItems] = useState<Item[]>([]);
  const [categories, setCategories] = useState<Ref[]>([]);
  const [skills, setSkills] = useState<Ref[]>([]);
  const [editing, setEditing] = useState<Item | null>(null);
  const [form, setForm] = useState(empty);
  const [saving, setSaving] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [preview, setPreview] = useState(false);
  const [uploadedFiles, setUploadedFiles] = useState<
    Array<{ id: string; name: string; type: string }>
  >([]);
  const freelancer = user?.capabilities.includes("FREELANCER") ?? false;
  const load = useCallback(async () => {
    const response = await fetch("/api/v1/me/portfolio", {
      credentials: "same-origin",
      cache: "no-store",
    });
    if (!response.ok) throw new Error();
    setItems((await response.json()).data ?? []);
  }, []);
  useEffect(() => {
    if (!freelancer) return;
    Promise.all([
      load(),
      fetch("/api/v1/categories").then((r) => r.json()),
      fetch("/api/v1/skills?limit=100").then((r) => r.json()),
    ])
      .then(([, c, s]) => {
        setCategories(c.data ?? []);
        setSkills(s.data ?? []);
      })
      .catch(() =>
        push({ kind: "error", title: "Не удалось загрузить портфолио" }),
      );
  }, [freelancer, load, push]);
  const selectedSkills = useMemo(
    () => new Set(form.skill_ids),
    [form.skill_ids],
  );
  function start(item?: Item) {
    if (item) {
      setEditing(item);
      setForm({
        title: item.title,
        description: item.description ?? "",
        external_url: item.external_url ?? "",
        visibility: item.visibility,
        sort_order: String(item.sort_order),
        category_ids: item.categories.map((v) => v.id),
        skill_ids: item.skills.map((v) => v.id),
        media_object_ids: item.media.map((v) => v.id),
      });
      setUploadedFiles(
        item.media.map((media, index) => ({
          id: media.id,
          name: `Изображение ${index + 1}`,
          type: "image/*",
        })),
      );
      window.setTimeout(
        () =>
          document
            .querySelector(".portfolio-editor")
            ?.scrollIntoView({ behavior: "smooth", block: "start" }),
        0,
      );
    } else {
      setEditing(null);
      setForm(empty);
      setUploadedFiles([]);
    }
    setPreview(false);
  }
  async function upload(file: File) {
    setUploading(true);
    try {
      const presign = await fetch("/api/v1/uploads/presign", {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          purpose: "PORTFOLIO",
          filename: file.name,
          mime_type: file.type,
          size_bytes: file.size,
        }),
      });
      if (!presign.ok)
        throw new Error(
          "Выберите изображение JPG, PNG или WebP до допустимого размера.",
        );
      const { data } = await presign.json();
      const put = await fetch(data.upload_url, {
        method: "PUT",
        headers: data.headers,
        body: file,
      });
      if (!put.ok) throw new Error("Не удалось загрузить изображение.");
      const complete = await fetch(
        `/api/v1/uploads/${data.media_id}/complete`,
        { method: "POST", credentials: "same-origin" },
      );
      if (!complete.ok) throw new Error("Не удалось завершить загрузку.");
      for (let attempt = 0; attempt < 15; attempt++) {
        const response = await fetch(`/api/v1/uploads/${data.media_id}`, {
          credentials: "same-origin",
        });
        const result = await response.json();
        if (result.data?.scan_status === "CLEAN") {
          setForm((current) => ({
            ...current,
            media_object_ids: [
              ...current.media_object_ids,
              data.media_id,
            ].slice(0, 20),
          }));
          setUploadedFiles((current) =>
            [
              ...current,
              { id: data.media_id, name: file.name, type: file.type },
            ].slice(0, 20),
          );
          push({ kind: "success", title: "Изображение добавлено" });
          return;
        }
        if (["FAILED", "INFECTED"].includes(result.data?.scan_status))
          throw new Error("Изображение отклонено проверкой безопасности.");
        await new Promise((resolve) => setTimeout(resolve, 700));
      }
      throw new Error(
        "Проверка изображения ещё идёт. Попробуйте снова чуть позже.",
      );
    } catch (error) {
      push({
        kind: "error",
        title: "Не удалось добавить изображение",
        message: error instanceof Error ? error.message : "Попробуйте ещё раз.",
      });
    } finally {
      setUploading(false);
    }
  }
  async function save(event: FormEvent) {
    event.preventDefault();
    setSaving(true);
    const payload = {
      ...form,
      title: form.title.trim(),
      slug:
        editing?.slug ||
        `${slugFor(form.title)}-${createRandomID().slice(0, 6)}`,
      description: form.description.trim(),
      external_url: form.external_url.trim(),
      sort_order: Number(form.sort_order),
      price_min_kopecks: null,
      price_max_kopecks: null,
      completed_on: "",
    };
    try {
      const response = await fetch(
        editing ? `/api/v1/me/portfolio/${editing.id}` : "/api/v1/me/portfolio",
        {
          method: editing ? "PATCH" : "POST",
          credentials: "same-origin",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(payload),
        },
      );
      const body = await response.json().catch(() => null);
      if (!response.ok)
        throw new Error(body?.error?.message || "Проверьте заполненные поля.");
      push({
        kind: "success",
        title: editing ? "Работа обновлена" : "Работа добавлена",
      });
      start();
      await load();
    } catch (error) {
      push({
        kind: "error",
        title: "Не удалось сохранить работу",
        message: error instanceof Error ? error.message : "Попробуйте ещё раз.",
      });
    } finally {
      setSaving(false);
    }
  }
  async function remove(item: Item) {
    if (!window.confirm(`Удалить «${item.title}» из портфолио?`)) return;
    const response = await fetch(`/api/v1/me/portfolio/${item.id}`, {
      method: "DELETE",
      credentials: "same-origin",
    });
    if (response.ok) {
      push({ kind: "success", title: "Работа удалена" });
      await load();
    } else push({ kind: "error", title: "Не удалось удалить работу" });
  }
  if (state === "loading")
    return <AuthBootstrapLoader />;
  if (!freelancer)
    return (
      <main>
        <Breadcrumbs
          items={[
            { label: "Главная", href: "/" },
            { label: "Кабинет", href: "/dashboard" },
            { label: "Портфолио" },
          ]}
        />
        <div className="notice">
          <h1>Портфолио исполнителя</h1>
          <p>Этот раздел доступен аккаунтам с ролью исполнителя.</p>
        </div>
      </main>
    );
  return (
    <main>
      <Breadcrumbs
        items={[
          { label: "Главная", href: "/" },
          { label: "Кабинет", href: "/dashboard" },
          { label: "Портфолио" },
        ]}
      />
      <div className="page-heading">
        <div>
          <p className="eyebrow">Профессиональный профиль</p>
          <h1>Портфолио</h1>
          <p className="lead">
            Покажите задачу, ход работы и результат. Опубликованные кейсы сразу
            появляются в вашем профиле.
          </p>
        </div>
        <button type="button" onClick={() => start()}>
          Добавить работу
        </button>
      </div>
      <div className="portfolio-manager">
        <form className="portfolio-editor" onSubmit={save}>
          <div className="form-section__heading">
            <span>{editing ? "✎" : "+"}</span>
            <div>
              <h2>{editing ? "Редактирование работы" : "Новая работа"}</h2>
              <p>Заполните главное — детали можно обновить позже.</p>
            </div>
          </div>
          <label>
            Название
            <input
              required
              maxLength={180}
              value={form.title}
              onChange={(e) => setForm({ ...form, title: e.target.value })}
              placeholder="Например, редизайн кабинета сервиса"
            />
          </label>
          <label>
            Описание
            <textarea
              maxLength={5000}
              rows={7}
              value={form.description}
              onChange={(e) =>
                setForm({ ...form, description: e.target.value })
              }
              placeholder="Задача, решения и измеримый результат"
            />
          </label>
          <div className="field-row">
            <label>
              Категория
              <CustomSelect
                value={form.category_ids[0] ?? ""}
                onChange={(e) =>
                  setForm({
                    ...form,
                    category_ids: e.target.value ? [e.target.value] : [],
                  })
                }
              >
                <option value="">Без категории</option>
                {categories.map((v) => (
                  <option key={v.id} value={v.id}>
                    {v.name}
                  </option>
                ))}
              </CustomSelect>
            </label>
            <label>
              Видимость
              <CustomSelect
                value={form.visibility}
                onChange={(e) =>
                  setForm({ ...form, visibility: e.target.value })
                }
              >
                <option value="PUBLIC">В публичном профиле</option>
                <option value="PRIVATE">Только мне</option>
              </CustomSelect>
            </label>
            <label>
              Порядок
              <input
                type="number"
                min="0"
                max="10000"
                value={form.sort_order}
                onChange={(e) =>
                  setForm({ ...form, sort_order: e.target.value })
                }
              />
            </label>
          </div>
          <label>
            Ссылка на проект
            <input
              type="url"
              maxLength={2048}
              value={form.external_url}
              onChange={(e) =>
                setForm({ ...form, external_url: e.target.value })
              }
              placeholder="https://…"
            />
          </label>
          <fieldset>
            <legend>Технологии и навыки</legend>
            <div className="skill-picker">
              {skills.slice(0, 40).map((skill) => (
                <button
                  type="button"
                  className={
                    selectedSkills.has(skill.id)
                      ? "skill-option is-selected"
                      : "skill-option"
                  }
                  aria-pressed={selectedSkills.has(skill.id)}
                  key={skill.id}
                  onClick={() =>
                    setForm((current) => ({
                      ...current,
                      skill_ids: selectedSkills.has(skill.id)
                        ? current.skill_ids.filter((id) => id !== skill.id)
                        : [...current.skill_ids, skill.id].slice(0, 20),
                    }))
                  }
                >
                  {skill.name}
                </button>
              ))}
            </div>
          </fieldset>
          <label className="portfolio-upload">
            Обложка и изображения
            <input
              type="file"
              accept="image/jpeg,image/png,image/webp"
              disabled={uploading || form.media_object_ids.length >= 20}
              onChange={(e) => {
                const file = e.target.files?.[0];
                if (file) void upload(file);
                e.target.value = "";
              }}
            />
            <small>
              {uploading
                ? "Загружаем и проверяем…"
                : form.media_object_ids.length
                  ? `Добавлено изображений: ${form.media_object_ids.length}`
                  : "До 20 изображений"}
            </small>
          </label>
          {uploadedFiles.length ? (
            <ul className="uploaded-file-list">
              {uploadedFiles.map((uploaded) => (
                <li key={uploaded.id}>
                  <FileTypeBadge
                    name={uploaded.name}
                    mimeType={uploaded.type}
                  />
                  <span>
                    <strong>{uploaded.name}</strong>
                    <small>Добавлен в портфолио</small>
                  </span>
                </li>
              ))}
            </ul>
          ) : null}
          <div className="button-group">
            <button disabled={saving || uploading}>
              {saving ? "Сохраняем…" : "Сохранить"}
            </button>
            <button
              type="button"
              className="button button--quiet"
              onClick={() => setPreview((value) => !value)}
            >
              {preview ? "Скрыть предпросмотр" : "Предпросмотр"}
            </button>
            {editing ? (
              <button
                type="button"
                className="button button--quiet"
                onClick={() => start()}
              >
                Отмена
              </button>
            ) : null}
          </div>
          {preview ? (
            <article className="portfolio-preview">
              <Cover
                id={editing?.id || form.title}
                title={form.title || "Новая работа"}
              />
              <h3>{form.title || "Название работы"}</h3>
              <p>{form.description || "Описание результата появится здесь."}</p>
            </article>
          ) : null}
        </form>
        <section className="portfolio-owned">
          <h2>Мои работы</h2>
          {items.length ? (
            <ul className="portfolio-owned__grid">
              {items.map((item) => (
                <li key={item.id}>
                  <article>
                    <Cover id={item.id} title={item.title} />
                    <div>
                      <span className="badge">
                        {item.visibility === "PUBLIC"
                          ? "Опубликовано"
                          : "Скрыто"}
                      </span>
                      <h3>{item.title}</h3>
                      <p>{item.description || "Описание не добавлено."}</p>
                      <div className="button-group">
                        <button
                          type="button"
                          className="button button--quiet"
                          onClick={() => start(item)}
                        >
                          Редактировать
                        </button>
                        <button
                          type="button"
                          className="button button--danger"
                          onClick={() => remove(item)}
                        >
                          Удалить
                        </button>
                      </div>
                    </div>
                  </article>
                </li>
              ))}
            </ul>
          ) : (
            <div className="empty">
              <h3>Работ пока нет</h3>
              <p>
                Добавьте первый кейс — он усилит профиль и поможет заказчику
                оценить ваш подход.
              </p>
            </div>
          )}
        </section>
      </div>
    </main>
  );
}
