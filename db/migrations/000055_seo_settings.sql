-- 000055_seo_settings.sql
-- SEO global settings, page overrides, entity templates, and IndexNow configuration

INSERT INTO feature_flags (
    key,
    enabled,
    config,
    description,
    updated_at
) VALUES (
    'seo_settings',
    true,
    '{
        "general": {
            "title_template": "%s — Naimio",
            "default_title": "Naimio — Маркетплейс проверенных фрилансеров и цифровых услуг",
            "default_description": "Биржа фриланса Naimio. Проверенные исполнители, безопасная сделка, прозрачные цены, каталог IT-услуг и вакансий.",
            "default_og_image": "/media/covers/cover-01.svg",
            "canonical_base_url": "https://naimio.ru",
            "robots_policy": "INDEX_FOLLOW",
            "custom_robots_txt": "",
            "schema_organization_name": "Naimio",
            "schema_legal_name": "ООО «Наймио»",
            "schema_support_email": "support@naimio.ru",
            "schema_support_phone": "+7 (495) 000-00-00"
        },
        "pages": {
            "/": {
                "title": "Naimio — Маркетплейс проверенных фрилансеров и услуг",
                "description": "Найдите лучших специалистов для бизнеса: разработка, дизайн, маркетинг, аналитика. Безопасная сделка и гарантия результата.",
                "no_index": false
            },
            "/categories": {
                "title": "Категории и направления услуг | Naimio",
                "description": "Полный каталог категорий специалистов и услуг на бирже Naimio: IT, разработка, дизайн, маркетинг, AI и маркетплейсы.",
                "no_index": false
            },
            "/freelancers": {
                "title": "Каталог проверенных специалистов и фрилансеров | Naimio",
                "description": "Специалисты с подтверждённым опытом и отзывами. Фильтры по стеку, рейтингу, категориям и занятости.",
                "no_index": false
            },
            "/services": {
                "title": "Каталог услуг и готовых предложений | Naimio",
                "description": "Заказ услуг с фиксированной ценой и сроками: разработка сайтов, ботов, дизайн, аудит и консультации.",
                "no_index": false
            },
            "/projects": {
                "title": "Открытые проекты и заказы для фрилансеров | Naimio",
                "description": "Актуальные заказы для IT-специалистов. Откликайтесь на проекты с безопасной сделкой и прямым контрактом.",
                "no_index": false
            },
            "/vacancies": {
                "title": "Вакансии и предложения работы | Naimio",
                "description": "Вакансии в продуктовых компаниях и стартапах. Удалённая работа и офис, проверенные работодатели.",
                "no_index": false
            },
            "/education": {
                "title": "Обучение, менторинг и консультации | Naimio",
                "description": "Индивидуальный менторинг, код-ревью и консультации от ведущих практиков рынка.",
                "no_index": false
            },
            "/check-offer": {
                "title": "Проверить коммерческое предложение онлайн | Naimio",
                "description": "Бесплатный разбор КП: оценка адекватности стоимости, рисков и состава работ.",
                "no_index": false
            },
            "/price": {
                "title": "Калькуляторы стоимости IT-услуг | Naimio",
                "description": "Рассчитайте ориентировочный бюджет на разработку Telegram-бота, лендинга или SEO-продвижения.",
                "no_index": false
            },
            "/blog": {
                "title": "Блог Naimio — статьи о фрилансе, разработке и бизнесе",
                "description": "Практические руководства, аналитика рынка, советы заказчикам и кейсы экспертов.",
                "no_index": false
            },
            "/pro": {
                "title": "PRO-подписка для фрилансеров | Naimio",
                "description": "Получайте в 3 раза больше заказов, PRO-значок в каталоге и доступ к закрытым проектам.",
                "no_index": false
            }
        },
        "templates": {
            "category": {
                "title_template": "{category} — фрилансеры и услуги | Naimio",
                "description_template": "Специалисты и услуги в категории {category}. Заказывайте работы с гарантией безопасной сделки на Naimio."
            },
            "freelancer": {
                "title_template": "{name} — {specialty} | Naimio",
                "description_template": "Профиль специалиста {name}. Рейтинг {rating}, примеры работ, отзывы и прямой заказ услуг на Naimio."
            },
            "service": {
                "title_template": "{service_title} — заказать от {price} | Naimio",
                "description_template": "Услуга: {service_title}. Исполнитель {name}. Срок выполнения от {duration} дн. Безопасная сделка на Naimio."
            },
            "project": {
                "title_template": "{project_title} — проект на Naimio",
                "description_template": "Заказ: {project_title}. Бюджет {budget}. Приём откликов специалистов на бирже Naimio."
            },
            "vacancy": {
                "title_template": "Вакансия {job_title} | Naimio",
                "description_template": "Открыта вакансия {job_title}. Условия, требования и прямой отклик на Naimio."
            },
            "calculator": {
                "title_template": "{calculator_title} — онлайн расчет стоимости | Naimio",
                "description_template": "Калькулятор расчета стоимости: {calculator_title}. Быстрая оценка бюджета и сроков на Naimio."
            },
            "blog": {
                "title_template": "{post_title} | Блог Naimio",
                "description_template": "{excerpt} Читайте полную статью на Naimio."
            }
        },
        "indexnow": {
            "enabled": true,
            "api_key": "naimio-indexnow-production-key-2026",
            "key_location": "https://naimio.ru/naimio-indexnow-production-key-2026.txt",
            "auto_submit": true,
            "host": "naimio.ru"
        }
    }'::jsonb,
    'SEO global settings, page overrides, dynamic title/description templates, and IndexNow configuration.',
    now()
)
ON CONFLICT (key) DO UPDATE SET
    config = COALESCE(feature_flags.config, EXCLUDED.config),
    updated_at = now();
