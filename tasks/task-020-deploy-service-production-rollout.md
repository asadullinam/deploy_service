# [Done] TASK-020: Production rollout для самого deploy-service

## Проблема

Платформа уже умеет раскатывать пользовательские приложения, но сам `deploy-service` тоже должен обновляться как нормальный production-сервис.

Сейчас это важно по трем причинам:

- изменения в backend должны попадать в кластер без ручных действий
- rollout должен быть проверяемым и откатываемым
- secrets и env для самого сервиса должны быть оформлены как часть релиза, а не как ручная магия

## Что сделать

### 1. Описать production deployment для deploy-service

Нужны Kubernetes manifests для самого сервиса:

- `Deployment`
- `Service`
- при необходимости `Secret` или `ConfigMap`

Сервис должен запускаться в отдельном namespace, например:

- `deploy-service`

### 2. Передавать env/secrets явно

Нужно оформить список обязательных переменных окружения для production-запуска:

- `DATABASE_URL`
- `JWT_SECRET`
- `ENCRYPTION_KEY`
- `KUBECONFIG_BASE64`
- `YC_OAUTH_TOKEN` или `YC_SERVICE_ACCOUNT_KEY_JSON`
- `GITHUB_WEBHOOK_SECRET`
- `APPS_BASE_DOMAIN`
- `DEFAULT_USER_BALANCE_USD`

Секреты не должны хардкодиться в workflow или image.

### 3. Добавить health/readiness verification

Rollout самого `deploy-service` должен проверяться не только фактом успешного `kubectl apply`.

Нужны проверки:

- `readinessProbe`
- `livenessProbe`
- `kubectl rollout status`
- проверка `GET /health`
- проверка хотя бы одного защищённого endpoint после старта

### 4. Подготовить rollback-сценарий

Если новая версия сервиса не проходит rollout, должен быть понятный способ отката:

- предыдущий image tag
- `kubectl rollout undo`
- либо возвращение на предыдущий revision

Rollback должен быть частью production процесса, а не ручным emergency-only действием.

### 5. Сделать CI/CD для deploy-service прозрачным

Нужно, чтобы новый коммит в `master`:

- собирал image
- пушил его в registry
- обновлял Kubernetes deployment
- ждал успешного rollout
- падал, если сервис не поднялся

### 6. Добавить тесты и контрольные точки

Нужны проверки:

- манифесты валидны
- workflow применяет правильные env/secrets
- rollout завершается успешно на тестовом кластере или в mock harness
- health endpoint доступен

## Готово, если

- `deploy-service` можно катить в кластер без ручных шагов
- новый релиз проверяется по health/readiness, а не только по факту `apply`
- есть понятный rollback
- secrets и env оформлены как часть production release process
