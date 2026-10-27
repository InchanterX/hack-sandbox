# API demo e2e

Run in compose service `dev` from the repository root:

```sh
npm --prefix tests/e2e ci
npm --prefix tests/e2e test
```

`package.json` and `package-lock.json` are scaffold files (T01). The scenario
itself (`*.test.ts`) belongs to stage S6, see SPEC.md#Verification: the test
builds and starts the server against compose `db` with `OP_DELAY_MS=50`,
`WORKERS=2` in an isolated schema and walks all nine demo steps plus the
restart. Until S6 lands, `demo.test.ts` fails on purpose.
