# [Done] TASK-012: Ужесточение авторизации и изоляции данных

## Проблема

Базовая JWT-аутентификация уже есть, но этого недостаточно для безопасной многопользовательской платформы. Мы уже нашли класс ошибок, где project-endpoints начинали работать только по `projectID`, а не по владельцу проекта. Такие баги особенно опасны для данных уровня:

- `kubeconfig`
- история релизов
- стоимость
- GitHub bootstrap

Даже если конкретный баг исправлен, нужно зафиксировать правила изоляции как архитектурный контракт.

## Что сделать

### 1. Явно оформить owner-based authorization как правило сервиса

Сейчас часть проверок делается в HTTP-адаптере. Нужно решить, где живёт источник истины:

- либо все проверки владения централизуются в `service/`
- либо вводится отдельный authorizer/guard слой

Цель: невозможно получить чужой проект, даже если появится новый входящий адаптер кроме HTTP.

### 2. Убрать “неявную фильтрацию” из UI-сценариев

Нельзя полагаться на то, что frontend просто не покажет чужой `projectId`. Backend должен быть безопасен сам по себе.

Проверить и покрыть авторизацией:

- `GET /projects`
- `GET /projects/{id}`
- `DELETE /projects/{id}`
- `GET /projects/{id}/cost`
- `POST /projects/{id}/github/questions`
- `POST /projects/{id}/github/bootstrap`
- `GET /projects/{id}/releases`
- `GET /projects/{id}/releases/{releaseId}`
- `POST /projects/{id}/releases/{releaseId}/rollback`
- `GET /projects/{id}/kubeconfig`
- `POST /projects/{id}/kubeconfig/rotate`
- `PUT /projects/{id}/deployment-settings`

### 3. Перейти с хранения JWT в JS-storage на cookie-based auth

Сейчас JWT живёт в браузерном storage текущей вкладки. Это лучше, чем `localStorage`, но всё ещё доступно JavaScript. Следующий шаг:

- `HttpOnly`
- `Secure`
- `SameSite=Lax` или `Strict`

Понадобится:

- новый login/register flow, который ставит cookie
- logout endpoint
- CSRF-стратегия для state-changing запросов

### 4. Добавить негативные тесты на утечки

Нужны тесты не только “своё открывается”, но и “чужое не открывается”.

Минимум:

- пользователь A не видит проекты пользователя B в списке
- пользователь A не получает kubeconfig пользователя B
- пользователь A не может rollback/release operations для проекта B
- пользователь A не может перезаписать deployment settings проекта B

## Готово, если

- Вся project/release/kubeconfig функциональность ограничена владельцем проекта
- Правило владения зафиксировано тестами
- JWT больше не хранится в доступном JavaScript storage
- Авторизация не зависит от конкретного входящего адаптера

## Статус проверки

**Выполнена частично.**

**Реализовано:**

- Все 12 project-scoped эндпоинтов используют `requireOwnedProject` хелпер в HTTP-адаптере — чужие проекты возвращают 403
- `GET /projects` фильтрует только по `ownerID` из токена
- Тесты `TestListProjectsReturnsOnlyOwnedProjects` и `TestGetProjectRejectsForeignOwner` существуют

**Не реализовано:**

- Авторизация живёт только в HTTP-адаптере (`internal/http/handler.go`), не в `service/` — второй адаптер не унаследует эти проверки автоматически
- JWT возвращается в теле JSON-ответа и хранится в JavaScript (`ui/app.js`) — нет HttpOnly cookie, нет CSRF-стратегии, нет эндпоинта `/auth/logout`
- Отсутствуют негативные тесты для kubeconfig, rollback и deployment-settings при cross-user сценариях
