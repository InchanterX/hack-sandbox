# S6 integration smoke

Checked on 2026-11-10 with Python 3.14.10, standard library only.

`tests/e2e/test_smoke.py` starts the actual application in a separate process
on an announced temporary localhost port and exercises the full demo in order:

| GET | Status | Content-Type | Exact JSON |
| --- | --- | --- | --- |
| `/healthz` | 200 | `application/json` | `{"status":"ok"}` |
| `/` | 200 | `application/json` | `{"message":"sandbox-repair smoke"}` |
| `/not-a-route` | 404 | `application/json` | `{"error":"not_found"}` |

Each response also checks Content-Length against the received bytes. The
existing 10-second startup deadline, five-second HTTP timeout, temporary port,
terminate/wait/kill fallback, reader join and stdout close are preserved.
No application, deployment or frozen contract-test changes were necessary.

## Results

| Command | Result |
| --- | --- |
| `PYTHONDONTWRITEBYTECODE=1 python -m unittest discover -s tests/contract -v` | PASS: 1 test, 0.509 s. |
| `PYTHONDONTWRITEBYTECODE=1 python -m unittest discover -s tests/e2e -v` | PASS: 1 test, 0.126 s; all three routes tested against the running application. |
| `git diff --check` | PASS. |

A one-off negative check ran the same e2e TestCase with its child process
launch patched to load `app/server.py` with `runpy`, set `MESSAGE` to `seed`
in memory and call the existing `main()`. It produced exactly one expected
assertion failure, at `/`, comparing `seed` with `sandbox-repair smoke`, with
no test errors. The child was reaped and stdout closed. No source file was
changed for this check.

Both prescribed container commands were attempted:

```sh
docker compose -f compose.yaml run --rm dev python -m unittest discover -s tests/contract -v
docker compose -f compose.yaml run --rm dev python -m unittest discover -s tests/e2e -v
```

Both exited 1 before starting tests because `/var/run/docker.sock` does not
exist. Compose execution remains unverified here; rerun these commands where
the Docker daemon is available. Local success does not establish container
or cluster release acceptance. Image publication, Argo synchronization and
port-forward verification of the deployed release remain S7 responsibilities.
