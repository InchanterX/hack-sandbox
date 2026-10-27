# Конвенции (общие)

> Попадает в каждый пакет задачи. Стиль кода областей — в `04_stack/<область>.md`.

## Структура репозитория

| Каталог | Что |
|---|---|
| `server/` | Go-сервис (API + воркеры) |
| `web/` | React SPA |
| `contracts/openapi.yaml` | контракт API — источник правды для сервера и клиента |
| `tests/contract/` | контрактные тесты (scaffold) |
| `tests/e2e/` | e2e сценария демо (S6), TypeScript, `node --test`; запускается в сервисе `dev` командой `npm --prefix tests/e2e test`: сам собирает и поднимает сервер в фоне (`OP_DELAY_MS=50`) против `db` из compose, ждёт `healthz`, проходит шаги сценария по API и останавливает сервер |
| `deploy/` | Kustomize, `images.yaml`, `secrets.yaml` |
| `compose.yaml`, `Dockerfile.dev` | локальная среда и среда проверок |

## Именование

- Файлы Go — `snake_case.go`; файлы React — `PascalCase.tsx` для компонентов, `camelCase.ts` для остального.
- JSON API — `snake_case` полей (`created_at`, `expression`); статусы — строчные (`queued`, `running`, `done`,
  `failed`).
- Коды ошибок API — `snake_case` (`syntax_error`, `login_taken`).

## Коммиты

- `T07: <что сделано>` в ветке `task/<id>`, коммит не реже раза в 10 минут.
- Генерированный код (`server/internal/openapi/`, `web/src/api/schema.d.ts`) коммитится вместе с изменением
  контракта и только задачей, которой принадлежит контракт.

## Общие файлы

Принадлежат scaffold: `contracts/openapi.yaml`, `server/go.mod`, `server/internal/server/routes.go`,
`server/migrations/`, `web/package.json`, `web/src/router.tsx`, `compose.yaml`, `Dockerfile.dev`,
`deploy/images.yaml`, `deploy/secrets.yaml`. Задача, которой нужна правка общего файла, не меняет его сама,
а пишет об этом в HANDOFF.md.
