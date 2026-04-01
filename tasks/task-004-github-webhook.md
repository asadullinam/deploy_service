# TASK-004: Обработка webhook-событий от GitHub Actions [Done]

## Проблема

Сейчас платформа запускает деплой (через PR -> merge), но не получает обратную связь: прошёл ли деплой успешно, упал ли билд, сколько времени занял. Пользователь должен сам идти в GitHub Actions и смотреть логи.

## Зависимости

ARCH-001, TASK-002 (нужна БД для хранения статусов деплоев).

## Что сделать

### 1. Модель Release (Деплой/Выпуск)

```go
// internal/domain/release.go
type ReleaseStatus string

const (
    ReleaseStatusPending    ReleaseStatus = "pending"
    ReleaseStatusBuilding   ReleaseStatus = "building"
    ReleaseStatusDeploying  ReleaseStatus = "deploying"
    ReleaseStatusSuccess    ReleaseStatus = "success"
    ReleaseStatusFailed     ReleaseStatus = "failed"
)

type Release struct {
    ID          string
    ProjectID   string
    CommitSHA   string
    CommitMessage string
    Status      ReleaseStatus
    WorkflowRunID int64       // ID запуска в GitHub Actions
    ImageTag    string        // ghcr.io/.../service:sha
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

```sql
-- migrations/003_create_releases.sql
CREATE TABLE releases (
    id              TEXT        PRIMARY KEY,
    project_id      TEXT        NOT NULL REFERENCES projects(id),
    commit_sha      TEXT        NOT NULL,
    commit_message  TEXT,
    status          TEXT        NOT NULL,
    workflow_run_id BIGINT,
    image_tag       TEXT,
    created_at      TIMESTAMPTZ NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_releases_project_id ON releases (project_id);
```

### 2. Эндпоинт для webhook

```
POST /webhooks/github
```

GitHub отправляет сюда события `workflow_run` при изменении статуса Actions.

```go
// internal/http/handler.go
func (h *Handler) GitHubWebhook(w http.ResponseWriter, r *http.Request) {
    // 1. Проверить подпись HMAC-SHA256 (заголовок X-Hub-Signature-256)
    if !verifyGitHubSignature(r, webhookSecret) {
        writeJSON(w, http.StatusUnauthorized, ...)
        return
    }

    // 2. Прочитать тип события
    eventType := r.Header.Get("X-GitHub-Event")
    if eventType != "workflow_run" {
        w.WriteHeader(http.StatusNoContent) // игнорируем другие события
        return
    }

    // 3. Десериализовать payload, найти нужный workflow
    // 4. Найти Release по workflow_run_id, обновить статус
}
```

### 3. Проверка подписи (ОБЯЗАТЕЛЬНО)

GitHub подписывает каждый webhook HMAC-SHA256 с секретом, который задаётся при регистрации webhook. Без проверки любой может подделать событие.

```go
func verifyGitHubSignature(r *http.Request, secret string) bool {
    sig := r.Header.Get("X-Hub-Signature-256")
    body, _ := io.ReadAll(r.Body)
    r.Body = io.NopCloser(bytes.NewBuffer(body)) // вернуть body

    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write(body)
    expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
    return hmac.Equal([]byte(sig), []byte(expected))
}
```

### 4. Обновление статусов

Маппинг GitHub Actions -> внутренние статусы:

| GitHub `conclusion` | GitHub `status` | Release.Status |
| ------------------- | --------------- | -------------- |
| —                   | `in_progress`   | `building`     |
| `success`           | `completed`     | `success`      |
| `failure`           | `completed`     | `failed`       |
| `cancelled`         | `completed`     | `failed`       |

### 5. Новые API-эндпоинты

```
GET /projects/{id}/releases          — список деплоев проекта
GET /projects/{id}/releases/{releaseId} — детали конкретного деплоя
```

### 6. Регистрация webhook в GitHub

При `BootstrapRepositoryFlow` добавить шаг: зарегистрировать webhook в репозитории через GitHub API.

```
POST /repos/{owner}/{repo}/hooks
{
  "config": {
    "url": "https://<платформа>/webhooks/github",
    "content_type": "json",
    "secret": "<WEBHOOK_SECRET>"
  },
  "events": ["workflow_run"]
}
```

## Новые переменные окружения

| Переменная              | Описание                                                     |
| ----------------------- | ------------------------------------------------------------ |
| `GITHUB_WEBHOOK_SECRET` | Секрет для проверки подписи входящих webhook                 |
| `PUBLIC_URL`            | Публичный адрес платформы (для регистрации webhook в GitHub) |

## Готово, если

- Merge PR в репозитории -> через <30 секунд Release в БД меняет статус
- Неподписанный или неверно подписанный webhook отклоняется с 401
- `GET /projects/{id}/releases` возвращает историю деплоев
