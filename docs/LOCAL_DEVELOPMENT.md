# Локальная разработка

## Доступ с устройств в домашней Wi-Fi сети

Узнайте IPv4-адрес Mac в настройках Wi-Fi и добавьте в `.env`:

```dotenv
local_ip_address=192.168.1.23
BIND_ADDRESS=0.0.0.0
```

В режиме разработки `local_ip_address` (также поддерживается `LOCAL_IP_ADDRESS`) имеет приоритет над локальным `PUBLIC_BASE_URL` для ссылок приложения. `BIND_ADDRESS=0.0.0.0` нужен отдельно: он публикует порт Docker на все сетевые интерфейсы Mac, чтобы сайт открывался с других устройств той же Wi-Fi сети по `http://192.168.1.23:8088`. При первом запуске macOS может запросить разрешение принимать входящие подключения для Docker — его нужно разрешить. Не используйте этот режим в публичной или недоверенной сети.

После запуска контейнеров и миграций заполните базу демонстрационными данными:

```bash
make dev-seed
```

Команда работает только с `APP_ENV=development`, использует детерминированные идентификаторы и безопасна для повторного запуска.

## Тестовые аккаунты

Пароль для всех аккаунтов: `LocalDemo2026!`

| Роль | Логин | Сценарий |
|---|---|---|
| Заказчик | `customer@example.test` | проекты, отклики, сообщения, уведомления, «Моя команда», избранное и Safe Deal |
| Исполнитель | `freelancer@example.test` | публичный профиль, проекты, отклики, репутация, услуги, сообщения и Safe Deal |
| Администратор | `admin@example.test` | ручной подбор, реферальные правила, вакансии и споры Safe Deal |
| Модератор | `moderator@example.test` | проверка ролевых ограничений и очередей модерации |

Дополнительные профили используют адреса `flutter@example.test`, `go@example.test`, `designer@example.test`, `ml@example.test`, `seo@example.test` и другие адреса из домена `example.test` с тем же локальным паролем.

Данные предназначены только для локальной проверки. Sandbox Safe Deal не выполняет реальных платежей.

## Demo accounts for manual QA

All deterministic development accounts use the password `LocalDemo2026!` and are created only when `APP_ENV=development`.

- Customer: `customer@example.test`
- Freelancer: `freelancer@example.test`
- Moderator: `moderator@example.test`
- Admin: `admin@example.test`
- Super admin: `superadmin@example.test`
- Suspended fixture: `suspended@example.test`
- Banned fixture: `banned@example.test`

The seed also creates public freelancers, services, projects, vacancies, conversations, notifications, reputation states, Safe Deal states, reports, fraud signals, feature flags and audit entries. Running `make dev-seed` repeatedly is intended to be idempotent.

## Готовые сценарии после `make dev-seed`

- Админка: `http://localhost:8088/admin` — пользователи, категории/навыки, репутация, жалобы, fraud-сигналы, проекты, услуги, вакансии, отзывы, feature flags, аудит, matching, Safe Deal и споры.
- Принятое приглашение исполнителя: токен `demo-project-invite` (полезен для проверки идемпотентного повторного открытия).
- Неиспользованное приглашение заказчика: токен `demo-customer-invite`.
- В базе есть заявки на вакансии в статусах `SUBMITTED`, `VIEWED`, `SHORTLISTED`.
- Есть Safe Deal в состояниях `AWAITING_FUNDING`, `IN_PROGRESS`, `SUBMITTED`, `REVISION_REQUESTED`, `DISPUTED`, `COMPLETED`.
- Есть очередь ручной проверки внешней репутации, жалобы и fraud-сигналы для модератора/администратора.

Для проверки разделения ролей используйте `moderator@example.test`, `admin@example.test` и `superadmin@example.test` — пароль у всех `LocalDemo2026!`.

## Первый запуск из чистого архива

Не распаковывайте новую поставку поверх старой директории проекта: Finder может объединить содержимое и оставить устаревшие исходники. Распакуйте ZIP в новую пустую папку.

```bash
cp .env.example .env
# задайте локальные POSTGRES_PASSWORD, REDIS_PASSWORD, SMTP_ADDRESS/SMTP_FROM
make dev
```

`make dev` теперь сначала поднимает PostgreSQL/Redis и применяет миграции, затем собирает и запускает приложение.

В отдельном терминале:

```bash
make dev-seed
```

`make dev-seed` пересобирает seed-бинарник перед запуском, поэтому устаревший Docker image больше не может скрыть ошибку компиляции текущего исходного кода.

## Проверка БД

Обычный локальный сценарий не требует устанавливать `psql` и вручную задавать `DATABASE_URL`:

```bash
make test-db
```

Если `DATABASE_URL` не задан, скрипт создаёт отдельный временный PostgreSQL-контейнер и отдельную Docker network, применяет миграции с нуля, запускает DB-инварианты и Go integration tests, после чего удаляет временную БД. Основная локальная база `freelance` не затрагивается.

Если `DATABASE_URL` задан явно, он по-прежнему обязан указывать на пустую disposable БД; в этом режиме используется локальный `psql`.

### Миграции и существующая локальная БД

Миграции теперь ведутся через таблицу `schema_migrations` и повторный `make migrate` не переисполняет уже применённые файлы. Для БД, созданной предыдущей версией проекта без таблицы миграций, скрипт автоматически записывает baseline только если обнаруживает полный актуальный schema-набор. Частично мигрированную старую БД он намеренно не пытается «угадывать».

Если локальные данные не нужны и база оказалась частично мигрированной:

```bash
make dev-reset
make dev
make dev-seed
```

`make dev-reset` удаляет локальные Docker volumes, поэтому не используйте его для базы, которую нужно сохранить.
