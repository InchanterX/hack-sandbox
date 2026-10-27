# Стек: backend (Go)

> Потребители: исполнители задач с `stack: [backend]`, их ревьюеры, scaffold.
> Правила кода — в разделе «Go style guide» ниже; каждое правило имеет ID (`ERR-5`), ссылайтесь на него,
> когда следуете правилу или отступаете от него.

## Технологии и версии

| Что | Выбор | Версия | Комментарий |
|---|---|---|---|
| Язык / рантайм | Go | 1.26 | `go 1.26` в `server/go.mod`; новые API stdlib разрешены (`sync.WaitGroup.Go`, `testing/synctest`, `b.Loop`) |
| HTTP | `net/http` (`http.ServeMux` с методами и шаблонами путей) | stdlib | без веб-фреймворков |
| Контракт API | `oapi-codegen` по `contracts/openapi` | v2 | сгенерированный код в `server/internal/openapi/`, руками не правится |
| БД и драйвер | PostgreSQL + `github.com/jackc/pgx/v5` (`pgxpool`) | Postgres 17, pgx v5 | SQL руками, без ORM |
| Миграции | `github.com/pressly/goose/v3` | v3 | файлы в `server/migrations/` |
| Логи | `log/slog` | stdlib | JSON-хендлер в проде, текстовый локально |
| Тесты | `testing` + `github.com/google/go-cmp/cmp`, `go.uber.org/goleak` | — | **без testify** |
| Линтер | golangci-lint с `server/.golangci.yml` | 2.x | версия фиксируется в образе `dev` |

## Структура модуля

```
server/
  cmd/server/main.go          # сборка зависимостей, запуск, graceful shutdown; единственный os.Exit
  internal/openapi/           # сгенерировано из contracts/openapi (scaffold), не трогать
  internal/<module>/
    handler.go                # HTTP: разбор запроса по контракту, вызов сервиса, ответ
    service.go                # бизнес-логика, без net/http; объявляет нужные ей интерфейсы (CORE-2)
    store.go                  # доступ к БД через pgx
    handler_test.go service_test.go   # герметичные unit-тесты на фейках
    store_integration_test.go         # //go:build integration — против Postgres из compose
  migrations/                 # goose, общий файл scaffold
  .golangci.yml               # scaffold копирует из brief/assets/go/golangci.yml
```

- Роуты регистрируются в одном общем файле scaffold (`internal/server/routes.go`); задача, которой нужен новый
  роут, добавляет его только в своём модуле и пишет о регистрации в HANDOFF.md, если файл ей не принадлежит.
- Имена пакетов — по тому, что они дают (NAME-6): не `util`, `common`, `models`, `types`, `api`.

### Модули этого проекта

| Пакет | Что внутри |
|---|---|
| `internal/calc` | разбор и вычисление выражения (рекурсивный спуск), без I/O; `Parse(expr) (Expr, error)`, обход с колбэком на каждую бинарную операцию (для задержки воркера); ошибки `ErrSyntax`, `ErrDivisionByZero` |
| `internal/auth` | регистрация, вход, выход, middleware `Authorization: Bearer`; пароли — bcrypt (`golang.org/x/crypto/bcrypt`), в БД хранится sha256 токена сессии; пользователь в `context` через типизированный ключ |
| `internal/tasks` | CRUD задач пользователя; изменение и удаление — условным `UPDATE/DELETE ... WHERE status = ...` (0 строк → 409) |
| `internal/queue` | пул воркеров: забор задачи `FOR UPDATE SKIP LOCKED`, вычисление через `calc` с задержкой `OP_DELAY_MS` на операцию, запись результата; при старте `running` → `queued`; остановка по `ctx` |
| `internal/server` | `routes.go` (общий файл scaffold), helper ошибок `{"error": {"code", "message"}}`, `GET /api/healthz` |

Переменные окружения сервера: `DATABASE_URL` (или `DB_HOST`, `DB_NAME`, `DB_USER`, `POSTGRES_PASSWORD`),
`PORT` (8080), `WORKERS` (2), `OP_DELAY_MS` (700), `POLL_INTERVAL_MS` (300), `SESSION_TTL_HOURS` (24).
Миграции встроены в бинарник (`embed`) и применяются при старте.

