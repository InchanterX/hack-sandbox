# SPEC

## Scope

Calc Queue — русский интерфейс фонового вычисления арифметики: регистрация →
выражение → `queued → running → done|failed` → изменение/удаление → смена аккаунта.
Реализуем только Must из [brief/02_scope.md](brief/02_scope.md). Источники:
[PLAN.md](PLAN.md), [DECISIONS.md](DECISIONS.md), весь `brief/`.
`ASSUMPTIONS.md` пуст; дорогих неразрешённых архитектурных решений нет.

React 19 SPA → Go 1.26 HTTP API + два воркера в одном процессе → PostgreSQL 17.
Кластер: один домен, `/api` → server:8080, `/` → web:8080; API-префикс сохраняется.
Локально Vite:5173 проксирует `/api` в server. Внешних API, AI/ML и брокера нет.
Go: net/http, pgx/v5, goose/v3, slog, bcrypt. Web: TypeScript strict, Vite,
React Router 7, TanStack Query 5, openapi-fetch, CSS Modules. Полные ограничения
и стиль из `brief/04_stack/` обязательны для соответствующих задач.

## Contracts

- [contracts/openapi.yaml](contracts/openapi.yaml): OpenAPI **3.1**, десять операций,
  поля и nullable, безопасность, ответы и ошибки. Источник истины для HTTP.
- [contracts/schema.sql](contracts/schema.sql): целевая схема PostgreSQL 17,
  ограничения состояний и индексы. Scaffold переносит DDL в встроенную goose-миграцию.
- [contracts/types.ts](contracts/types.ts): общие wire-типы для контрактов/e2e;
  production Go/TS генерируются из OpenAPI, не дублируются вручную.
- [contracts/module-interfaces.md](contracts/module-interfaces.md): стабильные
  конструкторы/экспорты, callback вычисления, транзакции, жизненный цикл и UI wiring.
- [contracts/examples.json](contracts/examples.json): примеры арифметики/ошибок;
  случаи `calculator_only_overflow` проверяют напрямую calc вне HTTP-лимита длины.
- [contracts/README.md](contracts/README.md): генерация и команды contract checks.

JSON snake_case; UUID; время UTC/RFC 3339. Все поля Task, включая nullable,
присутствуют; `result: 0` не пустота; пустой список — `{"items":[]}`.
Пароли, хеши и user_id задач в wire-модель не входят.

| Метод / путь | Успех | Основные ошибки |
|---|---|---|
| POST /api/auth/register | 201 AuthSession | 409 login_taken; 422 validation_error |
| POST /api/auth/login | 200 AuthSession | 401 invalid_credentials; 422 validation_error |
| POST /api/auth/logout | 204 без тела | 401 unauthorized |
| GET /api/me | 200 User | 401 unauthorized |
| POST /api/tasks | 201 Task, queued snapshot | 422 validation_error или syntax_error |
| GET /api/tasks | 200 TaskList, новые сверху | 401 unauthorized |
| GET /api/tasks/{id} | 200 Task | 404 not_found |
| PATCH /api/tasks/{id} | 200 Task, queued snapshot | 404; 409 not_queued; 422 validation_error/syntax_error |
| DELETE /api/tasks/{id} | 204 без тела | 404; 409 task_running |
| GET /api/healthz | 200 `{"status":"ok"}` | 503 unavailable |

Кроме register/login/healthz нужен Bearer; отсутствующий, истёкший или отозванный
токен → 401 unauthorized. Сначала аутентификация, затем поиск ресурса. Все 4xx/5xx
имеют `{"error":{"code","message"}}`; message русский, без SQL/секретов.
Неожиданная внутренняя ошибка → 500 internal_error. Некорректный JSON, отсутствие
обязательного поля, лишние поля, неверный тип или тело свыше 16 KiB → 422
validation_error. Для валидной структуры PATCH проверка принадлежности/статуса
предшествует ошибке выражения. Некорректный UUID обработчик возвращает как 404,
поэтому path-параметр намеренно не имеет `format: uuid` в OpenAPI.

Дополнение к брифу для однозначной реализации: невалидная структура login-запроса
даёт 422; валидные, но неверные credentials — 401 invalid_credentials. Упорядочение
при одинаковом created_at — id DESC в списке, id ASC в очереди. Разрешённый
пробельный символ выражения — ASCII space, не tab/newline. Эти небольшие уточнения
записаны также в HANDOFF.md; принятые человеком решения не изменяются.

