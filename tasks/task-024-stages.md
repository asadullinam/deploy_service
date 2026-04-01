# [Done] TASK-024: Stages — мультиконтурные окружения для проекта

## Проблема

Сейчас каждый проект имеет ровно одно окружение: один vcluster, один namespace внутри него, один GitHub Actions workflow. Типичный production workflow выглядит иначе:

```
feature branch -> test -> preprod -> prod
```

Пользователю нужна возможность создавать несколько изолированных контуров внутри одного проекта и выбирать, в какой из них деплоить.

## Архитектурный контекст

**Текущая модель**: один проект = один хост-namespace `project-<id>` = один vcluster внутри него. Пользовательские workload-ы деплоятся **внутрь** vcluster (через его kubeconfig). vcluster синкает поды обратно в хост-namespace.

**Модель со stages**: vcluster остаётся **одним на проект** — он и есть граница изоляции между проектами. Stage — это **namespace внутри vcluster** (например `production`, `test`, `preprod`).

```
Хост-кластер
└── namespace: project-<id>          <- хост-namespace проекта (не меняется)
    └── vcluster: vc-<id>            <- один vcluster на проект (не меняется)
        ├── namespace: production    <- stage "production"
        ├── namespace: test          <- stage "test"
        └── namespace: preprod       <- stage "preprod"
```

Создание stage не трогает хост-кластер — только создаёт namespace внутри уже запущенного vcluster.

## Что сделать

### 1. Доменная модель — `domain/stage.go`

```go
type StageStatus string

const (
    StageStatusCreating StageStatus = "creating"
    StageStatusActive   StageStatus = "active"
    StageStatusDeleting StageStatus = "deleting"
    StageStatusDeleted  StageStatus = "deleted"
    StageStatusFailed   StageStatus = "failed"
)

type Stage struct {
    ID        string      `json:"id"`
    ProjectID string      `json:"projectId"`
    Name      string      `json:"name"` // отображаемое имя, например "Production"
    Slug      string      `json:"slug"` // имя namespace внутри vcluster: "production"
    Status    StageStatus `json:"status"`
    PublicURL string      `json:"publicUrl,omitempty"`
    CreatedAt time.Time   `json:"createdAt"`
    UpdatedAt time.Time   `json:"updatedAt"`
}

type CreateStageRequest struct {
    Name string `json:"name"`
}
```

`Release` получает поле `StageID string` — к какому контуру относится релиз.

Stage не имеет собственного kubeconfig: пользователь использует kubeconfig проекта (vcluster), выбирая нужный namespace (`kubectl -n <slug>`). Опционально — отдавать kubeconfig с уже прописанным `namespace: <slug>` в context.

### 2. Порт хранилища — `service/deps.go`

```go
type StageStore interface {
    Create(ctx context.Context, stage domain.Stage) error
    GetByID(ctx context.Context, stageID string) (domain.Stage, bool)
    GetBySlug(ctx context.Context, projectID, slug string) (domain.Stage, bool)
    ListByProject(ctx context.Context, projectID string) []domain.Stage
    Update(ctx context.Context, stage domain.Stage) error
}
```

### 3. Provisioner — расширить интерфейс

Stage не требует создания нового vcluster или хост-namespace. Новые методы работают с namespace **внутри vcluster** через vcluster kubeconfig проекта.

```go
// Новые методы:
CreateStageEnvironment(ctx context.Context, projectID, stageSlug string) error
DeleteStageEnvironment(ctx context.Context, projectID, stageSlug string) error
ApplyImageToStage(ctx context.Context, projectID, stageSlug, imageTag string) error
GetStageRuntimeStatus(ctx context.Context, projectID, stageSlug string) (domain.ProjectRuntimeStatus, error)
```

**Реализация**:

- `CreateStageEnvironment` — kubectl с vcluster kubeconfig: `kubectl create namespace <slug>`
- `DeleteStageEnvironment` — `kubectl delete namespace <slug>`
- `ApplyImageToStage` — `kubectl patch deployment -n <slug> ...` (аналог текущего `ApplyImage`)
- `GetStageRuntimeStatus` — `kubectl get deployment,pods -n <slug>`

Suspend/resume на уровне stage **не вводится** — приостановка происходит на уровне vcluster (проекта целиком), это задача уже реализована.

### 4. Хранилища

**`store/memory/stage_store.go`** — in-memory реализация `StageStore` (thread-safe map).

**`store/postgres/migrations/012_create_stages.sql`**:

```sql
CREATE TABLE IF NOT EXISTS stages (
    id         TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    name       TEXT NOT NULL,
    slug       TEXT NOT NULL,
    status     TEXT NOT NULL DEFAULT 'creating',
    public_url TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, slug)
);
CREATE INDEX IF NOT EXISTS stages_project_id_idx ON stages (project_id);
```

**`store/postgres/migrations/013_add_stage_id_to_releases.sql`**:

```sql
ALTER TABLE releases ADD COLUMN IF NOT EXISTS stage_id TEXT NOT NULL DEFAULT '';
```

**`store/postgres/stage_store.go`** — postgres реализация.

### 5. Сервисный слой — `service/project_service.go`

Новые методы:

```go
CreateStage(ctx, projectID, userID string, req domain.CreateStageRequest) (domain.Stage, error)
ListStages(ctx, projectID, userID string) ([]domain.Stage, error)
GetStage(ctx, projectID, stageID, userID string) (domain.Stage, error)
DeleteStage(ctx, projectID, stageID, userID string) error
GetStageRuntimeStatus(ctx, projectID, stageID, userID string) (domain.ProjectRuntimeStatus, error)
```

