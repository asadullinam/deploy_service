# [Done] TASK-016: Сквозные integration/e2e тесты пользовательского сценария

## Проблема

Сейчас тесты хорошо покрывают отдельные куски:

- auth service
- project service
- http handlers
- github workflow generation
- memory/postgres store contracts

Но у платформы главный риск находится на стыке слоёв. Можно иметь зелёные unit-тесты и всё равно получить сломанный пользовательский путь:

`register -> create project -> save deployment settings -> bootstrap -> merge -> workflow -> release visible in UI`

## Что сделать

### 1. Собрать минимальный test matrix

Нужны хотя бы два сквозных сценария:

#### Сценарий A. API-only

1. register
2. login
3. create project
4. save deployment settings
5. request github questions
6. bootstrap
7. get kubeconfig

#### Сценарий B. Webhook/release lifecycle

1. create project
2. simulate workflow_run webhook started
3. simulate workflow_run webhook success/failure
4. check releases list
5. rollback

### 2. Поднять test harness

Варианты:

- `httptest.Server` + memory stores + stub integrations
- отдельный integration suite с PostgreSQL через `TEST_DATABASE_URL`

### 3. Добавить contract-тесты для adapters

Особенно полезны общие contract tests для:

- `ProjectStore`
- `UserStore`
- `ReleaseStore`
- `Provisioner`

Идея: одна таблица поведения, много реализаций.

### 4. Отдельно покрыть workflow generation snapshots

Хорошо иметь snapshot/fixture tests на:

- `deployment.yaml`
- `service.yaml`
- `httproute.yaml`
- `.github/workflows/deploy-service.yml`

Это поможет безопасно развивать генератор без случайных регрессий.

## Готово, если

- Есть минимум один автоматический тест, который проходит путь пользователя от auth до deploy bootstrap
- Есть автоматический тест жизненного цикла release/webhook
- Изменения в workflow/manifests ловятся тестами до ручной проверки в GitHub Actions

## Статус проверки

**Выполнена частично.**

**Реализовано:**

- Обширный интеграционный тест-сьют `tests/api/` на `httptest.Server` + in-memory stores: auth, CRUD проектов, lifecycle, releases, webhook, kubeconfig, cost
- Unit-тесты для `renderWorkflowYAML`, `renderHTTPRouteYAML`, `detectPortFromDockerfile`

**Не реализовано:**

- Нет сквозного теста **Сценарий A**: `register -> login -> create project -> deployment-settings -> questions -> bootstrap -> kubeconfig` в одном тесте
- Нет сквозного теста **Сценарий B**: `create project -> webhook in_progress -> webhook success -> check releases -> rollback`
- Нет snapshot-тестов на сгенерированные артефакты: `deployment.yaml`, `service.yaml`, `httproute.yaml`, `deploy-service.yml` (изменения в шаблонах не поймать без ручной проверки в GitHub Actions)
- Нет contract-тестов для реализаций store (memory vs postgres соответствуют одному интерфейсу, но не тестируются совместно)

## Реализация

### Файлы

**`tests/api/e2e_test.go`** — два сквозных сценария поверх живого `httptest.Server` с in-memory stores и stub-интеграциями (mock k8s + mock github).

**`internal/integration/github/snapshot_test.go`** — snapshot-тесты для всех генерируемых артефактов.

**`internal/store/memory/contract_test.go`** — contract-тесты для store-интерфейсов, запускаемые на memory-реализациях.

---

### Сценарий A — `TestE2E_ScenarioA`

Полный путь от регистрации до получения kubeconfig в одном тесте:

1. `POST /auth/register` -> получаем токен
2. `POST /auth/login` -> логинимся, берём свежий токен
3. `POST /projects` -> создаём проект, запоминаем `projectID`
4. `PUT /projects/{id}/deployment-settings` -> сохраняем все 8 полей (serviceName, dockerfilePath, containerPort, servicePort, serviceType, baseBranch, repositoryOwner, repositoryName)
5. `GET /projects/{id}` -> проверяем, что поля персистировались
6. `POST /billing/top-up` -> пополняем баланс (bootstrap требует ненулевой баланс)
7. `POST /projects/{id}/github/questions` -> получаем вопросы от mock-автоматизации
8. `POST /projects/{id}/github/bootstrap` -> получаем `branchName` и `pullRequestUrl`
9. `GET /projects/{id}/kubeconfig` -> получаем YAML (plain text, не JSON)