## Modules

Все пути ниже закреплены за модулем; файлы тестов лежат рядом с кодом, кроме
scaffold contract tests и e2e S6. Владение T01 `**` действует только на S4;
после него task ownership ограничено `tasks.yaml`, shared-файлы остаются T01.
DAG последовательный по PLAN: T01 → T02 → T03 → T04 → T05 → T06 → T07.
Параллельных исполнителей и fusion-решений не требуется.

| Модуль / исполнитель | Принадлежащие файлы | Ответственность / зависимости |
|---|---|---|
| Contracts / T01 | `contracts/**`, `server/internal/openapi/**`, `web/src/api/schema.d.ts`, `tests/contract/**` | Генерация, контрактные проверки, общие wire-типы |
| Data / T01 | `server/migrations/**`, `server/internal/testdb/**` | Встроенные goose-миграции, изолированная тестовая схема; contracts/schema.sql |
| HTTP infrastructure / T01 | `server/internal/server/**`, `server/internal/httpjson/**` | Роуты, healthz, конфиг, JSON/ошибки; API и auth/tasks stubs |
| Calculator / T02 | `server/internal/calc/**` | Лексер, AST, Parse/Eval; только stdlib |
| Auth / T03 | `server/internal/auth/**` | Сервис, store, handlers, middleware и тесты; БД/httpjson |
| Tasks / T04 | `server/internal/tasks/**` | CRUD, принадлежность, транзакции и тесты; calc/auth context/httpjson |
| Queue + composition / T05 | `server/internal/queue/**`, `server/cmd/server/main.go`, `server/cmd/server/main_test.go` | Claim, Eval, recover, остановка и композиция; calc/БД/все handlers |
| Frontend / T06 | `web/src/api/client.ts`, `web/src/auth/**`, `web/src/screens/**`, `web/src/components/**`, `web/src/hooks/**`, `web/src/styles/**` | Единый клиент, сессия, три экрана, компоненты, CSS и тесты; API/schema |
| Production / T07 | `server/Dockerfile`, `web/Dockerfile`, `web/nginx.conf`, `deploy/base/**`, `deploy/overlays/demo/**` | Завершённые образы и Kustomize; приложение, shared image/secret catalogs |
| Tooling / T01 | Все пути из Shared-files плюс initial stubs и initial deploy-файлы T07 | Compose/dev/CI, manifests зависимостей, scaffold сборка |
| Demo verification / S6, вне DAG S4/S5 | `tests/e2e/**`, кроме shared `package.json` и `package-lock.json` | TypeScript node --test API-сценарий и restart; готовое приложение |

### Scaffold

T01 создаёт работающий каркас всех пакетов и экспортов из module-interfaces.md,
router/main.tsx, placeholder-экраны, типизированные handlers, Dockerfiles,
compose `db/server/web/dev`, CI и deploy. Healthz, config, httpjson, migrations,
изолирующие DB helpers и route wiring реализуются сразу. Доменные stubs должны
компилироваться; их 501 нельзя считать приёмкой API. Contract tests в S4
проверяют схему, fixtures, типы и отсутствие дрейфа генерации. Поведенческие
assertions handlers/store принадлежат задачам модулей и S6; не пропускать тесты
реализованного поведения и не возвращать фиктивные успехи ради зелёного CI.

Сначала проверить генерацию `oapi-codegen v2.8.0` и openapi-typescript 7.
Сгенерированные Go-типы — `server/internal/openapi/`, TS —
`web/src/api/schema.d.ts`; оба коммитятся. Если выбранный Go generator не читает
3.1, не понижать исходный контракт: зафиксировать несовместимость, выполнить
проверяемое преобразование только для генератора в tooling scaffold.

T01 заранее добавляет все разрешённые зависимости модулей (pgx, goose, bcrypt,
OpenAPI runtime, cmp, goleak; frontend/test/lint-инструменты), lockfiles и команды
проверок. Корневой package.json/npm workspace не нужен: web, tests/contract,
tests/e2e независимы. `tests/e2e/package.json` заранее задаёт `npm test` через
Node 24 `node --test` с поддержкой TypeScript; сценарий и assertions пишет S6.
S4 не объявляет пустой/пропущенный e2e успешным прохождением демо.

