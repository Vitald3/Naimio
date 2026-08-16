/**
 * DEV/QA-ONLY fixture API — impersonates the Go API on :8080 so `next dev` (with
 * the non-production rewrite in next.config.js) renders populated marketplace
 * pages for visual QA. This is NOT part of the app or its build; it ships no
 * behavior into production. Run: `node apps/web/tests/devqa/fixture-api.mjs`.
 *
 * Auth state is mutable for screenshots:
 *   GET /api/v1/__demo/as?role=guest|customer|freelancer|admin
 */
import { createServer } from "node:http";

const PORT = process.env.FIXTURE_PORT ? Number(process.env.FIXTURE_PORT) : 8080;
const k = (rub) => Math.round(rub * 100); // rubles -> kopecks

let role = "guest";
const USERS = {
  // Matches the real API contract: CUSTOMER/FREELANCER are capabilities (user_capabilities);
  // ADMIN/MODERATOR/SUPER_ADMIN are roles (user_roles). Regular users have no roles.
  customer: { id: "u-cust", email: "maria@studio.example", display_name: "Мария Кравцова", roles: [], capabilities: ["CUSTOMER"] },
  freelancer: { id: "u-free", email: "anton@dev.example", display_name: "Антон Лебедев", roles: [], capabilities: ["FREELANCER"] },
  admin: { id: "u-admin", email: "ops@platform.example", display_name: "Ирина Соколова", roles: ["SUPER_ADMIN"], capabilities: [] },
};

const CATEGORIES = [
  ["web-development", "Веб-разработка"], ["mobile", "Мобильная разработка"], ["design", "Дизайн и UX"],
  ["ai-data", "AI и данные"], ["marketing", "Маркетинг и реклама"], ["seo", "SEO и контент"],
  ["video", "Видео и анимация"], ["devops", "DevOps и инфраструктура"], ["copywriting", "Тексты и переводы"],
  ["business", "Бизнес и аналитика"], ["support", "Поддержка и QA"], ["blockchain", "Блокчейн"],
].map(([slug, name], i) => ({ id: `cat-${i + 1}`, slug, name }));

const skill = (name) => ({ id: name.toLowerCase().replace(/[^a-z0-9]+/g, "-"), name });