## Как писать

- Обработчик: декодирование тела с лимитом размера (`http.MaxBytesReader`), валидация по типам из
  `internal/openapi`, вызов сервиса, ответ через общий helper ошибок. Логики в обработчике нет.
- Сервис принимает `context.Context` первым параметром и маленькие интерфейсы своих зависимостей, возвращает
  конкретные типы (CORE-1). Интерфейс хранилища объявляется в пакете сервиса, а не хранилища.
- Транзакции — в store-методе, который делает несколько запросов: `pool.BeginTx` + `defer tx.Rollback(ctx)`
  (ошибку Rollback после Commit игнорировать с комментарием, ERR-2).
- Остальное — в «Go style guide».

## Как не писать

- Логика в обработчиках, SQL в сервисе, глобальные синглтоны (`var db *pgxpool.Pool`), `init()` с
  побочными эффектами (CORE-36, CORE-37).
- `panic` для ожидаемых ошибок, лог-и-возврат одной ошибки (ERR-5), «failed to …» в обёртках (ERR-7).
- Горутины без гарантированной остановки (CONC-1), `time.Sleep` для синхронизации (CONC-16).
- Отладочные `fmt.Println`, закомментированный код, `//nolint` без объяснения.
- Свои велосипеды вместо выбранной библиотеки; новая зависимость без согласования (см. «Зависимости»).

## Ошибки, логи, конфигурация

- Формат ошибки API — по контракту в `contracts/openapi`; сервисные ошибки — sentinel `ErrXxx` или типы
  `XxxError` (ERR-10, ERR-11); маппинг в HTTP-статус — только в обработчике (ERR-13).
- Логирует тот слой, который решает, что делать с ошибкой (обычно обработчик или `main`), через `slog`
  с полями; секреты и персональные данные в логи и тексты ошибок не попадают (ERR-8).
- Конфигурация — только из переменных окружения, читается один раз в `main` и передаётся в конструкторы.
  Секреты — только имена переменных (`DATABASE_URL`), значения в коде, конфиге и тестах запрещены.

## Данные и миграции

- Миграции — общий файл scaffold: задача, которой нужна новая таблица или поле, просит дирижёра через
  HANDOFF.md, а не правит `migrations/` сама.
- Таблицы и поля — `snake_case`, таблицы во множественном числе; `id` — `uuid` или `bigint identity`
  (как решит scaffold, одинаково во всём проекте); `created_at`/`updated_at` — `timestamptz`.
- Время в Go — `time.Time` в UTC, в API — RFC 3339 (CORE-30, CORE-32).

## Тесты

- Unit-тесты герметичны (TEST-11): без сети, без БД, без `sleep`; зависимости — рукописные фейки рядом
  с интерфейсом потребителя (TEST-10). `httptest` для обработчиков.
- **Исключение из TEST-11:** тесты хранилища (`*_integration_test.go`, тег `integration`) идут против Postgres
  из compose; каждый тест работает в своей транзакции или своей схеме и убирает за собой.
- Контрактные тесты лежат в `tests/contract/**`, их пишет scaffold, исполнители не трогают.
- `-race` обязателен; `goleak.VerifyTestMain` в пакетах, которые запускают горутины.

## Команды проверок

Ровно эти команды выполнит Check Job для backend-задач (в compose-сервисе `dev`, из корня репозитория):

- `cd server && golangci-lint run ./...` (линтеры и gofmt/goimports)
- `cd server && go build ./...`
- `cd server && go test -race -tags=integration ./...`

Образ `dev` — `golang:1.26` (Debian: `-race` нужен cgo, на alpine не работает) + golangci-lint 2.x;
`DATABASE_URL` указывает на Postgres того же compose. Перед завершением задачи прогоните эти команды сами
и исправьте всё, что они нашли.

## Зависимости

