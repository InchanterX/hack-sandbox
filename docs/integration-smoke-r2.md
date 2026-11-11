# S6-r2 integration smoke

Verified on 2026-11-11 with Python 3.14.10 and the standard library only.

The existing `tests/e2e/test_smoke.py` already starts the application as a
separate process on an ephemeral localhost port and covers the complete demo:

| GET | Status | Exact JSON |
| --- | --- | --- |
| `/healthz` | 200 | `{"status":"ok"}` |
| `/` | 200 | `{"message":"sandbox-repair smoke"}` |
| `/not-a-route` | 404 | `{"error":"not_found"}` |

Extended that process test with `/?demo=1`, `/healthz?demo=1`, `/healthz/`
and `/not-a-route/nested`. Each must return 404 with exactly
`{"error":"not_found"}`, preserving the literal request-target contract.
All seven responses check `Content-Type: application/json` and byte
Content-Length. Startup and HTTP deadlines and subprocess cleanup are unchanged.
No application, contract, container or deployment changes were needed.

## Verification

| Command | Result |
| --- | --- |
| `python -m unittest discover -s tests/contract -v` | PASS: 1 test, 0.518 s. |
| `python -m unittest discover -s tests/e2e -v` | PASS: 1 test, seven request cases, 0.195 s. |
| `git diff --check` | PASS. |

A one-off negative check patched the test's child launch to load the same
application with `runpy`, change `MESSAGE` to `seed` in memory and invoke
`main()`. The unchanged e2e assertions produced exactly one failure at `/`,
with no errors. The check also confirmed the child exited and stdout closed.
No application source was modified for this check.

Both prescribed Compose commands were attempted:

```sh
docker compose -f compose.yaml run --rm dev python -m unittest discover -s tests/contract -v
docker compose -f compose.yaml run --rm dev python -m unittest discover -s tests/e2e -v
```

Both exited 1 before tests started because `/var/run/docker.sock` is absent.
Compose verification remains outstanding and must be repeated with a running
Docker daemon. The local results do not establish container or release acceptance.
Readiness fault injection/recovery, image publication, Argo verification and
port-forward verification remain S7 operator responsibilities.