const FREELANCERS = [
  { username: "anton-lebedev", display_name: "Антон Лебедев", professional_title: "Senior Full-stack инженер · React / Go", availability: "AVAILABLE", experience_years: 8, hourly_rate_kopecks: k(4500), rating: 4.9, reviews_count: 63, completed_projects: 87, location: "Москва", verified: true, bio: "Проектирую и запускаю продукты под ключ: от архитектуры до релиза. Специализация — высоконагруженные сервисы и платёжные интеграции.", skills: ["React", "TypeScript", "Go", "PostgreSQL", "Kubernetes"].map(skill), languages: ["Русский", "English"], response_time: "~1 час", member_since: "2021", external_reputation: [{ source: "GitHub", label: "Verified", metric: "3.1k stars" }, { source: "Stack Overflow", label: "Verified", metric: "18k rep" }] },
  { username: "polina-orlova", display_name: "Полина Орлова", professional_title: "Продуктовый дизайнер · UX / UI", availability: "AVAILABLE", experience_years: 6, hourly_rate_kopecks: k(3800), rating: 5.0, reviews_count: 41, completed_projects: 54, location: "Санкт-Петербург", verified: true, bio: "Дизайн-системы, сложные интерфейсы и исследования. Помогаю командам довести продукт до ясной и удобной формы.", skills: ["Figma", "Design Systems", "UX Research", "Prototyping"].map(skill), languages: ["Русский", "English"], response_time: "~2 часа", member_since: "2020", external_reputation: [{ source: "Dribbble", label: "Verified", metric: "Top 5%" }, { source: "Behance", label: "Verified", metric: "12 проектов" }] },
  { username: "ruslan-mag", display_name: "Руслан Магомедов", professional_title: "ML-инженер · LLM / Computer Vision", availability: "PARTIALLY_BUSY", experience_years: 7, hourly_rate_kopecks: k(5200), rating: 4.8, reviews_count: 29, completed_projects: 38, location: "Удалённо", verified: true, bio: "Внедряю ML в продукты: рекомендательные системы, RAG-ассистенты, обработка документов.", skills: ["Python", "PyTorch", "LLM", "RAG", "MLOps"].map(skill), languages: ["Русский"], response_time: "~4 часа", member_since: "2019", external_reputation: [{ source: "Kaggle", label: "Verified", metric: "Master" }] },
  { username: "elena-vodo", display_name: "Елена Водолазова", professional_title: "Perfomance-маркетолог · Ads / Analytics", availability: "AVAILABLE", experience_years: 5, hourly_rate_kopecks: k(3200), rating: 4.7, reviews_count: 52, completed_projects: 71, location: "Казань", verified: false, bio: "Веду платный трафик и аналитику под выручку, а не показы. Google Ads, Яндекс Директ, сквозная аналитика.", skills: ["Google Ads", "Яндекс Директ", "GA4", "Аналитика"].map(skill), languages: ["Русский"], response_time: "~3 часа", member_since: "2021", external_reputation: [] },
  { username: "mikhail-tk", display_name: "Михаил Ткаченко", professional_title: "Мобильный разработчик · Flutter / iOS", availability: "AVAILABLE", experience_years: 6, hourly_rate_kopecks: k(4100), rating: 4.9, reviews_count: 37, completed_projects: 49, location: "Новосибирск", verified: true, bio: "Кроссплатформенные приложения на Flutter и нативный iOS. Публикация в сторах, CI/CD, аналитика.", skills: ["Flutter", "Dart", "Swift", "Firebase"].map(skill), languages: ["Русский", "English"], response_time: "~2 часа", member_since: "2020", external_reputation: [{ source: "GitHub", label: "Verified", metric: "1.2k stars" }] },
  { username: "darya-sok", display_name: "Дарья Соколова", professional_title: "Копирайтер · Контент и бренд", availability: "PARTIALLY_BUSY", experience_years: 9, hourly_rate_kopecks: k(2600), rating: 4.9, reviews_count: 88, completed_projects: 140, location: "Удалённо", verified: true, bio: "Тексты, которые продают и объясняют: лендинги, email-цепочки, продуктовые статьи.", skills: ["Копирайтинг", "Редактура", "Email", "Сторителлинг"].map(skill), languages: ["Русский", "English"], response_time: "~1 час", member_since: "2018", external_reputation: [] },
  { username: "sergey-dev", display_name: "Сергей Демин", professional_title: "DevOps-инженер · Cloud / SRE", availability: "BUSY", experience_years: 10, hourly_rate_kopecks: k(5600), rating: 5.0, reviews_count: 24, completed_projects: 33, location: "Москва", verified: true, bio: "Инфраструктура как код, наблюдаемость и надёжность. Kubernetes, Terraform, снижение затрат на облако.", skills: ["Kubernetes", "Terraform", "AWS", "Observability"].map(skill), languages: ["Русский", "English"], response_time: "~6 часов", member_since: "2017", external_reputation: [{ source: "GitHub", label: "Verified", metric: "4.4k stars" }] },
  { username: "nina-art", display_name: "Нина Артемьева", professional_title: "Моушн-дизайнер · Motion / 3D", availability: "AVAILABLE", experience_years: 4, hourly_rate_kopecks: k(3000), rating: 4.8, reviews_count: 31, completed_projects: 44, location: "Екатеринбург", verified: false, bio: "Анимация интерфейсов, промо-ролики и 3D. Превращаю идеи в динамичный визуал.", skills: ["After Effects", "Cinema 4D", "Motion", "3D"].map(skill), languages: ["Русский"], response_time: "~5 часов", member_since: "2022", external_reputation: [] },
];

