# TASK-021: Управление секретами и шифрование чувствительных данных

## Проблема

Сейчас платформа уже шифрует `kubeconfig`, не сохраняет GitHub token пользователя в БД и хэширует пароль через `bcrypt`. Но в системе всё ещё есть несколько важных классов данных, для которых нужна более строгая и единообразная политика:

- пользовательские kubeconfig и любые будущие cluster credentials
- webhook secrets и platform secrets в Kubernetes
- чувствительные интеграционные токены, если позже появится их сохранение
- потенциальные billing и audit события, которые могут содержать чувствительные идентификаторы

Кроме того, пока нет зафиксированного ответа на вопросы:

- что мы шифруем в БД, а что нет
- где проходят границы между шифрованием, хэшированием и plain-text хранением
- как ротируется мастер-ключ
- как платформе безопасно жить в Kubernetes и GitHub Actions

## Что сделать

### 1. Зафиксировать data-classification policy

Нужно разбить данные по классам:

- `public` — можно хранить открыто
- `internal` — внутренняя служебная информация
- `sensitive` — требует шифрования at-rest
- `secret` — требует шифрования, redaction в логах и отдельной политики доступа

Минимально классифицировать:

- `email`
- `password_hash`
- `kubeconfig_encrypted`
- billing summaries / ledger entries
- GitHub tokens
- webhook secrets
- cluster credentials

### 2. Ввести единый encryption contract

Сейчас шифрование используется точечно. Нужно оформить это как платформенный контракт:

- какие поля в доменной модели хранятся только зашифрованными
- где происходит encrypt/decrypt
- где запрещено логирование plaintext

Выбор:

- все поля типа `...Encrypted` остаются на store-уровне
- decrypt выполняется только в сервисе, где это действительно нужно
- HTTP-адаптер не работает с raw secrets напрямую

### 3. Подготовить key rotation

Нужно предусмотреть:

- `ENCRYPTION_KEY_CURRENT`
- `ENCRYPTION_KEY_PREVIOUS` или keyring модель
- batch re-encryption job для старых записей

Иначе первая же смена ключа станет сложной ручной операцией.

### 4. Защитить секреты в Kubernetes rollout

Сейчас часть чувствительных env already попадает в runtime deployment. Нужно оформить задачу:

- хранить runtime secrets в Kubernetes Secret
- не писать секреты в ConfigMap
- не печатать значения в startup logs
- ограничить RBAC доступ к namespace/secret ресурсам

### 5. Усилить redaction и аудит

Проверить, что в логах не оказываются:

- Bearer token
- GitHub token
- kubeconfig
- webhook secret
- YAML с embedded credentials

Для ошибок и audit trail добавить redaction policy:

- логируем только факт использования секрета
- логируем источник и scope
- не логируем значение

### 6. Добавить тесты и security checklist

Нужны тесты на:

- encrypt/decrypt roundtrip
- невозможность случайно сериализовать secret в JSON-response
- отсутствие plaintext sensitive fields в store integration tests
- redaction в логах/ошибках

И отдельный checklist для ревью:

- новый sensitive field?
- нужен ли encrypt at rest?
- нужен ли redaction?
- нужен ли rotate path?

## Готово, если

- Для всех чувствительных данных определена и задокументирована стратегия хранения
- Появляется путь ротации encryption key без ручной миграции по месту
- Секреты не утекают в логи, JSON responses и GitHub Actions output
- Политика secret handling становится частью архитектуры, а не набором разрозненных решений

## Статус проверки

**Выполнена частично.**

**Реализовано:**

- AES-256-GCM шифрование в `internal/crypto/aes.go`, kubeconfig шифруется при сохранении в БД
- `ENCRYPTION_KEY` (64 hex символа) читается из env, при отсутствии — нулевой ключ с предупреждением
- GitHub-токен пользователя не сохраняется в БД — передаётся только в рамках запроса
- Пароли хешируются через `bcrypt`
- В production все секреты передаются через Kubernetes Secret (не hardcode)

**Не реализовано:**

- Нет механизма ротации ключа: отсутствует поддержка `ENCRYPTION_KEY_CURRENT` / `ENCRYPTION_KEY_PREVIOUS` для re-encryption без даунтайма
- Нет batch-задачи для повторного шифрования существующих записей при смене ключа
- Нет формального документа классификации данных
- Нет тестов, проверяющих что секреты не утекают в логи и JSON-ответы
- Нет чеклиста security review как части процесса релиза
