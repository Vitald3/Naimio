"use client";

import { CSSProperties, FormEvent, useCallback, useEffect, useState } from "react";
import {
  AdminError,
  AdminHeader,
  AdminLoading,
  adminRequest,
} from "../admin-ui";
import { defaultSiteSettings, SiteSettings } from "../../site-settings";
import { useToast } from "../../toast";
import { CustomSelect } from "../../custom-select";

type Flag = { key: string; enabled: boolean; config: Record<string, unknown> };
type LegalPost = { id: string; title: string; slug: string; status: string };
type ColorKey = "primary_button_color" | "button_hover_color" | "green_heading_color" | "bright_blue_color" | "heading_color" | "body_text_color" | "page_background_color";

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

export default function ProjectSettingsPage() {
  const { push } = useToast();
  const [settings, setSettings] = useState(defaultSiteSettings);
  const [storageSettings, setStorageSettings] = useState<StorageSettings>(defaultStorageSettings);
  const [s3Secret, setS3Secret] = useState("");
  const [testingS3, setTestingS3] = useState(false);
  const [testResult, setTestResult] = useState<{ success: boolean; message: string } | null>(null);

  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [legalPosts, setLegalPosts] = useState<LegalPost[]>([]);
  const [error, setError] = useState("");

  const load = useCallback(() => {
    setLoading(true);
    Promise.all([
      adminRequest<{ data: Flag[] }>("/api/v1/admin/feature-flags"),
      adminRequest<{ data: { posts: { items: LegalPost[] } } }>("/api/v1/admin/blog"),
      adminRequest<{ data: StorageSettings }>("/api/v1/admin/storage-settings").catch(() => null),
    ])
      .then(([body, blog, storage]) => {
        setLegalPosts((blog.data.posts.items ?? []).filter((post) => post.status === "PUBLISHED"));
        const flag = body.data.find((item) => item.key === "site_appearance");
        if (!flag)
          throw new Error("Примените актуальные миграции базы данных.");
        setSettings({ ...defaultSiteSettings, ...flag.config } as SiteSettings);
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
          reason: "Обновление настроек проекта",
        }),
      });

      // 2. Save storage settings
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
        message: "Настройки темы и хранилища файлов успешно обновлены.",
      });
    } catch (reason) {
      push({
        kind: "error",
        title: "Не удалось сохранить настройки",
        message: reason instanceof Error ? reason.message : "Проверьте поля.",
      });
    } finally {
      setSaving(false);
    }
  }

  return (
    <>
      <AdminHeader
        title="Настройки проекта"
        description="Название, контакты, единая цветовая тема и конфигурация файлового хранилища."
      />
      {loading ? (
        <AdminLoading />
      ) : error ? (
        <AdminError message={error} onRetry={load} />
      ) : (
        <div className="admin-settings-grid">
          <form className="admin-settings-form" onSubmit={save}>
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
                Подпись в подвале
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
                  <small className="form-hint">Выберите опубликованную статью из CMS.</small>
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
                  <small className="form-hint">Выберите опубликованную статью из CMS.</small>
                </label>
              </div>
            </section>

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

            <section className="admin-settings-section">
              <h2>Каталоги</h2>
              <label>
                Карточек в одной порции
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
              <h2>Рассылка новых предложений</h2>
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
                  Отправлять при накоплении
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
              <h2>Цвета интерфейса</h2>
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

            <button disabled={saving}>
              {saving ? "Сохраняем…" : "Сохранить настройки"}
            </button>
          </form>
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
            <p className="eyebrow">Предпросмотр</p>
            <h2>{settings.project_name}</h2>
            <p>{settings.project_description || "Краткое описание проекта"}</p>
            <span className="admin-theme-preview__blue">Ярко-синий акцент</span>
            <button type="button">Основная кнопка</button>
          </aside>
        </div>
      )}
    </>
  );
}
