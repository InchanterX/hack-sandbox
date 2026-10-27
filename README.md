# Calc Queue

Arithmetic expressions computed by a PostgreSQL-backed queue: React SPA (`web/`),
Go API with in-process workers (`server/`), PostgreSQL 17. See `SPEC.md` and
`contracts/` for the API, schema and module boundaries.

## Local environment

```sh
cp .env.example .env              # set POSTGRES_PASSWORD (never commit it)
docker compose up --build         # db, server (:8080), web (Vite, :5173), dev
curl http://localhost:8080/api/healthz
```

`server` applies the embedded goose migrations on start; `web` proxies `/api`
to `server`. The password always comes from the environment: compose has no
default, and the server builds its DSN with URL escaping.

## Checks

Every check runs in compose service `dev` (no Docker inside it); the pipeline's
Conductor runs them on each branch, so there is no GitHub CI workflow:

```sh
docker compose run --rm dev sh -c 'npm --prefix tests/contract ci && npm --prefix tests/contract test'
docker compose run --rm dev sh -c 'cd server && golangci-lint run ./... && go test -race -tags=integration ./...'
docker compose run --rm dev sh -c 'cd web && npm ci && npm run lint && npm run typecheck && npm test -- --run && npm run build'
docker compose run --rm dev sh -c 'kustomize build deploy/overlays/demo > /dev/null && hadolint server/Dockerfile web/Dockerfile Dockerfile.dev'
```

The commands are those of `tasks.yaml` (`defaults.checks` and `stack_checks`).
`npm --prefix tests/contract run generate` regenerates the committed Go
(`server/internal/openapi/`) and TypeScript (`web/src/api/schema.d.ts`) bindings
from `contracts/openapi.yaml`; the contract tests fail on any drift.

The API demo e2e (`npm --prefix tests/e2e test`) is written in stage S6; until
then its placeholder fails on purpose.

## Deployment

`deploy/overlays/demo` (Kustomize) targets namespace `hack-sandbox`; images
`server` and `web` are listed in `deploy/images.yaml`, the only secret
(`db`/`POSTGRES_PASSWORD`) in `deploy/secrets.yaml`, created by a human.
