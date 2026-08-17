"use client";
import { FormEvent, useCallback, useEffect, useState } from "react";
import {
  AdminError,
  AdminHeader,
  AdminTable,
  AdminTaxonomySkeleton,
  StatusPill,
  adminRequest,
} from "../admin-ui";
import { useToast } from "../../toast";
import { useAutoSlug } from "../../slug";

type Category = {
  id: string;
  parent_id?: string | null;
  slug: string;
  name: string;
  description?: string;
  sort_order: number;
  is_active: boolean;
};
type Skill = {
  id: string;
  slug: string;
  name: string;
  is_active: boolean;
};

export default function TaxonomyPage() {
  const { push } = useToast();
  const [categories, setCategories] = useState<Category[]>([]),
    [skills, setSkills] = useState<Skill[]>([]),
    [error, setError] = useState(""),
    [loading, setLoading] = useState(true);

  const [cat, setCat] = useState({
    slug: "",
    name: "",
    description: "",
    sort_order: 100,
    is_active: true,
  });
  const [skill, setSkill] = useState({ slug: "", name: "", is_active: true });

  const { handleSlugInput: handleCatSlugInput } = useAutoSlug({
    title: cat.name,
    onSlugChange: (slug) => setCat((prev) => ({ ...prev, slug })),
  });

  const { handleSlugInput: handleSkillSlugInput } = useAutoSlug({
    title: skill.name,
    onSlugChange: (slug) => setSkill((prev) => ({ ...prev, slug })),
  });

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const [c, s] = await Promise.all([
        adminRequest<{ data: Category[] }>("/api/v1/admin/categories"),
        adminRequest<{ data: Skill[] }>("/api/v1/admin/skills"),
      ]);
      setCategories(c.data ?? []);
      setSkills(s.data ?? []);
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "Не удалось загрузить таксономию",
      );
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  async function createCategory(e: FormEvent) {
    e.preventDefault();
    setError("");
    try {
      await adminRequest("/api/v1/admin/categories", {
        method: "POST",
        body: JSON.stringify({
          ...cat,
          name: cat.name.trim(),
          slug: cat.slug.trim(),
          parent_id: null,
        }),
      });
      setCat({
        slug: "",
        name: "",
        description: "",
        sort_order: 100,
        is_active: true,
      });
      push({
        kind: "success",
        title: "Категория создана",
        message: "Категория добавлена в каталог и записана в аудит.",
      });
      await load();
    } catch (x) {
      push({
        kind: "error",
        title: "Ошибка создания категории",
        message: x instanceof Error ? x.message : "Не удалось создать категорию",
      });
    }
  }

  async function toggleCategory(v: Category) {
    setError("");
    try {
      await adminRequest(`/api/v1/admin/categories/${v.id}`, {
        method: "PUT",
        body: JSON.stringify({
          parent_id: v.parent_id ?? null,
          slug: v.slug,
          name: v.name,
          description: v.description ?? "",
          sort_order: v.sort_order,
          is_active: !v.is_active,
        }),
      });
      push({
        kind: "success",
        title: v.is_active ? "Категория отключена" : "Категория включена",
      });
      await load();
    } catch (x) {
      push({
        kind: "error",
        title: "Ошибка изменения категории",
        message: x instanceof Error ? x.message : "Не удалось изменить категорию",
      });
    }
  }

  async function deleteCategory(v: Category) {
    setError("");
    if (
      !confirm(
        `Удалить категорию «${v.name}»? Удаление возможно только если категория нигде не используется.`,
      )
    )
      return;
    try {
      await adminRequest(`/api/v1/admin/categories/${v.id}`, {
        method: "DELETE",
      });
      push({
        kind: "success",
        title: "Категория удалена",
      });
      await load();
    } catch (x) {
      push({
        kind: "error",
        title: "Категорию нельзя удалить",
        message:
          x instanceof Error
            ? x.message
            : "Категория используется в услугах или проектах.",
      });
    }
  }

  async function createSkill(e: FormEvent) {
    e.preventDefault();
    setError("");
    try {
      await adminRequest("/api/v1/admin/skills", {
        method: "POST",
        body: JSON.stringify({
          name: skill.name.trim(),
          slug: skill.slug.trim(),
          is_active: skill.is_active,
        }),
      });
      setSkill({ slug: "", name: "", is_active: true });
      push({
        kind: "success",
        title: "Навык создан",
        message: "Навык добавлен в поиск и записан в аудит.",
      });
      await load();
    } catch (x) {
      push({
        kind: "error",
        title: "Ошибка создания навыка",
        message: x instanceof Error ? x.message : "Не удалось создать навык",
      });
    }
  }

  async function deleteSkill(v: Skill) {
    setError("");
    if (
      !confirm(
        `Удалить навык «${v.name}»? Удаление возможно только если навык нигде не используется.`,
      )
    )
      return;
    try {
      await adminRequest(`/api/v1/admin/skills/${v.id}`, {
        method: "DELETE",
      });
      push({
        kind: "success",
        title: "Навык удалён",
      });
      await load();
    } catch (x) {
      push({
        kind: "error",
        title: "Навык нельзя удалить",
        message:
          x instanceof Error
            ? x.message
            : "Навык привязан к профилям специалистов или заказам.",
      });
    }
  }

  async function toggleSkill(v: Skill) {
    setError("");
    try {
      await adminRequest(`/api/v1/admin/skills/${v.id}`, {
        method: "PUT",
        body: JSON.stringify({
          slug: v.slug,
          name: v.name,
          is_active: !v.is_active,
        }),
      });
      push({
        kind: "success",
        title: v.is_active ? "Навык отключён" : "Навык включён",
      });
      await load();
    } catch (x) {
      push({
        kind: "error",
        title: "Ошибка изменения навыка",
        message: x instanceof Error ? x.message : "Не удалось изменить навык",
      });
    }
  }

  return (
    <div className="admin-taxonomy-page">
      <AdminHeader
        title="Категории и навыки"
        description="Управляйте канонической таксономией поиска. Изменения применяются через реальные catalog API и попадают в аудит."
      />
      {error ? <AdminError message={error} onRetry={load} /> : null}

      <div className="admin-quick-grid admin-taxonomy-create">
        <section className="admin-panel admin-taxonomy-card">
          <p className="eyebrow">Новая категория</p>
          <form
            className="admin-config-form admin-taxonomy-form"
            onSubmit={createCategory}
          >
            <label>
              Название
              <input
                required
                maxLength={160}
                placeholder="Например: Мобильная разработка"
                value={cat.name}
                onChange={(e) =>
                  setCat((prev) => ({ ...prev, name: e.target.value }))
                }
              />
            </label>
            <label>
              Slug
              <input
                required
                pattern="[a-z0-9-]+"
                data-pattern-message="Используйте только строчные латинские буквы, цифры и дефисы."
                maxLength={120}
                placeholder="avto-slug-ili-vvedite-vruchnuyu"
                value={cat.slug}
                onChange={(e) => handleCatSlugInput(e.target.value)}
              />
            </label>
            <div className="admin-taxonomy-form__details">
              <label>
                Описание
                <textarea
                  maxLength={2000}
                  placeholder="Описание для каталога"
                  value={cat.description}
                  onChange={(e) =>
                    setCat((prev) => ({
                      ...prev,
                      description: e.target.value,
                    }))
                  }
                />
              </label>
              <label>
                Порядок
                <input
                  type="number"
                  min={0}
                  max={10000}
                  value={cat.sort_order}
                  onChange={(e) =>
                    setCat((prev) => ({
                      ...prev,
                      sort_order: Number(e.target.value),
                    }))
                  }
                />
              </label>
            </div>
            <button>Создать категорию</button>
          </form>
        </section>

        <section className="admin-panel admin-taxonomy-card">
          <p className="eyebrow">Новый навык</p>
          <form
            className="admin-config-form admin-taxonomy-form"
            onSubmit={createSkill}
          >
            <label>
              Название
              <input
                required
                maxLength={160}
                placeholder="Например: Golang"
                value={skill.name}
                onChange={(e) =>
                  setSkill((prev) => ({ ...prev, name: e.target.value }))
                }
              />
            </label>
            <label>
              Slug
              <input
                required
                pattern="[a-z0-9-]+"
                data-pattern-message="Используйте только строчные латинские буквы, цифры и дефисы."
                maxLength={120}
                placeholder="avto-slug-ili-vvedite-vruchnuyu"
                value={skill.slug}
                onChange={(e) => handleSkillSlugInput(e.target.value)}
              />
            </label>
            <div className="admin-taxonomy-form__details admin-taxonomy-form__details--skill">
              <strong>Где используется навык</strong>
              <p>
                После создания он станет доступен в профилях, проектах и
                фильтрах каталога. Ниже его можно отключить или удалить, если он
                больше не нужен.
              </p>
            </div>
            <button>Создать навык</button>
          </form>
        </section>
      </div>

      {loading && !categories.length && !skills.length ? (
        <AdminTaxonomySkeleton />
      ) : (
        <>
          <section className="admin-taxonomy-section" style={{ marginBottom: 32 }}>
            <h2>Категории ({categories.length})</h2>
            <AdminTable
              columns={["Название", "Slug", "Порядок", "Статус", "Действие"]}
              empty={!categories.length}
              loading={loading && !categories.length}
            >
              {categories.map((v) => (
                <tr key={v.id}>
                  <td>
                    <strong>{v.name}</strong>
                    <small>{v.description || "Без описания"}</small>
                  </td>
                  <td><code>{v.slug}</code></td>
                  <td>{v.sort_order}</td>
                  <td>
                    <StatusPill value={v.is_active ? "ACTIVE" : "DISABLED"} />
                  </td>
                  <td>
                    <div className="admin-row-actions">
                      <button
                        className="button button--quiet button--compact"
                        type="button"
                        onClick={() => toggleCategory(v)}
                      >
                        {v.is_active ? "Отключить" : "Включить"}
                      </button>
                      <button
                        className="button button--danger button--compact"
                        type="button"
                        onClick={() => deleteCategory(v)}
                      >
                        Удалить
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </AdminTable>
          </section>

          <section className="admin-taxonomy-section">
            <h2>Навыки ({skills.length})</h2>
            <AdminTable
              columns={["Навык", "Slug", "Статус", "Действие"]}
              empty={!skills.length}
              loading={loading && !skills.length}
            >
              {skills.map((v) => (
                <tr key={v.id}>
                  <td>
                    <strong>{v.name}</strong>
                  </td>
                  <td><code>{v.slug}</code></td>
                  <td>
                    <StatusPill value={v.is_active ? "ACTIVE" : "DISABLED"} />
                  </td>
                  <td>
                    <div className="admin-row-actions">
                      <button
                        className="button button--quiet button--compact"
                        type="button"
                        onClick={() => toggleSkill(v)}
                      >
                        {v.is_active ? "Отключить" : "Включить"}
                      </button>
                      <button
                        className="button button--danger button--compact"
                        type="button"
                        onClick={() => deleteSkill(v)}
                      >
                        Удалить
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </AdminTable>
          </section>
        </>
      )}
    </div>
  );
}