const SERVICES = [
  { id: "svc-1", slug: "landing-under-key", title: "Лендинг под ключ: дизайн + разработка", short_description: "Продающая посадочная страница с адаптивом, аналитикой и SEO-базой за 10 дней.", service_type: "PROFESSIONAL_SERVICE", price_type: "FROM", price_from: { amount_kopecks: k(45000) }, delivery_days: 10, rating: 4.9, reviews_count: 34, seller_display_name: "Полина Орлова", seller_username: "polina-orlova", category: { name: "Дизайн и UX" } },
  { id: "svc-2", slug: "telegram-bot-sales", title: "Telegram-бот для продаж и заявок", short_description: "Бот с оплатой, CRM-интеграцией и админкой. Разворачиваю на вашем сервере.", service_type: "PROFESSIONAL_SERVICE", price_type: "FROM", price_from: { amount_kopecks: k(60000) }, delivery_days: 14, rating: 4.8, reviews_count: 27, seller_display_name: "Антон Лебедев", seller_username: "anton-lebedev", category: { name: "Веб-разработка" } },
  { id: "svc-3", slug: "seo-audit", title: "SEO-аудит сайта с планом роста", short_description: "Технический и контентный аудит, семантика и приоритизированный план на 90 дней.", service_type: "PROFESSIONAL_SERVICE", price_type: "FIXED", price_from: { amount_kopecks: k(28000) }, delivery_days: 7, rating: 4.7, reviews_count: 45, seller_display_name: "Елена Водолазова", seller_username: "elena-vodo", category: { name: "SEO и контент" } },
  { id: "svc-4", slug: "ml-consult", title: "Консультация по внедрению AI-ассистента", short_description: "Разбираем задачу, данные и архитектуру RAG. Дорожная карта внедрения за 3 сессии.", service_type: "CONSULTATION", price_type: "FROM", price_from: { amount_kopecks: k(9000) }, delivery_days: 3, rating: 5.0, reviews_count: 12, seller_display_name: "Руслан Магомедов", seller_username: "ruslan-mag", category: { name: "AI и данные" } },
  { id: "svc-5", slug: "mobile-mvp", title: "MVP мобильного приложения на Flutter", short_description: "От макета до публикации в сторах: авторизация, каталог, оплата, аналитика.", service_type: "PROFESSIONAL_SERVICE", price_type: "FROM", price_from: { amount_kopecks: k(180000) }, delivery_days: 30, rating: 4.9, reviews_count: 18, seller_display_name: "Михаил Ткаченко", seller_username: "mikhail-tk", category: { name: "Мобильная разработка" } },
  { id: "svc-6", slug: "promo-video", title: "Промо-ролик 30 секунд: сценарий + анимация", short_description: "Динамичный ролик для продукта: сценарий, озвучка, моушн-дизайн, адаптации под соцсети.", service_type: "PROFESSIONAL_SERVICE", price_type: "FIXED", price_from: { amount_kopecks: k(52000) }, delivery_days: 12, rating: 4.8, reviews_count: 22, seller_display_name: "Нина Артемьева", seller_username: "nina-art", category: { name: "Видео и анимация" } },
  { id: "svc-7", slug: "react-intensive", title: "Интенсив по React и TypeScript", short_description: "Пятидневный практический курс: хуки, состояние, тестирование и продакшн-паттерны.", service_type: "EDUCATION", price_type: "FIXED", price_from: { amount_kopecks: k(24000) }, delivery_days: 5, rating: 4.9, reviews_count: 31, education_details: { format: "ONLINE", audience_type: "GROUP", duration_minutes: 600 }, seller_display_name: "Дмитрий Соловьёв", seller_username: "dmitry-sol", category: { name: "Веб-разработка" } },
  { id: "svc-8", slug: "design-mentoring", title: "Менторинг для продуктовых дизайнеров", short_description: "Индивидуальные сессии: портфолио, процесс, карьерный трек и разбор реальных задач.", service_type: "MENTORING", price_type: "HOURLY", price_from: { amount_kopecks: k(4000) }, delivery_days: 1, rating: 5.0, reviews_count: 16, education_details: { format: "ONLINE", audience_type: "INDIVIDUAL", duration_minutes: 60 }, seller_display_name: "Полина Орлова", seller_username: "polina-orlova", category: { name: "Дизайн и UX" } },
];

const PROJECTS = [
  { id: "prj-1", title: "Разработать приложение для доставки на Flutter", description: "Нужно кроссплатформенное приложение с авторизацией, каталогом, корзиной и онлайн-оплатой. Есть готовый дизайн в Figma и backend на REST API. Важны сроки и качество кода.", category: { name: "Мобильная разработка" }, skills: ["Flutter", "Dart", "REST", "Оплата"].map(skill), budget: { min_kopecks: k(150000), max_kopecks: k(300000) }, created_at: "2026-08-10T09:00:00Z", proposals_count: 12, deadline: "6 недель", status: "OPEN", customer: { display_name: "Мария Кравцова", rating: 4.8, projects_posted: 9, verified: true } },
  { id: "prj-2", title: "Редизайн корпоративного сайта и дизайн-система", description: "Требуется современный редизайн, компонентная дизайн-система в Figma и адаптив. Ориентируемся на ясность и премиальность. Дальше — долгосрочное сотрудничество.", category: { name: "Дизайн и UX" }, skills: ["Figma", "UI", "Design System"].map(skill), budget: { min_kopecks: k(120000), max_kopecks: k(220000) }, created_at: "2026-08-11T12:30:00Z", proposals_count: 8, deadline: "5 недель", status: "OPEN", customer: { display_name: "ООО «Северный поток»", rating: 4.6, projects_posted: 4, verified: true } },
  { id: "prj-3", title: "Внедрить RAG-ассистента по внутренней базе знаний", description: "Есть 4 000 документов. Нужен ассистент с поиском и ссылками на источники, аккуратной обработкой прав доступа и оценкой качества ответов.", category: { name: "AI и данные" }, skills: ["Python", "LLM", "RAG", "Vector DB"].map(skill), budget: { min_kopecks: k(250000), max_kopecks: k(500000) }, created_at: "2026-08-09T15:10:00Z", proposals_count: 6, deadline: "8 недель", status: "OPEN", customer: { display_name: "FinTech Lab", rating: 4.9, projects_posted: 15, verified: true } },
  { id: "prj-4", title: "Настроить платный трафик и сквозную аналитику", description: "Запускаем новый продукт. Нужны кампании в Яндекс Директ и Google Ads, настройка GA4 и отчётность по выручке. Бюджет на тесты обсуждается отдельно.", category: { name: "Маркетинг и реклама" }, skills: ["Яндекс Директ", "Google Ads", "GA4"].map(skill), budget: { min_kopecks: k(60000), max_kopecks: k(120000) }, created_at: "2026-08-12T08:05:00Z", proposals_count: 15, deadline: "Ежемесячно", status: "OPEN", customer: { display_name: "Артём Волков", rating: 4.5, projects_posted: 2, verified: false } },
  { id: "prj-5", title: "Промо-ролик для запуска SaaS-продукта", description: "Нужен 45-секундный ролик с моушн-дизайном, сценарием и озвучкой. Есть брендбук. Требуются адаптации под YouTube, Instagram и сайт.", category: { name: "Видео и анимация" }, skills: ["Motion", "After Effects", "Сценарий"].map(skill), budget: { min_kopecks: k(70000), max_kopecks: k(140000) }, created_at: "2026-08-08T18:40:00Z", proposals_count: 9, deadline: "3 недели", status: "OPEN", customer: { display_name: "Кристина Юдина", rating: 4.7, projects_posted: 6, verified: true } },
  { id: "prj-6", title: "Инфраструктура и CI/CD для команды из 6 инженеров", description: "Переезд в Kubernetes, инфраструктура как код, наблюдаемость и снижение затрат. Нужен опытный DevOps на проект с возможностью поддержки.", category: { name: "DevOps и инфраструктура" }, skills: ["Kubernetes", "Terraform", "CI/CD", "AWS"].map(skill), budget: { min_kopecks: k(200000), max_kopecks: k(400000) }, created_at: "2026-08-07T10:20:00Z", proposals_count: 5, deadline: "6 недель", status: "OPEN", customer: { display_name: "DevHouse", rating: 5.0, projects_posted: 21, verified: true } },
];