### Data

UUID генерируются встроенным PostgreSQL `gen_random_uuid()`. users: уникальный
login, bcrypt password_hash, created_at. sessions: 32-байтовый token_sha256 PK,
user_id FK ON DELETE CASCADE, expires_at. tasks: владелец, выражение, статус,
результат или error_code/error_message, три времени. Использовать точные DDL
constraints `contracts/schema.sql`; очередь `(status, created_at)`, список
`(user_id, created_at DESC)`. Нефинитные результаты запрещены, error_code и
error_message либо оба NULL, либо точная пара арифметической ошибки.

Миграции в `server/migrations/` включают SQL и Go embed/Apply wrapper, чтобы
server build context не зависел от файлов вне server/. Повторный старт
идемпотентен. Тесты store работают в отдельной схеме на тест и убирают её;
никакого общего TRUNCATE или конкурирующего сервера на e2e-очереди.

### Calculator

Рекурсивный спуск по грамматике в module-interfaces.md, без eval-библиотек.
Приоритет: unary minus, затем `* /`, затем `+ -`; бинарные левоассоциативны.
Допустимы `007`, `--5`, литерал без операции. `.5`, `5.`, unary plus, экспонента,
буквы и `^` — syntax_error. Конец ввода обязателен.

Parse не вычисляет выражение. Eval вызывает отменяемый beforeBinary ровно на
каждую исполненную бинарную операцию; литерал, скобки и unary minus без задержки.
Float64, проверка конечности литералов и каждого результата, округление только
конечного значения до 10 дробных знаков, проверка переполнения масштабирования,
нормализация -0 → 0. Ошибки matchable через errors.Is/As: ErrSyntax,
ErrDivisionByZero, ErrOverflow. Две runtime-ошибки дают failed с русскими текстами
«деление на ноль» / «переполнение»; context cancellation не арифметическая ошибка.
Табличные тесты покрывают contracts/examples.json и callback/cancellation.

### Auth

Login строго `[a-z0-9_-]{3,32}`, не trim/lowercase. Пароль **8–72 байта UTF-8**,
не символов (Q5/Q11). bcrypt. Токен: crypto/rand 32 байта, raw base64url (43
символа); в БД только SHA-256. Регистрация атомарно создаёт пользователя и сессию;
конкурирующий unique conflict → 409 login_taken. Каждый успешный login выдаёт
новую сессию с UTC expires_at; по умолчанию 24 часа. Logout удаляет только текущую
сессию. Middleware проверяет срок, кладёт User в context с typed private key.
Публичный accessor нужен tasks; HTTP-зависимости не входят в сервис.

### Tasks

Длина expression 1–200 code points → validation_error при нарушении; затем тот
же calc.Parse при POST/PATCH, запрещённый символ и grammar → syntax_error
(Q4/Q10). HTTP сохраняет исходный текст. Только собственные задачи; user_id берётся
из context, никогда из JSON. Все пользовательские SQL фильтруются user_id.
GET list сортирует новые сверху, чужой/отсутствующий/невалидный UUID → 404.

PATCH только queued, иначе 409 not_queued. Сохраняет created_at и место в очереди
(Q3/Q9). DELETE запрещён только running, иначе 204. Обе мутации блокируют
принадлежащую пользователю строку и проверяют статус внутри одной транзакции;
последующий UPDATE/DELETE повторяет условие статуса. При PATCH→claim оба запроса
могут быть успешны, но вычисляется новое выражение. При claim→PATCH получаем 409.
Тесты управляют обоими порядками блокировок; неверное выражение не меняет строку.

### Queue

Состояния и заполнение полей определены OpenAPI/DDL. Claim — атомарный UPDATE
с подзапросом `FOR UPDATE SKIP LOCKED`, FIFO по created_at, id; RETURNING id,
expression. Транзакция завершена до расчёта/таймера. Вычисляется именно возвращённое
выражение; очередь обращается к своей SQL store, не к HTTP/tasks service.

Два воркера; 700 ms на бинарную операцию; idle poll 300 ms. Таймеры отменяются
context, синхронизация тестов через testing/synctest/каналы. Успех — атомарно
done/result/finished_at; арифметическая ошибка — failed/error/finished_at.
Infrastructure/cancellation оставляют running для восстановления. Ошибки БД
логируются один раз, retry имеет отменяемую задержку. После миграций и до старта
воркеров/приёма трафика сбросить running → queued и все поля исполнения → NULL.

