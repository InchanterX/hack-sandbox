# Contracts

`openapi.yaml` (OpenAPI 3.1) is the source of truth for the ten HTTP operations,
wire schemas, status codes, authentication and error envelopes. `schema.sql` is
the PostgreSQL 17 target DDL. `types.ts` records shared wire types for contract
checks and API e2e; production types are generated from OpenAPI, never copied
from the reference file. `module-interfaces.md` fixes package composition and
cross-module behavior. `examples.json` contains deterministic calculator cases.

T01 owns all these files after S3, generates and commits
`server/internal/openapi/` and `web/src/api/schema.d.ts`, copies DDL into an
embedded goose migration, and writes `tests/contract/**`. Later tasks do not
edit contracts, generated files, migrations or contract tests; request necessary
changes through HANDOFF.md. Contract changes and regenerated bindings belong in
the same commit. Go generation is pinned to `oapi-codegen v2.8.0` per PLAN.md;
verify actual OpenAPI 3.1 support at the start of T01. Do not downgrade the source
contract; report incompatibility and use a generation-only compatibility
conversion if needed, checked against the canonical schemas.

T01 creates the following commands, all run in compose service `dev` from the
repository root, without nested Docker:

- `npm --prefix tests/contract ci` installs pinned contract check dependencies.
- `npm --prefix tests/contract test` validates OpenAPI, SQL/migration parity,
  shared reference types, examples and generated binding drift. Hermetic HTTP
  contract checks use module handlers with in-memory fakes; stubs need only build
  and expose typed entry points at S4. Full behavior is asserted as modules land.
- `npm --prefix tests/contract run generate` regenerates both Go and web bindings.
  Generation drift checks write to temporary files and compare with committed
  files, so checks never mutate the working tree.

`tests/contract/package.json` is an independent tooling manifest (not a root npm
workspace). T01 pins all tools in its lockfile, including openapi-typescript 7,
a 3.1-capable schema validator, TypeScript 5 and the Go generator invocation.
Module tests check actual HTTP statuses/codes and SQL semantics; API demo e2e is
written in S6. A contract check must not require unfinished features or connect
to a shared running server. See SPEC.md#Verification for the complete boundary.
