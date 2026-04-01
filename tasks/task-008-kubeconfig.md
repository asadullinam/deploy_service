# TASK-008: Выдача kubeconfig пользователю [Done]

## Контекст

Решение из RFC-001: платформа выдаёт kubeconfig для подключения к виртуальному кластеру проекта. Пользователь может использовать `kubectl` напрямую — устанавливать Helm-чарты, смотреть логи, управлять ресурсами внутри своей зоны. Безопасно: kubeconfig ведёт во vcluster, не в хост-кластер.

## Зависимости

ARCH-001, TASK-002 (нужна БД для хранения kubeconfig), TASK-003 (auth — kubeconfig только своего проекта).

## Что сделать

### 1. Получение kubeconfig при создании vcluster

При `CreateProjectEnvironment` после создания vcluster вызвать:

```bash
vcluster connect <name> --namespace <namespace> --print-config
```

Это возвращает kubeconfig в stdout. Сохранить его в БД.

```go
// internal/integration/kubernetes/provisioner.go — расширить интерфейс
type Provisioner interface {
    CreateProjectEnvironment(ctx context.Context, projectID string) error
    DeleteProjectEnvironment(ctx context.Context, projectID string) error
    GetProjectKubeconfig(ctx context.Context, projectID string) (string, error)
}
```

### 2. Хранение в БД

```sql
-- migrations/002_add_kubeconfig.sql
ALTER TABLE projects ADD COLUMN kubeconfig_encrypted TEXT;
```

Kubeconfig шифруется перед записью — он содержит токены доступа. Для MVP можно использовать AES-256-GCM с ключом из env-переменной `ENCRYPTION_KEY`.

```go
// internal/crypto/aes.go
func Encrypt(plaintext string, key []byte) (string, error)
func Decrypt(ciphertext string, key []byte) (string, error)
```

### 3. Новые поля в domain.Project

```go
type Project struct {
    // ... существующие поля
    KubeconfigEncrypted string `json:"-"` // не отдавать в JSON по умолчанию
}
```

### 4. Новые эндпоинты

```
GET  /projects/{id}/kubeconfig          — вернуть kubeconfig (YAML)
POST /projects/{id}/kubeconfig/rotate   — перегенерировать kubeconfig
```

`GET /projects/{id}/kubeconfig` возвращает Content-Type: `application/yaml`, а не JSON.

### 5. Сервисный метод

```go
// service/port.go — добавить
GetProjectKubeconfig(ctx context.Context, projectID string) (string, error)
```

```go
func (s *ProjectService) GetProjectKubeconfig(ctx context.Context, projectID string) (string, error) {
    project, exists := s.store.GetByID(ctx, projectID)
    if !exists {
        return "", domain.ErrProjectNotFound
    }
    if project.Status != domain.ProjectStatusActive {
        return "", errors.New("project is not active")
    }
    // расшифровать и вернуть
    return s.crypto.Decrypt(project.KubeconfigEncrypted)
}
```

### 6. Схема хранения в store

```go
// store — добавить в интерфейс
UpdateKubeconfig(ctx context.Context, projectID string, encryptedKubeconfig string) error
```

## Новые переменные окружения

| Переменная       | Описание                                                                      |
| ---------------- | ----------------------------------------------------------------------------- |
| `ENCRYPTION_KEY` | 32-байтный ключ для AES-256-GCM (base64). Обязателен если DATABASE_URL задан. |

## Готово, если

- `GET /projects/{id}/kubeconfig` возвращает рабочий kubeconfig для vcluster
- Kubeconfig хранится зашифрованным в БД
- Пользователь без токена или чужого проекта получает 401/403
- `kubectl get pods --kubeconfig <файл>` работает и показывает поды проекта
