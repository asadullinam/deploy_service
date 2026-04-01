# [Done] TASK-018: Enforcement баланса и авто-suspend при нулевом остатке

## Проблема

Даже если billing ledger и rating engine работают, платформе нужен жёсткий guardrail:

- если денег нет, новые деплои нельзя запускать
- если баланс ушёл в минус, проект нужно переводить в ограниченный режим
- если после grace period пополнения нет, проект надо приостанавливать автоматически

Сейчас этот сценарий частично есть как ручная проверка перед bootstrap, но этого недостаточно как продуктовой политики.

## Зависимости

TASK-017.

## Что сделать

### 1. Ввести правило доступного баланса

Нужен единый алгоритм:

- `balanceUsd`
- минус `chargedUsd`
- плюс `refundUsd`
- минус pending charges

Результат должен использоваться не только для UI, но и для backend-gates.

### 2. Запретить новые deploy/bootstrap при недостатке средств

Перед:

- `Create PR`
- `BootstrapGitHubFlow`
- `Rollback`

нужно проверять, что `availableUsd` достаточен для запуска новой операции.

### 3. Добавить grace period

Когда баланс уходит в ноль или минус, проект не должен падать мгновенно в suspended.

Нужна политика:

- `grace period` в часах или днях
- уведомление пользователю
- повторная проверка после истечения срока

### 4. Автоматический suspend

Если баланс не пополнен в пределах grace period:

- project status переводится в `suspended`
- новые деплои блокируются
- существующий runtime либо останавливается, либо переводится в минимальный режим по выбранной политике

### 5. Дать понятную причину блокировки

При попытке деплоя пользователь должен видеть:

- текущий баланс
- сколько уже списано
- сколько нужно для нового действия
- сколько осталось до автопаузы

### 6. Подготовить background job для enforcement

Нужен периодический job, который:

- проходит по активным проектам
- считает available balance
- ставит `suspended`, если срок grace period истёк
- пишет понятный reason в audit/event log

## Готово, если

- При нулевом балансе новые деплои блокируются
- После grace period проект автоматически уходит в suspended
- Пользователь видит понятное объяснение, почему действие запрещено
- Политика enforcement работает одинаково для UI, API и background jobs

## Статус проверки

**Выполнена.**

## Мини-отчет

- В `ProjectService` реализован единый guard-алгоритм с учетом `pending charges`: добавлены in-memory reservations на время deploy-операций, чтобы конкурентные `bootstrap`/`rollback` не проходили одновременно при нехватке средств.
- `BillingSummary` расширен данными `refundUsd`, `pendingChargesUsd`, `gracePeriodEndsAt`, `gracePeriodRemainingSeconds`; `availableUsd` теперь считается с учетом pending/reserve.
- Для блокировки deploy-операций добавлена подробная причина ошибки (`ErrInsufficientBalance`): баланс, списано за месяц, refund, pending, available, required reserve и (при отрицательном остатке) оставшееся время до автопаузы.
- Guard применен к `BootstrapGitHubFlow` и `RollbackToRelease`; для rollback добавлено корректное HTTP-отображение `ErrInsufficientBalance` в `402 Payment Required`.
- `EnforceBillingGuard` переведен на grace-policy: при `available <= 0` запускается grace-window, авто-suspend происходит только после его истечения; при восстановлении баланса grace-состояние сбрасывается.
- В `app.go` добавлена настройка `BILLING_GUARD_GRACE_PERIOD` (по умолчанию `24h`), используется вместе с существующим `BILLING_GUARD_INTERVAL`.
- Обновлены и добавлены тесты (service/http/api): grace before suspend, suspend after grace expiry, rollback guard, конкурентный bootstrap с pending reserve, а также e2e-сценарий rollback с предварительным top-up.
- Прогон проверки: `go test ./...` — успешно.
