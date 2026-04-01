# [Done] TASK-011: Suspend / Resume проекта

## Контекст

Решение из RFC-001: vcluster останавливается только явно пользователем. «Нет новых деплоев» не означает «неактивен» — приложение может работать стабильно месяцами. Suspend — это осознанное действие: пользователь хочет сэкономить, приложение временно недоступно.

## Зависимости

ARCH-001, TASK-002 (нужна БД для сохранения статуса), TASK-003 (auth).

## Что сделать

### 1. Новый статус в domain

```go
// internal/domain/project.go
const (
    ProjectStatusCreating  ProjectStatus = "creating"
    ProjectStatusActive    ProjectStatus = "active"
    ProjectStatusSuspended ProjectStatus = "suspended"  // новый
    ProjectStatusDeleting  ProjectStatus = "deleting"
    ProjectStatusDeleted   ProjectStatus = "deleted"
)
```

Граф переходов:

```
creating -> active <-> suspended
                     |
                     v
             deleting -> deleted
```

Запрещённые переходы (проверять в сервисе):

- `suspended -> deleting` — сначала нужно resume, потом delete (или удалять напрямую)
- `creating -> suspended` — нельзя приостановить то, что ещё создаётся

### 2. Новые методы в Provisioner

```go
// internal/service/deps.go
type Provisioner interface {
    CreateProjectEnvironment(ctx context.Context, projectID string) error
    DeleteProjectEnvironment(ctx context.Context, projectID string) error
    SuspendProjectEnvironment(ctx context.Context, projectID string) error  // новый
    ResumeProjectEnvironment(ctx context.Context, projectID string) error   // новый
}
```

Реализация через vcluster:

```go
// internal/integration/kubernetes/kubectl_provisioner.go
func (p *KubectlProvisioner) SuspendProjectEnvironment(ctx context.Context, projectID string) error {
    name := vclusterNameFromProjectID(projectID)
    namespace := namespaceFromProjectID(projectID)
    _, err := p.runVCluster(ctx, []string{"pause", name, "--namespace", namespace})
    return err
}

func (p *KubectlProvisioner) ResumeProjectEnvironment(ctx context.Context, projectID string) error {
    name := vclusterNameFromProjectID(projectID)
    namespace := namespaceFromProjectID(projectID)
    _, err := p.runVCluster(ctx, []string{"resume", name, "--namespace", namespace})
    return err
}
```

`vcluster pause` останавливает все поды vcluster (control plane + синхронизированные поды). Ресурсы освобождаются, данные etcd сохраняются.

### 3. Сервисные методы

```go
// internal/service/project_service.go
func (s *ProjectService) SuspendProject(ctx context.Context, projectID string) error {
    project, exists := s.store.GetByID(ctx, projectID)
    if !exists {
        return domain.ErrProjectNotFound
    }
    if project.Status != domain.ProjectStatusActive {
        return fmt.Errorf("can only suspend an active project, current status: %s", project.Status)
    }

    project.Status = domain.ProjectStatusSuspended
    project.UpdatedAt = time.Now().UTC()
    if err := s.store.Update(ctx, project); err != nil {
        return err
    }

    return s.provisioner.SuspendProjectEnvironment(ctx, projectID)
}

func (s *ProjectService) ResumeProject(ctx context.Context, projectID string) error {
    project, exists := s.store.GetByID(ctx, projectID)
    if !exists {
        return domain.ErrProjectNotFound
    }
    if project.Status != domain.ProjectStatusSuspended {
        return fmt.Errorf("can only resume a suspended project, current status: %s", project.Status)
    }

    if err := s.provisioner.ResumeProjectEnvironment(ctx, projectID); err != nil {
        return err
    }

    project.Status = domain.ProjectStatusActive
    project.UpdatedAt = time.Now().UTC()
    return s.store.Update(ctx, project)
}
```

### 4. Новые эндпоинты

```
POST /projects/{id}/suspend   — приостановить проект
POST /projects/{id}/resume    — возобновить проект
```

```go
// internal/http/router.go
mux.HandleFunc("POST /projects/{id}/suspend", handler.SuspendProject)
mux.HandleFunc("POST /projects/{id}/resume",  handler.ResumeProject)
```

### 5. Биллинг при suspended

При статусе `suspended` CPU и память не потребляются — не тарифицируются. В будущем: хранилище данных (PVC) продолжает тарифицироваться.

### 6. Блокировка деплоев при suspended

В `BootstrapGitHubFlow` добавить проверку:

```go
if project.Status == domain.ProjectStatusSuspended {
    return domain.BootstrapGitHubFlowResponse{}, errors.New("project is suspended, resume it first")
}
```

## Готово, если

- `POST /projects/{id}/suspend` — vcluster останавливается, статус меняется на `suspended`
- `POST /projects/{id}/resume` — vcluster запускается, статус меняется на `active`
- Попытка задеплоить в suspended проект — ошибка с понятным сообщением
- Mock-реализация Provisioner обновлена с новыми методами
