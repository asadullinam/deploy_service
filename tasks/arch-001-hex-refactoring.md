# ARCH-001: Приведение кода к гексагональной архитектуре [Done]

## Контекст

Текущий код близок к гексагональной архитектуре, но есть несколько нарушений, которые нужно исправить до того, как начнётся расширение функциональности. Откладывать эти правки опасно — чем дольше ждём, тем дороже рефакторинг.

## Что такое гексагональная архитектура в этом проекте

```
          ┌─────────────────────────────────┐
          │         Ядро (Core)             │
          │                                 │
          │  domain/     — модели и ошибки  │
          │  service/    — бизнес-логика    │
          │                                 │
          └──────────────┬──────────────────┘
                         │ зависит только от domain
            ┌────────────┴────────────┐
            │                         │
    Входящие порты            Исходящие порты
    (Driving Ports)           (Driven Ports)
            │                         │
    service.Port              store.ProjectStore
    (интерфейс для HTTP)      kubernetes.Provisioner
                              github.Automation
                              monetization.Engine
            │                         │
    Входящие адаптеры         Исходящие адаптеры
    http/Handler              store/memory/
                              store/postgres/   (будет)
                              integration/kubernetes/
                              integration/github/
                              monetization/
```

**Правило:** слои зависят только внутрь. Адаптер знает о порте, порт не знает об адаптере.

## Найденные нарушения

### 0. Интерфейсы определены в пакетах адаптеров, а не в ядре (КРИТИЧНО)

**Файлы:** `integration/kubernetes/provisioner.go`, `integration/github/automation.go`, `monetization/engine.go`, `store/project_store.go`

В Go интерфейс принадлежит потребителю, а не реализации. `ProjectService` — потребитель `Provisioner`, `Automation`, `Store` и `Engine`. Значит, эти интерфейсы должны быть объявлены в `service/`, а не в пакетах адаптеров.

Сейчас `service/` импортирует `integration/kubernetes` и `integration/github` только ради интерфейсов — стрелка зависимости направлена в неверную сторону. Ядро зависит от адаптера.

**Исправление:** создать `internal/service/deps.go` со всеми исходящими портами. Адаптеры перестают объявлять интерфейсы и только реализуют те, что определены в `service/`. Добавить compile-time проверки:

```go
// internal/integration/kubernetes/kubectl_provisioner.go
var _ service.Provisioner = (*KubectlProvisioner)(nil)
```

Структура после исправления:

```
internal/service/
├── project_service.go   — бизнес-логика
├── port.go              — входящий порт (для HTTP)
└── deps.go              — исходящие порты: ProjectStore, Provisioner, GitHubAutomation, MonetizationEngine

internal/integration/kubernetes/
├── kubectl_provisioner.go   — реализует service.Provisioner
└── mock.go

internal/integration/github/
├── github_automation.go     — реализует service.GitHubAutomation
└── mock.go
```

### 1. Handler зависит от конкретного \*service.ProjectService (КРИТИЧНО)

**Файл:** `internal/http/handler.go:13`

```go
// Сейчас — нарушение: HTTP-адаптер зависит от конкретной реализации
type Handler struct {
    projects *service.ProjectService
}
```

Handler — это входящий адаптер. Он должен зависеть от интерфейса (входящего порта), а не от конкретного сервиса. Сейчас невозможно протестировать Handler без реального ProjectService со всеми его зависимостями.

**Исправление:** ввести интерфейс `service.Port` (или `ProjectServicePort`) и использовать его в Handler.

```go
// internal/service/port.go — новый файл
type Port interface {
    CreateProject(ctx context.Context, r domain.CreateProjectRequest) (domain.Project, error)
    ListProjects() []domain.Project
    GetProject(projectID string) (domain.Project, error)
    DeleteProject(ctx context.Context, projectID string) error
    GetProjectCost(ctx context.Context, projectID string) (domain.CostBreakdown, error)
    BootstrapGitHubFlow(ctx context.Context, projectID string, r domain.BootstrapGitHubFlowRequest) (domain.BootstrapGitHubFlowResponse, error)
    BuildGitHubBootstrapQuestions(ctx context.Context, projectID string, r domain.GitHubBootstrapQuestionsRequest) (domain.GitHubBootstrapQuestionsResponse, error)
}

// internal/http/handler.go — исправленный
type Handler struct {
    projects service.Port
}
```

