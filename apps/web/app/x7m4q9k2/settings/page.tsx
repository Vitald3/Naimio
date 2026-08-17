"use client";

import { CSSProperties, FormEvent, useCallback, useEffect, useState } from "react";
import {
  AdminError,
  AdminHeader,
  AdminLoading,
  adminRequest,
} from "../admin-ui";
import {
  defaultSiteSettings,
  defaultSEOSettings,
  SiteSettings,
  SEOSettings,
  SEOPageOverride,
  SEOTemplateOverride,
} from "../../site-settings";
import { useToast } from "../../toast";
import { CustomSelect } from "../../custom-select";

type TabKey = "appearance" | "seo_general" | "seo_pages" | "seo_templates" | "indexnow" | "storage" | "system";

type Flag = { key: string; enabled: boolean; config: Record<string, unknown>; description?: string };
type LegalPost = { id: string; title: string; slug: string; status: string };
type ColorKey =
  | "primary_button_color"
  | "button_hover_color"
  | "green_heading_color"
  | "bright_blue_color"
  | "heading_color"
  | "body_text_color"
  | "page_background_color";

type StorageSettings = {
  provider: "local" | "s3";
  s3: {
    endpoint: string;
    region: string;
    bucket: string;
    access_key: string;
    secret_key_configured: boolean;
    secret_key_masked: string;
    use_ssl: boolean;
    path_style: boolean;
    public_url: string;
  };
};

const defaultStorageSettings: StorageSettings = {
  provider: "local",
  s3: {
    endpoint: "",
    region: "ru-central1",
    bucket: "",
    access_key: "",
    secret_key_configured: false,
    secret_key_masked: "",
    use_ssl: true,
    path_style: true,
    public_url: "",
  },
};

const colors: Array<[ColorKey, string]> = [
  ["primary_button_color", "Основные кнопки"],
  ["button_hover_color", "Кнопки при наведении"],
  ["green_heading_color", "Зелёные заголовки и акценты"],
  ["bright_blue_color", "Ярко-синий акцент"],
  ["heading_color", "Основные заголовки"],
  ["body_text_color", "Основной текст"],
  ["page_background_color", "Фон страниц"],
];

const publicPagesList = [
  { path: "/", name: "Главная страница" },
  { path: "/categories", name: "Каталог категорий" },
  { path: "/freelancers", name: "Каталог специалистов" },
  { path: "/services", name: "Каталог услуг" },
  { path: "/projects", name: "Каталог проектов" },
  { path: "/vacancies", name: "Каталог вакансий" },
  { path: "/education", name: "Обучение и менторинг" },
  { path: "/check-offer", name: "Проверка КП" },
  { path: "/price", name: "Калькуляторы цен" },
  { path: "/blog", name: "Блог платформы" },
  { path: "/pro", name: "PRO-подписка" },
];

const templateTypesList = [
  { key: "category", name: "Страница категории", vars: ["{category}", "{project_name}"] },
  { key: "freelancer", name: "Профиль специалиста", vars: ["{name}", "{specialty}", "{rating}", "{project_name}"] },
  { key: "service", name: "Карточка услуги", vars: ["{service_title}", "{name}", "{price}", "{duration}", "{project_name}"] },
  { key: "project", name: "Карточка проекта", vars: ["{project_title}", "{budget}", "{project_name}"] },
  { key: "vacancy", name: "Карточка вакансии", vars: ["{job_title}", "{location}", "{project_name}"] },
  { key: "calculator", name: "Страница калькулятора", vars: ["{calculator_title}", "{project_name}"] },
  { key: "blog", name: "Статья блога", vars: ["{post_title}", "{excerpt}", "{project_name}"] },
];

