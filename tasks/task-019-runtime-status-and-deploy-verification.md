# [Done] TASK-019: Runtime status и подтверждение реального деплоя

## Проблема

Сейчас пользователь может увидеть, что GitHub Actions или backend “успешно отработали”, но этого недостаточно, чтобы понять, что приложение действительно поднялось в Kubernetes и стало доступно.

Нужно отделить:

- успешную сборку image
- успешный `kubectl apply`
- реальный живой runtime в кластере

Иначе платформа даёт ложное ощущение готовности.

## Что сделать

### 1. Добавить runtime status как отдельную сущность ответа

Нужен отдельный DTO/API-ответ, который показывает:

- существует ли namespace проекта
- существует ли `Deployment`
- существует ли `Service`
- есть ли `HTTPRoute`, если он используется
- сколько pod’ов запущено
- сколько pod’ов `Ready`
- сколько реплик ожидается и сколько реально доступно
- timestamp последней проверки

### 2. Добавить endpoint проверки состояния развертывания

Нужен отдельный endpoint вида:

```http
GET /projects/{id}/runtime-status
```

Он должен возвращать не “красивую абстракцию”, а реально полезное состояние:

- namespace найден или нет
- deployment найден или нет
- service найден или нет
- route найден или нет
- pod phase / ready / restart count

### 3. Реализовать проверку через Kubernetes adapter

Provisioner должен уметь читать состояние кластера, а не только создавать ресурсы.

Минимальный набор проверок:

- `kubectl get namespace`
- `kubectl get deployment -n <namespace> -o json`
- `kubectl get service -n <namespace> -o json`
- `kubectl get pods -n <namespace> -o json`
- при наличии Gateway API:
  - `kubectl get httproute -n <namespace> -o json`

### 4. Показать это в UI

В консоли нужно добавить отдельный блок “Состояние в кластере” или похожий:

- `namespace exists`
- `deployment ready`
- `service exists`
- `pods ready`
- `last checked`

Если деплой не найден, UI должен показывать это прямо, а не оставлять пользователя гадать.

### 5. Добавить тесты поведения

Нужны тесты на:

- отсутствие namespace
- namespace есть, но deployment еще не применён
- deployment есть, pod’ы не `Ready`
- deployment есть, pod’ы `Ready`
- route присутствует / отсутствует

## Готово, если

- Пользователь может понять, что приложение реально развернуто, а не просто “workflow прошёл”
- Runtime status доступен через API
- Runtime status виден в UI
- Есть тесты на ключевые сценарии состояния кластера

## Статус проверки

**Выполнена.**

## Мини-отчет

- Расширен DTO `domain.ProjectRuntimeStatus`: добавлено поле `httpRouteExists` для явной проверки Gateway API маршрута.
- В `KubectlProvisioner.GetProjectRuntimeStatus` добавлена проверка `kubectl get httproute -n <namespace> -o json` с безопасным fallback, если `HTTPRoute` CRD недоступен в кластере.
- В `KubectlProvisioner.GetStageRuntimeStatus` добавлены недостающие проверки `Service` и `HTTPRoute`, чтобы runtime во вкладке `Состояние` был консистентен для stage-aware режима.
- Обновлены mock/test provisioner-ы: runtime теперь умеет возвращать `httpRouteExists`, чтобы тестовые и UI-сценарии отражали реальное состояние.
- UI-блок runtime доработан: в карточках состояния добавлен индикатор `HTTPRoute`, а тексты пустого состояния обновлены (теперь явно показываются namespace/deployment/service/HTTPRoute/pods).
- Добавлены юнит-тесты для Kubernetes adapter:
  - namespace not found;
  - namespace exists, deployment not applied;
  - deployment exists, pods not ready;
  - ready replicas, но route отсутствует;
  - ready replicas + route присутствует;
  - stage runtime с проверкой service + route.
- Прогон проверки: `go test ./...` — успешно.