const VACANCIES = [
  { id: "vac-1", title: "Frontend-разработчик (React/Next.js)", company: { name: "Nimbus", website: "https://example.com/nimbus", description: "Продуктовая команда, которая развивает платёжный сервис для малого бизнеса.", verification_status: "VERIFIED" }, employment_type: "FULL_TIME", remote: true, location: "Удалённо", experience_level: "INTERMEDIATE", salary_min_kopecks: k(200000), salary_max_kopecks: k(320000), skills: ["React", "Next.js", "TypeScript"].map(skill), description: "Развиваем платёжный продукт. Ищем инженера в команду фронтенда.", published_at: "2026-08-10T09:00:00Z" },
  { id: "vac-2", title: "Продуктовый дизайнер", company: { name: "Kontur", website: "https://example.com/kontur", description: "B2B-платформа для документооборота и отчётности.", verification_status: "VERIFIED" }, employment_type: "FULL_TIME", remote: false, location: "Москва / гибрид", experience_level: "ADVANCED", salary_min_kopecks: k(180000), salary_max_kopecks: k(260000), skills: ["Figma", "UX"].map(skill), description: "Проектируем сложные B2B-интерфейсы.", published_at: "2026-08-11T09:00:00Z" },
  { id: "vac-3", title: "ML-инженер (LLM)", company: { name: "DataForge", website: "https://example.com/dataforge", description: "Строим ML-инфраструктуру и ассистентов для крупных компаний.", verification_status: "VERIFIED" }, employment_type: "CONTRACT", remote: true, location: "Удалённо", experience_level: "EXPERT", salary_min_kopecks: k(280000), salary_max_kopecks: k(420000), skills: ["Python", "LLM", "RAG"].map(skill), description: "Строим ассистентов на базе внутренних данных.", published_at: "2026-08-09T09:00:00Z" },
];

const EDUCATION = [
  { id: "edu-1", title: "Наставничество: путь к Senior Frontend", mentor_display_name: "Антон Лебедев", format: "MENTORING", price_from_kopecks: k(6000), duration: "8 недель", rating: 4.9, skills: ["React", "Архитектура"].map(skill) },
  { id: "edu-2", title: "Дизайн-системы с нуля до продакшена", mentor_display_name: "Полина Орлова", format: "COURSE", price_from_kopecks: k(24000), duration: "Самостоятельно", rating: 5.0, skills: ["Figma", "Design Systems"].map(skill) },
  { id: "edu-3", title: "Введение в RAG-ассистентов", mentor_display_name: "Руслан Магомедов", format: "COURSE", price_from_kopecks: k(18000), duration: "4 недели", rating: 4.8, skills: ["LLM", "RAG"].map(skill) },
];

const NOTIFICATIONS = [
  { id: "n1", type: "PROPOSAL_RECEIVED", read_at: null, created_at: "2026-08-12T08:30:00Z", title: "Новый отклик на проект «Приложение доставки»" },
  { id: "n2", type: "MESSAGE", read_at: null, created_at: "2026-08-12T07:10:00Z", title: "Сообщение от Полины Орловой" },
  { id: "n3", type: "SAFE_DEAL_FUNDED", read_at: "2026-08-11T20:00:00Z", created_at: "2026-08-11T19:55:00Z", title: "Сделка профинансирована" },
];