export default function ProjectSettingsPage() {
  const { push } = useToast();
  const [activeTab, setActiveTab] = useState<TabKey>("appearance");

  const [settings, setSettings] = useState(defaultSiteSettings);
  const [seoSettings, setSeoSettings] = useState<SEOSettings>(defaultSEOSettings);
  const [storageSettings, setStorageSettings] = useState<StorageSettings>(defaultStorageSettings);
  const [featureFlags, setFeatureFlags] = useState<Flag[]>([]);

  const [s3Secret, setS3Secret] = useState("");
  const [testingS3, setTestingS3] = useState(false);
  const [testResult, setTestResult] = useState<{ success: boolean; message: string } | null>(null);

  // SEO Page editor state
  const [selectedPagePath, setSelectedPagePath] = useState("/");

  // IndexNow batch submitter state
  const [indexNowUrls, setIndexNowUrls] = useState("");
  const [submittingIndexNow, setSubmittingIndexNow] = useState(false);
  const [indexNowResult, setIndexNowResult] = useState<{ success: boolean; message: string } | null>(null);

  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [legalPosts, setLegalPosts] = useState<LegalPost[]>([]);
  const [error, setError] = useState("");

  const load = useCallback(() => {
    setLoading(true);
    Promise.all([
      adminRequest<{ data: Flag[] }>("/api/v1/admin/feature-flags"),
      adminRequest<{ data: { posts: { items: LegalPost[] } } }>("/api/v1/admin/blog").catch(() => ({ data: { posts: { items: [] } } })),
      adminRequest<{ data: StorageSettings }>("/api/v1/admin/storage-settings").catch(() => null),
    ])
      .then(([body, blog, storage]) => {
        setFeatureFlags(body.data ?? []);
        setLegalPosts((blog.data.posts.items ?? []).filter((post) => post.status === "PUBLISHED"));

        const appearanceFlag = body.data.find((item) => item.key === "site_appearance");
        if (appearanceFlag?.config) {
          setSettings((curr) => ({ ...curr, ...appearanceFlag.config }));
        }

        const seoFlag = body.data.find((item) => item.key === "seo_settings");
        if (seoFlag?.config) {
          const cfg = seoFlag.config as Partial<SEOSettings>;
          setSeoSettings({
            general: { ...defaultSEOSettings.general, ...(cfg.general || {}) },
            pages: { ...defaultSEOSettings.pages, ...(cfg.pages || {}) },
            templates: { ...defaultSEOSettings.templates, ...(cfg.templates || {}) },
            indexnow: { ...defaultSEOSettings.indexnow, ...(cfg.indexnow || {}) },
          });
        }

        if (storage?.data) {
          setStorageSettings(storage.data);
        }
        setError("");
      })
      .catch((reason) =>
        setError(
          reason instanceof Error
            ? reason.message
            : "Не удалось загрузить настройки",
        ),
      )
      .finally(() => setLoading(false));
  }, []);

  useEffect(load, [load]);

  const set = (key: keyof SiteSettings, value: string) =>
    setSettings((current) => ({ ...current, [key]: value }));
  const setNumber = (key: keyof SiteSettings, value: number) =>
    setSettings((current) => ({ ...current, [key]: value }));
  const setBoolean = (key: keyof SiteSettings, value: boolean) =>
    setSettings((current) => ({ ...current, [key]: value }));

  const setSEOGeneral = <K extends keyof SEOSettings["general"]>(key: K, value: SEOSettings["general"][K]) => {
    setSeoSettings((curr) => ({
      ...curr,
      general: { ...curr.general, [key]: value },
    }));
  };

  const setSEOPage = (path: string, field: keyof SEOPageOverride, value: string | boolean) => {
    setSeoSettings((curr) => {
      const existing = curr.pages[path] || {
        title: "",
        description: "",
        no_index: false,
      };
      return {
        ...curr,
        pages: {
          ...curr.pages,
          [path]: { ...existing, [field]: value },
        },
      };
    });
  };

  const setSEOTemplate = (typeKey: string, field: keyof SEOTemplateOverride, value: string) => {
    setSeoSettings((curr) => {
      const existing = curr.templates[typeKey] || {
        title_template: "",
        description_template: "",
      };
      return {
        ...curr,
        templates: {
          ...curr.templates,
          [typeKey]: { ...existing, [field]: value },
        },
      };
    });
  };

  const setIndexNow = <K extends keyof SEOSettings["indexnow"]>(key: K, value: SEOSettings["indexnow"][K]) => {
    setSeoSettings((curr) => ({
      ...curr,
      indexnow: { ...curr.indexnow, [key]: value },
    }));
  };

  const setS3Field = <K extends keyof StorageSettings["s3"]>(key: K, value: StorageSettings["s3"][K]) => {
    setStorageSettings((curr) => ({
      ...curr,
      s3: { ...curr.s3, [key]: value },
    }));
    setTestResult(null);
  };

  async function testS3Connection() {
    setTestingS3(true);
    setTestResult(null);
    try {
      const payload = {
        endpoint: storageSettings.s3.endpoint,
        region: storageSettings.s3.region || "ru-central1",
        bucket: storageSettings.s3.bucket,
        access_key: storageSettings.s3.access_key,
        secret_key: s3Secret || (storageSettings.s3.secret_key_configured ? "********" : ""),
        use_ssl: storageSettings.s3.use_ssl,
        path_style: storageSettings.s3.path_style,
        public_url: storageSettings.s3.public_url,
      };

      if (!payload.endpoint || !payload.bucket || !payload.access_key || (!payload.secret_key && !storageSettings.s3.secret_key_configured)) {
        setTestResult({
          success: false,
          message: "Заполните Endpoint, Бакет, Access Key и Secret Key для проверки.",
        });
        return;
      }

      await adminRequest("/api/v1/admin/storage-settings/test", {
        method: "POST",
        body: JSON.stringify(payload),
      });

      setTestResult({
        success: true,
        message: "Подключение к S3 успешно проверено (запись, чтение и удаление работают).",
      });
    } catch (reason) {
      setTestResult({
        success: false,
        message: reason instanceof Error ? reason.message : "Не удалось подключиться к S3",
      });
    } finally {
      setTestingS3(false);
    }
  }

  async function submitIndexNowBatch() {
    const urls = indexNowUrls
      .split("\n")
      .map((u) => u.trim())
      .filter(Boolean);

    if (urls.length === 0) {
      setIndexNowResult({
        success: false,
        message: "Введите хотя бы один URL адрес для отправки.",
      });
      return;
    }

    setSubmittingIndexNow(true);
    setIndexNowResult(null);
    try {
      const res = await adminRequest<{ data: { success: boolean; submitted_count: number; message: string } }>("/api/v1/admin/indexnow/submit", {
        method: "POST",
        body: JSON.stringify({
          urls,
          key: seoSettings.indexnow.api_key,
          key_location: seoSettings.indexnow.key_location,
          host: seoSettings.indexnow.host || "naimio.ru",
        }),
      });

      setIndexNowResult({
        success: true,
        message: res.data?.message || `Успешно отправлено ${urls.length} URL в IndexNow`,
      });
      push({
        kind: "success",
        title: "IndexNow",
        message: `Отправлено ${urls.length} URL в Яндекс и Bing`,
      });
    } catch (reason) {
      const msg = reason instanceof Error ? reason.message : "Ошибка при отправке в IndexNow";
      setIndexNowResult({ success: false, message: msg });
      push({ kind: "error", title: "Ошибка IndexNow", message: msg });
    } finally {
      setSubmittingIndexNow(false);
    }
  }

  async function save(event: FormEvent) {
    event.preventDefault();
    setSaving(true);
    try {
      // 1. Save site appearance
      await adminRequest("/api/v1/admin/feature-flags/site_appearance", {
        method: "PATCH",
        body: JSON.stringify({
          enabled: true,
          config: settings,
          reason: "Обновление настроек внешнего вида проекта",
        }),
      });

      // 2. Save SEO settings
      await adminRequest("/api/v1/admin/feature-flags/seo_settings", {
        method: "PATCH",
        body: JSON.stringify({
          enabled: true,
          config: seoSettings,
          reason: "Обновление настроек SEO и IndexNow",
        }),
      });

      // 3. Save storage settings
      const storagePayload = {
        provider: storageSettings.provider,
        s3: {
          endpoint: storageSettings.s3.endpoint,
          region: storageSettings.s3.region,
          bucket: storageSettings.s3.bucket,
          access_key: storageSettings.s3.access_key,
          secret_key: s3Secret ? s3Secret : (storageSettings.s3.secret_key_configured ? "********" : ""),
          use_ssl: storageSettings.s3.use_ssl,
          path_style: storageSettings.s3.path_style,
          public_url: storageSettings.s3.public_url,
        },
      };

      const updatedStorage = await adminRequest<{ data: StorageSettings }>("/api/v1/admin/storage-settings", {
        method: "PUT",
        body: JSON.stringify(storagePayload),
      });

      if (updatedStorage?.data) {
        setStorageSettings(updatedStorage.data);
        setS3Secret("");
      }

      push({
        kind: "success",
        title: "Настройки проекта сохранены",
        message: "Все разделы (Внешний вид, SEO, Хранилище, Каталоги) успешно обновлены.",
      });
    } catch (reason) {
      push({
        kind: "error",
        title: "Не удалось сохранить настройки",
        message: reason instanceof Error ? reason.message : "Проверьте корректность полей.",
      });
    } finally {
      setSaving(false);
    }
  }

  const currentPageOverride = seoSettings.pages[selectedPagePath] || {
    title: "",
    description: "",
    canonical_url: "",
    og_image: "",
    no_index: false,
  };

  return (
    <>
      <AdminHeader
        title="Настройки платформы"
        description="Внешний вид, единая цветовая тема, глобальное SEO, шаблоны метатегов, IndexNow и хранилище файлов."
      />

      <div className="admin-settings-tabs" role="tablist">
        <button
          type="button"
          role="tab"
          aria-selected={activeTab === "appearance"}
          className={`admin-settings-tab ${activeTab === "appearance" ? "is-active" : ""}`}
          onClick={() => setActiveTab("appearance")}
        >
          🎨 Внешний вид & Проект
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={activeTab === "seo_general"}
          className={`admin-settings-tab ${activeTab === "seo_general" ? "is-active" : ""}`}
          onClick={() => setActiveTab("seo_general")}
        >
          🌐 SEO Общие
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={activeTab === "seo_pages"}
          className={`admin-settings-tab ${activeTab === "seo_pages" ? "is-active" : ""}`}
          onClick={() => setActiveTab("seo_pages")}
        >
          📄 SEO Страницы
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={activeTab === "seo_templates"}
          className={`admin-settings-tab ${activeTab === "seo_templates" ? "is-active" : ""}`}
          onClick={() => setActiveTab("seo_templates")}
        >
          🏷️ SEO Шаблоны
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={activeTab === "indexnow"}
          className={`admin-settings-tab ${activeTab === "indexnow" ? "is-active" : ""}`}
          onClick={() => setActiveTab("indexnow")}
        >
          ⚡ IndexNow & Индексация
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={activeTab === "storage"}
          className={`admin-settings-tab ${activeTab === "storage" ? "is-active" : ""}`}
          onClick={() => setActiveTab("storage")}
        >
          💾 Хранилище файлов
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={activeTab === "system"}
          className={`admin-settings-tab ${activeTab === "system" ? "is-active" : ""}`}
          onClick={() => setActiveTab("system")}
        >
          ⚙️ Система & Каталоги
        </button>
      </div>

      {loading ? (
        <AdminLoading />
      ) : error ? (
        <AdminError message={error} onRetry={load} />
      ) : (
        <div className="admin-settings-grid">
          <form className="admin-settings-form" onSubmit={save}>
            {/* 1. APPEARANCE & PROJECT */}
            {activeTab === "appearance" && (
              <>
                <section className="admin-settings-section">
                  <h2>Проект и поддержка</h2>
                  <label>
                    Название проекта
                    <input
                      required
                      minLength={2}
                      maxLength={80}
                      value={settings.project_name}
                      onChange={(e) => set("project_name", e.target.value)}
                    />
                  </label>
                  <label>
                    Краткое описание
                    <textarea
                      maxLength={240}
                      rows={3}
                      value={settings.project_description}
                      onChange={(e) => set("project_description", e.target.value)}
                    />
                  </label>
                  <div className="field-row">
                    <label>
                      Email поддержки
                      <input
                        type="email"
                        maxLength={200}
                        value={settings.support_email}
                        onChange={(e) => set("support_email", e.target.value)}
                      />
                    </label>
                    <label>
                      Телефон поддержки
                      <input
                        maxLength={50}
                        value={settings.support_phone}
                        onChange={(e) => set("support_phone", e.target.value)}
                      />
                    </label>
                  </div>
                  <label>
                    Юридическое название
                    <input
                      maxLength={180}
                      value={settings.legal_company_name}
                      onChange={(e) => set("legal_company_name", e.target.value)}
                    />
                  </label>
                  <label>
                    Подпись в подвале (Copyright)
                    <input
                      maxLength={180}
                      value={settings.footer_copyright}
                      onChange={(e) => set("footer_copyright", e.target.value)}
                    />
                  </label>
                  <div className="field-row">
                    <label>
                      Политика конфиденциальности
                      <CustomSelect
                        value={settings.privacy_policy_slug}
                        onChange={(e) => set("privacy_policy_slug", e.target.value)}
                      >
                        <option value="">Не выбрана</option>
                        {legalPosts.map((post) => (
                          <option key={post.id} value={post.slug}>
                            {post.title}
                          </option>
                        ))}
                      </CustomSelect>
                      <small className="form-hint">Опубликованная статья из CMS блога.</small>
                    </label>
                    <label>
                      Условия соглашения
                      <CustomSelect
                        value={settings.terms_slug}
                        onChange={(e) => set("terms_slug", e.target.value)}
                      >
                        <option value="">Не выбраны</option>
                        {legalPosts.map((post) => (
                          <option key={post.id} value={post.slug}>
                            {post.title}
                          </option>
                        ))}
                      </CustomSelect>
                      <small className="form-hint">Опубликованная статья из CMS блога.</small>
                    </label>
                  </div>
                </section>

                <section className="admin-settings-section">
                  <h2>Цвета интерфейса</h2>
                  <p className="form-hint" style={{ marginTop: "-6px", marginBottom: "14px" }}>
                    Единая палитра для кнопок, заголовков и акцентов всей платформы.
                  </p>
                  <div className="admin-color-grid">
                    {colors.map(([key, label]) => (
                      <label className="admin-color-field" key={key}>
                        <span>{label}</span>
                        <input
                          required
                          pattern="#[0-9a-fA-F]{6}"
                          value={settings[key]}
                          onChange={(e) => set(key, e.target.value)}
                        />
                        <input
                          type="color"
                          aria-label={`${label}: выбор цвета`}
                          value={settings[key]}
                          onChange={(e) => set(key, e.target.value)}
                        />
                      </label>
                    ))}
                  </div>
                </section>
              </>
            )}

            {/* 2. SEO GENERAL */}
            {activeTab === "seo_general" && (
              <section className="admin-settings-section">
                <h2>Глобальные настройки SEO</h2>
                <p className="form-hint" style={{ marginTop: "-6px", marginBottom: "16px" }}>
                  Базовые метатеги, формат заголовков, поисковые роботы и микроразметка Schema.org.
                </p>

                <div className="field-row">
                  <label>
                    Шаблон заголовка сайта (Title Template)
                    <input
                      placeholder="%s — Naimio"
                      value={seoSettings.general.title_template}
                      onChange={(e) => setSEOGeneral("title_template", e.target.value)}
                    />
                    <small className="form-hint">Используйте %s для названия текущей страницы.</small>
                  </label>
                  <label>
                    Канонический домен (Canonical Base URL)
                    <input
                      placeholder="https://naimio.ru"
                      value={seoSettings.general.canonical_base_url}
                      onChange={(e) => setSEOGeneral("canonical_base_url", e.target.value)}
                    />
                    <small className="form-hint">Основной домен для canonical ссылок и sitemap.</small>
                  </label>
                </div>

                <label>
                  Главный заголовок сайта по умолчанию (Default Title)
                  <input
                    maxLength={200}
                    value={seoSettings.general.default_title}
                    onChange={(e) => setSEOGeneral("default_title", e.target.value)}
                  />
                </label>

                <label>
                  Главное описание сайта (Default Description)
                  <textarea
                    rows={3}
                    maxLength={320}
                    value={seoSettings.general.default_description}
                    onChange={(e) => setSEOGeneral("default_description", e.target.value)}
                  />
                  <small className="form-hint">Рекомендуемая длина: 140–160 символов.</small>
                </label>

                <div className="field-row">
                  <label>
                    Изображение для соцсетей по умолчанию (OG / Twitter Image)
                    <input
                      placeholder="/media/covers/cover-01.svg"
                      value={seoSettings.general.default_og_image}
                      onChange={(e) => setSEOGeneral("default_og_image", e.target.value)}
                    />
                  </label>
                  <label>
                    Политика индексации роботов (Robots Policy)
                    <CustomSelect
                      value={seoSettings.general.robots_policy}
                      onChange={(e) => setSEOGeneral("robots_policy", e.target.value)}
                    >
                      <option value="INDEX_FOLLOW">index, follow (Разрешить полную индексацию)</option>
                      <option value="NOINDEX_FOLLOW">noindex, follow (Не индексировать, переходить по ссылкам)</option>
                      <option value="NOINDEX_NOFOLLOW">noindex, nofollow (Закрыть от всех поисковиков)</option>
                    </CustomSelect>
                  </label>
                </div>

                <label>
                  Дополнительные правила Robots.txt
                  <textarea
                    rows={3}
                    placeholder="Disallow: /temp/&#10;Clean-param: ref"
                    value={seoSettings.general.custom_robots_txt}
                    onChange={(e) => setSEOGeneral("custom_robots_txt", e.target.value)}
                  />
                  <small className="form-hint">Дополнительные директивы к стандартному файлу robots.txt.</small>
                </label>

                <h3 style={{ fontSize: "16px", marginTop: "20px", marginBottom: "10px" }}>
                  Микроразметка Schema.org Organization
                </h3>
                <div className="field-row">
                  <label>
                    Название организации
                    <input
                      value={seoSettings.general.schema_organization_name}
                      onChange={(e) => setSEOGeneral("schema_organization_name", e.target.value)}
                    />
                  </label>
                  <label>
                    Юридическое наименование
                    <input
                      value={seoSettings.general.schema_legal_name}
                      onChange={(e) => setSEOGeneral("schema_legal_name", e.target.value)}
                    />
                  </label>
                </div>

                <div className="admin-snippet-card">
                  <div className="admin-snippet-card__engine">
                    <span>🔍 Предпросмотр сниппета в поисковиках (Google / Яндекс)</span>
                  </div>
                  <div className="admin-snippet-card__url">
                    {seoSettings.general.canonical_base_url || "https://naimio.ru"}
                  </div>
                  <div className="admin-snippet-card__title">
                    {seoSettings.general.default_title || "Naimio"}
                  </div>
                  <div className="admin-snippet-card__desc">
                    {seoSettings.general.default_description || "Описание сайта"}
                  </div>
                </div>
              </section>
            )}

            {/* 3. SEO PAGES */}
            {activeTab === "seo_pages" && (
              <section className="admin-settings-section">
                <h2>SEO Настройки отдельных страниц</h2>
                <p className="form-hint" style={{ marginTop: "-6px", marginBottom: "16px" }}>
                  Настройте индивидуальные Title, Description и параметры индексации для ключевых публичных разделов.
                </p>

                <div className="admin-page-list">
                  {publicPagesList.map((p) => {
                    const isSelected = selectedPagePath === p.path;
                    const override = seoSettings.pages[p.path];
                    const hasCustom = !!(override?.title || override?.description);
                    return (
                      <button
                        key={p.path}
                        type="button"
                        className={`admin-page-item ${isSelected ? "is-selected" : ""}`}
                        onClick={() => setSelectedPagePath(p.path)}
                      >
                        <strong>{p.name}</strong>
                        <small>{p.path}</small>
                        {hasCustom && (
                          <span style={{ fontSize: "10px", color: "#16a34a", fontWeight: 700, marginTop: "2px" }}>
                            ● Настроена
                          </span>
                        )}
                      </button>
                    );
                  })}
                </div>

                <div
                  style={{
                    padding: "18px",
                    borderRadius: "14px",
                    background: "#f8fafc",
                    border: "1px solid var(--line)",
                  }}
                >
                  <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: "14px" }}>
                    <h3 style={{ margin: 0, fontSize: "16px" }}>
                      Страница: <span style={{ color: "var(--accent, #15956a)" }}>{selectedPagePath}</span>
                    </h3>
                    <label className="calculator-switch" style={{ margin: 0 }}>
                      <input
                        type="checkbox"
                        checked={currentPageOverride.no_index}
                        onChange={(e) => setSEOPage(selectedPagePath, "no_index", e.target.checked)}
                      />
                      <span>Закрыть от индексации (noindex)</span>
                    </label>
                  </div>

                  <label>
                    Meta Title
                    <input
                      placeholder="Заголовок для страницы..."
                      value={currentPageOverride.title}
                      onChange={(e) => setSEOPage(selectedPagePath, "title", e.target.value)}
                    />
                  </label>

                  <label>
                    Meta Description
                    <textarea
                      rows={3}
                      placeholder="Описание страницы для поисковиков..."
                      value={currentPageOverride.description}
                      onChange={(e) => setSEOPage(selectedPagePath, "description", e.target.value)}
                    />
                  </label>

                  <div className="field-row">
                    <label>
                      Канонический URL (Canonical URL, опционально)
                      <input
                        placeholder={`${seoSettings.general.canonical_base_url || "https://naimio.ru"}${selectedPagePath}`}
                        value={currentPageOverride.canonical_url || ""}
                        onChange={(e) => setSEOPage(selectedPagePath, "canonical_url", e.target.value)}
                      />
                    </label>
                    <label>
                      OG Image (опционально)
                      <input
                        placeholder="/media/covers/cover-02.svg"
                        value={currentPageOverride.og_image || ""}
                        onChange={(e) => setSEOPage(selectedPagePath, "og_image", e.target.value)}
                      />
                    </label>
                  </div>

                  <div className="admin-snippet-card" style={{ marginTop: "16px" }}>
                    <div className="admin-snippet-card__engine">
                      <span>Сниппет в выдаче</span>
                    </div>
                    <div className="admin-snippet-card__url">
                      {seoSettings.general.canonical_base_url || "https://naimio.ru"}{selectedPagePath}
                    </div>
                    <div className="admin-snippet-card__title">
                      {currentPageOverride.title || `${publicPagesList.find((p) => p.path === selectedPagePath)?.name} — Naimio`}
                    </div>
                    <div className="admin-snippet-card__desc">
                      {currentPageOverride.description || seoSettings.general.default_description}
                    </div>
                  </div>
                </div>
              </section>
            )}

            {/* 4. SEO TEMPLATES */}
            {activeTab === "seo_templates" && (
              <section className="admin-settings-section">
                <h2>Динамические SEO Шаблоны</h2>
                <p className="form-hint" style={{ marginTop: "-6px", marginBottom: "16px" }}>
                  Шаблоны для карточек фрилансеров, услуг, проектов, вакансий, категорий и статей блога.
                </p>

                <div style={{ display: "flex", flexDirection: "column", gap: "18px" }}>
                  {templateTypesList.map((tmpl) => {
                    const currentTmpl = seoSettings.templates[tmpl.key] || {
                      title_template: "",
                      description_template: "",
                    };
                    return (
                      <div
                        key={tmpl.key}
                        style={{
                          padding: "16px",
                          borderRadius: "14px",
                          background: "#f8fafc",
                          border: "1px solid var(--line)",
                        }}
                      >
                        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: "10px" }}>
                          <strong style={{ fontSize: "15px" }}>{tmpl.name}</strong>
                          <span style={{ fontSize: "12px", color: "#64748b", fontFamily: "monospace" }}>
                            тип: {tmpl.key}
                          </span>
                        </div>

                        <div className="admin-chip-group">
                          <span style={{ fontSize: "12px", color: "#64748b", alignSelf: "center", marginRight: "4px" }}>
                            Переменные:
                          </span>
                          {tmpl.vars.map((v) => (
                            <button
                              key={v}
                              type="button"
                              className="admin-chip"
                              onClick={() => {
                                const newTitle = currentTmpl.title_template ? `${currentTmpl.title_template} ${v}` : v;
                                setSEOTemplate(tmpl.key, "title_template", newTitle);
                              }}
                            >
                              <code>{v}</code>
                            </button>
                          ))}
                        </div>

                        <label>
                          Шаблон Title
                          <input
                            value={currentTmpl.title_template}
                            onChange={(e) => setSEOTemplate(tmpl.key, "title_template", e.target.value)}
                          />
                        </label>

                        <label>
                          Шаблон Description
                          <textarea
                            rows={2}
                            value={currentTmpl.description_template}
                            onChange={(e) => setSEOTemplate(tmpl.key, "description_template", e.target.value)}
                          />
                        </label>
                      </div>
                    );
                  })}
                </div>
              </section>
            )}

            {/* 5. INDEXNOW & SUBMITTER */}
            {activeTab === "indexnow" && (
              <section className="admin-settings-section">
                <h2>IndexNow и быстрая индексация</h2>
                <p className="form-hint" style={{ marginTop: "-6px", marginBottom: "16px" }}>
                  Протокол IndexNow моментально оповещает поисковые системы (Яндекс, Bing, Seznam, Naver) о добавлении, изменении или удалении страниц.
                </p>

                <div style={{ display: "flex", flexDirection: "column", gap: "16px", marginBottom: "20px" }}>
                  <label className="calculator-switch">
                    <input
                      type="checkbox"
                      checked={seoSettings.indexnow.enabled}
                      onChange={(e) => setIndexNow("enabled", e.target.checked)}
                    />
                    <span>Включить IndexNow для платформы</span>
                  </label>

                  <label className="calculator-switch">
                    <input
                      type="checkbox"
                      checked={seoSettings.indexnow.auto_submit}
                      onChange={(e) => setIndexNow("auto_submit", e.target.checked)}
                    />
                    <span>Автоматически отправлять URL при публикации новых проектов, услуг, вакансий и статей</span>
                  </label>

                  <div className="field-row">
                    <label>
                      API Ключ IndexNow (Key)
                      <input
                        value={seoSettings.indexnow.api_key}
                        onChange={(e) => setIndexNow("api_key", e.target.value)}
                      />
                      <small className="form-hint">Уникальный 32-байтный hex/string ключ.</small>
                    </label>
                    <label>
                      Хост (Host)
                      <input
                        placeholder="naimio.ru"
                        value={seoSettings.indexnow.host}
                        onChange={(e) => setIndexNow("host", e.target.value)}
                      />
                    </label>
                  </div>

                  <label>
                    URL файла подтверждения ключа (Key Location)
                    <input
                      value={seoSettings.indexnow.key_location}
                      onChange={(e) => setIndexNow("key_location", e.target.value)}
                    />
                    <small className="form-hint">
                      Поисковый робот проверит наличие ключа по адресу: {seoSettings.indexnow.key_location || `https://${seoSettings.indexnow.host || "naimio.ru"}/${seoSettings.indexnow.api_key || "key"}.txt`}
                    </small>
                  </label>
                </div>

                <div
                  style={{
                    padding: "18px",
                    borderRadius: "14px",
                    background: "#f0fdf4",
                    border: "1px solid #bbf7d0",
                  }}
                >
                  <h3 style={{ margin: "0 0 8px 0", fontSize: "16px", color: "#166534" }}>
                    🚀 Ручная отправка URL в поисковики прямо сейчас
                  </h3>
                  <p style={{ fontSize: "13px", color: "#15803d", margin: "0 0 12px 0" }}>
                    Вставьте один или несколько URL (по одному на строку), чтобы мгновенно передать их роботам Яндекса и Bing.
                  </p>

                  <div style={{ display: "flex", gap: "8px", marginBottom: "8px", flexWrap: "wrap" }}>
                    <button
                      type="button"
                      style={{ background: "#fff", color: "#166534", border: "1px solid #86efac", padding: "4px 10px", fontSize: "12px", width: "auto" }}
                      onClick={() => {
                        const base = seoSettings.general.canonical_base_url || "https://naimio.ru";
                        const urls = publicPagesList.map((p) => `${base}${p.path}`).join("\n");
                        setIndexNowUrls(urls);
                      }}
                    >
                      + Заполнить всеми основными страницами
                    </button>
                    <button
                      type="button"
                      style={{ background: "#fff", color: "#64748b", border: "1px solid #cbd5e1", padding: "4px 10px", fontSize: "12px", width: "auto" }}
                      onClick={() => setIndexNowUrls("")}
                    >
                      Очистить
                    </button>
                  </div>

                  <textarea
                    rows={4}
                    placeholder="https://naimio.ru/projects&#10;https://naimio.ru/services&#10;https://naimio.ru/freelancers"
                    value={indexNowUrls}
                    onChange={(e) => setIndexNowUrls(e.target.value)}
                    style={{ background: "#fff", borderColor: "#86efac", fontFamily: "monospace", fontSize: "13px" }}
                  />

                  <div style={{ display: "flex", alignItems: "center", gap: "12px", marginTop: "10px", flexWrap: "wrap" }}>
                    <button
                      type="button"
                      disabled={submittingIndexNow}
                      onClick={submitIndexNowBatch}
                      style={{
                        background: "#16a34a",
                        borderColor: "#16a34a",
                        color: "#fff",
                        width: "auto",
                        padding: "8px 18px",
                        fontSize: "14px",
                        fontWeight: 600,
                      }}
                    >
                      {submittingIndexNow ? "Отправляем в Яндекс и Bing…" : "Отправить в IndexNow"}
                    </button>

                    {indexNowResult && (
                      <span
                        style={{
                          padding: "6px 12px",
                          borderRadius: "8px",
                          fontSize: "13px",
                          fontWeight: 600,
                          background: indexNowResult.success ? "#dcfce7" : "#fee2e2",
                          color: indexNowResult.success ? "#166534" : "#991b1b",
                        }}
                      >
                        {indexNowResult.message}
                      </span>
                    )}
                  </div>
                </div>
              </section>
            )}

            {/* 6. STORAGE */}
            {activeTab === "storage" && (
              <section className="admin-settings-section">
                <h2>Хранение файлов</h2>
                <p className="form-hint" style={{ marginTop: "-4px", marginBottom: "14px" }}>
                  Выберите активный режим для всех новых загрузок (аватары, портфолио, вложения, блог). Ранее загруженные файлы продолжат открываться из своего исходного хранилища.
                </p>

                <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(220px, 1fr))", gap: "12px", marginBottom: "16px" }}>
                  <label
                    style={{
                      display: "flex",
                      flexDirection: "column",
                      gap: "6px",
                      padding: "14px",
                      borderRadius: "14px",
                      cursor: "pointer",
                      border: storageSettings.provider === "local" ? "2px solid var(--accent, #15956a)" : "1px solid var(--line, #e2e8f0)",
                      background: storageSettings.provider === "local" ? "#f0fdf4" : "#fff",
                      transition: "all 0.15s ease",
                    }}
                  >
                    <div style={{ display: "flex", alignItems: "center", gap: "8px" }}>
                      <input
                        type="radio"
                        name="storage_provider"
                        checked={storageSettings.provider === "local"}
                        onChange={() => {
                          setStorageSettings((curr) => ({ ...curr, provider: "local" }));
                          setTestResult(null);
                        }}
                      />
                      <strong style={{ fontSize: "15px" }}>Локальный сервер</strong>
                    </div>
                    <small style={{ color: "#64748b", lineHeight: "1.4" }}>
                      Файлы хранятся на локальном диске сервера (/var/lib/naimio-media)
                    </small>
                  </label>

                  <label
                    style={{
                      display: "flex",
                      flexDirection: "column",
                      gap: "6px",
                      padding: "14px",
                      borderRadius: "14px",
                      cursor: "pointer",
                      border: storageSettings.provider === "s3" ? "2px solid var(--accent, #15956a)" : "1px solid var(--line, #e2e8f0)",
                      background: storageSettings.provider === "s3" ? "#f0fdf4" : "#fff",
                      transition: "all 0.15s ease",
                    }}
                  >
                    <div style={{ display: "flex", alignItems: "center", gap: "8px" }}>
                      <input
                        type="radio"
                        name="storage_provider"
                        checked={storageSettings.provider === "s3"}
                        onChange={() => {
                          setStorageSettings((curr) => ({ ...curr, provider: "s3" }));
                          setTestResult(null);
                        }}
                      />
                      <strong style={{ fontSize: "15px" }}>S3-хранилище</strong>
                    </div>
                    <small style={{ color: "#64748b", lineHeight: "1.4" }}>
                      Yandex Cloud, VK Cloud, Selectel, MinIO, AWS S3, Cloudflare R2
                    </small>
                  </label>
                </div>

                <div
                  style={{
                    display: "flex",
                    flexDirection: "column",
                    gap: "14px",
                    padding: "18px",
                    borderRadius: "14px",
                    background: "#f8fafc",
                    border: "1px solid var(--line, #e2e8f0)",
                  }}
                >
                  <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", flexWrap: "wrap", gap: "8px" }}>
                    <h3 style={{ fontSize: "15px", fontWeight: 700, margin: 0 }}>
                      Параметры S3-совместимого хранилища
                    </h3>
                    {storageSettings.provider === "s3" && (
                      <span style={{ fontSize: "12px", background: "#dcfce7", color: "#166534", padding: "3px 8px", borderRadius: "6px", fontWeight: 600 }}>
                        Активное хранилище
                      </span>
                    )}
                  </div>

                  <div className="field-row">
                    <label>
                      Endpoint (URL S3 API)
                      <input
                        placeholder="https://s3.yandexcloud.net"
                        value={storageSettings.s3.endpoint}
                        onChange={(e) => setS3Field("endpoint", e.target.value)}
                      />
                      <small className="form-hint">Адрес сервиса S3 (с http:// или https://)</small>
                    </label>
                    <label>
                      Регион (Region)
                      <input
                        placeholder="ru-central1"
                        value={storageSettings.s3.region}
                        onChange={(e) => setS3Field("region", e.target.value)}
                      />
                      <small className="form-hint">Например: ru-central1, ru-1, us-east-1</small>
                    </label>
                  </div>

                  <div className="field-row">
                    <label>
                      Название бакета (Bucket)
                      <input
                        placeholder="naimio-media"
                        value={storageSettings.s3.bucket}
                        onChange={(e) => setS3Field("bucket", e.target.value)}
                      />
                      <small className="form-hint">Имя созданного бакета в хранилище</small>
                    </label>
                    <label>
                      Публичный URL (опционально)
                      <input
                        placeholder="https://cdn.example.com"
                        value={storageSettings.s3.public_url}
                        onChange={(e) => setS3Field("public_url", e.target.value)}
                      />
                      <small className="form-hint">Если используется CDN или собственный домен</small>
                    </label>
                  </div>

                  <div className="field-row">
                    <label>
                      Ключ доступа (Access Key)
                      <input
                        placeholder="YC..."
                        value={storageSettings.s3.access_key}
                        onChange={(e) => setS3Field("access_key", e.target.value)}
                      />
                      <small className="form-hint">Идентификатор ключа доступа</small>
                    </label>
                    <label>
                      Секретный ключ (Secret Key)
                      <input
                        type="password"
                        autoComplete="new-password"
                        placeholder={storageSettings.s3.secret_key_configured ? "•••••••••••••••• (сохранен)" : "Введите секретный ключ"}
                        value={s3Secret}
                        onChange={(e) => {
                          setS3Secret(e.target.value);
                          setTestResult(null);
                        }}
                      />
                      <small className="form-hint">
                        {storageSettings.s3.secret_key_configured
                          ? "Ключ сохранен в зашифрованном виде. Оставьте пустым, чтобы не менять."
                          : "Секретный ключ надежно шифруется перед сохранением в БД."}
                      </small>
                    </label>
                  </div>

                  <div style={{ display: "flex", gap: "20px", flexWrap: "wrap" }}>
                    <label className="calculator-switch" style={{ margin: 0 }}>
                      <input
                        type="checkbox"
                        checked={storageSettings.s3.use_ssl}
                        onChange={(e) => setS3Field("use_ssl", e.target.checked)}
                      />
                      <span>Использовать SSL / HTTPS</span>
                    </label>
                    <label className="calculator-switch" style={{ margin: 0 }}>
                      <input
                        type="checkbox"
                        checked={storageSettings.s3.path_style}
                        onChange={(e) => setS3Field("path_style", e.target.checked)}
                      />
                      <span>Path-Style адресация (endpoint/bucket/key)</span>
                    </label>
                  </div>

                  <div style={{ display: "flex", alignItems: "center", gap: "12px", marginTop: "4px", flexWrap: "wrap" }}>
                    <button
                      type="button"
                      disabled={testingS3}
                      onClick={testS3Connection}
                      style={{
                        background: "#475569",
                        borderColor: "#475569",
                        width: "auto",
                        padding: "8px 16px",
                        fontSize: "14px",
                      }}
                    >
                      {testingS3 ? "Проверяем подключение…" : "Проверить подключение S3"}
                    </button>
                    {testResult && (
                      <span
                        style={{
                          padding: "6px 12px",
                          borderRadius: "8px",
                          fontSize: "13px",
                          fontWeight: 600,
                          background: testResult.success ? "#dcfce7" : "#fee2e2",
                          color: testResult.success ? "#166534" : "#991b1b",
                          maxWidth: "100%",
                          wordBreak: "break-word",
                        }}
                      >
                        {testResult.message}
                      </span>
                    )}
                  </div>
                </div>
              </section>
            )}

            {/* 7. SYSTEM & CATALOGS */}
            {activeTab === "system" && (
              <>
                <section className="admin-settings-section">
                  <h2>Параметры каталогов</h2>
                  <label>
                    Карточек в одной порции каталога
                    <input
                      type="number"
                      min={10}
                      max={50}
                      step={5}
                      required
                      value={settings.catalog_page_size}
                      onChange={(e) => setNumber("catalog_page_size", Number(e.target.value))}
                    />
                    <small className="form-hint">После первых карточек следующая порция подгружается автоматически при прокрутке.</small>
                  </label>
                </section>

                <section className="admin-settings-section">
                  <h2>Рассылка новых предложений (Дайджест)</h2>
                  <label className="calculator-switch">
                    <input
                      type="checkbox"
                      checked={settings.marketplace_digest_enabled}
                      onChange={(event) => setBoolean("marketplace_digest_enabled", event.target.checked)}
                    />
                    <span>Дайджест включён</span>
                  </label>
                  <p className="form-hint">Рассылка уходит только пользователям, которые сами включили соответствующий канал в настройках уведомлений.</p>
                  <div className="field-row">
                    <label>
                      Отправлять при накоплении (штук)
                      <input
                        type="number"
                        min={1}
                        max={100}
                        required
                        value={settings.marketplace_digest_threshold}
                        onChange={(event) => setNumber("marketplace_digest_threshold", Number(event.target.value))}
                      />
                      <small className="form-hint">Количество новых проектов, вакансий или услуг одного типа.</small>
                    </label>
                    <label>
                      Или не реже, минут
                      <input
                        type="number"
                        min={5}
                        max={1440}
                        required
                        value={settings.marketplace_digest_interval_minutes}
                        onChange={(event) => setNumber("marketplace_digest_interval_minutes", Number(event.target.value))}
                      />
                      <small className="form-hint">Если накопился хотя бы один новый элемент.</small>
                    </label>
                  </div>
                </section>

                <section className="admin-settings-section">
                  <h2>Системные Feature Flags</h2>
                  <p className="form-hint" style={{ marginTop: "-6px", marginBottom: "14px" }}>
                    Текущий статус активных функциональных модулей платформы.
                  </p>
                  <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(240px, 1fr))", gap: "10px" }}>
                    {featureFlags.map((flag) => (
                      <div
                        key={flag.key}
                        style={{
                          padding: "12px",
                          borderRadius: "10px",
                          border: "1px solid var(--line)",
                          background: flag.enabled ? "#f0fdf4" : "#f8fafc",
                          display: "flex",
                          justifyContent: "space-between",
                          alignItems: "center",
                        }}
                      >
                        <div>
                          <strong style={{ fontSize: "13px", display: "block" }}>{flag.key}</strong>
                          <small style={{ color: "#64748b", fontSize: "11px" }}>{flag.description || "Системный флаг"}</small>
                        </div>
                        <span
                          style={{
                            fontSize: "11px",
                            padding: "3px 8px",
                            borderRadius: "6px",
                            fontWeight: 700,
                            background: flag.enabled ? "#dcfce7" : "#e2e8f0",
                            color: flag.enabled ? "#166534" : "#64748b",
                          }}
                        >
                          {flag.enabled ? "ВКЛ" : "ВЫКЛ"}
                        </span>
                      </div>
                    ))}
                  </div>
                </section>
              </>
            )}

            <button disabled={saving} style={{ marginTop: "16px" }}>
              {saving ? "Сохраняем…" : "Сохранить все настройки"}
            </button>
          </form>

          {/* Sticky preview sidebar for theme */}
          <aside
            className="admin-theme-preview"
            style={
              {
                "--preview-background": settings.page_background_color,
                "--preview-heading": settings.heading_color,
                "--preview-accent": settings.green_heading_color,
                "--preview-blue": settings.bright_blue_color,
                "--preview-button": settings.primary_button_color,
              } as CSSProperties
            }
          >
            <p className="eyebrow">Предпросмотр темы</p>
            <h2>{settings.project_name}</h2>
            <p>{settings.project_description || "Краткое описание проекта"}</p>
            <span className="admin-theme-preview__blue">Ярко-синий акцент</span>
            <button type="button">Основная кнопка</button>

            <div style={{ marginTop: "24px", paddingTop: "16px", borderTop: "1px solid var(--line)" }}>
              <strong style={{ fontSize: "13px", display: "block", marginBottom: "8px", color: "var(--ink-strong)" }}>
                SEO & Хранилище
              </strong>
              <div style={{ fontSize: "12px", color: "#64748b", display: "flex", flexDirection: "column", gap: "4px" }}>
                <div>Домен: <strong>{seoSettings.general.canonical_base_url || "https://naimio.ru"}</strong></div>
                <div>IndexNow: <strong style={{ color: seoSettings.indexnow.enabled ? "#16a34a" : "#dc2626" }}>{seoSettings.indexnow.enabled ? "Активен" : "Выключен"}</strong></div>
                <div>Хранилище: <strong>{storageSettings.provider === "s3" ? "S3 Cloud" : "Локальный диск"}</strong></div>
                <div>Порция: <strong>{settings.catalog_page_size} шт.</strong></div>
              </div>
            </div>
          </aside>
        </div>
      )}
    </>
  );
}
