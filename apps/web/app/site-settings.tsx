"use client";

import {
  createContext,
  ReactNode,
  useContext,
  useEffect,
  useState,
} from "react";

export type SEOGeneralSettings = {
  title_template: string;
  default_title: string;
  default_description: string;
  default_og_image: string;
  canonical_base_url: string;
  robots_policy: string;
  custom_robots_txt: string;
  schema_organization_name: string;
  schema_legal_name: string;
  schema_support_email: string;
  schema_support_phone: string;
};

export type SEOPageOverride = {
  title: string;
  description: string;
  canonical_url?: string;
  og_image?: string;
  no_index: boolean;
};

export type SEOTemplateOverride = {
  title_template: string;
  description_template: string;
};

export type IndexNowSettings = {
  enabled: boolean;
  api_key: string;
  key_location: string;
  auto_submit: boolean;
  host: string;
};

export type SEOSettings = {
  general: SEOGeneralSettings;
  pages: Record<string, SEOPageOverride>;
  templates: Record<string, SEOTemplateOverride>;
  indexnow: IndexNowSettings;
};

export const defaultSEOSettings: SEOSettings = {
  general: {
    title_template: "%s — Naimio",
    default_title: "Naimio — Маркетплейс проверенных фрилансеров и цифровых услуг",
    default_description: "Биржа фриланса Naimio. Проверенные исполнители, безопасная сделка, прозрачные цены, каталог IT-услуг и вакансий.",
    default_og_image: "/media/covers/cover-01.svg",
    canonical_base_url: "https://naimio.ru",
    robots_policy: "INDEX_FOLLOW",
    custom_robots_txt: "",
    schema_organization_name: "Naimio",
    schema_legal_name: "ООО «Наймио»",
    schema_support_email: "support@naimio.ru",
    schema_support_phone: "+7 (495) 000-00-00",
  },
  pages: {
    "/": {
      title: "Naimio — Маркетплейс проверенных фрилансеров и услуг",
      description: "Найдите лучших специалистов для бизнеса: разработка, дизайн, маркетинг, аналитика. Безопасная сделка и гарантия результата.",
      no_index: false,
    },
    "/categories": {
      title: "Категории и направления услуг | Naimio",
      description: "Полный каталог категорий специалистов и услуг на бирже Naimio: IT, разработка, дизайн, маркетинг, AI и маркетплейсы.",
      no_index: false,
    },
    "/freelancers": {
      title: "Каталог проверенных специалистов и фрилансеров | Naimio",
      description: "Специалисты с подтверждённым опытом и отзывами. Фильтры по стеку, рейтингу, категориям и занятости.",
      no_index: false,
    },
    "/services": {
      title: "Каталог услуг и готовых предложений | Naimio",
      description: "Заказ услуг с фиксированной ценой и сроками: разработка сайтов, ботов, дизайн, аудит и консультации.",
      no_index: false,
    },
    "/projects": {
      title: "Открытые проекты и заказы для фрилансеров | Naimio",
      description: "Актуальные заказы для IT-специалистов. Откликайтесь на проекты с безопасной сделкой и прямым контрактом.",
      no_index: false,
    },
    "/vacancies": {
      title: "Вакансии и предложения работы | Naimio",
      description: "Вакансии в продуктовых компаниях и стартапах. Удалённая работа и офис, проверенные работодатели.",
      no_index: false,
    },
    "/education": {
      title: "Обучение, менторинг и консультации | Naimio",
      description: "Индивидуальный менторинг, код-ревью и консультации от ведущих практиков рынка.",
      no_index: false,
    },
    "/check-offer": {
      title: "Проверить коммерческое предложение онлайн | Naimio",
      description: "Бесплатный разбор КП: оценка адекватности стоимости, рисков и состава работ.",
      no_index: false,
    },
    "/price": {
      title: "Калькуляторы стоимости IT-услуг | Naimio",
      description: "Рассчитайте ориентировочный бюджет на разработку Telegram-бота, лендинга или SEO-продвижения.",
      no_index: false,
    },
    "/blog": {
      title: "Блог Naimio — статьи о фрилансе, разработке и бизнесе",
      description: "Практические руководства, аналитика рынка, советы заказчикам и кейсы экспертов.",
      no_index: false,
    },
    "/pro": {
      title: "PRO-подписка для фрилансеров | Naimio",
      description: "Получайте в 3 раза больше заказов, PRO-значок в каталоге и доступ к закрытым проектам.",
      no_index: false,
    },
  },
  templates: {
    category: {
      title_template: "{category} — фрилансеры и услуги | Naimio",
      description_template: "Специалисты и услуги в категории {category}. Заказывайте работы с гарантией безопасной сделки на Naimio.",
    },
    freelancer: {
      title_template: "{name} — {specialty} | Naimio",
      description_template: "Профиль специалиста {name}. Рейтинг {rating}, примеры работ, отзывы и прямой заказ услуг на Naimio.",
    },
    service: {
      title_template: "{service_title} — заказать от {price} | Naimio",
      description_template: "Услуга: {service_title}. Исполнитель {name}. Срок выполнения от {duration} дн. Безопасная сделка на Naimio.",
    },
    project: {
      title_template: "{project_title} — проект на Naimio",
      description_template: "Заказ: {project_title}. Бюджет {budget}. Приём откликов специалистов на бирже Naimio.",
    },
    vacancy: {
      title_template: "Вакансия {job_title} | Naimio",
      description_template: "Открыта вакансия {job_title}. Условия, требования и прямой отклик на Naimio.",
    },
    calculator: {
      title_template: "{calculator_title} — онлайн расчет стоимости | Naimio",
      description_template: "Калькулятор расчета стоимости: {calculator_title}. Быстрая оценка бюджета и сроков на Naimio.",
    },
    blog: {
      title_template: "{post_title} | Блог Naimio",
      description_template: "{excerpt} Читайте полную статью на Naimio.",
    },
  },
  indexnow: {
    enabled: true,
    api_key: "naimio-indexnow-production-key-2026",
    key_location: "https://naimio.ru/naimio-indexnow-production-key-2026.txt",
    auto_submit: true,
    host: "naimio.ru",
  },
};

