# TASK-037: Bug fixes — session persistence and per-project GitHub token [Done]

## Контекст

Два независимых бага, мешающих нормальной работе консоли:

1. **Сброс сессии при перезагрузке.** JWT токен хранится только в памяти (`state.token`). При F5 / открытии новой вкладки пользователь видит форму логина вместо последнего открытого проекта. Сервер уже ставит `deploy_service_token` httpOnly-куку — нужно использовать её как источник истины для восстановления сессии, либо дублировать JWT в `sessionStorage`.

2. **GitHub токен на уровне пользователя вместо уровня проекта.** Кнопка «Токен» в верхней навигации и эндпоинт `/service/github-token` хранят один токен на весь аккаунт. Правильная модель: каждый проект хранит свой зашифрованный GitHub токен, управление — из вкладки «Деплой» конкретного проекта.

---

## Bug 1: Session persistence

### Проблема

```
bootstrap() ->  if (!state.token) return  ->  setMode("login")
```

После перезагрузки `state.token = ""`, поэтому всегда открывается форма логина, даже если кука `deploy_service_token` валидна.

### Решение

**Вариант A (рекомендуемый, минимальные правки):** использовать `sessionStorage` для дублирования JWT.

В `setToken(token)`:

```js
function setToken(token) {
  state.token = token;
  try {
    sessionStorage.setItem("deploy_token", token);
  } catch (_) {}
}
```

В `logout()`:

```js
try {
  sessionStorage.removeItem("deploy_token");
} catch (_) {}
```

В `bootstrap()` перед проверкой `state.token`:

```js
if (!state.token) {
  try {
    state.token = sessionStorage.getItem("deploy_token") || "";
  } catch (_) {}
}
```

Если токен просрочен, `loadBilling()` получит 401, `api()` вызовет `logout()`, который очистит `sessionStorage` — пользователь увидит форму логина.

**Вариант B:** заменить в `bootstrap()` проверку на пинг `/health` или `/billing/summary` с cookie-аутентификацией (уже работает через httpOnly-куку), а `state.token` восстанавливать из ответа сервера. Требует добавления эндпоинта `GET /auth/me`.

### Файлы

- `internal/http/ui/app.js` — `setToken`, `logout`, `bootstrap`

---

## Bug 2: GitHub токен per-project вместо per-user

### Проблема

Текущая архитектура:

- `users.github_token_encrypted` — один токен на пользователя
- `/service/github-token` (GET/PUT/DELETE) — API на уровне аккаунта
- Кнопка «Токен» в верхней навигации видна всегда

Нужная архитектура:

- `projects.github_token_encrypted` — токен на проект
- `/projects/{id}/github-token` (GET/PUT/DELETE) — API на уровне проекта
- Управление токеном — во вкладке «Деплой» каждого проекта
- Кнопка «Токен» из верхней навигации убирается

### Backend изменения

**1. Новая миграция** `017_add_project_github_token.sql`:

```sql
ALTER TABLE projects ADD COLUMN IF NOT EXISTS github_token_encrypted TEXT;
```

**2. Обновить `ProjectStore`** — добавить `UpdateGitHubToken(ctx, projectID, encrypted string)` и включить поле в SELECT/UPDATE.

**3. Новые методы сервиса** (`project_service.go`):

```go
func (s *ProjectService) GetProjectGitHubToken(ctx, projectID, userID string) (configured bool, err error)
func (s *ProjectService) UpsertProjectGitHubToken(ctx, projectID, userID, plainToken string) error
func (s *ProjectService) DeleteProjectGitHubToken(ctx, projectID, userID string) error
```

**4. Новые обработчики** (`handler.go`):

```go
func (h *Handler) GetProjectGitHubToken(...)
func (h *Handler) UpsertProjectGitHubToken(...)
func (h *Handler) DeleteProjectGitHubToken(...)
```

**5. Новые маршруты** (`router.go`):

```
GET    /projects/{id}/github-token
PUT    /projects/{id}/github-token
DELETE /projects/{id}/github-token
```

**6. Использование токена** в `GitHubBootstrap` и `GitHubQuestions`: если в payload `githubToken` пуст — брать из `projects.github_token_encrypted`, а не из `users.github_token_encrypted`.

**7. Совместимость:** пользовательский токен (`/service/github-token`) оставить как запасной fallback на переходный период (или удалить — на усмотрение), deprecate API.

### Frontend изменения

**1. Убрать из верхней навигации** (`index.html`):

- Кнопку `id="serviceTokenButton"`
- Модальное окно `id="serviceTokenModal"` и его содержимое
- Из `app.js`: функции `loadServiceGitHubTokenStatus`, `saveServiceGitHubToken`, `deleteServiceGitHubToken`, `renderServiceTokenButton`, `renderServiceTokenStatusText`, обработчики `serviceTokenButton`, `serviceTokenSaveButton`, `serviceTokenDeleteButton`

**2. Добавить в вкладку «Деплой»** секцию «GitHub токен»:

```html
<section class="panel">
  <div class="panel-head compact">
    <div>
      <p class="section-kicker">Интеграция</p>
      <h3>GitHub токен</h3>
    </div>
  </div>
  <p class="muted">
    Токен хранится в зашифрованном виде и используется для автозаполнения и создания PR.
  </p>
  <div class="field">
    <label>Токен</label>
    <input type="password" id="projectGitHubTokenInput" placeholder="ghp_..." />
  </div>
  <div class="action-row">
    <button id="deleteProjectGitHubTokenButton" class="ghost-button danger hidden">
      Удалить токен
    </button>
    <button id="saveProjectGitHubTokenButton" class="primary-button">Сохранить токен</button>
  </div>
  <p id="projectGitHubTokenStatus" class="muted"></p>
</section>
```

**3. Новые функции** в `app.js`:

```js
async function loadProjectGitHubTokenStatus()
async function saveProjectGitHubToken(token)
async function deleteProjectGitHubToken()
```

**4. Обновить `loadProjectWorkspaceData`**: добавить `loadProjectGitHubTokenStatus()` в параллельную загрузку.

**5. Обновить deploy form**: убрать хинт «Основной токен лучше сохранить через кнопку «Токен» в верхнем меню», заменить на «Используй сохранённый токен проекта выше или введи временный токен для этой операции».

### Файлы

- `internal/store/postgres/migrations/017_add_project_github_token.sql` (new)
- `internal/store/postgres/project_store.go`
- `internal/domain/project.go` (добавить поле `GitHubTokenEncrypted`)
- `internal/service/project_service.go`
- `internal/service/port.go` (новые методы в интерфейсе)
- `internal/http/handler.go`
- `internal/http/router.go`
- `internal/http/ui/index.html`
- `internal/http/ui/app.js`

---

## Готово, если

### Session persistence

- [x] Перезагрузка страницы восстанавливает сессию без повторного логина
- [x] Открытие нового таба в той же сессии браузера — токен подхватывается
- [x] Logout очищает `sessionStorage`, после перезагрузки открывается форма логина
- [x] Просроченный токен после перезагрузки -> автоматический логаут + форма логина

### Per-project GitHub token

- [x] Кнопка «Токен» пропала из верхней навигации
- [x] Во вкладке «Деплой» есть секция управления токеном проекта
- [x] Статус токена (настроен / не настроен) показывается для каждого проекта отдельно
- [x] Bootstrap и questions используют токен проекта если в форме не задан временный
- [x] Удаление токена работает
- [x] Сохранённый токен автоматически автозаполняет поле при следующем открытии
