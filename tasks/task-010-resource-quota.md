# [Done] TASK-010: Resource Quota на namespace проекта

## Контекст

Решение из RFC-001: при создании namespace устанавливать `ResourceQuota` с базовыми лимитами. Без квот один активный проект может поглотить ресурсы всего кластера и положить соседей.

## Зависимости

ARCH-001. Изменение только в `KubectlProvisioner`.

## Базовые лимиты (MVP)

| Ресурс            | Лимит   |
| ----------------- | ------- |
| CPU (requests)    | 100m    |
| CPU (limits)      | 2 cores |
| Память (requests) | 128Mi   |
| Память (limits)   | 2Gi     |
| Количество Pod    | 10      |

## Что сделать

### 1. Добавить ResourceQuota в CreateProjectEnvironment

```go
// internal/integration/kubernetes/kubectl_provisioner.go

func (p *KubectlProvisioner) CreateProjectEnvironment(ctx context.Context, projectID string) error {
    namespace := namespaceFromProjectID(projectID)
    // ... существующие шаги (namespace, vcluster, network policies) ...

    // Добавить после network policies:
    if err := p.applyResourceQuota(ctx, namespace); err != nil {
        return fmt.Errorf("failed to apply resource quota in namespace %s: %w", namespace, err)
    }

    return nil
}

func (p *KubectlProvisioner) applyResourceQuota(ctx context.Context, namespace string) error {
    manifest := fmt.Sprintf(`apiVersion: v1
kind: ResourceQuota
metadata:
  name: project-quota
  namespace: %s
spec:
  hard:
    requests.cpu:    "100m"
    requests.memory: "128Mi"
    limits.cpu:      "2"
    limits.memory:   "2Gi"
    pods:            "10"
`, namespace)
    return p.kubectlApply(ctx, manifest)
}
```

### 2. Конфигурируемые лимиты (для тарифных планов)

Сейчас лимиты захардкожены — это нормально для MVP. Когда появятся тарифные планы, нужно будет параметризовать:

```go
type QuotaConfig struct {
    CPURequests    string
    CPULimits      string
    MemoryRequests string
    MemoryLimits   string
    MaxPods        int
}

// Provisioner.CreateProjectEnvironment принимает QuotaConfig
```

### 3. Mock

```go
// internal/integration/kubernetes/mock.go — ничего менять не нужно
// ProvisionerMock.CreateProjectEnvironment уже возвращает nil
```

## Готово, если

- После `POST /projects` в namespace появляется `ResourceQuota`
- Попытка задеплоить приложение с resource requests > лимитов получает отказ от Kubernetes
- `kubectl describe resourcequota project-quota -n project-{id}` показывает правильные лимиты
