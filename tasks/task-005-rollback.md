# TASK-005: Откат к предыдущей версии (Rollback) [Done]

## Проблема

Сейчас нет возможности откатиться к предыдущей рабочей версии приложения. После неудачного деплоя пользователь застрял.

## Зависимости

TASK-004 (нужна история Release с image_tag каждого успешного деплоя).

## Что сделать

### 1. Новый эндпоинт

```
POST /projects/{id}/releases/{releaseId}/rollback
```

### 2. Логика отката

Откат — это новый деплой с образом из прошлого успешного Release.

```go
func (s *ProjectService) RollbackToRelease(ctx context.Context, projectID, releaseID string) (domain.Release, error) {
    // 1. Найти целевой release
    target, err := s.releaseStore.GetByID(ctx, releaseID)
    if err != nil || target.ProjectID != projectID {
        return domain.Release{}, domain.ErrReleaseNotFound
    }
    if target.Status != domain.ReleaseStatusSuccess {
        return domain.Release{}, errors.New("can only rollback to a successful release")
    }

    // 2. Создать новый Release с тем же ImageTag
    rollbackRelease := domain.Release{
        ID:        newID(),
        ProjectID: projectID,
        ImageTag:  target.ImageTag,    // образ из прошлого деплоя
        CommitSHA: target.CommitSHA,
        Status:    domain.ReleaseStatusPending,
    }
    s.releaseStore.Create(ctx, rollbackRelease)

    // 3. Применить манифест напрямую через Kubernetes
    err = s.provisioner.ApplyImage(ctx, projectID, target.ImageTag)

    // 4. Обновить статус
    rollbackRelease.Status = domain.ReleaseStatusSuccess
    s.releaseStore.Update(ctx, rollbackRelease)

    return rollbackRelease, nil
}
```

### 3. Новый метод в Kubernetes Provisioner

```go
// internal/integration/kubernetes/provisioner.go
type Provisioner interface {
    CreateProjectEnvironment(ctx context.Context, projectID string) error
    DeleteProjectEnvironment(ctx context.Context, projectID string) error
    ApplyImage(ctx context.Context, projectID string, imageTag string) error  // новый
}
```

`ApplyImage` делает `kubectl -n <namespace> set image deployment/<service> <container>=<imageTag>`.

### 4. Хранение image_tag

Для работы rollback нужно, чтобы каждый успешный Release хранил точный `image_tag` (включая SHA коммита). Это приходит в webhook (поле `head_sha` в событии `workflow_run`) или строится из формулы при bootstrap.

## Готово, если

- `POST /projects/{id}/releases/{releaseId}/rollback` переключает запущенный контейнер на образ из выбранного release
- Можно откатиться только к Release со статусом `success`
- Откат создаёт новую запись в истории Release (а не перезаписывает старую)
