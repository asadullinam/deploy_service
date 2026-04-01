# [Done] TASK-014: Надёжность GitHub Actions и реальная проверка деплоя

## Проблема

Мы уже столкнулись с несколькими характерными проблемами workflow:

- workflow был зелёным, хотя `kubectl apply` фактически не выполнился
- kubeconfig в GitHub Secrets указывал на `localhost:8080`
- runner не всегда мог аутентифицироваться в Yandex Cloud без дополнительного секрета
- success статуса workflow недостаточно, чтобы утверждать, что приложение реально развернуто

Для пользовательского сценария “нажал Deploy и приложение доступно” этого уровня надёжности недостаточно.

## Что сделать

### 1. Зафиксировать fail-fast поведение workflow

Любая неудача в:

- namespace apply
- deployment apply
- service apply
- httproute apply

должна завершать job с ошибкой.

### 2. Добавить post-deploy verification

После `kubectl apply` workflow должен проверять:

- что `Deployment` создан
- что rollout завершился успешно
- что `Pod` вышел в `Ready`

Минимум:

```bash
kubectl -n <namespace> rollout status deployment/<service> --timeout=180s
kubectl -n <namespace> get pods
```

### 3. Проверять доступность Service/Route

Если `ServiceType=LoadBalancer` или есть `HTTPRoute`, нужен ещё один слой проверки:

- Service существует
- endpoints появились
- если есть `HTTPRoute`, route accepted

### 4. Упростить модель секретов для Actions

Сейчас пользователю нужно понимать сразу несколько секретов:

- `KUBECONFIG_BASE64`
- `YC_OAUTH_TOKEN` или `YC_SERVICE_ACCOUNT_KEY_JSON`

Нужно оформить один рекомендованный путь и задокументировать его:

- какой kubeconfig нужен
- как именно его получить
- какие варианты аутентификации допустимы
- какие права нужны service account/token

### 5. Писать в release/status не только итог, но и причину падения

Если rollout упал, пользователю нужен не только `failed`, но и понятный summary:

- auth to cluster failed
- namespace apply failed
- rollout timeout
- image pull failed

## Готово, если

- Зелёный workflow означает реальный успешный rollout, а не только успешную сборку
- Красный workflow даёт понятную причину сбоя
- Документация по секретам и kubeconfig достаточно точная, чтобы новый пользователь мог настроиться без ручной отладки

## Статус проверки

**Выполнена частично.**

**Реализовано:**

- `retry_or_fail()` в сгенерированном workflow — 5 попыток с `sleep 10` для каждого `kubectl apply`

**Не реализовано:**

- Нет post-deploy verification шага: `kubectl rollout status deployment` и `kubectl get pods` после apply
- Нет проверки доступности Service/HTTPRoute после деплоя
- Release статус не содержит структурированной причины сбоя — только `failed` без деталей из workflow
- Нет документации по настройке секретов и kubeconfig для новых пользователей (отдельный раздел в onboarding)
