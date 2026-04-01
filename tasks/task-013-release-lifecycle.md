# [Done] TASK-013: Полный жизненный цикл релиза и связь с GitHub Actions

## Проблема

Сущность `Release` уже есть, webhook уже умеет обновлять статус, rollback уже умеет создавать release-запись. Но пользовательский сценарий всё ещё неполный:

- bootstrap/create PR не создаёт release-запись как начало деплоя
- merge PR и запуск workflow не всегда отражаются в истории релизов
- UI может показывать пустую историю, даже если Actions реально отработал

Итог: у нас есть данные о деплое в GitHub Actions, но не всегда есть нормальная внутренняя история платформы.

## Что сделать

### 1. Определить момент создания Release

Нужно выбрать и зафиксировать модель:

- либо `Release` создаётся при bootstrap PR
- либо при merge в целевую ветку
- либо при первом webhook `workflow_run`

Рекомендуемый вариант:

- создавать `Release` при старте workflow по webhook `workflow_run`
- связывать по `workflow_run.id`

### 2. Расширить webhook-обработку

Сейчас webhook в основном обновляет уже существующую запись. Нужно, чтобы он умел:

- создать новый `Release`, если это первый event по данному `workflow_run.id`
- заполнить `commit_sha`
- заполнить `image_tag`
- сохранить `commit_message`
- перевести статус по мере движения workflow

### 3. Согласовать branch/PR/workflow модель

Нужно определить:

- какие workflow считаются “нашими”
- как отличать bootstrap workflow платформы от других workflow репозитория
- как маппить workflow run на конкретный `projectID`

Возможные варианты:

- через имя workflow
- через labels/annotations в PR
- через специальный env/input в workflow
- через branch naming convention

### 4. Доделать UI истории релизов

После этого UI должен показывать:

- релиз создан
- сборка идёт
- деплой идёт
- успешно / failed
- image tag
- commit SHA
- timestamp

### 5. Добавить тесты жизненного цикла

Нужны сценарные тесты:

1. workflow started -> release created
2. workflow in_progress -> status building/deploying
3. workflow success -> status success
4. workflow failure -> status failed
5. rollback -> новая release-запись, старая остаётся в истории

## Готово, если

- Каждый реальный deploy-run отражён в `releases`
- UI перестаёт показывать пустую историю при реально выполнявшихся деплоях
- Release-история позволяет понять, что, когда и с каким image было задеплоено

## Статус проверки

**Выполнена частично.**

**Реализовано:**

- Домен `Release` с полями `Status`, `WorkflowRunID`, `ImageTag`, `CommitSHA`
- Webhook обновляет статус существующего Release по `WorkflowRunID` (`in_progress` -> `success`/`failed`)
- Rollback создаёт новую Release-запись
- UI отображает список релизов проекта

**Не реализовано:**

- Webhook **не создаёт** новую Release при первом появлении `workflow_run.id` — если запись не найдена, молча игнорирует (`Not our workflow — ignore`), что ломает автоматическую историю деплоев
- `commit_message` не заполняется из webhook (payload содержит только `head_sha`, message не парсится)
- Нет маппинга `workflow_run.id` -> `projectID` на этапе создания — webhook не может привязать run к проекту без предварительно созданной записи
- Нет сценарных тестов жизненного цикла: `queued -> in_progress -> success`, повторный деплой, failed workflow, rollback
