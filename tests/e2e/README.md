# API demo e2e

Run in compose service `dev` from the repository root:

```sh
npm --prefix tests/e2e ci
npm --prefix tests/e2e test
```

`demo.test.ts` walks the nine demo steps of `brief/01_product.md` plus crash
recovery through the API (SPEC.md#Verification). `harness.ts` builds the
server and `server/internal/testdb/e2eschema`, creates an isolated schema on
the compose `db` (`DATABASE_URL` or `DB_HOST`/`DB_NAME`/`DB_USER`/`POSTGRES_PASSWORD`),
starts the server on a free port with `WORKERS=2`, `OP_DELAY_MS=50`, waits for
healthz, and stops the server and drops the schema at the end. Server output is
printed when a step fails.

A separate `load_*` user keeps both workers busy with 199-character tasks
(99 operations, about 5 s) whenever a step needs the queue to be visible, so
alice's list matches the demo exactly. Needs `go` on PATH (it is in `dev`).
`package.json` and `package-lock.json` are scaffold files (T01).