const DEALS = [
  { id: "deal-1", project_id: "prj-1", project_title: "Приложение для доставки на Flutter", gross_amount_kopecks: k(240000), platform_fee_kopecks: k(24000), freelancer_amount_kopecks: k(216000), status: "SUBMITTED", viewer_role: "CUSTOMER", counterparty_name: "Антон Лебедев", payment: { provider: "YooKassa", provider_status: "succeeded" }, submission: { summary: "Готова первая версия: авторизация, каталог, корзина и онлайн-оплата. Сборки под Android и iOS приложены, инструкция по запуску — во вложении." }, conversation_id: "conv-1", provider_capability_notice: "Провайдер удерживает средства до подтверждения приёмки. Платформа не выступает гарантом сверх возможностей платёжного провайдера." },
  { id: "deal-2", project_id: "prj-3", project_title: "RAG-ассистент по базе знаний", gross_amount_kopecks: k(360000), platform_fee_kopecks: k(36000), freelancer_amount_kopecks: k(324000), status: "IN_PROGRESS", viewer_role: "CUSTOMER", counterparty_name: "Руслан Магомедов", payment: { provider: "YooKassa", provider_status: "succeeded" }, conversation_id: "conv-2", provider_capability_notice: "Средства зарезервированы провайдером и будут переданы исполнителю после приёмки работы." },
  { id: "deal-3", project_id: "prj-2", project_title: "Редизайн и дизайн-система", gross_amount_kopecks: k(180000), platform_fee_kopecks: k(18000), freelancer_amount_kopecks: k(162000), status: "AWAITING_FUNDING", viewer_role: "CUSTOMER", counterparty_name: "Полина Орлова" },
];

const CONVERSATIONS = [
  { id: "conv-1", kind: "PROJECT", project_id: "prj-1", title: "Приложение для доставки", counterparty_name: "Антон Лебедев", unread_count: 2, updated_at: "2026-08-12T08:40:00Z" },
  { id: "conv-2", kind: "DIRECT", counterparty_name: "Полина Орлова", title: "Полина Орлова", unread_count: 0, updated_at: "2026-08-11T16:20:00Z" },
];
const MESSAGES = [
  { id: "m1", conversation_id: "conv-1", sender_user_id: "u-free", type: "TEXT", body: "Отлично, спасибо! Приступаю к макетам ключевых экранов и вернусь с прогрессом.", media_ids: [], created_at: "2026-08-12T08:40:00Z" },
  { id: "m2", conversation_id: "conv-1", sender_user_id: "u-cust", type: "TEXT", body: "Согласна. Профинансирую первый этап сегодня через Безопасную сделку.", media_ids: [], created_at: "2026-08-12T08:35:00Z" },
  { id: "m3", conversation_id: "conv-1", sender_user_id: "u-free", type: "TEXT", body: "Понял. Предлагаю зафиксировать объём и начать с авторизации и каталога.", media_ids: [], created_at: "2026-08-12T08:20:00Z" },
  { id: "m4", conversation_id: "conv-1", sender_user_id: "u-cust", type: "TEXT", body: "Здравствуйте! Да, давайте. Приоритет — онлайн-оплата и каталог товаров.", media_ids: [], created_at: "2026-08-12T08:12:00Z" },
  { id: "m5", conversation_id: "conv-1", sender_user_id: "u-free", type: "TEXT", body: "Добрый день! Готов обсудить детали приложения — есть вопросы по срокам.", media_ids: [], created_at: "2026-08-12T08:10:00Z" },
];

const nf = (obj, code = 404) => ({ __status: code, error: { code: "not_found", message: "not found" } });
const list = (data, extra = {}) => ({ data, meta: { total: data.length, ...extra } });
const one = (data) => (data ? { data } : nf());

function findFreelancer(u) { return FREELANCERS.find((f) => f.username === u); }

