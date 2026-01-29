# AnimeEnigma

[🇬🇧 English version](README.md)

Самохостируемая платформа для просмотра аниме с интеграцией MAL/Shikimori. Построена как монорепозиторий Go микросервисов с Vue 3 фронтендом.

**Целевая аудитория**: Самостоятельный хостинг для небольших групп (CDN не требуется).

## Возможности

- 🎬 **Гибридный стриминг** - Просмотр через внешние API (Kodik, Aniboom) или собственное хранилище MinIO
- 🔍 **Каталог по запросу** - Данные об аниме загружаются из Shikimori в реальном времени при поиске
- 🎮 **Мультиплеерная игра** - Угадывание опенингов/эндингов в реальном времени через WebSocket
- 📊 **Отслеживание прогресса** - История просмотров, списки аниме, синхронизация позиции воспроизведения
- 🔐 **Аутентификация** - JWT авторизация с ролевым доступом (пользователь/админ)

## Архитектура

```
┌─────────────┐                         ┌──────────────┐
│  Фронтенд   │◄───── REST/GraphQL ────►│   Gateway    │
│   (Vue 3)   │                         └──────┬───────┘
└──────┬──────┘                                │
       │                     ┌─────────────────┼─────────────────┐
       │                     │                 │                 │
       │               ┌─────▼─────┐     ┌─────▼─────┐     ┌─────▼─────┐
       │               │   Auth    │     │  Catalog  │     │ Streaming │
       │               └───────────┘     │(Shikimori)│     │  (Proxy)  │
       │                                 └─────┬─────┘     └─────┬─────┘
       │                                       │                 │
       │    ┌──────────────────────────────────┘                 │
       │    │                                                    │
       │    ▼                                                    ▼
       │ ┌──────────┐   ┌──────────┐                    ┌──────────────┐
       │ │ Shikimori│   │  Kodik   │                    │    MinIO     │
       │ │   API    │   │   API    │                    │  (загрузки)  │
       │ └──────────┘   └────┬─────┘                    └──────────────┘
       │                     │
       └─────── iframe ──────┘ (прямое воспроизведение)
              ИЛИ
       └─── proxy stream ────► Streaming Service ────► Aniboom API
```

### Потоки видео

Видео получается тремя способами:

1. **Iframe (Kodik)** - Фронтенд встраивает плеер Kodik напрямую
2. **Проксированный поток (Aniboom)** - Бэкенд проксирует HLS потоки для обхода CORS
3. **Собственное хранилище (MinIO)** - Загруженные админом видео из MinIO

### Каталог по запросу

База данных аниме **НЕ заполняется заранее**. Вместо этого:

1. Пользователь ищет аниме во фронтенде
2. Сервис каталога запрашивает Shikimori GraphQL API
3. Результаты сопоставляются по **японскому названию** как первичному ключу
4. Метаданные аниме сохраняются в PostgreSQL для будущих запросов
5. Источники видео определяются через Kodik/Aniboom по совпадению названия

## Быстрый старт

### Требования

- Go 1.22+
- Node.js 20+
- Docker & Docker Compose
- Make

### Разработка

1. **Запустить инфраструктуру:**
   ```bash
   make dev
   ```

2. **Запустить бэкенд сервисы:**
   ```bash
   # В отдельных терминалах или через docker compose
   cd services/auth && go run ./cmd/auth-api
   cd services/catalog && go run ./cmd/catalog-api
   # ... и т.д.
   ```

3. **Запустить фронтенд:**
   ```bash
   cd frontend/web
   npm install
   npm run dev
   ```

### С Docker Compose

```bash
# Запустить всё
docker compose -f docker/docker-compose.yml up -d

# Просмотр логов
docker compose -f docker/docker-compose.yml logs -f

# Остановить
docker compose -f docker/docker-compose.yml down
```

## Структура проекта