### Runtime

Env читается один раз в main, передаётся конфигом. `DATABASE_URL` для dev/tests;
в кластере DSN строится в Go из DB_HOST/DB_NAME/DB_USER/POSTGRES_PASSWORD с
корректным URL-экранированием, пароль не логируется.

| Env | По умолчанию |
|---|---|
| PORT | 8080 |
| WORKERS | 2 |
| OP_DELAY_MS | 700 |
| POLL_INTERVAL_MS | 300 |
| SESSION_TTL_HOURS | 24 |
| DB_HOST / DB_NAME / DB_USER | db / calc / calc |

WORKERS/TTL/POLL_INTERVAL положительны; OP_DELAY_MS допускает 0. Плохой конфиг —
понятная ошибка запуска без секрета. Подключение к БД повторять с ограничением
60 s. Startup: config → db connect → migrations → recovery → handlers/workers.
SIGTERM: прекратить новые claims, запустить HTTP shutdown, дать активным
вычислениям до 10 s, затем отменить execution context и дождаться workers.
Остановка ограничена; прерванные running восстанавливаются следующим стартом.
Healthz проверяет БД; non-ready → 503 в стандартном error envelope.

### Frontend

Все запросы через `web/src/api/client.ts` (openapi-fetch), хуки TanStack Query;
никакого axios/второго fetch. localStorage хранит токен; bootstrap через /api/me
показывает loading, затем пускает в защищённый экран. Токен не отображается.
Неавторизованный пользователь с `/` и неизвестного пути попадает на `/login`.
Login/register доступны гостям, успешная сессия переводит на `/`.

User task key `['tasks', user.id]`, polling 1000 ms. После mutation invalidate
`['tasks']`. Logout, защищённый 401 и смена аккаунта отменяют запросы и очищают
сессию/пользовательский кэш; поздний ответ старого аккаунта его не восстанавливает.
Login 401 показывается в форме. Logout вызывает API, локальная сессия очищается
также при сетевой ошибке. Никакого серверного списка в useState и setInterval.
Состояние редактора выражения локальное. Конфликт 409 показывает русский message,
обновляет список и закрывает недоступное действие; введённый текст можно скопировать.
Пароль проверяется `TextEncoder`; expression count — `Array.from(value).length`.

## Design

Спокойный рабочий инструмент без изображений: светлый фон, одна колонка шириной
не более 720 px, горизонтальный отступ 16 px, много воздуха. На 360 px нет
горизонтального scroll. При ширине <=640 px задачи — карточки, выше — таблица.
Все цвета, радиусы, размеры и отступы через CSS variables из tokens.css.

| Токен | Значение |
|---|---|
| --color-bg | #f7f7f8 |
| --color-surface | #ffffff |
| --color-text | #1d1d1f |
| --color-muted | #6e6e73 |
| --color-accent | #2f6fed |
| --color-queued | #8e8e93 |
| --color-running | #d98b00 |
| --color-done | #1f9d55 |
| --color-failed | #d64545 |
| --radius | 8px |
| --space | 8px; отступы кратные |
| --font-ui / --font-mono | system-ui / ui-monospace |
| --font-size / --heading-size | 16px / 24px |
| --content-width / --page-gutter | 720px / 16px |

| Экран | Содержимое и состояния |
|---|---|
| /login | Label login/password, «Войти», ссылка «Регистрация»; ввод, loading на кнопке, inline «неверный логин или пароль», retry отправкой формы |
| /register | Login/password, «Зарегистрироваться», ссылка «Вход»; byte validation, inline 409 «логин занят», 422 у поля, disabled submit при запросе; успех → / |
| / | Header «Calc Queue», логин, «Выйти»; expression placeholder `2 + 3 * (4 - 1)`, «Вычислить»; loading, empty «Задач пока нет — отправьте первое выражение», network error/message + «Повторить»; успешный список без reload |

Компоненты T06: `StatusBadge`, `TaskList` (table/cards), `TaskItem` (inline editor),
`ExpressionForm`, `AuthForm`, `RequestState`. Каждый компонент: PascalCase.tsx,
CSS Module и тест рядом. StatusBadge — единственная карта:
queued → «в очереди»/серый; running → «выполняется»/янтарный с анимированной точкой;
done → «готово»/зелёный; failed → «ошибка»/красный. Текст делает статус понятным
без цвета; анимация учитывает prefers-reduced-motion.