---

### Сценарий B — `TestE2E_ScenarioB`

Полный жизненный цикл релиза через webhook:

1. `POST /projects` + `PUT /projects/{id}/deployment-settings` с `repositoryOwner`/`repositoryName`
2. Webhook `workflow_run` с `status=in_progress` -> Release автоматически создаётся в БД (новое поведение 013-B)
3. `GET /projects/{id}/releases` -> 1 запись, `status=building`, `workflowRunId` совпадает, `commitSha=deadbeef`
4. Webhook `workflow_run` с `status=completed`, `conclusion=success`
5. `GET /projects/{id}/releases/{id}` -> `status=success`
6. `POST /projects/{id}/releases/{id}/rollback` -> новая Release-запись с новым ID
7. `GET /projects/{id}/releases` -> 2 записи

Webhook подписывается корректной HMAC-SHA256 подписью через `githubSig()` из `webhook_test.go`.

---

### Snapshot-тесты — `snapshot_test.go`

Каждый тест вызывает пакетную функцию-рендерер напрямую и проверяет набор обязательных фрагментов:

| Тест                                          | Функция                                                               | Ключевые проверки                                                                                                                                                                                                            |
| --------------------------------------------- | --------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `TestSnapshotDeploymentYAML`                  | `renderDeploymentYAML("my-svc", 8080, 80)`                            | apiVersion, kind, IMAGE_PLACEHOLDER, containerPort, readinessProbe, livenessProbe, resource limits                                                                                                                           |
| `TestSnapshotServiceYAML_LoadBalancer`        | `renderServiceYAML(..., "LoadBalancer")`                              | port, targetPort, type: LoadBalancer                                                                                                                                                                                         |
| `TestSnapshotServiceYAML_ClusterIP`           | `renderServiceYAML(..., "ClusterIP")`                                 | type: ClusterIP, отсутствие LoadBalancer                                                                                                                                                                                     |
| `TestSnapshotHTTPRouteYAML`                   | `renderHTTPRouteYAML("my-svc", "prj-abc123", "apps.example.com", 80)` | gateway.networking.k8s.io/v1, platform-gateway, hostname prj-abc123.apps.example.com, backendRef                                                                                                                             |
| `TestSnapshotWorkflowYAML_WithHTTPRoute`      | `renderWorkflowYAML(..., true)`                                       | все шаги: checkout, docker login, build-push, configure kubectl, yc install, retry_or_fail, namespace apply, deployment/service/httproute apply, **Verify deployment**, rollout status, get pods, HTTPRoute acceptance check |
| `TestSnapshotWorkflowYAML_WithoutHTTPRoute`   | `renderWorkflowYAML(..., false)`                                      | httproute.yaml отсутствует, Verify deployment присутствует                                                                                                                                                                   |
| `TestSnapshotWorkflowYAML_DockerfileInSubdir` | `renderWorkflowYAML(..., "backend/Dockerfile", ...)`                  | `context: backend`, `file: backend/Dockerfile`                                                                                                                                                                               |

---

### Contract-тесты — `contract_test.go`

Паттерн: функция `RunXxxStoreContract(t, store)` принимает любую реализацию интерфейса и прогоняет набор sub-тестов через `t.Run`.

**`RunProjectStoreContract`** проверяет:

- `Create` + `GetByID` — объект сохраняется и читается корректно
- `GetByID` возвращает `false` для несуществующего ID
- `List` возвращает созданный проект
- `Update` персистирует изменения полей (Status, ServiceName)

**`RunReleaseStoreContract`** проверяет:

- `Create` + `GetByID`
- `GetByID` возвращает `false` для несуществующего
- `ListByProject` возвращает только релизы нужного проекта
- `ListByProject` возвращает пустой список для неизвестного проекта
- `GetByWorkflowRunID` находит релиз по run ID
- `GetByWorkflowRunID` возвращает `false` для несуществующего run ID
- `Update` персистирует изменение статуса

Вызовы на memory-реализациях:

```go
func TestMemoryProjectStoreContract(t *testing.T) { RunProjectStoreContract(t, NewProjectStore()) }
func TestMemoryReleaseStoreContract(t *testing.T)  { RunReleaseStoreContract(t, NewReleaseStore()) }
```

Для подключения postgres-реализации достаточно добавить аналогичные функции в `store/postgres/` с `TEST_DATABASE_URL`-условием.