// The real Go API serves richer, differently-shaped payloads on the profile
// detail endpoints than the list uses. Map the list fixture into those shapes
// so the detail page renders every section for visual QA.
const LANG = { "Русский": { code: "ru", level: "NATIVE" }, English: { code: "en", level: "FLUENT" } };
function toProfile(f) {
  if (!f) return undefined;
  return {
    username: f.username, display_name: f.display_name, professional_title: f.professional_title,
    bio: f.bio, location_text: f.location, availability: f.availability,
    experience_years: f.experience_years, hourly_rate_kopecks: f.hourly_rate_kopecks,
    categories: [], skills: (f.skills || []).map((s) => ({ ...s, slug: s.id, is_featured: false })),
    languages: (f.languages || []).map((n) => LANG[n] || { code: n.slice(0, 2).toLowerCase(), level: "FLUENT" }),
  };
}
function profilePortfolio(f) {
  if (!f || !f.skills?.length) return [];
  return [
    { id: `${f.username}-pf-1`, title: "Ключевой проект под ключ", description: "Полный цикл: исследование, реализация и запуск. Замеримый результат для клиента.", skills: f.skills.slice(0, 3), categories: [], completed_on: "2025-11", price_min_kopecks: k(120000), price_max_kopecks: k(180000) },
    { id: `${f.username}-pf-2`, title: "Оптимизация и поддержка", description: "Улучшение производительности и сопровождение продукта после релиза.", skills: f.skills.slice(1, 3), categories: [], completed_on: "2025-06" },
  ];
}
function profileReviews(f) {
  if (!f || !f.reviews_count) return { items: [], trust: { reviews_count: 0, completed_projects_count: 0 } };
  return {
    items: [
      { id: `${f.username}-rv-1`, project_id: "prj-1", reviewer_role: "CUSTOMER", rating_overall: 5, would_work_again: true, text: "Сделал больше, чем ожидали. Чёткая коммуникация и сроки.", dimensions: {}, created_at: "2025-12-01T10:00:00Z" },
      { id: `${f.username}-rv-2`, project_id: "prj-2", reviewer_role: "CUSTOMER", rating_overall: 5, would_work_again: true, text: "Глубокая экспертиза, аккуратная работа с деталями.", dimensions: {}, created_at: "2025-10-14T10:00:00Z" },
    ],
    trust: { native_rating: f.rating, reviews_count: f.reviews_count, completed_projects_count: f.completed_projects, recommendation_rate: 96 },
  };
}
function profileExternal(f) {
  if (!f || !f.external_reputation?.length) return [];
  return f.external_reputation.map((r) => ({
    platform: r.source, display_name: `${r.source} · ${r.metric}`,
    profile_url: `https://example.com/${r.source.toLowerCase().replace(/\s+/g, "")}`,
    verified: true, verified_at: "2026-01-10", account_since: f.member_since,
  }));
}

const BRIEF = {
  title: "Лендинг для SaaS-продукта на Next.js",
  summary: "Адаптивный лендинг для SaaS-продукта на Next.js с интеграцией платёжного провайдера, подключением аналитики и подготовкой к запуску за три недели.",
  scope: "Разработка адаптивного лендинга с интеграцией платежей.",
  requirements: ["Адаптивная вёрстка под мобильные и десктоп", "Интеграция платёжного провайдера", "Базовая SEO-подготовка и аналитика"],
  questions: ["Есть ли готовый дизайн-макет или требуется дизайн?", "Какой платёжный провайдер предпочтителен?"],
  assumptions: ["Тексты и изображения предоставляет заказчик"],
  category_candidates: [{ id: "cat-web", name: "Веб-разработка", slug: "web-development", confidence: 0.92 }],
  skills: [
    { id: "sk-next", name: "Next.js", slug: "nextjs", confidence: 0.91 },
    { id: "sk-react", name: "React", slug: "react", confidence: 0.88 },
    { id: "sk-ts", name: "TypeScript", slug: "typescript", confidence: 0.79 },
  ],
  budget: { min_kopecks: k(120000), max_kopecks: k(200000), currency: "RUB", confidence: "MEDIUM" },
  duration_days: { min: 14, max: 21 },
};