В строке/карточке: выражение mono, статус, результат mono или ошибка, время,
«Изменить» только queued, «Удалить» кроме running. Редактор: «Сохранить»/«Отмена»;
422 под полем, 409 у действия. `0` показывать явно через result !== null.
После создания поле очищается только при успехе; ошибочный ввод сохраняется.
Mutation loading блокирует повторный submit соответствующей формы/действия.
Polling обновляет статусы без потери ввода/фокуса. Semantic main/form/list/table,
label у каждого input, видимый keyboard focus, button для действий, status/errors
через aria-live; список не озвучивается целиком при каждом poll.

## Deploy

T01 создаёт shared catalogs, T07 доводит production-файлы в своей области.
`deploy/images.yaml`: version 1; images с **name/context/dockerfile**:
`{name: server, context: server, dockerfile: Dockerfile}` и
`{name: web, context: web, dockerfile: Dockerfile}` (dockerfile относительно context).
Полные пути файлов — server/Dockerfile и web/Dockerfile; Conductor подставляет
реестр и SHA tag, manifests используют имена server/web. Текущий placeholder
`app` T01 обязан заменить; S3 не владеет deploy/.

`deploy/secrets.yaml`: только `secrets: [{name: db, keys: [POSTGRES_PASSWORD],
description: пароль Postgres демо}]`. Значение создаёт человек, в manifest только
secretKeyRef (db и server). Даже в dev/tests пароль приходит из environment;
compose использует требуемую переменную, без hardcoded/default пароля.
`.env` исключён из git; .env.example содержит только пустые имена переменных.

Compose: db postgres:17-alpine + pg_isready и volume; server production build;
web Vite dev с /api proxy; dev из Dockerfile.dev на golang:1.26 Debian с Go,
CGO/race toolchain, golangci-lint 2.x, Node 24, kustomize 5.x, hadolint.
Версии инструментов pin. Dev mount /workspace, working_dir /workspace,
DATABASE_URL с корректным экранированием env-пароля, depends_on db healthy.
Проверки внутри dev не вызывают Docker; compose up запускается снаружи.

Server multi-stage golang:1.26 → distroless/static-debian12:nonroot, CGO_ENABLED=0,
бинарник /server, порт 8080. Web Node 24 → nginx-unprivileged:1.29-alpine, 8080,
SPA fallback index.html, кэш fingerprinted static, без API proxy в nginx.

`deploy/base/`: db StatefulSet 1 + Service + PVC 1Gi/local-path, server/web
Deployments + Services, ConfigMap, HTTPRoute. `deploy/overlays/demo/` — Kustomize
namespace hack-sandbox (поле namespace, **без объекта Namespace**). HTTPRoute
sandbox.hack.faberge-eggs.ru, parentRefs default/home-gateway, PathPrefix /api →
server:8080 и / → web:8080, без rewrite. Один server, strategy Recreate,
terminationGracePeriodSeconds 30. ConfigMap значения из Runtime и brief/08_deploy.md.

| Контейнер | CPU/memory requests | CPU/memory limits | Probes |
|---|---|---|---|
| server | 100m / 128Mi | 500m / 256Mi | readiness/liveness /api/healthz; liveness failureThreshold 6; startup окно 60 s |
| web | 50m / 64Mi | 200m / 128Mi | readiness/liveness / на 8080 |
| db | 100m / 256Mi | 500m / 512Mi | readiness/liveness pg_isready |

Не создавать Secret, Namespace, ServiceAccount, RBAC, network/quota policies,
AppProject или другие запрещённые cluster-scoped объекты. Не использовать latest,
privileged, hostPath, NodePort/LoadBalancer, imagePullPolicy Always. Используется
default ServiceAccount. Ранняя проверка внешних namespace, db Secret/key,
StorageClass, Gateway и политики Postgres — ответственность S7; их отсутствие
не блокирует независимую реализацию, но публичный релиз не считается успешным.

## Verification

