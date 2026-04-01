# [Deprecated] TASK-009: Gateway API — доступ к задеплоенным приложениям

## Контекст

Решение из RFC-001: Gateway API вместо Ingress. Один `Gateway` на кластер, каждый проект получает `HTTPRoute` в своём namespace с поддоменом `{project-id}.apps.deploy-platform.ru`. При bootstrap платформа автоматически добавляет `httproute.yaml` в PR вместе с `deployment.yaml` и `service.yaml`.

Gateway API выбран вместо Ingress из-за точного соответствия ролевой модели платформы:

- `GatewayClass` — инфраструктурная команда (один раз)
- `Gateway` — платформа управляет (один Gateway на кластер)
- `HTTPRoute` — владелец проект (живёт в namespace проекта)

## Зависимости

ARCH-001. Не требует аутентификации. Может выполняться параллельно с TASK-003.

## Требования к хост-кластеру

- Kubernetes 1.28+ (Gateway API GA)
- Установлен Gateway API контроллер (nginx gateway fabric, Envoy Gateway, Cilium или Istio)
- DNS: wildcard запись `*.apps.deploy-platform.ru -> Gateway external IP`
- Wildcard TLS-сертификат в namespace `gateway`

## Разовая настройка кластера (не в коде платформы)

```yaml
# GatewayClass — cluster-scoped, устанавливается один раз
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: nginx
spec:
  controllerName: k8s.nginx.org/nginx-gateway-controller
---
# Gateway — в выделенном namespace
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: platform-gateway
  namespace: gateway
spec:
  gatewayClassName: nginx
  listeners:
    - name: https
      port: 443
      protocol: HTTPS
      hostname: "*.apps.deploy-platform.ru"
      tls:
        mode: Terminate
        certificateRefs:
          - name: wildcard-tls
      allowedRoutes:
        namespaces:
          from: All
```

## Что сделать

### 1. Добавить `httproute.yaml` в генерацию bootstrap

В `BootstrapRepositoryFlow` добавить третий манифест рядом с `deployment.yaml` и `service.yaml`.

```go
// internal/integration/github/github_automation.go
httprouteContent := renderHTTPRouteYAML(serviceName, projectID, baseDomain)

a.upsertFile(ctx, ..., manifestBase+"/httproute.yaml", httprouteContent, "Add Kubernetes HTTPRoute")
```

```go
func renderHTTPRouteYAML(serviceName, projectID, baseDomain string) string {
    host := fmt.Sprintf("%s.%s", sanitizeName(projectID), baseDomain)
    name := sanitizeName(serviceName)
    return fmt.Sprintf(`apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: %s
spec:
  parentRefs:
    - name: platform-gateway
      namespace: gateway
  hostnames:
    - "%s"
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /
      backendRefs:
        - name: %s
          port: %d
`, name, host, name, servicePort)
}
```

### 2. Хранить публичный URL в Project

```go
// domain/project.go — добавить поле
type Project struct {
    // ... существующие поля
    PublicURL string `json:"publicUrl,omitempty"`
}
```

```sql
-- migrations/003_add_public_url.sql
ALTER TABLE projects ADD COLUMN public_url TEXT NOT NULL DEFAULT '';
```

После успешного bootstrap записывать URL в проект:

```
https://{project-id}.apps.deploy-platform.ru
```

### 3. Передавать baseDomain через конфигурацию

```go
// app/app.go
baseDomain := os.Getenv("APPS_BASE_DOMAIN") // например: apps.deploy-platform.ru
```

Передать в `GitHubAutomation` при инициализации.

### 4. Обновить NetworkPolicy

Текущая `default-deny-all` блокирует входящий трафик от Gateway-контроллера. Нужно добавить разрешающее правило для namespace, где живёт контроллер.

```go
// В kubectl_provisioner.go добавить манифест:
gatewayAllowPolicy := fmt.Sprintf(`apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-gateway-controller
  namespace: %s
spec:
  podSelector: {}
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: nginx-gateway
`, namespace)
```

Namespace `nginx-gateway` — стандартный для nginx gateway fabric. Для других контроллеров (Envoy Gateway — `envoy-gateway-system`, Cilium — `kube-system`) название отличается. Параметризовать через env `GATEWAY_CONTROLLER_NAMESPACE`.

## Новые переменные окружения

| Переменная                     | Пример                    | Описание                                        |
| ------------------------------ | ------------------------- | ----------------------------------------------- |
| `APPS_BASE_DOMAIN`             | `apps.deploy-platform.ru` | Базовый домен для поддоменов проектов           |
| `GATEWAY_CONTROLLER_NAMESPACE` | `nginx-gateway`           | Namespace Gateway-контроллера для NetworkPolicy |

## Готово, если

- После мержа PR приложение доступно по `https://{project-id}.apps.deploy-platform.ru`
- `GET /projects/{id}` возвращает `publicUrl` с адресом
- Трафик между проектами по-прежнему заблокирован (NetworkPolicy)
- `kubectl get httproute -n project-{id}` показывает созданный маршрут
