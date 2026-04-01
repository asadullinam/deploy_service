# TASK-036 [Done]: Bugfix — деплой должен идти в vcluster проекта, а не в host cluster

## Контекст

Сценарий пользователя: проект создается в deploy-service, затем через GitHub Actions выполняется деплой, после чего в UI должны отображаться pod'ы, runtime-статус, публичный адрес и релизы по выбранному stage.

Фактически наблюдается расхождение:

- GitHub Actions показывает успешный деплой.
- В UI по stage (`/projects/{id}/stages/{stageId}/runtime-status`) пусто: `deploymentExists=false`, `serviceExists=false`, `pods=[]`.
- В host cluster при этом ресурсы появляются в namespace `production`.

Это означает, что workflow деплоит не в vcluster проекта, а в host cluster.

## Проблема

- Текущий поток использует `KUBECONFIG_BASE64`, который на практике часто указывает на общий kubeconfig host cluster.
- UI и backend runtime для stage читают состояние из vcluster проекта.
- Из-за этого “успешный деплой” и “пустой UI” относятся к разным кластерам.

## Что сделать

### 1. Зафиксировать целевой контракт деплоя

- Деплой GitHub Actions для проекта обязан выполняться в vcluster этого проекта.
- Stage runtime в UI должен отражать тот же контур, куда выкатывает workflow.

### 2. Исправить источник kubeconfig для workflow

- Привязать `KUBECONFIG_BASE64` к kubeconfig конкретного проекта/vcluster.
- Убрать двусмысленность в инструкции первичной настройки (не подсказывать общий admin kubeconfig host cluster как основной путь для project deploy).

### 3. Добавить fail-fast проверку в workflow

- Перед `kubectl apply` проверять, что текущий kubeconfig указывает на vcluster проекта.
- Если kubeconfig указывает на host cluster — падать с понятной ошибкой и подсказкой.

### 4. Привести UI/тексты к корректному сценарию

- Обновить warning/инструкции в UI для `KUBECONFIG_BASE64`, чтобы пользователь не мог случайно настроить host cluster вместо project vcluster.

### 5. Тесты и регрессии

- Добавить/обновить тесты генерации workflow и проверки валидации kubeconfig.
- Добавить интеграционную проверку “деплой -> stage runtime непустой” в рамках существующего e2e сценария.

## Файлы (ориентировочно)

- `deploy-service/internal/integration/github/github_automation.go`
- `deploy-service/internal/integration/github/github_automation_test.go`
- `deploy-service/internal/integration/github/snapshot_test.go`
- `deploy-service/internal/http/ui/index.html`
- `deploy-service/internal/http/ui/app.js`
- `deploy-service/README.md`

## Готово, если

- [x] После успешного workflow деплоя по проекту stage runtime в UI показывает реальные ресурсы (`deploymentExists=true`, `serviceExists=true`, `pods` не пустой при запущенном приложении).
- [x] Workflow не может “молча” задеплоить в host cluster при неверном kubeconfig: должен падать с понятной диагностикой.
- [x] Инструкция в UI по `KUBECONFIG_BASE64` однозначно ведет к kubeconfig проекта/vcluster.
- [x] Регрессионные тесты покрывают этот сценарий.

## Мини-отчет

- В генерации workflow добавлена защита от деплоя в host cluster:
  - проверка, что kubeconfig не “видит” namespace `project-prj-*`/`project-${PROJECT_ID}`;
  - понятная ошибка с инструкцией использовать кнопку `Скопировать KUBECONFIG_BASE64` из UI.
- В workflow добавлены проверки источника auth/kubeconfig, нормализация image tag в lowercase и диагностика rollout timeout.
- В UI/README обновлены инструкции: `KUBECONFIG_BASE64` должен быть из kubeconfig проекта (vcluster), а не из admin kubeconfig host-кластера.
- Для кнопки `Скопировать KUBECONFIG_BASE64` добавлена стабильность:
  - backend теперь ждёт появление секрета `vc-vcluster-*` с ретраями;
  - при недоступном контуре возвращается читаемый `409` (`project environment unavailable`) вместо сырой ошибки `kubectl`.
- Во frontend улучшен разбор ошибок text-endpoints (`/kubeconfig`, `/kubeconfig/rotate`) — показывается человекочитаемое поле `error` из JSON.
- Обновлены/добавлены тесты:
  - `internal/integration/github`: workflow snapshot/unit проверки fail-fast и `PROJECT_ID`;
  - `internal/integration/kubernetes`: ретраи получения kubeconfig secret;
  - `internal/service` и `internal/http`: корректная маппинг/выдача `ErrProjectEnvironmentUnavailable`;
  - `internal/http` integration test: добавлена проверка stage runtime (`/projects/{id}/stages/{stageId}/runtime-status`) после simulated deploy.
- Локальная проверка: `go test ./...` в `deploy-service` — успешно.
