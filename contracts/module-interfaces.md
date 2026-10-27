# Module interfaces

These entry points are created as compiling stubs by T01. Feature tasks replace
bodies within their owned packages; the shared route table never needs changing.
Type names in this document denote generated OpenAPI types where applicable.
Domain services have no HTTP dependency. Only handlers import the generated wire
package; services may declare concrete domain records with explicit conversion.

## Backend composition

- `auth.NewHandler(pool *pgxpool.Pool, ttl time.Duration) *Handler` wires a service
  and SQL store. Expose `Register`, `Login`, `Logout`, `Me` as ordinary
  `(http.ResponseWriter, *http.Request)` methods and
  `RequireAuth(next http.Handler) http.Handler` as middleware.
- `auth.UserFromContext(ctx context.Context) (User, bool)` uses an unexported
  typed context key; User has ID, Login, CreatedAt. No token/hash/password fields
  reach responses or logs. `tasks` imports only this context accessor from auth.
- `tasks.NewHandler(pool *pgxpool.Pool) *Handler` wires service/store and exposes
  `Create`, `List`, `Get`, `Update`, `Delete` as HTTP handler methods.
- `server.NewHandler(pool *pgxpool.Pool, authHandler *auth.Handler,
  tasksHandler *tasks.Handler) http.Handler` in shared `routes.go` registers the
  ten routes from OpenAPI. Public register/login/healthz; wrap logout/me/all task
  routes with RequireAuth. Healthz uses a bounded database ping. Map methods and
  paths explicitly with Go's standard ServeMux, without another router library.
- Error/JSON helpers live in `internal/httpjson/` (owned by T01). This avoids
  an import cycle when the server route table imports auth/tasks. Handlers call
  `httpjson.Write(w, status, value)` and
  `httpjson.Error(w, status, code, russianMessage)`. Helpers log encoding failure
  without leaking response internals. They support a strict, size-bounded JSON
  decode: 16 KiB maximum, a single JSON object, required fields, no unknown fields;
  bad input maps to 422 validation_error. Task ID parsing happens in handlers,
  after authentication; invalid UUID gives 404.
- `queue.New(pool *pgxpool.Pool, cfg Config) *Pool`; Config contains Workers int,
  OpDelay time.Duration, PollInterval time.Duration.
  `Recover(ctx context.Context) error` resets interrupted rows;
  `Run(claimCtx, executionCtx context.Context) error` starts bounded workers,
  waits for their exit and returns cancellation/infrastructure errors.
  Separate contexts let main stop new claims immediately but give active work
  up to ten seconds before canceling execution. T01 creates this stub so main
  compiles; T05 completes it and `cmd/server/main.go`.
- `server/migrations.Apply(ctx context.Context, pool *pgxpool.Pool) error` runs
  embedded goose SQL before recovery. T01 provides and tests the real migration.
- `cmd/server/main.go` alone reads env, creates the pool, applies migrations,
  recovers queue, composes the shared route table and owns HTTP/worker shutdown.
  `internal/server/config.go`, `health.go` and their tests are T01-owned, complete
  code; T05 owns main, its tests and queue. Defaults are fixed in SPEC.md#Runtime.

Service/store consumer interfaces are package-local and split by capability
(1–3 methods) rather than one large CRUD interface. Exported constructors above
stay stable. Store methods own multi-query transactions, never handlers.

## Calculator

`calc.Parse(input string) (Expr, error)` creates an immutable AST without I/O.
`Expr.Eval(ctx context.Context, beforeBinary func(context.Context) error)
(float64, error)` evaluates left child, right child, calls beforeBinary exactly
once before each binary operation, then applies it. A nil callback means no delay.
Parentheses, literals and unary minus invoke no callback. Failed child evaluation
short-circuits the parent. Context cancellation is returned as such, never stored
as a failed arithmetic task. Sentinels are `ErrSyntax`, `ErrDivisionByZero`,
`ErrOverflow`; syntax errors may carry a 1-based position and match ErrSyntax.

Grammar (skip ASCII spaces between tokens only):

```text
expression := term { ('+' | '-') term }
term       := unary { ('*' | '/') unary }
unary      := '-' unary | primary
primary    := number | '(' expression ')'
number     := digit+ [ '.' digit+ ]
```

Consume the whole input. No unary plus/exponent. Length validation belongs to
HTTP/domain input validation; whitespace-only input of valid length is a syntax
error. A syntactically valid but out-of-range numeric literal remains parseable
and fails evaluation with ErrOverflow. Every intermediate operation must be
finite. Round once after the full evaluation with the equivalent of
`math.Round(value * 1e10) / 1e10`; validate finiteness of the scaled value and the
rounded result too. Negative zero becomes positive zero. Rounding overflow uses
ErrOverflow even if the unrounded arithmetic was finite.

## Queue and user mutation serialization

Claim in one atomic SQL statement from schema.sql, sorting created_at ASC, id ASC.
Commit/release locks before evaluation; calculate the RETURNING expression.
Final update is conditional on status running and sets all fields for done/failed
in a single statement. Runtime arithmetic failures persist division_by_zero or
overflow with the exact Russian messages; cancellation/infrastructure failure
leaves running for startup recovery, logs once and never fabricates a result.
Retry transient polling errors with the configured cancelable idle delay; keep
workers alive, bounded and observable through slog.

PATCH/DELETE select by id AND user_id FOR UPDATE inside their store transaction.
PATCH must validate after the lock/status check (service supplies a validation
callback or pre-parses and defers reporting its error until after status check).
Both mutations repeat the status predicate in their write. There is no separate
unlocked existence check. If PATCH wins the lock, its accepted expression is
claimed; if claim wins, PATCH returns 409. DELETE before claim removes the task;
claim before DELETE yields 409. Foreign IDs yield 404 in every state.

## Frontend composition

T01 creates `main.tsx` (QueryClientProvider, AuthProvider and RouterProvider),
shared `router.tsx` with `/login`, `/register` and protected `/`, plus an unknown
path redirect through `/`. It imports these stable symbols, replaced by T06:

- `auth/AuthProvider.tsx`: named `AuthProvider`; `auth/useAuth.ts`: `useAuth()`
  returns user, isLoading, signIn(session), signOut(). Session persists in
  localStorage; bootstrap validates it with GET /api/me before showing tasks.
- `auth/ProtectedRoute.tsx`: named `ProtectedRoute`, accepts children.
- `screens/Login/Login.tsx`, `screens/Register/Register.tsx`,
  `screens/Tasks/Tasks.tsx`: named `Login`, `Register`, `Tasks`.
- `api/client.ts`: singleton typed openapi-fetch client, configurable auth header,
  shared unauthorized handling. Every network request goes through this client.

Auth bootstrap/401/logout/account changes cancel outstanding requests and clear
user-scoped QueryClient state. Use task keys `['tasks', user.id]` and detail keys
`['tasks', user.id, task.id]`, refetchInterval 1000 and query invalidation prefix
`['tasks']` after each successful mutation. Late responses cannot restore the old
account's session or cache. Login 401 invalid_credentials remains an inline form
error; protected-request 401 clears session and navigates to /login.
