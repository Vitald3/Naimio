ALTER TABLE project_drafts DROP CONSTRAINT project_drafts_source_type_check;
ALTER TABLE project_drafts ADD CONSTRAINT project_drafts_source_type_check
  CHECK (source_type IN ('AI_BRIEF','IMPORT','COMMERCIAL_OFFER','CALCULATOR','MANUAL'));

CREATE TABLE calculator_definitions (
  id uuid PRIMARY KEY,
  slug varchar(160) NOT NULL CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
  title varchar(220) NOT NULL CHECK (char_length(title) BETWEEN 1 AND 220),
  category_id uuid REFERENCES categories(id),
  version int NOT NULL DEFAULT 1 CHECK (version > 0),
  schema jsonb NOT NULL,
  pricing_config jsonb NOT NULL,
  enabled boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (slug, version)
);
CREATE UNIQUE INDEX calculator_definitions_enabled_slug_unique ON calculator_definitions(slug) WHERE enabled=true;

INSERT INTO calculator_definitions(id,slug,title,version,schema,pricing_config,enabled) VALUES
('81818181-8181-4818-8818-818181818181','telegram-bot','Стоимость Telegram-бота',1,
 '{"intro":"Оцените разработку Telegram-бота по функциям и интеграциям.","questions":[{"key":"complexity","label":"Сценарий","type":"SELECT","required":true,"options":[{"value":"basic","label":"Простой информационный"},{"value":"workflow","label":"Бизнес-процесс"},{"value":"commerce","label":"Продажи и оплаты"}]},{"key":"integrations","label":"Количество интеграций","type":"NUMBER","required":true,"min":0,"max":10},{"key":"admin","label":"Нужна панель управления","type":"BOOLEAN","required":true}]}'::jsonb,
 '{"baseline_min_kopecks":12000000,"baseline_max_kopecks":22000000,"duration_min_days":10,"duration_max_days":25,"option_basis_points":{"complexity":{"basic":0,"workflow":5000,"commerce":9000}},"number_basis_points":{"integrations":1200},"boolean_basis_points":{"admin":2500},"category_slug":"development","skill_slugs":["telegram","backend"],"assumptions":["Готовый контент предоставляет заказчик","Инфраструктура оплачивается отдельно"]}'::jsonb,true),
('82828282-8282-4828-8828-828282828282','landing-page','Стоимость лендинга',1,
 '{"intro":"Рассчитайте диапазон стоимости лендинга без ложной точности.","questions":[{"key":"design","label":"Дизайн","type":"SELECT","required":true,"options":[{"value":"template","label":"На основе шаблона"},{"value":"custom","label":"Индивидуальный"}]},{"key":"sections","label":"Количество смысловых блоков","type":"NUMBER","required":true,"min":3,"max":20},{"key":"copywriting","label":"Нужны тексты","type":"BOOLEAN","required":true}]}'::jsonb,
 '{"baseline_min_kopecks":8000000,"baseline_max_kopecks":16000000,"duration_min_days":7,"duration_max_days":18,"option_basis_points":{"design":{"template":0,"custom":6000}},"number_basis_points":{"sections":350},"boolean_basis_points":{"copywriting":2500},"category_slug":"development","skill_slugs":["frontend","design"],"assumptions":["Одна языковая версия","Без сложного личного кабинета"]}'::jsonb,true),
('83838383-8383-4838-8838-838383838383','seo','Стоимость SEO-продвижения',1,
 '{"intro":"Оцените стартовый диапазон SEO-работ по размеру сайта и конкуренции.","questions":[{"key":"site_size","label":"Размер сайта","type":"SELECT","required":true,"options":[{"value":"small","label":"До 50 страниц"},{"value":"medium","label":"50–500 страниц"},{"value":"large","label":"Более 500 страниц"}]},{"key":"competition","label":"Конкуренция","type":"SELECT","required":true,"options":[{"value":"low","label":"Низкая"},{"value":"medium","label":"Средняя"},{"value":"high","label":"Высокая"}]},{"key":"content","label":"Нужно производство контента","type":"BOOLEAN","required":true}]}'::jsonb,
 '{"baseline_min_kopecks":10000000,"baseline_max_kopecks":24000000,"duration_min_days":30,"duration_max_days":90,"option_basis_points":{"site_size":{"small":0,"medium":3500,"large":8000},"competition":{"low":0,"medium":3000,"high":7000}},"boolean_basis_points":{"content":3500},"category_slug":"marketing","skill_slugs":["seo"],"assumptions":["Диапазон за первый этап работ","Рекламный бюджет не включён"]}'::jsonb,true);

CREATE TABLE acquisition_events (
  id uuid PRIMARY KEY,
  anonymous_id uuid,
  user_id uuid REFERENCES users(id),
  event_type varchar(100) NOT NULL CHECK (event_type IN ('LANDING_VIEW','HOMEPAGE_TASK_STARTED','GUEST_PROJECT_ANALYSIS_COMPLETED','CALCULATOR_STARTED','CALCULATOR_COMPLETED','COMMERCIAL_OFFER_ANALYZED','REGISTRATION_COMPLETED','PROJECT_DRAFT_CLAIMED','PROJECT_PUBLISHED','PROPOSAL_RECEIVED','PROPOSAL_ACCEPTED','PROJECT_COMPLETED')),
  landing_path text CHECK (landing_path IS NULL OR char_length(landing_path) <= 500),
  utm_source varchar(160),
  utm_medium varchar(160),
  utm_campaign varchar(200),
  utm_content varchar(200),
  referral_code varchar(160),
  metadata jsonb NOT NULL DEFAULT '{}',
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK (anonymous_id IS NOT NULL OR user_id IS NOT NULL)
);
CREATE INDEX acquisition_events_type_created_idx ON acquisition_events(event_type,created_at DESC);
CREATE INDEX acquisition_events_user_created_idx ON acquisition_events(user_id,created_at DESC) WHERE user_id IS NOT NULL;
CREATE INDEX acquisition_events_anonymous_created_idx ON acquisition_events(anonymous_id,created_at DESC) WHERE anonymous_id IS NOT NULL;
