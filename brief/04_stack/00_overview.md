# Стек: общее

> Попадает в каждый пакет задачи. Правила областей — в `backend.md`, `frontend.md`, `devops.md`.

## Области и где их код

| Область | Файл правил | Каталоги в репозитории |
|---|---|---|
| backend | `backend.md` | `server/` |
| frontend | `frontend.md` | `web/` |
| devops | `devops.md` | `deploy/`, `server/Dockerfile`, `web/Dockerfile`, `Dockerfile.dev`, `compose.yaml` |

Общие для всех: `contracts/openapi.yaml` (контракт API), `tests/contract/**` (контрактные тесты, scaffold),
`tests/e2e/**` (e2e сценария демо, интегратор S6).

## Языки и общие инструменты

| Что | Выбор | Версия |
|---|---|---|
| Языки | Go (backend), TypeScript (frontend, e2e) | Go 1.26, TypeScript 5.x |
| Рантайм JS | Node.js | 24 LTS |
| Менеджер пакетов | Go modules; npm (`package-lock.json` коммитится) | — |
| Монорепо | без workspace-инструментов: `server/` и `web/` независимы, `tests/e2e/` — свой `package.json` | — |
| Контейнеры | Docker + compose | обязательно: проверки дирижёра идут в compose |

## Среда проверок

Все проверки дирижёра выполняются в compose-сервисе **`dev`** файла `compose.yaml` (`defaults.check_env:
{kind: compose, file: compose.yaml, service: dev}`).

- Образ `dev` собирается из `Dockerfile.dev` (scaffold) на базе `golang:1.26` (Debian) и содержит: Go,
  golangci-lint 2.x, Node.js 24 и npm, kustomize 5.x, hadolint. Версии закреплены в `Dockerfile.dev`.
- Репозиторий смонтирован в `/workspace`, это рабочий каталог; команды проверок запускаются из корня.
- `dev` зависит от сервиса `db` (Postgres 17 с healthcheck) и получает `DATABASE_URL` на него.
- Внутри `dev` нет Docker: проверки не поднимают контейнеры сами.

## Общие запреты

- Никаких внешних SaaS, платных API, телеметрии.
- Не коммитить секреты, `.env` с паролями, `node_modules/`, бинарники, `dist/`.
- Не добавлять новых языков и рантаймов.

## Внешние сервисы и ключи

Внешних API нет. Единственный секрет — пароль Postgres в кластере (`08_deploy.md`): переменная
`POSTGRES_PASSWORD`, значение заводит человек.