- Разрешены: `pgx/v5`, `goose/v3`, `oapi-codegen` (рантайм), `go-cmp`, `goleak`, `golang.org/x/...`
  по необходимости.
- Запрещены: веб-фреймворки (gin, echo, fiber), ORM (gorm, ent), testify, logrus/zap, `encoding/json/v2`
  через `GOEXPERIMENT`.
- Новая зависимость — правка общего `server/go.mod`, только через дирижёра (HANDOFF.md).

---

## Go style guide

One line per rule. Rule keywords follow RFC 2119: **MUST/MUST NOT** = hard rule, violations are bugs;
**SHOULD/SHOULD NOT** = default, deviate only with a concrete reason; **MAY** = allowed option.
When you decline or deviate because of a rule, cite its ID.

Precedence when rules conflict: gofmt/goimports > this brief (this file, `00_overview.md`,
`05_conventions.md`, `contracts/`) > conventions of the existing code (style-only choices) > this guide.
Say one sentence when you follow existing code over this guide.

Prefer standard-library solutions over third-party dependencies.

### Golden rules

1. Format with gofmt/goimports; imports grouped: stdlib, then third-party (NAME-13, NAME-14).
2. Error is always the last return value; check it, never discard it (ERR-1, ERR-2).
3. Wrap errors with `%w` and short context; no "failed to" prefixes (ERR-6, ERR-7).
4. Handle each error exactly once: wrap-and-return **or** log-and-degrade, never both (ERR-5).
5. Never panic in library code; `recover` only at top-level boundaries (ERR-15, ERR-16, CORE-33).
6. Interfaces belong to the consumer; accept interfaces, return concrete types (CORE-1, CORE-2).
7. Declare interfaces where they are used; keep them small (1–3 methods) (CORE-3).
8. Verify interface compliance with `var _ Iface = (*T)(nil)` (CORE-5).
9. Always use the comma-ok idiom for type assertions and map reads you do not own (CORE-25).
10. Never start a goroutine without a documented, guaranteed way to stop it (CONC-1).
11. `context.Context` is the first parameter; propagate it, never store it in structs (CONC-5).
12. Only the sender closes a channel; receivers never close (CONC-8).
13. Use `time.Time` / `time.Duration`, never ints, for instants and periods (CORE-30).
14. Table-driven tests with `t.Run`; run tests with `-race` (TEST-1, TEST-11).
15. Names are `MixedCaps`, short, and scope-appropriate; packages are short, lowercase,
    singular-purpose (NAME-1, NAME-3).
16. Optimize only with a benchmark and a profile that prove the win (PERF-1).

### Core language idioms (CORE)

- **CORE-1.** Accept interfaces, return concrete types: parameters SHOULD be small interfaces; functions and
  constructors return concrete types.
- **CORE-2.** Interfaces belong to the consumer package; MUST NOT define producer-package interfaces
  "for mocking".
- **CORE-3.** Keep interfaces 1–3 methods; split larger ones by capability; one-method interfaces take the
  `-er` suffix (NAME-8).
- **CORE-4.** Never use a pointer to an interface.
- **CORE-5.** Verify interface compliance at compile time: `var _ Iface = (*T)(nil)`.
- **CORE-6.** Do not define an interface before it has a second real use (a test fake counts).
- **CORE-7.** One receiver type per type — MUST NOT mix value and pointer receivers; when in doubt, pointer.
- **CORE-8.** Pointer receiver when the method mutates, the struct holds a sync primitive, the struct is
  large, or mutation must be visible to concurrent methods.
- **CORE-9.** Value receiver only for small, immutable, value-like types (`time.Time`, small coordinate
  structs).
- **CORE-10.** Always use field names in struct literals (empty `T{}` is the only exception).
- **CORE-11.** Omit zero-value fields in struct literals.
- **CORE-12.** `var x T` for the zero value; `&T{...}` or a constructor for a pointer.
- **CORE-13.** Design the zero value to be useful; otherwise provide `NewXxx` returning `*T` (NAME-7).
- **CORE-14.** Every field of a marshaled struct MUST carry an explicit tag; unexported fields stay out of
  the wire format.