export type SiteSettings = {
  project_name: string;
  project_description: string;
  support_email: string;
  support_phone: string;
  legal_company_name: string;
  footer_copyright: string;
  primary_button_color: string;
  button_hover_color: string;
  green_heading_color: string;
  bright_blue_color: string;
  heading_color: string;
  body_text_color: string;
  page_background_color: string;
  catalog_page_size: number;
  marketplace_digest_enabled: boolean;
  marketplace_digest_threshold: number;
  marketplace_digest_interval_minutes: number;
  pro_subscriptions_enabled: boolean;
  blog_enabled: boolean;
  privacy_policy_slug: string;
  terms_slug: string;
  seo_settings?: SEOSettings;
};

export const defaultSiteSettings: SiteSettings = {
  project_name: "Naimio",
  project_description: "Маркетплейс профессиональных услуг",
  support_email: "",
  support_phone: "",
  legal_company_name: "",
  footer_copyright: "© Naimio",
  primary_button_color: "#15956a",
  button_hover_color: "#0d7452",
  green_heading_color: "#0d7452",
  bright_blue_color: "#2563a7",
  heading_color: "#0d1f16",
  body_text_color: "#13261d",
  page_background_color: "#ffffff",
  catalog_page_size: 50,
  marketplace_digest_enabled: true,
  marketplace_digest_threshold: 10,
  marketplace_digest_interval_minutes: 60,
  pro_subscriptions_enabled: false,
  blog_enabled: false,
  privacy_policy_slug: "",
  terms_slug: "",
  seo_settings: defaultSEOSettings,
};

const Context = createContext(defaultSiteSettings);
const safeColor = (value: string, fallback: string) =>
  /^#[0-9a-f]{6}$/i.test(value) ? value : fallback;

export function SiteSettingsProvider({ children }: { children: ReactNode }) {
  const [settings, setSettings] = useState(defaultSiteSettings);
  useEffect(() => {
    fetch("/api/v1/site-settings", { cache: "no-store" })
      .then((response) => (response.ok ? response.json() : null))
      .then((body) => {
        if (!body?.data) return;
        const next = { ...defaultSiteSettings, ...body.data } as SiteSettings;
        setSettings(next);
        const root = document.documentElement.style;
        root.setProperty(
          "--brand",
          safeColor(
            next.primary_button_color,
            defaultSiteSettings.primary_button_color,
          ),
        );
        root.setProperty(
          "--brand-dark",
          safeColor(
            next.button_hover_color,
            defaultSiteSettings.button_hover_color,
          ),
        );
        root.setProperty(
          "--green-heading-color",
          safeColor(
            next.green_heading_color,
            defaultSiteSettings.green_heading_color,
          ),
        );
        root.setProperty(
          "--bright-blue",
          safeColor(
            next.bright_blue_color,
            defaultSiteSettings.bright_blue_color,
          ),
        );
        root.setProperty(
          "--ink-strong",
          safeColor(next.heading_color, defaultSiteSettings.heading_color),
        );
        root.setProperty(
          "--ink",
          safeColor(next.body_text_color, defaultSiteSettings.body_text_color),
        );
        root.setProperty(
          "--page-background",
          safeColor(
            next.page_background_color,
            defaultSiteSettings.page_background_color,
          ),
        );
      })
      .catch(() => undefined);
  }, []);
  return <Context.Provider value={settings}>{children}</Context.Provider>;
}

export const useSiteSettings = () => useContext(Context);