**Изменение `CreateProject`**: после успешного создания vcluster автоматически вызывает `CreateStage` для stage с name=`Production`, slug=`production`.

**Изменение `HandleGitHubWebhook`**: при создании `Release` проставляется `StageID` по `stageSlug` из payload (если не указан — ищет stage с slug `production`).

**Изменение `RollbackToRelease`**: откат деплоится в тот же stage, к которому привязан целевой release.

### 6. Port — `service/port.go`

```go
CreateStage(ctx context.Context, projectID string, req domain.CreateStageRequest) (domain.Stage, error)
ListStages(ctx context.Context, projectID string) ([]domain.Stage, error)
GetStage(ctx context.Context, projectID, stageID string) (domain.Stage, error)
DeleteStage(ctx context.Context, projectID, stageID string) error
GetStageRuntimeStatus(ctx context.Context, projectID, stageID string) (domain.ProjectRuntimeStatus, error)
```

### 7. HTTP layer

| Method   | Path                                             | Описание                                     |
| -------- | ------------------------------------------------ | -------------------------------------------- |
| `POST`   | `/projects/{id}/stages`                          | Создать новый контур                         |
| `GET`    | `/projects/{id}/stages`                          | Список контуров проекта                      |
| `GET`    | `/projects/{id}/stages/{stageId}`                | Получить контур                              |
| `DELETE` | `/projects/{id}/stages/{stageId}`                | Удалить контур (нельзя удалить `production`) |
| `GET`    | `/projects/{id}/stages/{stageId}/runtime-status` | Runtime статус контура                       |

**Изменение bootstrap**: `BootstrapGitHubFlowRequest` получает поле `StageSlug string` (по умолчанию `production`). Генерируемый GitHub Actions workflow получает:

```yaml
env:
  STAGE_SLUG: ${{ vars.STAGE_SLUG || 'production' }}
```

Все kubectl-команды внутри workflow, касающиеся namespace пользовательских ресурсов (`deployment.yaml`, `service.yaml`, `httproute.yaml`), параметризуются через `STAGE_SLUG`.

**Изменение releases**: `GET /projects/{id}/releases` принимает опциональный query param `?stageId=`.

### 9. UI — управление контурами в консоли

Добавить поддержку `stages` в web UI:

- Вкладка проекта `Контуры`:
  - список контуров проекта (`name`, `slug`, `status`);
  - создание контура (`POST /projects/{id}/stages`);
  - удаление контура (`DELETE /projects/{id}/stages/{stageId}`), кроме `production`.
- Вкладка `Деплой`:
  - выбор stage для bootstrap (поле `stageSlug` в `BootstrapGitHubFlowRequest`).
- Вкладка `Состояние`:
  - runtime-статус запрашивается для выбранного stage через `GET /projects/{id}/stages/{stageId}/runtime-status`.
- Вкладка `Релизы`:
  - фильтрация списка релизов по выбранному stage (`GET /projects/{id}/releases?stageId=...`).

### 8. Обратная совместимость

Существующие проекты, созданные до введения stages, не трогаются на уровне хост-кластера. При первом обращении к `/projects/{id}/stages` система создаёт stage `production`, указывающий на namespace `production` внутри vcluster (который должен уже существовать, т.к. GitHub Actions деплоил именно туда).

Postgres-миграция создаёт запись `production` для каждого существующего проекта:

```sql
INSERT INTO stages (id, project_id, name, slug, status, created_at, updated_at)
SELECT 'stage-' || id || '-production', id, 'Production', 'production', 'active', created_at, updated_at
FROM projects
WHERE status NOT IN ('deleted', 'deleting')
ON CONFLICT DO NOTHING;
```

## Готово, если

- [x] Новый проект автоматически получает stage `production` (namespace `production` внутри vcluster)
- [x] Пользователь может создать дополнительный контур — создаётся namespace внутри того же vcluster
- [x] Bootstrap принимает `stageSlug`; workflow параметризован через `STAGE_SLUG`
- [x] `Release` привязан к конкретному stage через `StageID`
- [x] Rollback деплоится в stage исходного релиза
- [x] Нельзя удалить stage `production`
- [x] Старые проекты получают stage `production` через миграцию без изменения k8s-инфраструктуры
- [x] Все тесты зелёные (`go test ./...`)
- [x] В UI есть управление stage: список/создание/удаление, выбор stage для bootstrap, runtime и релизов

## Мини-отчет

- Доделан полный backend-контур для stages: доменная модель, storage (memory/postgres), сервисные методы и HTTP-эндпоинты `/projects/{id}/stages*`.
- В bootstrap добавлен `stageSlug`; генерация GitHub Actions workflow переведена на `STAGE_SLUG` (с дефолтом `production`) и все `kubectl -n ...` шаги параметризованы через него.
- Логика релизов и rollback доведена до stage-aware поведения: релизы получают `StageID`, rollback применяет образ в stage исходного релиза.
- Добавлена/актуализирована миграционная часть для stages, включая `public_url` и backfill `production` stage.
- Добавлена UI-поддержка stages: новая вкладка `Контуры`, создание/удаление и выбор stage, stage-aware bootstrap, stage runtime и фильтрация релизов по выбранному stage.
- Обновлены и расширены unit-тесты (service/http/github templates), итоговый прогон: `go test ./...` — успешно.
