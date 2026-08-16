"use client";
import { FormEvent, useCallback, useEffect, useState } from "react";
import { CustomSelect } from "../../custom-select";
import BlogEditor from "../../blog-editor";
import AdminReasonEditor from "../admin-reason-editor";
import { useToast } from "../../toast";
import {
  AdminError,
  AdminHeader,
  AdminLoading,
  AdminReasonAction,
  StatusPill,
  adminRequest,
  formatDate,
} from "../admin-ui";
type Category = { id: string; name: string; slug: string; description: string };
type Tag = { id: string; name: string; slug: string };
type Post = {
  id: string;
  title: string;
  slug: string;
  excerpt: string;
  content_html: string;
  category_id?: string;
  cover_media_object_id?: string;
  cover_url?: string;
  cover_alt?: string;
  status: string;
  seo_title?: string;
  seo_description?: string;
  canonical_url?: string;
  scheduled_at?: string;
  published_at?: string;
  tag_ids?: string[];
  tags?: Tag[];
  updated_at: string;
};
type Data = { posts: { items: Post[] }; categories: Category[]; tags: Tag[] };
const empty = (): Post => ({
  id: "",
  title: "",
  slug: "",
  excerpt: "",
  content_html: "<p></p>",
  status: "DRAFT",
  updated_at: "",
  tag_ids: [],
});
export default function ContentPage() {
  const [data, setData] = useState<Data | null>(null),
    [error, setError] = useState(""),
    [loading, setLoading] = useState(true),
    [editing, setEditing] = useState<Post | null>(null);
  const load = useCallback(() => {
    setLoading(true);
    adminRequest<{ data: Data }>("/api/v1/admin/blog")
      .then((b) => {
        setData(b.data);
        setError("");
      })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, []);
  useEffect(load, [load]);
  return (
    <>
      <AdminHeader
        title="Блог / CMS"
        description="Пишите, планируйте и публикуйте SEO-материалы. Публичный флаг не удаляет редакционные данные."
        actions={
          <button onClick={() => setEditing(empty())}>+ Новая статья</button>
        }
      />
      {editing && data ? (
        <PostEditor
          initial={editing}
          categories={data.categories}
          tags={data.tags}
          done={() => {
            setEditing(null);
            load();
          }}
        />
      ) : null}
      {loading ? (
        <AdminLoading />
      ) : error ? (
        <AdminError message={error} onRetry={load} />
      ) : data ? (
        <>
          <div className="cms-layout">
            <section className="admin-section">
              <h2>Материалы</h2>
              <div className="cms-post-list">
                {data.posts.items.map((p) => (
                  <button
                    key={p.id}
                    onClick={() =>
                      setEditing({
                        ...p,
                        tag_ids: p.tags?.map((t) => t.id) ?? [],
                      })
                    }
                  >
                    <span>
                      <StatusPill value={p.status} />
                      <small>{formatDate(p.updated_at)}</small>
                    </span>
                    <strong>{p.title}</strong>
                    <small>/{p.slug}</small>
                  </button>
                ))}
              </div>
            </section>
            <Taxonomy
              title="Категории"
              kind="categories"
              items={data.categories}
              reload={load}
            />
            <Taxonomy
              title="Теги"
              kind="tags"
              items={data.tags}
              reload={load}
            />
          </div>
        </>
      ) : null}
      {!data?.posts.items.length && !loading ? (
        <div className="empty">
          <h2>Создайте первый материал</h2>
        </div>
      ) : null}
    </>
  );
}
function PostEditor({
  initial,
  categories,
  tags,
  done,
}: {
  initial: Post;
  categories: Category[];
  tags: Tag[];
  done: () => void;
}) {
  const { push } = useToast();
  const [value, setValue] = useState(initial),
    [saving, setSaving] = useState(false),
    [preview, setPreview] = useState(false),
    [coverBusy, setCoverBusy] = useState(false),
    [reason, setReason] = useState("Редакционное сохранение материала");
  const set = <K extends keyof Post>(k: K, v: Post[K]) =>
    setValue((x) => ({ ...x, [k]: v }));
  async function uploadCover(file?: File) {
    if (!file) return;
    setCoverBusy(true);
    try {
      const p = await fetch("/api/v1/uploads/presign", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          purpose: "BLOG_COVER",
          filename: file.name,
          mime_type: file.type,
          size_bytes: file.size,
        }),
      });
      if (!p.ok) throw new Error("Не удалось подготовить загрузку");
      const x = (await p.json()).data;
      const put = await fetch(x.upload_url, {
        method: "PUT",
        headers: x.headers,
        body: file,
      });
      if (!put.ok) throw new Error("Не удалось загрузить файл");
      const done = await fetch(`/api/v1/uploads/${x.media_id}/complete`, {
        method: "POST",
      });
      if (!done.ok) throw new Error("Не удалось проверить файл");
      setValue((v) => ({
        ...v,
        cover_media_object_id: x.media_id,
        cover_url: `/api/v1/blog/media/${x.media_id}`,
      }));
    } catch (e) {
      push({
        kind: "error",
        title: "Обложка не загружена",
        message: e instanceof Error ? e.message : "Ошибка",
      });
    } finally {
      setCoverBusy(false);
    }
  }
  async function save(e: FormEvent) {
    e.preventDefault();
    if (!reason.trim()) return;
    setSaving(true);
    try {
      const post = {
        title: value.title,
        slug: value.slug,
        excerpt: value.excerpt,
        content_html: value.content_html,
        category_id: value.category_id || "",
        tag_ids: value.tag_ids ?? [],
        cover_media_object_id: value.cover_media_object_id || "",
        cover_alt: value.cover_alt || "",
        status: value.status,
        seo_title: value.seo_title || "",
        seo_description: value.seo_description || "",
        canonical_url: value.canonical_url || "",
        scheduled_at:
          value.status === "SCHEDULED" && value.scheduled_at
            ? new Date(value.scheduled_at).toISOString()
            : null,
      };
      await adminRequest(
        value.id
          ? `/api/v1/admin/blog/posts/${value.id}`
          : "/api/v1/admin/blog",
        {
          method: value.id ? "PATCH" : "POST",
          body: JSON.stringify({ post, reason: reason.trim() }),
        },
      );
      push({
        kind: "success",
        title:
          value.status === "PUBLISHED"
            ? "Статья опубликована"
            : "Материал сохранён",
      });
      done();
    } catch (e) {
      push({
        kind: "error",
        title: "Не удалось сохранить",
        message: e instanceof Error ? e.message : "Проверьте поля",
      });
    } finally {
      setSaving(false);
    }
  }
  async function destructive(action: "archive" | "delete", reason: string) {
    await adminRequest(
      `/api/v1/admin/blog/posts/${value.id}${action === "archive" ? "/archive" : ""}`,
      {
        method: action === "archive" ? "POST" : "DELETE",
        body: JSON.stringify({ reason }),
      },
    );
    done();
  }
  return (
    <section className="cms-editor-panel">
      <form className="cms-editor" onSubmit={save}>
        <header>
          <div>
            <p className="eyebrow">
              {value.id ? "Редактирование" : "Новый материал"}
            </p>
            <h2>{value.title || "Без названия"}</h2>
          </div>
          <div className="inline-actions">
            <button
              type="button"
              className="button button--quiet"
              onClick={() => setPreview(!preview)}
            >
              {preview ? "Редактор" : "Предпросмотр"}
            </button>
            <button disabled={saving}>
              {saving ? "Сохраняем…" : "Сохранить"}
            </button>
            <button
              type="button"
              className="button button--quiet"
              onClick={done}
            >
              Закрыть
            </button>
          </div>
        </header>
        <div className="field-row">
          <label>
            Заголовок
            <input
              required
              maxLength={220}
              value={value.title}
              onChange={(e) => set("title", e.target.value)}
            />
          </label>
          <label>
            Slug
            <input
              required
              pattern="[a-z0-9]+(?:-[a-z0-9]+)*"
              maxLength={240}
              value={value.slug}
              onChange={(e) =>
                set(
                  "slug",
                  e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, ""),
                )
              }
            />
          </label>
        </div>
        <label>
          Лид
          <textarea
            required
            maxLength={600}
            rows={3}
            value={value.excerpt}
            onChange={(e) => set("excerpt", e.target.value)}
          />
        </label>
        <label>
          Причина изменения для аудита
          <AdminReasonEditor value={reason} onChange={setReason}/>
        </label>
        <div className="field-row">
          <label>
            Категория
            <CustomSelect
              value={value.category_id || ""}
              onChange={(e) => set("category_id", e.target.value)}
            >
              <option value="">Без категории</option>
              {categories.map((c) => (
                <option value={c.id} key={c.id}>
                  {c.name}
                </option>
              ))}
            </CustomSelect>
          </label>
          <label>
            Статус
            <CustomSelect
              value={value.status}
              onChange={(e) => set("status", e.target.value)}
            >
              <option value="DRAFT">Черновик</option>
              <option value="SCHEDULED">По расписанию</option>
              <option value="PUBLISHED">Опубликовано</option>
            </CustomSelect>
          </label>
          {value.status === "SCHEDULED" ? (
            <label>
              Дата публикации
              <input
                required
                type="datetime-local"
                value={value.scheduled_at?.slice(0, 16) || ""}
                onChange={(e) => set("scheduled_at", e.target.value)}
              />
            </label>
          ) : null}
        </div>
        <fieldset>
          <legend>Теги</legend>
          <div className="checkbox-grid">
            {tags.map((t) => (
              <label key={t.id}>
                <input
                  type="checkbox"
                  checked={value.tag_ids?.includes(t.id) ?? false}
                  onChange={(e) =>
                    set(
                      "tag_ids",
                      e.target.checked
                        ? [...(value.tag_ids ?? []), t.id]
                        : (value.tag_ids ?? []).filter((id) => id !== t.id),
                    )
                  }
                />
                {t.name}
              </label>
            ))}
          </div>
        </fieldset>
        {preview ? (
          <div
            className="article-content cms-preview"
            dangerouslySetInnerHTML={{ __html: value.content_html }}
          />
        ) : (
          <BlogEditor
            value={value.content_html}
            onChange={(v) => set("content_html", v)}
          />
        )}
        <section className="cms-editor__seo">
          <h3>Обложка и SEO</h3>
          <div className="field-row">
            <label>
              Обложка
              <input
                type="file"
                accept="image/jpeg,image/png,image/webp"
                disabled={coverBusy}
                onChange={(e) => void uploadCover(e.target.files?.[0])}
              />
            </label>
            <label>
              Alt-текст
              <input
                maxLength={300}
                value={value.cover_alt || ""}
                onChange={(e) => set("cover_alt", e.target.value)}
              />
            </label>
          </div>
          <label>
            SEO title
            <input
              maxLength={220}
              value={value.seo_title || ""}
              onChange={(e) => set("seo_title", e.target.value)}
            />
          </label>
          <label>
            Meta description
            <textarea
              maxLength={320}
              rows={2}
              value={value.seo_description || ""}
              onChange={(e) => set("seo_description", e.target.value)}
            />
          </label>
          <label>
            Внешний canonical (необязательно)
            <input
              type="url"
              placeholder="https://…"
              value={value.canonical_url || ""}
              onChange={(e) => set("canonical_url", e.target.value)}
            />
          </label>
        </section>
        {value.id ? (
          <div className="inline-actions">
            <AdminReasonAction
              label="В архив"
              tone="danger"
              title="Архивировать статью"
              onConfirm={(r) => destructive("archive", r)}
            />
            {["DRAFT", "ARCHIVED"].includes(value.status) ? (
              <AdminReasonAction
                label="Удалить"
                tone="danger"
                title="Удалить материал безвозвратно"
                onConfirm={(r) => destructive("delete", r)}
              />
            ) : null}
          </div>
        ) : null}
      </form>
    </section>
  );
}
function Taxonomy({
  title,
  kind,
  items,
  reload,
}: {
  title: string;
  kind: "categories" | "tags";
  items: Array<Category | Tag>;
  reload: () => void;
}) {
  const [name, setName] = useState(""),
    [slug, setSlug] = useState("");
  async function submit(e: FormEvent) {
    e.preventDefault();
    await adminRequest(`/api/v1/admin/blog/${kind}`, {
      method: "POST",
      body: JSON.stringify({
        item: {
          name,
          slug,
          ...(kind === "categories" ? { description: "" } : {}),
        },
        reason: `Создание: ${title}`,
      }),
    });
    setName("");
    setSlug("");
    reload();
  }
  return (
    <section className="admin-section">
      <h2>{title}</h2>
      <form className="taxonomy-quick" onSubmit={submit}>
        <input
          required
          placeholder="Название"
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
        <input
          required
          pattern="[a-z0-9]+(?:-[a-z0-9]+)*"
          placeholder="slug"
          value={slug}
          onChange={(e) =>
            setSlug(e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, ""))
          }
        />
        <button>Добавить</button>
      </form>
      <div className="chip-row">
        {items.map((x) => (
          <span className="chip" key={x.id}>
            {x.name}
          </span>
        ))}
      </div>
    </section>
  );
}
