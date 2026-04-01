# TASK-003: Аутентификация и авторизация [Done]

## Проблема

Сейчас любой, кто знает адрес сервиса, может создать, удалить или просмотреть любой проект. `OwnerID` передаётся в теле запроса без проверки — пользователь сам пишет свой идентификатор. Это неприемлемо даже для MVP.

## Зависимости

Выполнять после ARCH-001 и TASK-002.

## Выбранный подход: JWT-токены

Детали выбора — в RFC-003. Кратко: для stateless HTTP API JWT — прагматичный выбор. Не требует отдельного сервиса сессий, токен несёт claims (userId, email).

## Что сделать

### 1. Модель пользователя

```sql
-- migrations/002_create_users.sql
CREATE TABLE users (
    id           TEXT        PRIMARY KEY,
    email        TEXT        NOT NULL UNIQUE,
    password_hash TEXT       NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL
);
```

### 2. Новые эндпоинты

```
POST /auth/register   — регистрация (email + password)
POST /auth/login      — логин, возвращает JWT
```

### 3. Middleware аутентификации

```go
// internal/http/middleware/auth.go
func RequireAuth(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        token := extractBearerToken(r)
        claims, err := verifyJWT(token, secret)
        if err != nil {
            writeJSON(w, http.StatusUnauthorized, ...)
            return
        }
        // кладём userID в context
        ctx := context.WithValue(r.Context(), userIDKey, claims.UserID)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

### 4. Авторизация на уровне проектов

Проект принадлежит конкретному пользователю. При каждом запросе к `/projects/{id}` нужно проверять, что `project.OwnerID == userID из токена`.

```go
func (s *ProjectService) GetProject(ctx context.Context, projectID string) (domain.Project, error) {
    project, exists := s.store.GetByID(ctx, projectID)
    if !exists {
        return domain.Project{}, domain.ErrProjectNotFound
    }
    callerID := userIDFromContext(ctx)
    if project.OwnerID != callerID {
        return domain.Project{}, domain.ErrForbidden
    }
    return project, nil
}
```

### 5. Удалить OwnerID из CreateProjectRequest

После введения auth `OwnerID` берётся из токена, не из тела запроса:

```go
// было:
type CreateProjectRequest struct {
    Name    string `json:"name"`
    OwnerID string `json:"ownerId"`
}

// станет:
type CreateProjectRequest struct {
    Name string `json:"name"`
    // OwnerID заполняется сервисом из context
}
```

### 6. Защита маршрутов

```go
// router.go
protected := middleware.RequireAuth(mux)
// все /projects/* маршруты — через protected
// /auth/* и /health — публичные
```

## Новые переменные окружения

| Переменная   | Описание                                        |
| ------------ | ----------------------------------------------- |
| `JWT_SECRET` | Секрет для подписи токенов (минимум 32 символа) |
| `JWT_TTL`    | Время жизни токена, например `24h`              |

## Готово, если

- Без токена — 401 на все `/projects/*` запросы
- Пользователь не может видеть/удалять чужие проекты — 403
- `OwnerID` больше не принимается из тела запроса
