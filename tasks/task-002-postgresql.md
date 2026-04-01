# TASK-002: Замена in-memory хранилища на PostgreSQL [Done]

## Проблема

Сейчас все проекты хранятся в `map[string]domain.Project` в памяти процесса. При рестарте сервиса все данные теряются. Это делает сервис непригодным для любого реального использования.

## Зависимости

Выполнять после ARCH-001 — store-интерфейс уже должен принимать `context.Context`.

## Что сделать

### 1. Выбрать инструмент для работы с БД

Выбор: **sqlc** — генерирует типизированный Go-код из SQL-запросов.

Почему:

- Нет ORM-магии, SQL пишется явно и читается в ревью
- Типизация на уровне компиляции
- Легко тестировать — интерфейсы генерируются автоматически

Альтернативы: `pgx` напрямую (больше бойлерплейта), `gorm` (скрывает SQL, сложно дебажить). Подробнее — RFC-002.

### 2. Схема БД

```sql
-- migrations/001_create_projects.sql

CREATE TABLE projects (
    id          TEXT        PRIMARY KEY,
    name        TEXT        NOT NULL,
    owner_id    TEXT        NOT NULL,
    status      TEXT        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_projects_owner_id ON projects (owner_id);
CREATE INDEX idx_projects_status   ON projects (status);
```

### 3. Структура пакета

```
internal/store/
├── project_store.go          — интерфейс (уже есть)
├── memory/
│   └── project_store.go      — in-memory (оставить для тестов и mock-режима)
└── postgres/
    ├── project_store.go      — реализация через pgx/sqlc
    └── migrations/
        └── 001_create_projects.sql
```

### 4. Реализация интерфейса

```go
// internal/store/postgres/project_store.go
type ProjectStore struct {
    db *pgxpool.Pool
}

func (s *ProjectStore) Create(ctx context.Context, project domain.Project) error {
    _, err := s.db.Exec(ctx,
        `INSERT INTO projects (id, name, owner_id, status, created_at, updated_at)
         VALUES ($1, $2, $3, $4, $5, $6)`,
        project.ID, project.Name, project.OwnerID,
        string(project.Status), project.CreatedAt, project.UpdatedAt,
    )
    return err
}
// ... GetByID, List, Update аналогично
```

### 5. Инициализация в app.go

```go
// Добавить env-переменную DATABASE_URL
// Если пустая — использовать memory store (для local dev без docker)

dbURL := os.Getenv("DATABASE_URL")
var projectStore store.ProjectStore
if dbURL != "" {
    pool, err := pgxpool.New(ctx, dbURL)
    // ...
    projectStore = postgres.NewProjectStore(pool)
} else {
    projectStore = memory.NewProjectStore()
}
```

### 6. Миграции

Использовать `golang-migrate` или просто применять SQL-файлы при старте. Для MVP подойдёт простое применение при старте:

```go
func runMigrations(db *pgxpool.Pool) error {
    // читать файлы из migrations/ и выполнять по порядку
}
```

## Новые переменные окружения

| Переменная     | Пример                                               | Описание           |
| -------------- | ---------------------------------------------------- | ------------------ |
| `DATABASE_URL` | `postgres://user:pass@localhost:5432/deploy_service` | DSN для PostgreSQL |

## Готово, если

- При `DATABASE_URL` пустой — работает как раньше (memory store)
- При заполненном `DATABASE_URL` — данные переживают рестарт
- Интерфейс `store.ProjectStore` не изменился — сервис не знает, какая реализация используется
