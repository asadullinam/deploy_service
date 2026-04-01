# [Done] TASK-006: Реальный движок монетизации

## Проблема

Сейчас `monetization.EngineMock` возвращает захардкоженные числа. Для MVP-биллинга нужно собирать реальное потребление ресурсов из Kubernetes и рассчитывать стоимость.

## Зависимости

ARCH-001, TASK-002.

## Что сделать

### 1. Схема хранения телеметрии

```sql
-- migrations/004_create_resource_usage.sql
CREATE TABLE resource_usage (
    id              TEXT        PRIMARY KEY,
    project_id      TEXT        NOT NULL REFERENCES projects(id),
    period_start    TIMESTAMPTZ NOT NULL,
    period_end      TIMESTAMPTZ NOT NULL,
    cpu_core_hours  FLOAT       NOT NULL DEFAULT 0,
    memory_gb_hours FLOAT       NOT NULL DEFAULT 0,
    storage_gb      FLOAT       NOT NULL DEFAULT 0,
    egress_gb       FLOAT       NOT NULL DEFAULT 0,
    recorded_at     TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_resource_usage_project_period ON resource_usage (project_id, period_start);
```

### 2. Сбор метрик из Kubernetes

Kubernetes Metrics API (`kubectl top pod`) возвращает мгновенные значения CPU и памяти. Нужно снимать их по расписанию (раз в час) и записывать в `resource_usage`.

```go
// internal/integration/kubernetes/metrics.go
type MetricsCollector interface {
    CollectProjectUsage(ctx context.Context, projectID string) (domain.ResourceSnapshot, error)
}

type domain.ResourceSnapshot struct {
    CPUCores    float64  // текущее потребление ядер
    MemoryGB    float64  // текущее потребление памяти
    StorageGB   float64  // постоянное хранилище
}
```

Реализация через `kubectl top pod -n <namespace> --no-headers`.

### 3. Фоновый агрегатор

```go
// internal/app/app.go — добавить горутину
go func() {
    ticker := time.NewTicker(1 * time.Hour)
    for range ticker.C {
        collectAndStoreUsage(ctx, projects, metricsCollector, usageStore)
    }
}()
```

### 4. Тарифные правила

```go
// internal/monetization/tariff.go
type Tariff struct {
    CPUCoreHourUSD    float64  // цена за 1 ядро в час
    MemoryGBHourUSD   float64  // цена за 1 GB памяти в час
    StorageGBMonthUSD float64  // цена за 1 GB хранилища в месяц
    EgressGBUSD       float64  // цена за 1 GB исходящего трафика
}

var DefaultTariff = Tariff{
    CPUCoreHourUSD:    0.048,
    MemoryGBHourUSD:   0.006,
    StorageGBMonthUSD: 0.10,
    EgressGBUSD:       0.09,
}
```

### 5. Реальная реализация Engine

```go
// internal/monetization/engine.go — новая реализация
type PostgresEngine struct {
    usageStore UsageStore
    tariff     Tariff
}

func (e *PostgresEngine) GetProjectCost(ctx context.Context, projectID string) (domain.CostBreakdown, error) {
    // Агрегировать resource_usage за текущий месяц
    usage, err := e.usageStore.AggregateForProject(ctx, projectID, monthStart, now)
    // Применить тариф
    return domain.CostBreakdown{
        ProcessorCoreHours:  usage.CPUCoreHours,
        MemoryGigabyteHours: usage.MemoryGBHours,
        Total: usage.CPUCoreHours * e.tariff.CPUCoreHourUSD +
               usage.MemoryGBHours * e.tariff.MemoryGBHourUSD + ...,
    }, nil
}
```

### 6. Лимиты и оповещения (после базовой реализации)

- Мягкий лимит: отправить уведомление (email/webhook) при достижении 80% лимита
- Жёсткий лимит: блокировать новые деплои (проверять в `BootstrapGitHubFlow`)

## Готово, если

- `GET /projects/{id}/cost` возвращает реальные числа из БД, а не захардкоженные
- Данные обновляются раз в час
- Тариф конфигурируется (не захардкожен в коде)

## Статус проверки

**Реализовано:**

- `PostgresEngine` в `internal/monetization/engine.go` — считает стоимость из БД по тарифу
- `EngineMock` остаётся для режима без БД (хардкод $13.74)
- Миграция `005_create_resource_usage.sql` — таблица `resource_usage` с полями из задачи (плюс дополнительные: `replica_count`, `pod_uptime_hours`, `cpu_cores`, `memory_gb`)
- `KubectlMetricsCollector` в `internal/integration/kubernetes/metrics_collector.go` — собирает CPU/память через Metrics API, storage через PVC, egress через Prometheus
- Фоновый агрегатор `runMetricsAggregator` в `app.go` — запускается каждые 5 минут (настраивается через `METRICS_COLLECTOR_INTERVAL`, по умолчанию 5m вместо 1h из задачи)
- `DefaultTariff` задан в `internal/monetization/tariff.go`
- `BillingGuard` (из TASK-018) блокирует деплои при превышении лимита

**Не выполнено:**

- Тариф захардкожен: `app.go` всегда передаёт `monetization.DefaultTariff`, нет env-переменных для переопределения ставок (`CPU_PRICE_USD`, `MEMORY_PRICE_USD` и т.д.)
- Мягкий лимит с оповещением (80%) — не реализован
- `OwnerID` в `CreateProjectRequest` по-прежнему существует в структуре (незначительно)