// ---- Admin fixtures (dev QA only) ----
const ADMIN_USERS = [
  { id: "u-cust", email: "maria@studio.example", username: "maria-k", display_name: "Мария Кравцова", status: "ACTIVE", email_verified: true, roles: [], capabilities: ["CUSTOMER"], created_at: "2024-11-02T10:00:00Z", last_seen_at: "2026-08-13T08:12:00Z", active_sessions: 2 },
  { id: "u-free", email: "anton@dev.example", username: "anton-lebedev", display_name: "Антон Лебедев", status: "ACTIVE", email_verified: true, roles: [], capabilities: ["FREELANCER"], created_at: "2021-03-15T10:00:00Z", last_seen_at: "2026-08-13T09:40:00Z", active_sessions: 1 },
  { id: "u-pol", email: "polina@design.example", username: "polina-orlova", display_name: "Полина Орлова", status: "ACTIVE", email_verified: true, roles: [], capabilities: ["FREELANCER", "CUSTOMER"], created_at: "2020-06-01T10:00:00Z", last_seen_at: "2026-08-12T20:05:00Z", active_sessions: 3 },
  { id: "u-mod", email: "mod@platform.example", username: "moderator-1", display_name: "Игорь Панов", status: "ACTIVE", email_verified: true, roles: ["MODERATOR"], capabilities: [], created_at: "2023-01-10T10:00:00Z", last_seen_at: "2026-08-13T07:00:00Z", active_sessions: 1 },
  { id: "u-susp", email: "review@bad.example", username: "account-review", display_name: "Аккаунт на проверке", status: "SUSPENDED", email_verified: false, roles: [], capabilities: ["FREELANCER"], created_at: "2026-08-01T10:00:00Z", last_seen_at: "2026-08-05T10:00:00Z", active_sessions: 0 },
  { id: "u-admin", email: "ops@platform.example", username: "irina-s", display_name: "Ирина Соколова", status: "ACTIVE", email_verified: true, roles: ["SUPER_ADMIN"], capabilities: [], created_at: "2019-05-20T10:00:00Z", last_seen_at: "2026-08-13T10:15:00Z", active_sessions: 1 },
];
const ADMIN_DEALS = [
  { id: "deal-1", project_id: "prj-1", gross_amount_kopecks: k(240000), platform_fee_kopecks: k(24000), freelancer_amount_kopecks: k(216000), status: "SUBMITTED", payment: { provider: "YooKassa", provider_status: "succeeded", provider_payment_id: "yk_2f9a1c7b" }, provider_operational: true, updated_at: "2026-08-12T18:00:00Z" },
  { id: "deal-2", project_id: "prj-3", gross_amount_kopecks: k(360000), platform_fee_kopecks: k(36000), freelancer_amount_kopecks: k(324000), status: "IN_PROGRESS", payment: { provider: "YooKassa", provider_status: "succeeded", provider_payment_id: "yk_88bd0e12" }, provider_operational: true, updated_at: "2026-08-11T14:30:00Z" },
  { id: "deal-4", project_id: "prj-4", gross_amount_kopecks: k(90000), platform_fee_kopecks: k(9000), freelancer_amount_kopecks: k(81000), status: "DISPUTED", payment: { provider: "YooKassa", provider_status: "succeeded", provider_payment_id: "yk_4410ffa2" }, provider_operational: true, dispute: { reason_code: "WORK_DOES_NOT_MATCH_SCOPE", status: "UNDER_REVIEW", description: "Результат не соответствует согласованному заданию." }, updated_at: "2026-08-13T09:00:00Z" },
  { id: "deal-3", project_id: "prj-2", gross_amount_kopecks: k(180000), platform_fee_kopecks: k(18000), freelancer_amount_kopecks: k(162000), status: "AWAITING_FUNDING", updated_at: "2026-08-13T07:20:00Z" },
];
const ADMIN_DISPUTES = [
  { id: "dsp-1", reason_code: "WORK_DOES_NOT_MATCH_SCOPE", description: "Результат не соответствует согласованному заданию, часть функций отсутствует.", opened_at: "2026-08-12T10:00:00Z", project_title: "Настроить платный трафик и сквозную аналитику", customer_name: "Артём Волков", freelancer_name: "Елена Водолазова", amount_kopecks: k(90000), deal_status: "DISPUTED", deal_id: "deal-4", status: "UNDER_REVIEW" },
  { id: "dsp-2", reason_code: "CUSTOMER_UNRESPONSIVE", description: "Заказчик не выходит на связь для приёмки более семи дней.", opened_at: "2026-08-10T13:20:00Z", project_title: "Промо-ролик для запуска SaaS-продукта", customer_name: "Кристина Юдина", freelancer_name: "Нина Артемьева", amount_kopecks: k(70000), deal_status: "DISPUTED", deal_id: "deal-5", status: "EVIDENCE_COLLECTION" },
];

function bySearch(items, q) {
  if (!q) return items;
  const s = q.toLowerCase();
  return items.filter((i) => JSON.stringify(i).toLowerCase().includes(s));
}

