# Контракты

> S3 переносит это в `contracts/openapi.yaml` (OpenAPI 3.1); расхождения решаются в пользу этого файла.

## API

Все пути — под `/api`. Тела — JSON. Кроме регистрации, входа и `healthz`, нужен заголовок
`Authorization: Bearer <token>`; без него или с недействительным токеном — `401 unauthorized`.

Формат ошибки во всех ответах 4xx/5xx:
```json
{"error": {"code": "syntax_error", "message": "unexpected '*' at position 4"}}
```

| Метод и путь | Тело запроса | Ответ | Ошибки |
|---|---|---|---|
| `POST /api/auth/register` | `{"login", "password"}` | `201 {"token", "expires_at", "user": User}` | `409 login_taken`, `422 validation_error` |
| `POST /api/auth/login` | `{"login", "password"}` | `200 {"token", "expires_at", "user": User}` | `401 invalid_credentials` |
| `POST /api/auth/logout` | — | `204` | `401` |
| `GET /api/me` | — | `200 User` | `401` |
| `POST /api/tasks` | `{"expression"}` | `201 Task` (статус `queued`) | `422 syntax_error`, `422 validation_error` |
| `GET /api/tasks` | — (Nice: `?status=queued`) | `200 {"items": [Task]}`, новые сверху | `401` |
| `GET /api/tasks/{id}` | — | `200 Task` | `404 not_found` (и для чужой задачи) |
| `PATCH /api/tasks/{id}` | `{"expression"}` | `200 Task` | `404`, `409 not_queued`, `422 syntax_error` |
| `DELETE /api/tasks/{id}` | — | `204` | `404`, `409 task_running` |
| `GET /api/healthz` | — | `200 {"status": "ok"}`; `503`, если БД недоступна | — |

Валидация:
- `login` — 3–32 символа `[a-z0-9_-]`; `password` — 8–72 символа.
- `expression` — 1–200 символов; разрешены цифры, `.`, `+ - * /`, скобки, пробелы. Проверка синтаксиса — при
  создании и изменении (тем же `calc.Parse`).

## Модель данных

```
User  {id: uuid, login: string, created_at: datetime}
Task  {id: uuid, expression: string, status: "queued"|"running"|"done"|"failed",
       result: number|null, error: {code, message}|null,
       created_at: datetime, started_at: datetime|null, finished_at: datetime|null}
```

Таблицы:
- `users (id uuid pk, login text unique, password_hash text, created_at timestamptz)`
- `sessions (token_sha256 bytea pk, user_id uuid fk → users on delete cascade, expires_at timestamptz)`
- `tasks (id uuid pk, user_id uuid fk → users, expression text, status text check (...), result double
  precision null, error_code text null, error_message text null, created_at, started_at, finished_at
  timestamptz)`, индекс `(status, created_at)` для очереди и `(user_id, created_at desc)` для списка.

Вычисление:
- Приоритет: унарный минус > `* /` > `+ -`; левая ассоциативность; скобки.
- `result` — `float64`, округлённый до 10 знаков после запятой; `-0` → `0`.
- Деление на ноль → `status: failed`, `error.code: division_by_zero`, `error.message: "деление на ноль"`.

## WebSocket / события

Нет: статусы клиент опрашивает (`GET /api/tasks` раз в секунду). SSE — Nice.

## Внешние интеграции

Нет.