```
animeenigma/
├── services/           # Go микросервисы
│   ├── auth/           # Сервис аутентификации
│   ├── catalog/        # Каталог аниме с интеграцией Shikimori
│   ├── streaming/      # Сервис стриминга/прокси видео
│   ├── player/         # Прогресс просмотра и списки
│   ├── rooms/          # Игровые комнаты и WebSocket
│   ├── scheduler/      # Фоновые задачи
│   └── gateway/        # API шлюз
│
├── frontend/
│   └── web/            # Vue 3 SPA
│
├── libs/               # Общие Go библиотеки
│   ├── logger/         # Структурированное логирование
│   ├── errors/         # Обработка ошибок
│   ├── cache/          # Redis кэширование
│   ├── database/       # PostgreSQL соединения
│   ├── authz/          # JWT аутентификация
│   ├── httputil/       # HTTP утилиты
│   ├── pagination/     # Пагинация
│   ├── animeparser/    # Парсеры видео источников (Kodik, Aniboom)
│   ├── videoutils/     # Работа с видео и MinIO
│   └── tracing/        # OpenTelemetry
│
├── api/                # API контракты
│   ├── openapi/        # OpenAPI спецификации
│   ├── proto/          # Protobuf определения
│   ├── graphql/        # GraphQL схема
│   └── events/         # CloudEvents для асинхронных сообщений
│
├── docker/             # Docker Compose для локальной разработки
├── deploy/             # Kubernetes конфигурации
│   └── kustomize/
├── infra/              # Helm чарты
│   └── helm/
└── scripts/            # Скрипты сборки и утилиты
```

## Сервисы

| Сервис | Порт | Описание |
|--------|------|----------|
| Gateway | 8000 | API шлюз, rate limiting, маршрутизация |
| Auth | 8080 | Аутентификация и управление пользователями |
| Catalog | 8081 | Каталог аниме, интеграция с Shikimori |
| Streaming | 8082 | Стриминг/прокси видео |
| Player | 8083 | Прогресс просмотра и списки аниме |
| Rooms | 8084 | Игровые комнаты и WebSocket |
| Scheduler | 8085 | Фоновые задачи |
| Frontend | 3000 | Vue 3 SPA |

## Конфигурация

Сервисы настраиваются через переменные окружения. См. `internal/config/config.go` каждого сервиса.

### Основные сервисы

| Переменная | Описание | По умолчанию |
|------------|----------|--------------|
| `JWT_SECRET` | Секрет для подписи JWT | - |
| `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME` | PostgreSQL | localhost:5432 |
| `REDIS_HOST`, `REDIS_PORT` | Redis | localhost:6379 |
| `MINIO_ENDPOINT`, `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY` | MinIO хранилище | localhost:9000 |

### Провайдеры видео

| Переменная | Описание | Обязательно |
|------------|----------|-------------|
| `KODIK_API_KEY` | API ключ Kodik для поиска видео | Для поддержки Kodik |
| `KODIK_BASE_URL` | Базовый URL API Kodik | `https://kodikapi.com` |
| `ANIBOOM_BASE_URL` | Базовый URL API Aniboom | Для поддержки Aniboom |
| `SHIKIMORI_CLIENT_ID` | OAuth client ID Shikimori | Опционально |
| `SHIKIMORI_CLIENT_SECRET` | OAuth секрет Shikimori | Опционально |

### Пример `.env`

```env
# База данных
DB_HOST=localhost
DB_PORT=5432
DB_USER=animeenigma
DB_PASSWORD=secret
DB_NAME=animeenigma

# Redis
REDIS_HOST=localhost
REDIS_PORT=6379

# MinIO (для загрузок админа)
MINIO_ENDPOINT=localhost:9000
MINIO_ACCESS_KEY=minioadmin
MINIO_SECRET_KEY=minioadmin
MINIO_BUCKET=animeenigma

# Аутентификация
JWT_SECRET=your-super-secret-key

# Провайдеры видео
KODIK_API_KEY=your-kodik-api-key
# ANIBOOM_BASE_URL=https://api.aniboom.one
```

## Разработка

```bash
# Запустить все Go тесты
make test

# Линтинг Go кода
make lint

# Собрать все сервисы
make build

# Сгенерировать API код
make generate

# Собрать Docker образы
make docker-build
```

## Деплой

### Kubernetes с Kustomize

```bash
# Деплой в dev
kubectl apply -k deploy/kustomize/overlays/dev

# Деплой в prod
kubectl apply -k deploy/kustomize/overlays/prod
```

### Helm

```bash
cd infra/helm
helm install animeenigma ./gateway -f gateway/values.yaml
```

## Документация API

- OpenAPI спецификации: `api/openapi/`
- GraphQL схема: `api/graphql/schema.graphql`
- Proto определения: `api/proto/`

## Мультиплеерная игра

AnimeEnigma включает игру по угадыванию опенингов/эндингов:

1. Создайте комнату с настройками (количество раундов, время, режим)
2. Пригласите друзей по ссылке
3. Видео опенинга/эндинга проигрывается
4. Игроки вводят название аниме
5. Очки начисляются за скорость и правильность ответа
6. Глобальная и сессионная таблица лидеров

## Устаревшая настройка

Оригинальный NestJS бэкенд и Express rooms бэкенд сохранены в `services/backend/` и `services/roomsBackend/` для справки во время миграции.

## Лицензия

MIT
