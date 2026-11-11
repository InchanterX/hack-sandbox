# S6-r3 integration smoke

Verified on 2026-11-11 with Python 3.14.10 and the standard library only.
The existing `tests/e2e/test_smoke.py` already covers every step of the demo
in `brief/01_product.md`, in order, against the actual server running in a
separate process:

| GET | Status | Exact JSON |
| --- | --- | --- |
| `/healthz` | 200 | `{"status":"ok"}` |
| `/` | 200 | `{"message":"sandbox-repair smoke"}` |
| `/not-a-route` | 404 | `{"error":"not_found"}` |

The same process test also checks 404 for `/?demo=1`, `/healthz?demo=1`,
`/healthz/` and `/not-a-route/nested`, as required by literal request-target
matching. All seven responses check `Content-Type: application/json` and
byte Content-Length. It retains the ephemeral port, ten-second startup
limit, five-second HTTP timeout and terminate/wait/kill/stdout cleanup.

No application or test defect was found. Existing scenario coverage, runtime,
contracts, container configuration, deployment manifests and historical
reports are preserved; this round adds only fresh verification evidence.

## Results

| Command | Result |
| --- | --- |
| `PYTHONDONTWRITEBYTECODE=1 python -m unittest discover -s tests/contract -v` | PASS: 1 test, 0.510 s. |
| `PYTHONDONTWRITEBYTECODE=1 python -m unittest discover -s tests/e2e -v` | PASS: 1 test, seven request cases, 0.122 s. |
| `git diff --check` | PASS. |

A one-off negative check ran the unchanged process TestCase with its child
launch patched to load `app/server.py` with `runpy`, set `MESSAGE` to `seed`
in memory and invoke `main()`. It confirmed exactly one assertion failure,
for `/`, comparing `seed` with `sandbox-repair smoke`, and no errors. It also
confirmed that the child exited and stdout closed. No source was modified.

Both prescribed Compose checks were attempted with Docker Compose v5.7.0:

```sh
docker compose -f compose.yaml run --rm dev python -m unittest discover -s tests/contract -v
docker compose -f compose.yaml run --rm dev python -m unittest discover -s tests/e2e -v
```

Both exited 1 before running tests because `/var/run/docker.sock` is absent.
Compose verification remains outstanding in an environment with a running
Docker daemon. This environment limitation does not justify changing the
application or its container configuration.

These local results do not establish S7 release acceptance. Inject Git
authorization, readiness fault injection/recovery, image publication, Argo
verification and deployed-release port-forward checks remain operator/S7 work,
following `docs/smoke/repair-round.md` with fresh evidence for the repeat run.
