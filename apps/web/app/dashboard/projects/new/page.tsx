"use client";
import PrettyDateInput from "../../../pretty-date-input";
import { CustomSelect } from "../../../custom-select";

import { FormEvent, useEffect, useMemo, useState } from "react";
import Breadcrumbs from "../../../breadcrumbs";
import ProjectDescriptionEditor from "../../../project-description-editor";
import { IconImage } from "../../../icons";
import { useToast } from "../../../toast";
import FileTypeBadge from "../../../file-type-badge";
import { createRandomID } from "../../../random-id";

type Ref = { id: string; slug?: string; name: string };
type AIData = {
  title?: string;
  summary?: string;
  scope?: string;
  category_candidates?: Ref[];
  skills?: Ref[];
  budget?: { min_kopecks: number; max_kopecks: number };
  benchmark?: { min_kopecks: number; max_kopecks: number };
  duration_days?: { min: number; max: number };
};
type BudgetType = "FIXED" | "RANGE" | "HOURLY" | "NEGOTIABLE";
type Attachment = { id: string; name: string; mime: string; size: number };
const experiences = {
  "": "Не важно",
  BEGINNER: "Начинающий",
  INTERMEDIATE: "Средний",
  ADVANCED: "Опытный",
  EXPERT: "Эксперт",
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
const slugBase = (value: string) =>
  value
    .toLowerCase()
    .split("")
    .map((char) => translit[char] ?? char)
    .join("")
    .normalize("NFKD")
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/(^-|-$)/g, "")
    .slice(0, 225) || "project";
const generatedSlug = (title: string) =>
  `${slugBase(title)}-${createRandomID().slice(0, 6)}`;
const toRub = (kopecks?: number) =>
  kopecks ? String(Math.round(kopecks / 100)) : "";
const fileSize = (bytes: number) =>
  bytes < 1024 * 1024
    ? `${Math.max(1, Math.round(bytes / 1024))} КБ`
    : `${(bytes / 1024 / 1024).toFixed(1)} МБ`;
function normalizedMime(file: File) {
  if (file.type) return file.type;
  const ext = file.name.toLowerCase().split(".").pop();
  return ext === "pdf"
    ? "application/pdf"
    : ext === "docx"
      ? "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
      : ext === "doc"
        ? "application/msword"
        : ext === "txt"
          ? "text/plain"
          : ext === "png"
            ? "image/png"
            : ext === "webp"
              ? "image/webp"
              : ext === "jpg" || ext === "jpeg"
                ? "image/jpeg"
                : "";
}