function route(pathname, query) {
  const seg = pathname.replace(/^\/api\/v1\//, "").replace(/\/$/, "").split("/");
  const [a, b, c] = seg;

  if (a === "__demo" && b === "as") { role = query.role || "guest"; return { data: { role } }; }
  if (a === "auth" && b === "session") return role === "guest" ? nf(401) : { data: USERS[role] || USERS.customer };
  if (a === "notifications") return list(NOTIFICATIONS);

  if (a === "categories" && !b) return list(CATEGORIES);
  if (a === "categories" && b) return one(CATEGORIES.find((x) => x.slug === b));

  if (a === "freelancers" && !b) return list(bySearch(FREELANCERS, query.q).slice(0, Number(query.limit) || 24));
  if (a === "freelancers" && b) return one(findFreelancer(b));

  if (a === "services" && !b) {
    let out = bySearch(SERVICES, query.q);
    if (query.service_type) out = out.filter((s) => s.service_type === query.service_type);
    if (query.price_type) out = out.filter((s) => s.price_type === query.price_type);
    return list(out.slice(0, Number(query.limit) || 24));
  }
  if (a === "services" && b) {
    const s = SERVICES.find((x) => x.id === b || x.slug === b);
    if (!s) return nf();
    return one({ ...s, description: s.description || `${s.short_description} Работаю прозрачно: фиксируем цель и объём, согласуем этапы и сроки, держим связь на всём проекте. Перед стартом обсудим детали и подберём оптимальное решение под вашу задачу.`, included_revisions: s.included_revisions ?? 2 });
  }

  if (a === "projects" && !b) return list(bySearch(PROJECTS, query.q).slice(0, Number(query.limit) || 24));
  if (a === "projects" && b === "mine") return list(PROJECTS.slice(0, 4));
  if (a === "projects" && b) {
    const p = PROJECTS.find((x) => x.id === b);
    if (!p) return nf();
    return one({
      ...p,
      budget: { type: "RANGE", ...p.budget },
      proposal_count: p.proposal_count ?? p.proposals_count ?? 0,
      customer_display_name: p.customer_display_name ?? p.customer?.display_name,
      experience_level: p.experience_level ?? "INTERMEDIATE",
    });
  }

  if (a === "vacancies" && !b) {
    let out = bySearch(VACANCIES, query.q);
    if (query.employment_type)
      out = out.filter((v) => v.employment_type === query.employment_type);
    if (query.remote === "true") out = out.filter((v) => v.remote === true);
    if (query.remote === "false") out = out.filter((v) => v.remote === false);
    return list(out);
  }
  if (a === "vacancies" && b) return one(VACANCIES.find((x) => x.id === b));

  if (a === "education") return list(EDUCATION);
  if (a === "profiles" && b && c === "portfolio") return list(profilePortfolio(findFreelancer(b)));
  if (a === "profiles" && b && c === "external-reputations") return list(profileExternal(findFreelancer(b)));
  if (a === "profiles" && b && c === "reviews") {
    const f = findFreelancer(b);
    if (!f) return { data: [], trust: { reviews_count: 0, completed_projects_count: 0 } };
    const r = profileReviews(f);
    return { data: r.items, trust: r.trust };
  }
  if (a === "profiles" && b) return one(toProfile(findFreelancer(b)));

  if (a === "me" && b === "projects") return list(PROJECTS.slice(0, 3));
  if (a === "me" && b === "services") return list(SERVICES.slice(0, 2));
  if (a === "me" && b === "proposals") return list([{ id: "pp-1" }, { id: "pp-2" }]);
  if (a === "me" && b === "safe-deals" && !c) return list(DEALS);
  if (a === "me" && b === "safe-deals" && c) return one(DEALS.find((x) => x.id === c) || DEALS[0]);

  if (a === "conversations" && !b) return list(CONVERSATIONS);
  if (a === "conversations" && b && c === "messages") return list(MESSAGES.filter((m) => m.conversation_id === b));
  if (a === "conversations" && b && c === "read") return { data: { ok: true } };

  if (a === "project-drafts" && !b) return { draft_token: "draft-demo" };
  if (a === "project-drafts" && b && c === "claim") return role === "guest" ? nf(401) : { data: { id: "prj-draft-demo" } };
  if (a === "project-drafts" && b) return { data: { raw_input: { text: "" }, normalized_data: BRIEF } };
  if (a === "ai" && (b === "project-brief" || b === "project-import")) return { data: BRIEF };

  if (a === "admin") {
    if (b === "dashboard") return { data: { users_total: 1284, users_new_7d: 37, projects_open: 41, projects_active: 96, pending_reputation: 5, open_reports: 3, open_fraud_signals: 2, open_disputes: ADMIN_DISPUTES.length, active_safe_deals: 18, services_active: 213, vacancies_published: 27, recent_admin_actions: 12 } };
    if (b === "users" && !c) return { data: bySearch(ADMIN_USERS, query.q), page: { has_more: false } };
    if (b === "users" && c) return one(ADMIN_USERS.find((u) => u.id === c) || ADMIN_USERS[0]);
    if (b === "safe-deals") return { data: ADMIN_DEALS };
    if (b === "disputes") return { data: ADMIN_DISPUTES, page: { has_more: false } };
    // Unmodeled admin surfaces: details -> null, collections -> empty (with page so pagers don't crash).
    return c && /[0-9a-f-]{6,}/.test(c) ? { data: null } : { data: [], page: { has_more: false } };
  }

  return null; // signal: unknown -> permissive fallback
}

const server = createServer((req, res) => {
  const url = new URL(req.url, "http://localhost");
  const query = Object.fromEntries(url.searchParams.entries());
  res.setHeader("Content-Type", "application/json; charset=utf-8");
  res.setHeader("Cache-Control", "no-store");

  if (req.method === "OPTIONS") { res.statusCode = 204; return res.end(); }
  if (url.pathname.startsWith("/health")) { res.statusCode = 200; return res.end(JSON.stringify({ status: "ok" })); }

  if (!url.pathname.startsWith("/api/v1/")) { res.statusCode = 404; return res.end(JSON.stringify(nf())); }

  let body;
  try { body = route(url.pathname, query); } catch (e) { res.statusCode = 500; return res.end(JSON.stringify({ error: { code: "internal", message: String(e) } })); }

  if (body === null) {
    // Permissive fallback for endpoints not yet modelled: never 500 the UI.
    const looksCollection = !/[0-9a-f-]{6,}$/.test(url.pathname) && !url.pathname.match(/\/(me|session|summary)$/);
    res.statusCode = 200;
    return res.end(JSON.stringify(looksCollection ? list([]) : { data: null, meta: {} }));
  }
  if (body.__status) { res.statusCode = body.__status; delete body.__status; return res.end(JSON.stringify(body)); }
  res.statusCode = 200;
  res.end(JSON.stringify(body));
});

server.listen(PORT, "127.0.0.1", () => console.log(`[fixture-api] listening on http://127.0.0.1:${PORT} (role=${role})`));
