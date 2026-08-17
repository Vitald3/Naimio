# Naimio Production Deployment Guide

## Production Deploy Checklist

Эта инструкция описывает перенос Naimio на production VPS.

Архитектура:

-   Go API
-   Worker
-   Next.js Web
-   PostgreSQL
-   Redis
-   Nginx на host-системе
-   Docker Compose
-   `.env.production`

------------------------------------------------------------------------

# 0. Подготовка локально

## Проверка состояния Git

``` bash
git status
```

Рабочая директория должна быть чистой.

## Запуск проверок

Минимальные проверки:

``` bash
make test
make lint
make build
```

Полная проверка:

``` bash
make checkpoint-mvp
```

## Отправка изменений

``` bash
git add .
git commit -m "description"
git push
```

------------------------------------------------------------------------

# 1. Подключение к серверу

``` bash
ssh naimio@SERVER_IP
```

Перейти в проект:

``` bash
cd /opt/freelance
```

------------------------------------------------------------------------

# 2. Backup перед обновлением

Перед каждым production deploy делать backup базы.

``` bash
sudo docker compose --env-file .env.production exec postgres \
pg_dump -U freelance freelance > backup_$(date +%Y-%m-%d_%H-%M).sql
```

Проверить:

``` bash
ls -lh backup*
```

------------------------------------------------------------------------

# 3. Обновление кода

Получить свежий код:

``` bash
git pull
```

Проверить:

``` bash
git status
```

------------------------------------------------------------------------

# 4. Проверка production env

Файл:

``` text
.env.production
```

Обязательно проверить:

``` env
APP_ENV=production

PUBLIC_BASE_URL=https://naimio.ru

POSTGRES_PASSWORD=
REDIS_PASSWORD=

OBJECT_STORAGE_ENDPOINT=
OBJECT_STORAGE_REGION=
OBJECT_STORAGE_BUCKET=
OBJECT_STORAGE_ACCESS_KEY=
OBJECT_STORAGE_SECRET_KEY=

STORAGE_MASTER_KEY=
PAYMENT_CONFIG_MASTER_KEY=

SMTP_ADDRESS=
SMTP_FROM=
```

------------------------------------------------------------------------

# 5. Проверка Docker Compose

Перед запуском:

``` bash
ln -s docker-compose.production.yml docker-compose.yml
sudo docker compose \
--env-file .env.production \
config
```

Не должно быть ошибок.

------------------------------------------------------------------------

# 6. Production deploy

Основная команда:

``` bash
make prod-deploy
```

Команда выполняет:

1.  Миграции базы данных

``` bash
docker compose --env-file .env.production --profile migration run --rm migrate
```

2.  Сборку и запуск контейнеров

``` bash
docker compose --env-file .env.production up -d --build
```

3.  Создание администратора

```bash
sudo docker compose --env-file .env.production build api create-admin
docker compose --env-file .env.production run --rm create-admin
```

4.  Проверку состояния:

``` bash
docker compose --env-file .env.production ps
```

------------------------------------------------------------------------

# 7. Проверка контейнеров

Ожидаем:

``` text
api        Up (healthy)
web        Up (healthy)
worker     Up (healthy)
postgres   Up (healthy)
redis      Up (healthy)
```

Команда:

``` bash
sudo docker compose --env-file .env.production ps
```

------------------------------------------------------------------------

# 8. Логи при ошибках

API:

``` bash
sudo docker compose --env-file .env.production logs api --tail=200
```

Worker:

``` bash
sudo docker compose --env-file .env.production logs worker --tail=200
```

Web:

``` bash
sudo docker compose --env-file .env.production logs web --tail=200
```

------------------------------------------------------------------------

# 9. Проверка сервисов

## API

``` bash
curl http://127.0.0.1:8080/health/ready
```

Ожидается успешный ответ.

## Web

``` bash
curl http://127.0.0.1:3000
```

------------------------------------------------------------------------

# 10. Nginx

После изменения конфигурации:

``` bash
sudo nginx -t
```

Если проверка успешна:

``` bash
sudo systemctl reload nginx
```

------------------------------------------------------------------------

# 11. Проверка сайта

Открыть:

    https://naimio.ru

Проверить:

-   главную страницу
-   регистрацию
-   авторизацию
-   API запросы
-   загрузку файлов
-   WebSocket чат
-   админку

------------------------------------------------------------------------

# 12. Мониторинг ресурсов

Память:

``` bash
free -h
```

Docker:

``` bash
sudo docker stats
```

Диск:

``` bash
df -h
```

------------------------------------------------------------------------

# 13. Очистка Docker

Проверить образы:

``` bash
docker images
```

Удаление мусора:

``` bash
sudo docker system prune
```

Не выполнять сразу после деплоя.

------------------------------------------------------------------------

# 14. Откат версии

Посмотреть историю:

``` bash
git log --oneline
```

Перейти на старый commit:

``` bash
git checkout COMMIT_HASH
```

Пересобрать:

``` bash
make prod-deploy
```

------------------------------------------------------------------------

# Быстрый production deploy

Для обычного обновления:

``` bash
ssh naimio@SERVER_IP

cd /opt/freelance

git pull

sudo docker compose --env-file .env.production exec postgres \
pg_dump -U freelance freelance > backup.sql

make prod-deploy

sudo docker compose --env-file .env.production ps

curl http://127.0.0.1:8080/health/ready
```

------------------------------------------------------------------------

# Production структура

    /opt/freelance

    ├── docker-compose.yml
    ├── .env.production
    ├── db/
    │   └── migrations/
    ├── scripts/
    │   └── migrate.sh
    └── backups/

------------------------------------------------------------------------

# Важные правила

-   Не запускать тесты на production.
-   Не использовать `SAFE_DEAL_PROVIDER=sandbox`.
-   Не хранить платежные секреты в worker.
-   Не открывать PostgreSQL и Redis наружу.
-   Перед каждым обновлением делать backup базы.
-   Проверять healthcheck после deploy.
