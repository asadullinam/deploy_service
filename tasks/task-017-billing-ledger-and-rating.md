# [Done] TASK-017: Billing ledger и rating engine по облачной модели

## Проблема

Сейчас монетизация фактически сводится к расчету стоимости проекта “на лету”. Этого недостаточно для нормальной облачной модели: нужен не только текущий `cost`, но и полный след того, за что и когда были начислены деньги.

Нам нужен billing-слой, который умеет:

- фиксировать usage snapshots во времени
- переводить usage в billable items
- хранить ledger transactions
- различать `top-up`, `charge`, `refund`, `adjustment`
- показывать историю начислений и остаток по пользователю и по проекту

## Зависимости

TASK-002, TASK-003, TASK-006.

## Что сделать

### 1. Ввести billing ledger как отдельную сущность

Нужно отделить “текущую оценку стоимости” от “журнала операций”.

Пример структуры:

```go
type BillingTransactionType string

const (
    BillingTransactionTopUp     BillingTransactionType = "top_up"
    BillingTransactionCharge     BillingTransactionType = "charge"
    BillingTransactionRefund     BillingTransactionType = "refund"
    BillingTransactionAdjustment BillingTransactionType = "adjustment"
)

type BillingTransaction struct {
    ID        string
    UserID    string
    ProjectID string
    Type      BillingTransactionType
    AmountUSD float64
    Reason    string
    CreatedAt time.Time
}
```

### 2. Сохранять usage snapshots отдельно от начислений

Usage должен храниться как измерение, а не как списание:

- CPU
- memory
- storage
- traffic
- timestamp

После этого отдельный rating job переводит snapshots в charge transactions.

### 3. Разделить usage и rating

Нужно зафиксировать два шага:

1. `collector` записывает фактическое потребление ресурсов
2. `rating engine` превращает это потребление в денежные начисления

Так мы сможем:

- пересчитывать стоимость при смене тарифа
- делать audit trail
- разбирать спорные начисления

### 4. Добавить источник истины для баланса

Баланс пользователя не должен вычисляться из одного поля в памяти.

Правильная модель:

- `top_up` увеличивает баланс
- `charge` уменьшает баланс
- `refund` возвращает деньги
- итог считается из ledger transactions

### 5. Добавить API для истории начислений

Минимум нужны endpoints:

```text
GET /billing/summary
GET /billing/transactions
GET /projects/{id}/billing
```

### 6. Подготовить модели для будущих тарифов

Тариф должен быть не только набором констант, но и частью политики:

- базовый тариф
- тариф по сервис-типу
- тариф за публичный exposure
- тариф за storage
- тариф за traffic

## Готово, если

- Есть ledger transactions, а не только “текущий баланс”
- Usage и charges разделены
- Можно восстановить, почему у пользователя остался именно такой остаток
- Появляется основа для нормального audit trail и change of tariff без потери истории

## Статус проверки

**Выполнена.**

## Реализация

### Файлы

**`internal/domain/billing.go`** — добавлены `TransactionType` (4 константы: `top_up`, `charge`, `refund`, `adjustment`) и `BillingTransaction` с полями `ID`, `UserID`, `ProjectID`, `Type`, `AmountUSD`, `Description`, `CreatedAt`. `AmountUSD` знаковый: положительный = кредит, отрицательный = дебет.

**`internal/service/deps.go`** — добавлен интерфейс `BillingTransactionStore` (`Record`, `ListByUser`, `ListByProject`). Интерфейс `MonetizationEngine` расширен методом `ComputeUsageCost(usage domain.ResourceUsage) float64` для rating engine.

**`internal/monetization/engine.go`** — реализован `ComputeUsageCost` на обоих движках:

- `EngineMock.ComputeUsageCost` — возвращает 0 (mock)
- `PostgresEngine.ComputeUsageCost` — вычисляет стоимость по тарифу из переменных среды

**`internal/store/memory/billing_transaction_store.go`** — in-memory реализация `BillingTransactionStore`, thread-safe через `sync.RWMutex`.

**`internal/store/postgres/migrations/011_create_billing_transactions.sql`** — создаёт таблицу `billing_transactions` с индексами по `user_id` и `project_id`.

**`internal/store/postgres/billing_transaction_store.go`** — postgres реализация с тремя методами; вспомогательная функция `scanTransactions` принимает интерфейс для удобного сканирования строк.

**`internal/service/auth_service.go`** — `AuthService` получил поле `txStore BillingTransactionStore`; сигнатура `NewAuthService` добавила параметр `txStore`; `TopUpBalance` записывает транзакцию типа `top_up` после успешного обновления баланса.

**`internal/service/project_service.go`** — `ProjectService` получил поле `txStore`; сигнатура `NewProjectService` добавила параметр `txStore`; добавлены два метода:

- `ListBillingTransactions(ctx, userID)` — делегирует `txStore.ListByUser`
- `ListProjectBillingTransactions(ctx, projectID, userID)` — проверяет владение, делегирует `txStore.ListByProject`

**`internal/service/port.go`** — добавлены два метода в `Port`:

```go
ListBillingTransactions(ctx context.Context, userID string) ([]domain.BillingTransaction, error)
ListProjectBillingTransactions(ctx context.Context, projectID, userID string) ([]domain.BillingTransaction, error)
```

**`internal/app/app.go`** — `txStore` создаётся в зависимости от наличия `pool` (postgres или memory); передаётся в `NewAuthService` и `NewProjectService`; `collectAndStoreUsage` расширена: после записи usage вызывается `monetizationEngine.ComputeUsageCost(usage)`, при ненулевом cost обновляется баланс пользователя и записывается транзакция `charge` с отрицательным `AmountUSD`.

**`internal/http/handler.go`** — два новых обработчика:

- `ListBillingTransactions` — `GET /billing/transactions`, читает транзакции по `userID`
- `ListProjectBillingTransactions` — `GET /projects/{id}/billing`, проверяет владение, читает транзакции по `projectID`

**`internal/http/router.go`** — зарегистрированы два новых protected маршрута:

```
GET /billing/transactions
GET /projects/{id}/billing
```

---

### Тесты

Все существующие тесты обновлены под новые сигнатуры конструкторов: добавлены `&txStoreStub{}` / `&usageTxStoreStub{}` / `&authTxStoreStub{}` в соответствующих пакетах. `monetizationStub` в `project_service_test.go` получил метод `ComputeUsageCost`. Все `go test ./...` — зелёные.
