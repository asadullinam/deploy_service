# [Done] TASK-029: Ошибка создания stage при отсутствии namespace проекта

## Проблема

При создании нового stage возникает ошибка вида:

`get vcluster kubeconfig for project <id> ... namespaces "project-<id>" not found`

Система падает на получении kubeconfig/secret, если namespace проекта отсутствует в хост-кластере.

## Что сделать

### 1. Разобрать первопричину

- Проверить, при каких условиях namespace проекта отсутствует (удален вручную, не создан, проект в неконсистентном состоянии).
- Зафиксировать ожидаемый контракт: когда stage можно создавать, а когда нужно возвращать понятную бизнес-ошибку.

### 2. Исправить обработку создания stage

- Добавить предвалидацию состояния проекта перед `CreateStageEnvironment`.
- Вернуть пользователю читаемую ошибку без "сырого" трассинга внутренних команд.
- Если возможно, выполнить безопасное восстановление (reconcile) перед отказом.

### 3. Логирование и диагностика

- В сервисных логах сохранить техническую причину с деталями.
- В UI/API отдать краткое и понятное сообщение с рекомендацией следующего шага.

## Файлы (ориентировочно)

- `deploy-service/internal/service/project_service.go`
- `deploy-service/internal/infrastructure/provisioner.go`
- `deploy-service/internal/http/handler.go`
- `deploy-service/internal/domain/errors.go`

## Готово, если

- [x] Сценарий создания stage не падает "сырым" `kubectl` сообщением
- [x] При отсутствии namespace проекта пользователь получает понятную ошибку
- [x] Добавлены тесты на негативный сценарий (namespace/secret not found)
- [x] Логи содержат технические детали для диагностики

## Мини-отчет

- В `ProjectService.CreateStage` добавлена предвалидация окружения через `GetProjectKubeconfig` до вызова `CreateStageEnvironment`, чтобы не уходить в сырой `kubectl` трейс при битом окружении.
- Добавлен маппинг технических ошибок `namespace/secret not found` в доменную ошибку `ErrProjectEnvironmentUnavailable` с читаемым сообщением и рекомендацией следующего шага.
- В `createStage` сохранено техническое логирование полной причины ошибки, а в ответ пользователю отдается только санитизированная бизнес-ошибка.
- В `Handler.CreateStage` добавлена отдельная обработка `ErrProjectEnvironmentUnavailable` с HTTP `409 Conflict`.
- В `KubectlProvisioner.GetProjectKubeconfig` добавлен безопасный reconcile: при `namespace not found` provisioner пытается восстановить namespace и повторяет чтение kubeconfig secret.
- Добавлены unit-тесты для негативных сценариев в `project_service_test.go`, `handler_test.go` и `kubectl_provisioner_test.go`.