### 2. ErrProjectNotFound живёт в service, используется в http (КРИТИЧНО)

**Файл:** `internal/service/project_service.go:17`, `internal/http/handler.go:83`

```go
// service — определяет ошибку
var ErrProjectNotFound = errors.New("project not found")

// http — импортирует service ради одной ошибки
errors.Is(err, service.ErrProjectNotFound)
```

Ошибки предметной области должны жить в `domain`. Сейчас http-адаптер импортирует `service` ради одной ошибки — это создаёт нежелательную связь.

**Исправление:**

```go
// internal/domain/errors.go — новый файл
var ErrProjectNotFound = errors.New("project not found")

// internal/service/project_service.go — убрать объявление, использовать domain.ErrProjectNotFound
// internal/http/handler.go — заменить service.ErrProjectNotFound на domain.ErrProjectNotFound
```

### 3. Store-интерфейс не принимает context.Context

**Файл:** `internal/store/project_store.go`

```go
// Сейчас — нет context
type ProjectStore interface {
    Create(project domain.Project) error
    GetByID(projectID string) (domain.Project, bool)
    List() []domain.Project
    Update(project domain.Project) error
}
```

При переходе на PostgreSQL каждый вызов к БД должен уважать отмену контекста. Добавить context сейчас дёшево; потом — сломает весь код.

**Исправление:**

```go
type ProjectStore interface {
    Create(ctx context.Context, project domain.Project) error
    GetByID(ctx context.Context, projectID string) (domain.Project, bool)
    List(ctx context.Context) []domain.Project
    Update(ctx context.Context, project domain.Project) error
}
```

### 4. Парсинг путей в Handler, а не в Router

**Файл:** `internal/http/handler.go:49-68`

Функция `ProjectByID` сама разбирает URL и диспетчеризует вызовы по sub-handlers. Это обязанность роутера.

```go
// Сейчас handler.go делает это:
func (h *Handler) ProjectByID(w http.ResponseWriter, r *http.Request) {
    projectID, subpath := parseProjectPath(r.URL.Path)
    if subpath == "cost" { ... }
    if subpath == "github/bootstrap" { ... }
    ...
}
```

**Исправление:** использовать Go 1.22 enhanced routing (`{id}` параметры в ServeMux) и зарегистрировать каждый маршрут явно.

```go
// internal/http/router.go
mux.HandleFunc("GET /projects/{id}",                    handler.GetProject)
mux.HandleFunc("DELETE /projects/{id}",                 handler.DeleteProject)
mux.HandleFunc("GET /projects/{id}/cost",               handler.GetProjectCost)
mux.HandleFunc("POST /projects/{id}/github/questions",  handler.GitHubQuestions)
mux.HandleFunc("POST /projects/{id}/github/bootstrap",  handler.GitHubBootstrap)
```

В handler методы разбиваются на отдельные однозадачные функции, `r.PathValue("id")` даёт projectID.

### 5. NewRouter принимает \*Handler (конкретный тип)

**Файл:** `internal/http/router.go:5`

```go
func NewRouter(handler *Handler) http.Handler {
```

Это позволяет подключить только один конкретный Handler. Для тестов достаточно передавать `*Handler` — проблема минорная, но для единообразия лучше исправить вместе с п.4.

## Порядок выполнения

1. Создать `internal/domain/errors.go`, перенести туда `ErrProjectNotFound`
2. Обновить `service/project_service.go` — убрать объявление, использовать `domain.ErrProjectNotFound`
3. Обновить `http/handler.go` — заменить `service.ErrProjectNotFound` на `domain.ErrProjectNotFound`
4. Создать `internal/service/port.go` с интерфейсом `Port`
5. Изменить `http/handler.go` — поле `projects` стало `service.Port`
6. Добавить `context.Context` в `store.ProjectStore` и обновить `store/memory/project_store.go`
7. Обновить `service/project_service.go` — прокидывать context в store-вызовы
8. Переписать `http/router.go` на Go 1.22 route patterns с `{id}`
9. Разбить `ProjectByID` на отдельные handler-методы

## Готово, если

- Каждый пакет зависит только на пакеты, которые «глубже» него в луке
- `http` не импортирует `service` напрямую (только через интерфейс `service.Port`)
- `http` не импортирует `service` ради ошибок (только `domain`)
- Store-методы принимают context
- Каждый HTTP-маршрут — отдельный handler-метод