`tasks.yaml` задаёт `defaults.check_env` compose.yaml/dev. Общие checks:
`npm --prefix tests/contract ci`, затем `npm --prefix tests/contract test`.
Scaffold создаёт реальные scripts: OpenAPI 3.1/schema/example/type/generation drift,
SQL/migration parity и статическая проверка deploy manifests (secretKeyRef,
ресурсы, images, namespace/route и отсутствие запрещённых объектов).
Schema/fixtures validation не зависит от ещё не реализованной семантики handlers.

Stack checks запускаются дополнительно, из корня отдельными командами:

| Stack | Команды |
|---|---|
| backend | `cd server && golangci-lint run ./...`; `cd server && go build ./...`; `cd server && go test -race -tags=integration ./...` |
| frontend | `cd web && npm ci`; `cd web && npm run lint`; `cd web && npm run typecheck`; `cd web && npm test -- --run`; `cd web && npm run build` |
| devops | `kustomize build deploy/overlays/demo > /dev/null`; `hadolint server/Dockerfile web/Dockerfile Dockerfile.dev` |

Scaffold и интегратор S6 запускают все области; обычная задача — общие checks и
свои stack checks. Unit герметичны (httptest/handwritten fakes), store tests с
integration tag против compose db в отдельной схеме. cmp вместо testify, race
обязателен, goleak там, где запускаются горутины. Время виртуализировать
synctest, не синхронизироваться sleep. Тесты проверяют наблюдаемое поведение.

S6 пишет `tests/e2e/*.test.ts`; `defaults.e2e` —
`npm --prefix tests/e2e test`. Тест сам собирает/запускает server против compose db
с OP_DELAY_MS=50, WORKERS=2, ждёт healthz, создаёт уникальные логины, использует
изолированную схему и завершает процесс/чистит схему. Никакого второго сервера на
этой очереди. Endpoints/status ожидать bounded polling с таймаутом; subprocess
stderr доступен при ошибке. Проверить все девять шагов brief/01_product.md,
результаты 11/2/93, division_by_zero, 401, login_taken 409, 404 для чужого и
некорректного ID, PATCH не queued 409, DELETE running 409, syntax/length 422.
Невалидное выражение не увеличивает список; rejected PATCH не меняет задачу.

Для видимой очереди и редактирования (Q1/Q7) занять **оба** воркера двумя
выражениями до 200 символов, например 100 единиц через `+` (199 символов, 99
операций). Дождаться running обеих, создать `100 / 7 + 1`, PATCH → `100 - 7`,
убедиться в queued/сохранении created_at и результате 93. Для рестарта занять
воркера долгим заданием, дождаться running, принудительно убить процесс, запустить
заново на той же схеме и дождаться завершения восстановленной задачи. Модульный
тест Recover отдельно подтверждает промежуточный queued и NULL execution fields.
Проверка graceful shutdown отдельная от crash recovery.

S7 ручная проверка публичного URL: redirect гостей и прямых SPA-ссылок,
регистрация/login/logout, полный демо-сценарий, статусы без reload <=2 s,
360/640/1280 px и keyboard navigation. Browser e2e/Playwright вне scope.
Публичный релиз требует настоящих образов и внешних cluster prerequisites.

## Shared-files

Точное соответствие `shared_files` в tasks.yaml (включая root-anchored globs).
Только T01 создаёт/меняет их; последующие задачи не включают их в owned_files:

```text
contracts/**
server/go.mod
server/go.sum
server/.golangci.yml
server/internal/openapi/**
server/internal/server/**
server/internal/httpjson/**
server/internal/testdb/**
server/migrations/**
web/package.json
web/package-lock.json
web/index.html
web/vite.config.ts
web/vitest.config.ts
web/tsconfig.json
web/tsconfig.app.json
web/tsconfig.node.json
web/eslint.config.js
web/src/main.tsx
web/src/router.tsx
web/src/api/schema.d.ts
web/src/test/**
tests/contract/**
tests/e2e/package.json
tests/e2e/package-lock.json
compose.yaml
Dockerfile.dev
deploy/images.yaml
deploy/secrets.yaml
.github/**
.gitignore
.env.example
README.md
```

Новый dependency, route или schema change не правится владельцем фичи: записать
точную потребность в исключённый HANDOFF.md для Conductor/scaffold. По текущему
Must все десять routes, зависимости и таблицы известны заранее. SPEC/tasks —
планирование S3, их изменение не входит в feature ownership.