- **CORE-15.** Never copy structs containing locks or runtime-registered state (`sync` types, atomics,
  `time.Timer`); pass by pointer; treat vet lock-copy warnings as errors.
- **CORE-16.** Do not pass pointers just to save bytes; small read-only values go by value.
- **CORE-17.** Prefer `var t []T` over `t := []T{}`; non-nil `[]T{}` only when the wire format requires it;
  never let API behavior distinguish nil from empty.
- **CORE-18.** Check emptiness with `len(s) == 0`, never `s == nil`.
- **CORE-19.** Copy slices and maps at ownership boundaries; never store or return caller-owned backing
  storage.
- **CORE-20.** Specify capacity in `make` for slices and maps when the size is known.
- **CORE-21.** Guard returned subslices with a full slice expression (`buf[:n:n]`) so append cannot clobber
  the tail.
- **CORE-22.** Raw string literals (backticks) for multi-line strings, regexes, and anything containing
  escapes.
- **CORE-23.** `strconv` over `fmt` for number↔string; `strings.Builder` over `+=` in loops.
- **CORE-24.** Keep format strings adjacent to the call; `Printf` wrappers get an `f` suffix and
  `args ...any` so vet can check callers.
- **CORE-25.** Comma-ok for type assertions unless failure is impossible and commented as such.
- **CORE-26.** Type switch over chains of comma-ok when dispatching on more than one type.
- **CORE-27.** Enums start at one so zero means "unset"; deliberate zero default is the documented exception.
- **CORE-28.** Exported enums are a named type with a `String()` method; members are `MixedCaps`, not
  `SCREAMING_CASE`.
- **CORE-29.** Generics only for genuinely identical logic across types: concrete types first, then
  interfaces, then generics; no speculative type parameters.
- **CORE-30.** `time.Time` for instants, `time.Duration` for periods; never ints in APIs (if forced, name the
  unit: `IntervalMillis`).
- **CORE-31.** Compare and manipulate time via methods (`Before`, `After`, `Equal`, `Add`, `AddDate`, `Sub`);
  pick `Add(24h)` vs `AddDate` by intent.
- **CORE-32.** `time.RFC3339` for interchange; load locations explicitly; keep `time.Now()`'s monotonic
  reading for elapsed time.
- **CORE-33.** Never panic in library code for expected failures — return an error; panic is for programmer
  errors and impossible states only.
- **CORE-34.** `recover` only at well-defined boundaries (handlers, worker loops, `main`); convert to error
  or log, never swallow.
- **CORE-35.** `os.Exit` / `log.Fatal` only in `main`, at most once, after flushing output.
- **CORE-36.** Avoid `init()`; when unavoidable it MUST be minimal, deterministic, with no goroutines, no
  panics on normal input, and no init-order dependencies.
- **CORE-37.** Avoid mutable global state; package-level vars effectively immutable after startup; config
  goes through constructors.
- **CORE-38.** Never shadow built-ins: `error`, `string`, `any`, `len`, `cap`, `make`, `new`, `min`, `max`,
  `append`, `copy`, `nil`, `true`, `false`.
- **CORE-39.** `crypto/rand` for keys, tokens, and anything security-sensitive; `math/rand` and
  `math/rand/v2` MUST NOT be used for security.

### Naming and code layout (NAME)

- **NAME-1.** `MixedCaps`/`mixedCaps` everywhere; no underscores or `SCREAMING_CASE`, including constants
  (`maxLength`, not `MAX_LENGTH`).
- **NAME-2.** Consistent initialisms: `URL`, `ID`, `HTTP`, `API` all-caps or all-lowercase (`appID`,
  `userID`, `ServeHTTP`; never `Url`, `AppId`).
- **NAME-3.** Names are short and scope-proportional; loop indices `i`/`j`, readers `r`, counters `n`.
- **NAME-4.** No stuttering between package and identifier: `chubby.File`, not `ChubbyFile`.
- **NAME-5.** Package names are short, lowercase, single words; no underscores, `mixedCaps`, or version
  suffixes.
