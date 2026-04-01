# [Done] TASK-022: Мигрировать с Ingress на Gateway API

## Контекст

### Текущее состояние

TASK-009 была помечена как выполненная, однако реализация отклонилась от RFC-001:
вместо `HTTPRoute` (Gateway API) во всех местах создаётся обычный `Ingress` с `ingressClassName: nginx`.

Затронутые места:

- `internal/integration/github/github_automation.go` — `renderIngressYAML()` генерирует `Ingress` в PR при bootstrap
- `internal/integration/kubernetes/kubectl_provisioner.go` — `applyProjectGrafana()` создаёт `Ingress` для Grafana
- `k8s/ingress-nginx/` — манифесты для ingress-nginx Controller (NodePort)

### Почему Gateway API лучше Ingress

#### 1. Разделение ролей

Ingress — плоская модель без ролей. Любое нестандартное поведение требует **аннотаций**, специфичных для контроллера:

```yaml
# Работает только с ingress-nginx — не переносимо
nginx.ingress.kubernetes.io/proxy-read-timeout: "60"
nginx.ingress.kubernetes.io/ssl-redirect: "true"
```

Gateway API вводит три ресурса с чёткими ролями:

| Ресурс         | Кто управляет  | Что задаёт                  |
| -------------- | -------------- | --------------------------- |
| `GatewayClass` | ops/infra      | какой контроллер, параметры |
| `Gateway`      | платформа (мы) | внешний IP, TLS wildcard    |
| `HTTPRoute`    | проект         | поддомен -> сервис          |

Для нашей платформы это точное соответствие: **платформа управляет Gateway, каждый проект владеет только своим HTTPRoute**.

#### 2. Переносимость

Ingress с `ingressClassName: nginx` привязан к ingress-nginx. Смена на Cilium, Envoy, Traefik — переписывать манифесты и аннотации.

HTTPRoute работает одинаково с nginx-gateway-fabric, Envoy Gateway, Cilium, Istio — один YAML.

#### 3. Текущий Ingress — минимальный

Весь наш Ingress сейчас — только host + path prefix + backend, никаких аннотаций:

```yaml
# Это всё что мы используем — HTTPRoute покрывает 1:1
ingressClassName: nginx
rules:
  - host: "{id}.{domain}"
    http:
      paths:
        - path: /
          pathType: Prefix
          backend: service:port
```

Миграция не теряет никакой функциональности.

#### 4. Правильное владение ресурсом

`HTTPRoute` живёт в namespace проекта. При удалении namespace он удаляется автоматически — так же как Ingress. Но HTTPRoute явно моделирует, что маршрут принадлежит проекту, а не платформе.

### Почему TASK-009 была закрыта неправильно

Скорее всего, Ingress выбрали ради скорости: ingress-nginx проще поднять локально и в CI, не требует установки Gateway API CRDs. Это технический долг — модель работает, но нарушает решение RFC-001 и привязывает платформу к конкретному контроллеру.

## Зависимости

Нет блокирующих зависимостей. Можно выполнять параллельно с другими задачами.

## Требования к хост-кластеру (разовая настройка, не в коде)

- Kubernetes 1.28+ (Gateway API GA)
- Установлен Gateway API контроллер (nginx-gateway-fabric, Envoy Gateway, Cilium или Istio)
- Удалён / не используется ingress-nginx
- DNS: wildcard запись `*.apps.<domain> -> Gateway external IP`
- Wildcard TLS-сертификат в namespace `gateway`

Разовые манифесты (`k8s/gateway/` — создать):

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: nginx
spec:
  controllerName: k8s.nginx.org/nginx-gateway-controller
---
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

### 1. `github_automation.go` — заменить `renderIngressYAML` на `renderHTTPRouteYAML`

```go
// Убрать:
func renderIngressYAML(serviceName, projectID, baseDomain string, servicePort int) string

// Добавить:
func renderHTTPRouteYAML(serviceName, projectID, baseDomain string, servicePort int) string {
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

Переменная `ingressContent` переименовывается в `httprouteContent`, файл в PR — `httproute.yaml` вместо `ingress.yaml`.

Убрать флаг `includeIngress` из `renderWorkflowYAML`, заменить на `includeHTTPRoute`. В workflow шаг деплоя применяет `httproute.yaml` вместо `ingress.yaml`.

### 2. `kubectl_provisioner.go` — заменить Ingress Grafana на HTTPRoute

В `projectGrafanaManifest()` заменить блок `kind: Ingress` на `kind: HTTPRoute`:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: project-grafana
  namespace: { namespace }
spec:
  parentRefs:
    - name: platform-gateway
      namespace: gateway
  hostnames:
    - "{grafana-host}"
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /
      backendRefs:
        - name: project-grafana
          port: 3000
```

### 3. NetworkPolicy — разрешить трафик от Gateway контроллера

Текущая `default-deny-all` блокирует входящий трафик. Добавить разрешающую политику в `CreateProjectEnvironment`:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-gateway-controller
  namespace: { namespace }
spec:
  podSelector: {}
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: { GATEWAY_CONTROLLER_NAMESPACE }
```

### 4. Новая переменная окружения

| Переменная                     | Пример          | Описание                                        |
| ------------------------------ | --------------- | ----------------------------------------------- |
| `GATEWAY_CONTROLLER_NAMESPACE` | `nginx-gateway` | Namespace Gateway-контроллера для NetworkPolicy |

Стандартные значения: nginx-gateway-fabric -> `nginx-gateway`, Envoy Gateway -> `envoy-gateway-system`.

### 5. Убрать `k8s/ingress-nginx/`

Директория с манифестами ingress-nginx становится неактуальной. Создать `k8s/gateway/` с манифестами GatewayClass и Gateway.

### 6. Обновить `.github/workflows/deploy-deploy-service.yml`

Убрать шаги установки ingress-nginx, добавить шаги установки Gateway API CRDs и контроллера.

## Готово, если

- `kubectl get httproute -n project-{id}` показывает созданный маршрут после bootstrap
- Приложение доступно по `https://{project-id}.{APPS_BASE_DOMAIN}`
- Grafana доступна по `https://grafana-{project-id}.{APPS_BASE_DOMAIN}`
- Трафик между namespace проектов по-прежнему заблокирован
- В репозитории нет ни одного `kind: Ingress` кроме тестов
- `k8s/ingress-nginx/` удалена, `k8s/gateway/` добавлена
