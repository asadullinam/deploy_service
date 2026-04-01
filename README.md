<div align="center">

# Deploy Service

**Платформа для развертывания приложений в Kubernetes без DevOps-экспертизы.**

Подключите GitHub-репозиторий, настройте сервис и выпускайте релизы в один клик.

[Быстрый старт](#быстрый-старт) &bull; [Архитектура](#архитектура) &bull; [API](#api)

</div>

---

## Обзор

Сайт сервиса: https://158.160.241.48.sslip.io/

Deploy Service - платформа, которая автоматизирует полный путь от исходного кода до работающего приложения в Kubernetes:

```
GitHub-репозиторий + Dockerfile  -->  Изолированная среда в Kubernetes  -->  Работающее приложение с публичным URL
```

**Основные возможности:**

- **Изоляция проектов** - каждый проект получает собственный [vcluster](https://www.vcluster.com/) с сетевыми политиками, RBAC и выделенным kubeconfig
- **Интеграция с GitHub CI/CD** - автоматически генерирует GitHub Actions workflow и манифесты Kubernetes, открывает PR в репозитории
- **Встроенная веб-консоль** - управление проектами, релизами, логами и доступами из браузера
- **Оплата по потреблению** - учет CPU, памяти и хранилища по каждому проекту с настраиваемыми лимитами расходов
- **Откат релизов** - возврат к любому предыдущему успешному релизу в один клик
- **Наблюдаемость** - интеграция с Prometheus, Loki и Grafana
- **Уведомления** - Telegram-бот для событий деплоя и биллинговых оповещений

## Технологии

| Слой            | Технология                                     |
| --------------- | ---------------------------------------------- |
| Бэкенд          | Go 1.25, HTTP-сервер на стандартной библиотеке |
| База данных     | PostgreSQL 16                                  |
| Фронтенд        | Vanilla JS/CSS (встроен в Go-бинарник)         |
| Kubernetes      | kubectl v1.35, vcluster v0.31                  |
| CI/CD           | GitHub Actions (генерируется автоматически)    |
| Платежи         | ЮKassa                                         |
| Уведомления     | Telegram Bot API                               |
| Наблюдаемость   | Prometheus, Loki, Grafana                      |
| Контейнеризация | Docker, multi-stage сборка (~50 MB образ)      |

## Быстрый старт

### Требования

- Docker и Docker Compose
- Go 1.25+ (для локальной разработки без Docker)
- Node.js (для форматирования кода и git-хуков)

### Запуск через Docker Compose

```bash
# Клонировать репозиторий
git clone https://github.com/<your-org>/deploy_service.git
cd deploy_service

# Скопировать шаблон переменных окружения
cp .env.example .env

# Запустить PostgreSQL + Deploy Service
docker-compose up
```

Веб-консоль будет доступна по адресу **http://localhost:8080**.

### Локальная разработка

```bash
cd deploy-service

# Установить зависимости
go mod download

# Запуск с in-memory хранилищами и мок-интеграциями
KUBERNETES_PROVISIONER=mock GITHUB_AUTOMATION_MODE=mock go run ./cmd/server
```

### Конфигурация

Настройка выполняется через переменные окружения. Полный список - в [`.env.example`](.env.example).

Основные переменные:

| Переменная               | Описание                        | По умолчанию |
| ------------------------ | ------------------------------- | ------------ |
| `HTTP_ADDRESS`           | Адрес сервера                   | `:8080`      |
| `DATABASE_URL`           | Строка подключения к PostgreSQL | -            |
| `JWT_SECRET`             | Ключ подписи токенов            | -            |
| `ENCRYPTION_KEY`         | Ключ шифрования kubeconfig      | -            |
| `KUBERNETES_PROVISIONER` | `kubectl` (реальный) или `mock` | `mock`       |
| `GITHUB_AUTOMATION_MODE` | `real` или `mock`               | `mock`       |
| `APPS_BASE_DOMAIN`       | Базовый домен для приложений    | -            |

## Архитектура

Проект построен по принципу **гексагональной архитектуры** (Ports & Adapters). Правило зависимостей: адаптеры зависят от сервисного слоя, сервисный слой - от домена, но не наоборот.

```
cmd/server/main.go                          <- точка входа
  └── internal/app/                         <- composition root
        ├── internal/domain/                <- модели (Project, Release, Billing)
        ├── internal/service/               <- бизнес-логика + интерфейсы портов
        ├── internal/http/                  <- HTTP-обработчики, роутер, встроенный веб-интерфейс
        ├── internal/store/
        │     ├── memory/                   <- in-memory реализации (dev/тесты)
        │     └── postgres/                 <- продакшн-хранилище
        └── internal/integration/
              ├── kubernetes/               <- провизионер vcluster + метрики
              ├── github/                   <- вебхуки, создание PR
              ├── yookassa/                 <- обработка платежей
              ├── telegram/                 <- уведомления
              └── logs/                     <- агрегация логов через Loki
```

### Основной процесс

1. **Регистрация** - пользователь регистрируется, получает JWT и стартовый баланс
2. **Создание проекта** - платформа создает изолированный vcluster
3. **Подключение GitHub** - регистрация вебхука, генерация CI/CD workflow и K8s-манифестов, открытие PR
4. **Деплой** - при мерже GitHub Actions собирает образ и деплоит в пространство проекта
5. **Биллинг** - фоновый процесс снимает метрики потребления ресурсов, приостанавливает проекты при нулевом балансе

### Архитектурные решения

Ключевые проектные решения задокументированы в формате RFC:

- [RFC-001: Изоляция в Kubernetes](rfc/rfc-001-kubernetes-isolation.md) - vcluster vs пространства имен vs отдельные кластеры
- [RFC-002: База данных](rfc/rfc-002-database.md) - PostgreSQL + sqlc для типобезопасных запросов
- [RFC-003: Аутентификация](rfc/rfc-003-authentication.md) - stateless JWT с симметричной подписью
- [RFC-004: Монетизация](rfc/rfc-004-monetization-model.md) - оплата по потреблению на основе метрик CPU и памяти

### Диаграммы

Архитектурные диаграммы (PlantUML, модель C4):

- [Контекстная диаграмма](diagrams/c4-context.puml)
- [Контейнерная диаграмма](diagrams/c4-container.puml)
- [Компонентная диаграмма](diagrams/c4-component-control-plane.puml)
- [Диаграмма развертывания](diagrams/deployment-platform.puml)
- [Последовательность: жизненный цикл проекта](diagrams/sequence-project-lifecycle.puml)

## API

API задокументирован в формате OpenAPI 3.0. При запущенном сервере Swagger UI доступен по адресу:

```
http://localhost:8080/swagger/
```

Файл спецификации: [`internal/http/swagger/openapi.yaml`](deploy-service/internal/http/swagger/openapi.yaml)

## Тестирование

```bash
cd deploy-service

# Юнит-тесты (без внешних зависимостей)
../scripts/test-unit.sh

# Отдельный тест
go test ./internal/service/... -run TestName

# Интеграционные тесты (требуется PostgreSQL)
../scripts/test-integration.sh

# Системные тесты (требуется Docker + k3s)
../scripts/test-system.sh
```

## Структура проекта

```
.
├── deploy-service/          # Go-приложение
│   ├── cmd/server/          # Точка входа
│   ├── internal/            # Весь код приложения
│   ├── tests/               # Интеграционные и системные тесты
│   └── Dockerfile           # Multi-stage сборка
├── k8s/                     # Манифесты Kubernetes
│   ├── deploy-service/      # Деплой приложения
│   ├── gateway/             # Ingress / gateway
│   └── monitoring/          # Prometheus, Loki, Grafana
├── diagrams/                # Архитектурные диаграммы PlantUML
├── rfc/                     # Документы архитектурных решений
├── scripts/                 # Утилиты форматирования и тестирования
├── .github/workflows/       # CI/CD пайплайн
└── docker-compose.yml       # Стек локальной разработки
```

## Лицензия

Проект распространяется под лицензией MIT - см. файл [LICENSE](LICENSE).