- **NAME-6.** Name packages after what they provide; MUST NOT use buckets: `util`, `common`, `misc`,
  `helpers`, `base`, `models`, `types`, `interfaces`, `api`.
- **NAME-7.** Constructors are `NewXxx` (or `New` in the type's home package); getters carry no `Get`
  prefix, setters do (`SetOwner`).
- **NAME-8.** One-method interfaces take the method name plus `-er`; no `I` prefixes or `Impl` suffixes;
  interface and implementation never share a name in one package.
- **NAME-9.** Receiver names are 1–2 lowercase letters, consistent across all methods of the type; never
  `this`, `self`, `me`.
- **NAME-10.** Constants are `MixedCaps`, grouped by topic; enum members are named relative to their type.
- **NAME-11.** File names are lowercase, short, underscores only for suffixes (`user_test.go`,
  `client_linux.go`, `store_integration_test.go`); one package per directory; `cmd/<name>/` for
  `package main`.
- **NAME-12.** Import only what you use; use everything you import; no blank imports to fight the compiler.
- **NAME-13.** Import groups separated by a blank line: stdlib first, then everything else.
- **NAME-14.** Alias imports only to resolve collisions or shorten a canonical long path, consistently
  project-wide; never `_`-prefixed or version-bearing.
- **NAME-15.** Dot imports forbidden (test-cycle exception only); blank imports only in `main`/tests for
  side-effect registration.
- **NAME-16.** Every exported identifier has a doc comment: complete sentences, starting with the
  identifier's name, ending with a period.
- **NAME-17.** One package comment per package, `// Package x ...` directly above the package clause.
- **NAME-18.** Comments are sentences explaining intent, not restating code; `// Deprecated:` as the first
  line for deprecations.
- **NAME-19.** Group similar declarations; declare types before the functions that use them.
- **NAME-20.** Exported functions first, helpers after their callers; one type's methods stay in one file.
- **NAME-21.** Reduce nesting with early returns; happy path at minimal indentation.
- **NAME-22.** Remove unnecessary `else` when both branches set a value or the `if` returns.
- **NAME-23.** Declare variables in the innermost block that uses them; prefer
  `if v, ok := m[k]; ok { ... }`.
- **NAME-24.** Annotate naked boolean parameters at call sites (comment or named constant); split into two
  named functions when the flag changes behavior substantially.
- **NAME-25.** Top-level declarations use `var` for zero values and grouped `const` blocks; `:=` is for
  locals; wrap lines by semantics, not columns.
- **NAME-26.** Logs start with a capital and are line-oriented; error strings stay lowercase without
  trailing punctuation; use `log/slog` for structured logging.
- **NAME-27.** Wrap `Printf`-family functions with an `f` suffix and `args ...any` so vet checks format
  strings.

### Error handling (ERR)

- **ERR-1.** `error` is always the last return value: `(T, error)`; no in-band sentinels like `-1` or `""`.
- **ERR-2.** Check every returned error; never discard with `_`; deliberately ignored errors (e.g. in
  `defer`) carry a comment explaining why.
- **ERR-3.** Indent the error flow, not the happy path: handle the error first and return.
- **ERR-4.** Preserve the `(nil, err)` contract: never return a non-nil result together with a non-nil
  error.
- **ERR-5.** Handle each error exactly once — wrap-and-return, log-and-degrade, or match-and-recover;
  MUST NOT both log and return it.
- **ERR-6.** Wrap with `%w` so callers can unwrap; `%v` only to hide an implementation detail from your
  contract.
- **ERR-7.** Keep wrap context short and forward-looking; MUST NOT prefix with "failed to" (it stacks).
- **ERR-8.** Error strings are lowercase, no trailing punctuation, include identifying values, never secrets
  or PII.
- **ERR-9.** `errors.New` when there is nothing to format; `fmt.Errorf` only for interpolation or wrapping.
- **ERR-10.** Sentinel errors are exported `ErrXxx` package-level vars, documented as API; unexported ones
  are `errXxx` and stay matchable in-package.
- **ERR-11.** Custom error types carry structured context and end in `Error`; create them only when callers
  must extract data or match dynamically.
- **ERR-12.** Match with `errors.Is` (sentinels) and `errors.As` (types); never `==` on wrapped errors,
  never string comparison.
- **ERR-13.** Match at the layer that knows how to react; wrap everywhere else (store wraps, handler maps
  to status, `main` logs).
- **ERR-14.** Aggregate parallel failures with `errors.Join` (or errgroup) so all causes survive unwrapping.
- **ERR-15.** No panics for expected failures; panic only for programmer errors (CORE-33).
- **ERR-16.** `recover` only at the top of a request/goroutine boundary, converting the panic into an error
  or a log record — never silently.
- **ERR-17.** Handle `Close`/`Flush`/`Shutdown` errors; when the close error carries meaning, return it
  instead of the happy-path result.
- **ERR-18.** Assert errors in tests with `errors.Is`/`errors.As`, not by comparing message strings.

### Concurrency (CONC)

- **CONC-1.** Never fire-and-forget: the code that starts a goroutine owns and guarantees its termination
  (context, done channel, or WaitGroup).
- **CONC-2.** Prefer synchronous functions; concurrency is the caller's choice, hidden async APIs cannot be
  un-wrapped by callers.
- **CONC-3.** Wait for goroutines to exit with `sync.WaitGroup.Go`.
- **CONC-4.** No goroutines in `init()`.
- **CONC-5.** `context.Context` is the first parameter, named `ctx`, propagated down I/O call chains;
  MUST NOT be stored in structs (interface-conformance exception only) or replaced by custom context types.
- **CONC-6.** Context values only for request-scoped cross-cutting data; `context.Background()` only in
  `main`/tests/top-level servers; never pass a nil context (`context.TODO()` instead); set deadlines at
  process boundaries.
- **CONC-7.** Long-running work loops over `ctx.Done()` in a `select`.
- **CONC-8.** Only the sender closes a channel; receivers never close; closing is optional and only when
  receivers range or need completion.
- **CONC-9.** Channel buffer size is one or none; larger buffers MUST be justified in a comment.
- **CONC-10.** `select` for multiplexing: `default` makes operations non-blocking, `time.After` adds
  timeouts, `ctx.Done()` adds cancellation.
- **CONC-11.** `errgroup` for fail-able goroutine groups (first error wins, siblings canceled); bound
  unbounded input with `SetLimit` or a semaphore.
- **CONC-12.** Loop variables are per iteration (Go 1.22+): do not copy them before `go` or closures.
- **CONC-13.** Zero-value mutexes as named fields, never embedded (embedding leaks `Lock`/`Unlock` into the
  API).
- **CONC-14.** Keep critical sections small; `defer Unlock()` is the default; no I/O or unknown calls under
  a lock; `RWMutex` for read-heavy state; one mutex per shared field, always the same one.
- **CONC-15.** `sync/atomic` for simple counters and flags; `sync.Once` / `sync.OnceFunc` for one-time init;
  atomics only on proven hot paths.
- **CONC-16.** Never `time.Sleep` to synchronize; use channels, WaitGroup, conditions, or context
  cancellation; always `Stop` tickers and timers.
- **CONC-17.** Every data race is a bug; `go test -race ./...` MUST pass; cover concurrent paths (the
  detector only sees exercised races).
- **CONC-18.** Check for goroutine leaks with `go.uber.org/goleak` in tests.
- **CONC-19.** Share memory by communicating: prefer channels and single-owner state; mutexes for naturally
  shared long-lived state.

### Testing (TEST)

- **TEST-1.** Table-driven tests: case struct in the function, `t.Run` loop at the bottom, receiver `tc`,
  `cmp.Diff` (`github.com/google/go-cmp/cmp`) for mismatch output; no testify.
- **TEST-2.** `TestXxx` names the unit under test; subtest names describe the scenario and must explain the
  failure without reading the body.
- **TEST-3.** Mark independent tests/subtests `t.Parallel()`; never parallelize tests mutating shared state.
- **TEST-4.** `t.Fatalf` only when continuing is pointless (setup broken); `t.Errorf` for value comparisons
  so all failures surface in one run.
- **TEST-5.** Failure messages are self-sufficient: function, inputs, got vs want — actual first, expected
  second.
- **TEST-6.** Helpers that can fail call `t.Helper()` so failures point at the caller.
- **TEST-7.** Clean up with `t.Cleanup` (not `defer`) inside helpers; `t.TempDir()` for scratch filesystems;
  `t.Context()` for request-scoped contexts.
- **TEST-8.** Test public API from the `foo_test` package; same-package tests only for unexported internals.
- **TEST-9.** `TestMain` only for process-wide setup/teardown (the database pool of integration tests,
  `goleak.VerifyTestMain`); per-test setup in helpers and `t.Cleanup`.
- **TEST-10.** Prefer small handwritten fakes over mock frameworks; fakes live next to the consumer's
  interface; never producer-side mock interfaces (CORE-2).
- **TEST-11.** Unit tests are hermetic: no real network, DB, or sleeps; `httptest` + in-memory fakes. The
  only exception: store tests with the `integration` build tag run against the Postgres of compose.
  `-race` MUST pass; goleak for leaks.
- **TEST-12.** Assert error behavior via `errors.Is`/`errors.As` against sentinels/types, not message text.
- **TEST-13.** Benchmarks are `BenchmarkXxx` looping over `b.Loop`.
- **TEST-14.** Public packages include testable `ExampleXxx` functions with an `// Output:` comment — they
  compile, run, and document.
- **TEST-15.** Golden-file tests accept an `-update` flag; regenerated fixtures are committed deliberately.
- **TEST-16.** Test behavior through the public API, not implementation details; refactors without behavior
  change MUST NOT break tests; chase meaningful coverage, not a percentage.
- **TEST-17.** Test concurrent and time-dependent code with `testing/synctest`: a `synctest.Test` bubble
  virtualizes time.

### Performance (PERF)

- **PERF-1.** Never optimize without measurement: a benchmark proving the win (`b.Loop`, `benchstat`) and a
  profile explaining it (pprof).
- **PERF-2.** Performance-critical code carries a comment naming the benchmark or profile that justifies it.
- **PERF-3.** Preallocate containers of known size (`make([]T, 0, n)`, `make(map[K]V, n)`).
- **PERF-4.** `strconv` over `fmt` for number↔string in hot paths; `fmt` stays for human-facing output.
- **PERF-5.** Build strings with `strings.Builder` (pre-size with `Grow`), never `+=` in loops;
  `strings.Join` for fixed part sets.
- **PERF-6.** Hoist string↔`[]byte` conversions out of loops (each copies); the map-lookup `m[string(b)]`
  pattern is the allocation-free exception.
- **PERF-7.** Reuse hot-path buffers with `sync.Pool` only after a profile shows allocation pressure: store
  pointers, `Reset` on checkout, `Put` with `defer`.
- **PERF-8.** Keep allocations out of hot loops: pass small values by value, avoid boxing into `any` per
  iteration, check escapes with `-gcflags='-m'`.
- **PERF-9.** Stream instead of slurp: `io.Reader`/`io.Writer`/`io.Copy`; MUST NOT `io.ReadAll` unbounded
  input.
- **PERF-10.** `encoding/json` is reflection-heavy: reuse codecs and preallocate targets; no
  `encoding/json/v2` (it needs `GOEXPERIMENT`), no hand-rolled serialization without a profile.
- **PERF-11.** Reset reused maps with `clear(m)` instead of rebuilding; deleting during `range` is safe.
- **PERF-12.** Goroutines are cheap but not free: bound unbounded fan-out; add parallelism only after the
  single-goroutine version is profile-clean.
- **PERF-13.** Attack lock contention with evidence: mutex profile first, then shrink critical sections,
  `RWMutex`/atomics, shard only with proof.
- **PERF-14.** No unproven micro-optimizations: pointers-everywhere, interface avoidance, hand-rolled
  caches, and `defer` removal are forbidden; keep code obvious and the benchmark next to it.