export default function NewProjectPage() {
  const { push } = useToast();
  const [categories, setCategories] = useState<Ref[]>([]);
  const [allSkills, setAllSkills] = useState<Ref[]>([]);
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [category, setCategory] = useState("");
  const [skills, setSkills] = useState<string[]>([]);
  const [skillQuery, setSkillQuery] = useState("");
  const [budgetType, setBudgetType] = useState<BudgetType>("NEGOTIABLE");
  const [budgetMin, setBudgetMin] = useState("");
  const [budgetMax, setBudgetMax] = useState("");
  const [deadline, setDeadline] = useState("");
  const [minDeadline] = useState(() => {
    const date = new Date();
    date.setDate(date.getDate() + 1);
    return date.toISOString().slice(0, 10);
  });
  const [experience, setExperience] = useState("");
  const [visibility, setVisibility] = useState("PUBLIC");
  const [token, setToken] = useState("");
  const [saving, setSaving] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [attachments, setAttachments] = useState<Attachment[]>([]);
  const [aiImported, setAiImported] = useState(false);

  useEffect(() => {
    Promise.all([
      fetch("/api/v1/categories"),
      fetch("/api/v1/skills?limit=100"),
    ])
      .then(async ([cat, skill]) =>
        Promise.all([
          cat.ok ? cat.json() : { data: [] },
          skill.ok ? skill.json() : { data: [] },
        ]),
      )
      .then(([catBody, skillBody]) => {
        setCategories(catBody.data ?? []);
        setAllSkills(skillBody.data ?? []);
      })
      .catch(() =>
        push({
          kind: "error",
          title: "Не удалось загрузить категории и навыки",
        }),
      );

    const value = new URLSearchParams(location.search).get("draft");
    if (!value) return;
    setToken(value);
    fetch(`/api/v1/project-drafts/${value}`, { credentials: "same-origin" })
      .then((response) => (response.ok ? response.json() : null))
      .then((body) => {
        const data = body?.data?.normalized_data as AIData | undefined;
        if (!data) return;
        setTitle(data.title || "");
        setDescription(data.summary || data.scope || "");
        setCategory(data.category_candidates?.[0]?.id || "");
        setSkills(data.skills?.map((item) => item.id).filter(Boolean) || []);
        const suggested = data.budget || data.benchmark;
        if (suggested) {
          setBudgetType("RANGE");
          setBudgetMin(toRub(suggested.min_kopecks));
          setBudgetMax(toRub(suggested.max_kopecks));
        }
        if (data.duration_days?.max) {
          const date = new Date(Date.now() + data.duration_days.max * 86400000);
          setDeadline(date.toISOString().slice(0, 10));
        }
        setAiImported(true);
      });
  }, [push]);

  const filteredSkills = useMemo(
    () =>
      allSkills
        .filter(
          (skill) =>
            !skillQuery ||
            skill.name.toLowerCase().includes(skillQuery.toLowerCase()),
        )
        .slice(0, 30),
    [allSkills, skillQuery],
  );
  const selectedCategory = categories.find((item) => item.id === category);

  function toggleSkill(id: string) {
    setSkills((current) =>
      current.includes(id)
        ? current.filter((item) => item !== id)
        : current.length < 30
          ? [...current, id]
          : current,
    );
  }
  function budgetPayload() {
    if (budgetType === "NEGOTIABLE")
      return { type: budgetType, currency: "RUB" };
    const min = Math.round(Number(budgetMin) * 100);
    if (budgetType === "FIXED")
      return { type: budgetType, min_kopecks: min, currency: "RUB" };
    return {
      type: budgetType,
      min_kopecks: min,
      max_kopecks: Math.round(Number(budgetMax) * 100),
      currency: "RUB",
    };
  }

  async function uploadOne(file: File): Promise<Attachment> {
    const mime = normalizedMime(file);
    if (!mime)
      throw new Error(`Формат файла «${file.name}» не поддерживается.`);
    const presign = await fetch("/api/v1/uploads/presign", {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        purpose: "PROJECT",
        filename: file.name,
        mime_type: mime,
        size_bytes: file.size,
      }),
    });
    const pbody = await presign.json().catch(() => null);
    if (!presign.ok)
      throw new Error(
        pbody?.error?.message ||
          `Не удалось подготовить загрузку «${file.name}».`,
      );
    const data = pbody.data;
    const put = await fetch(data.upload_url, {
      method: "PUT",
      headers: data.headers,
      body: file,
    });
    if (!put.ok) throw new Error(`Не удалось загрузить «${file.name}».`);
    const complete = await fetch(`/api/v1/uploads/${data.media_id}/complete`, {
      method: "POST",
      credentials: "same-origin",
    });
    if (!complete.ok)
      throw new Error(`Не удалось завершить загрузку «${file.name}».`);
    for (let attempt = 0; attempt < 20; attempt++) {
      const status = await fetch(`/api/v1/uploads/${data.media_id}`, {
        credentials: "same-origin",
      });
      const body = await status.json().catch(() => null);
      if (body?.data?.scan_status === "CLEAN")
        return { id: data.media_id, name: file.name, mime, size: file.size };
      if (["FAILED", "INFECTED"].includes(body?.data?.scan_status))
        throw new Error(`Файл «${file.name}» отклонён проверкой безопасности.`);
      await new Promise((resolve) => setTimeout(resolve, 500));
    }
    throw new Error(
      `Файл «${file.name}» ещё проверяется. Попробуйте добавить его позже.`,
    );
  }

  async function addFiles(files: FileList | null) {
    if (!files?.length) return;
    const freeSlots = 5 - attachments.length;
    if (freeSlots <= 0) {
      push({ kind: "error", title: "Можно приложить не более 5 файлов" });
      return;
    }
    setUploading(true);
    try {
      const added: Attachment[] = [];
      for (const file of Array.from(files).slice(0, freeSlots))
        added.push(await uploadOne(file));
      setAttachments((current) => [...current, ...added]);
      push({
        kind: "success",
        title:
          added.length === 1
            ? "Файл ТЗ прикреплён"
            : `Прикреплено файлов: ${added.length}`,
      });
    } catch (error) {
      push({
        kind: "error",
        title: "Не удалось прикрепить файл",
        message: error instanceof Error ? error.message : "Попробуйте ещё раз.",
      });
    } finally {
      setUploading(false);
    }
  }

  async function removeAttachment(item: Attachment) {
    await fetch(`/api/v1/uploads/${item.id}`, {
      method: "DELETE",
      credentials: "same-origin",
    }).catch(() => null);
    setAttachments((current) =>
      current.filter((value) => value.id !== item.id),
    );
  }

  async function create(event: FormEvent) {
    event.preventDefault();
    if (!category) {
      push({
        kind: "error",
        title: "Выберите категорию",
        message: "Категория нужна, чтобы специалисты нашли проект.",
      });
      return;
    }
    if (budgetType !== "NEGOTIABLE" && (!budgetMin || Number(budgetMin) <= 0)) {
      push({ kind: "error", title: "Укажите бюджет" });
      return;
    }
    if (
      ["RANGE", "HOURLY"].includes(budgetType) &&
      (!budgetMax || Number(budgetMax) < Number(budgetMin))
    ) {
      push({
        kind: "error",
        title: "Проверьте диапазон бюджета",
        message: "Максимальная сумма должна быть не меньше минимальной.",
      });
      return;
    }
    if (uploading) {
      push({ kind: "info", title: "Дождитесь окончания загрузки файлов" });
      return;
    }
    setSaving(true);
    try {
      const response = await fetch("/api/v1/me/projects", {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          category_id: category,
          title: title.trim(),
          slug: generatedSlug(title),
          description: description.trim(),
          budget: budgetPayload(),
          deadline_at: deadline
            ? new Date(`${deadline}T23:59:59`).toISOString()
            : null,
          experience_level: experience,
          visibility,
          skill_ids: skills,
          media_ids: attachments.map((item) => item.id),
          source_draft_token: token || undefined,
        }),
      });
      const body = await response.json().catch(() => ({}));
      if (!response.ok)
        throw new Error(body?.error?.message || "Проверьте заполненные поля");
      push({ kind: "success", title: "Черновик проекта создан" });
      location.assign(`/dashboard/projects/${body.data.id}`);
    } catch (error) {
      push({
        kind: "error",
        title: "Не удалось создать проект",
        message: error instanceof Error ? error.message : "Попробуйте ещё раз.",
      });
    } finally {
      setSaving(false);
    }
  }

  return (
    <main className="project-editor-page">
      <Breadcrumbs
        items={[
          { label: "Главная", href: "/" },
          { label: "Кабинет", href: "/dashboard" },
          { label: "Новый проект" },
        ]}
      />
      <header className="page-heading">
        <div>
          <p className="eyebrow">Разместить задачу</p>
          <h1>Новый проект</h1>
          <p className="lead">
            {aiImported
              ? "AI уже предложил черновик. Все поля ниже редактируются вручную — ничего не публикуется без вашей проверки."
              : "Заполните проект вручную. AI-помощник — необязательный способ ускорить подготовку, а не замена формы."}
          </p>
        </div>
        {!aiImported ? (
          <a className="button button--quiet" href="/create-project">
            Помочь составить через AI
          </a>
        ) : null}
      </header>
      <form className="project-editor" onSubmit={create}>
        <section className="form-section">
          <div className="form-section__heading">
            <span>01</span>
            <div>
              <h2>Что нужно сделать</h2>
              <p>
                Название, подробное ТЗ и материалы. Адрес страницы Naimio
                сформирует автоматически.
              </p>
            </div>
          </div>
          <label>
            Название проекта
            <input
              required
              minLength={3}
              maxLength={200}
              value={title}
              onChange={(event) => setTitle(event.target.value)}
              placeholder="Например, разработать мобильное приложение"
            />
          </label>
          <div className="editor-field">
            <span className="editor-field__label">Описание и ТЗ</span>
            <ProjectDescriptionEditor
              value={description}
              onChange={setDescription}
            />
          </div>
          <div className="project-attachments">
            <div className="project-attachments__head">
              <div>
                <strong>Файлы ТЗ и материалы</strong>
                <p>
                  PDF, DOC, DOCX, TXT или изображения. До 5 файлов, каждый
                  проходит проверку безопасности.
                </p>
              </div>
              <label className="button button--quiet file-button">
                <IconImage size={17} />
                {uploading ? "Загружаем…" : "Прикрепить файлы"}
                <input
                  type="file"
                  multiple
                  disabled={uploading || attachments.length >= 5}
                  accept=".pdf,.doc,.docx,.txt,.png,.jpg,.jpeg,.webp"
                  onChange={(event) => {
                    void addFiles(event.target.files);
                    event.currentTarget.value = "";
                  }}
                />
              </label>
            </div>
            {attachments.length ? (
              <ul className="project-attachment-list">
                {attachments.map((item) => (
                  <li key={item.id}>
                    <FileTypeBadge name={item.name} mimeType={item.mime} />
                    <div>
                      <strong>{item.name}</strong>
                      <small>{fileSize(item.size)}</small>
                    </div>
                    <button
                      type="button"
                      className="button button--quiet button--compact"
                      onClick={() => void removeAttachment(item)}
                    >
                      Удалить
                    </button>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="project-attachments__empty">
                Можно создать проект и без файла — текстовое ТЗ остаётся
                основным описанием.
              </p>
            )}
          </div>
        </section>
        <section className="form-section">
          <div className="form-section__heading">
            <span>02</span>
            <div>
              <h2>Категория и навыки</h2>
              <p>Так проект попадёт к релевантным специалистам.</p>
            </div>
          </div>
          <label>
            Категория
            <CustomSelect
              required
              value={category}
              onChange={(event) => setCategory(event.target.value)}
            >
              <option value="">Выберите категорию</option>
              {categories.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.name}
                </option>
              ))}
            </CustomSelect>
          </label>
          {selectedCategory ? (
            <p className="selection-summary">
              Выбрано направление: <strong>{selectedCategory.name}</strong>
            </p>
          ) : null}
          <label>
            Найти навык
            <input
              type="search"
              value={skillQuery}
              onChange={(event) => setSkillQuery(event.target.value)}
              placeholder="Flutter, Go, Figma…"
            />
          </label>
          <div className="skill-picker" aria-label="Навыки">
            {filteredSkills.map((skill) => (
              <button
                className={
                  skills.includes(skill.id)
                    ? "skill-option is-selected"
                    : "skill-option"
                }
                type="button"
                key={skill.id}
                onClick={() => toggleSkill(skill.id)}
                aria-pressed={skills.includes(skill.id)}
              >
                {skill.name}
              </button>
            ))}
          </div>
        </section>
        <section className="form-section">
          <div className="form-section__heading">
            <span>03</span>
            <div>
              <h2>Бюджет и сроки</h2>
              <p>Укажите ориентиры — условия можно обсудить со специалистом.</p>
            </div>
          </div>
          <div className="field-row">
            <label>
              Тип бюджета
              <CustomSelect
                value={budgetType}
                onChange={(event) =>
                  setBudgetType(event.target.value as BudgetType)
                }
              >
                <option value="NEGOTIABLE">По договорённости</option>
                <option value="FIXED">Фиксированная сумма</option>
                <option value="RANGE">Диапазон</option>
                <option value="HOURLY">Почасовой диапазон</option>
              </CustomSelect>
            </label>
            <label>
              Уровень специалиста
              <CustomSelect
                value={experience}
                onChange={(event) => setExperience(event.target.value)}
              >
                {Object.entries(experiences).map(([value, label]) => (
                  <option key={value} value={value}>
                    {label}
                  </option>
                ))}
              </CustomSelect>
            </label>
          </div>
          {budgetType !== "NEGOTIABLE" ? (
            <div className="field-row">
              <label>
                {budgetType === "FIXED" ? "Сумма, ₽" : "От, ₽"}
                <input
                  required
                  type="number"
                  min="1"
                  step="1"
                  value={budgetMin}
                  onChange={(event) => setBudgetMin(event.target.value)}
                />
              </label>
              {["RANGE", "HOURLY"].includes(budgetType) ? (
                <label>
                  До, ₽
                  <input
                    required
                    type="number"
                    min="1"
                    step="1"
                    value={budgetMax}
                    onChange={(event) => setBudgetMax(event.target.value)}
                  />
                </label>
              ) : null}
            </div>
          ) : null}
          <div className="field-row">
            <label>
              Желаемый срок
              <PrettyDateInput min={minDeadline} value={deadline} onChange={setDeadline} ariaLabel="Желаемый срок"/>
            </label>
            <label>
              Видимость после публикации
              <CustomSelect
                value={visibility}
                onChange={(event) => setVisibility(event.target.value)}
              >
                <option value="PUBLIC">Публичный — показывать в каталоге</option>
                <option value="PRIVATE">Приватный — не показывать в каталоге</option>
              </CustomSelect>
            </label>
          </div>
        </section>
        <div className="project-editor__footer">
          <div>
            <strong>Сначала создаётся черновик.</strong>
            <p>
              Перед публикацией вы ещё раз увидите все данные, файлы и сможете
              отредактировать проект.
            </p>
          </div>
          <button disabled={saving || uploading}>
            {saving
              ? "Создаём…"
              : uploading
                ? "Ждём файлы…"
                : "Создать черновик"}
          </button>
        </div>
      </form>
    </main>
  );
}
