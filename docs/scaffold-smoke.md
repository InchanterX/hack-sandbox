# T01 scaffold verification

Verified on 2026-11-10, before T02. `app/server.py` still contains
`MESSAGE = "seed"`. The existing contract and process smoke tests pass locally
on Python 3.14.10. Compose configuration resolves without environment variables;
container execution is blocked by the missing Docker daemon socket in this
workspace. This report does not establish a successful build or deployment.

## Verified files and correction

The only scaffold configuration change is `deploy/images.yaml`: renamed image
`app` to `api` and added the explicit Dockerfile path. The old metadata omitted
that path (the loader returned `dockerfile=None`), although the Dockerfile lives
at `app/Dockerfile` and there is no root `Dockerfile`.
The corrected file describes exactly one image:

```yaml
version: 1
images:
  - name: api
    context: .
    dockerfile: app/Dockerfile
```

The installed Conductor loader accepted it (exit 0):

```sh
/opt/venv/bin/python -c 'from orchestra_contracts.build_job import load_images_file; print(load_images_file("deploy/images.yaml").model_dump())'
```

Result: `version=1`, one image with `name=api`, `context=.`,
`dockerfile=app/Dockerfile`, default `target=None`, and empty `build_args`.
The Dockerfile exists relative to the repository root.

Inspected and preserved:

- `compose.yaml`: service `dev`, root build context, `app/Dockerfile`, working
  directory `/workspace`, checkout bind mount `.:/workspace`, default command
  `[python, app/server.py]`. No required environment variables, env files,
  secrets or external services. The command inherits the image user.
- `app/Dockerfile`: `FROM python:3.14-slim`, standard library application only,
  `USER 10001:10001`, `EXPOSE 8080`, image working directory `/app`, default
  command `["python", "server.py"]`. `PYTHONUNBUFFERED=1` and
  `PYTHONDONTWRITEBYTECODE=1` are built-in defaults, not external requirements.
- `.dockerignore`: existing exclusions retained (`.git`, `.venv`,
  `__pycache__`, `docs`, `brief`).
- `app/server.py`: existing `ThreadingHTTPServer`, CLI host `0.0.0.0`, default
  port `8080`, dynamic port support and flushed `LISTENING=<port>` readiness
  line. No application changes.
- `tests/contract/test_http.py`: unchanged health and unknown-path checks for
  status, JSON content type and exact body.
- `tests/e2e/test_smoke.py`: unchanged separate-process test with dynamic port,
  readiness wait, bounded timeouts and terminate/kill cleanup. At this stage it
  accepts a string root message; S6 owns the final exact-message assertion.
- `deploy/base/kustomization.yaml`, `deployment.yaml`, `service.yaml`, and
  `deploy/overlays/demo/kustomization.yaml`: unchanged and consistent as below.

## Deployment consistency

Local YAML assertions passed for the following existing settings:

| Setting | Verified value |
| --- | --- |
| Demo namespace | `hack-sandbox-release` |
| Demo resource | `../../base` |
| Base resources | Deployment and Service only |
| Deployment / Service names | `smoke-api` / `smoke-api` |
| Deployment selector, pod label, Service selector | `app: smoke-api` |
| Container name and image reference | `api` / `api` |
| Overlay image name | `api`, matching the loader metadata |
| Overlay image destination | `registry.faberge-eggs.ru/hackathons/sandbox-release/api`, existing `seed` tag retained |
| Container port | named `http`, `8080` |
| Service | implicit `ClusterIP`, port `8080`, `targetPort: http` |
| Readiness | `/healthz` on `http`; period 2s, timeout 1s, failure threshold 3 |
| Liveness | `/healthz` on `http`; initial delay 5s, period 5s |
| Pod security | non-root UID/GID `10001:10001`; service account token automount disabled |
| Container security | read-only root filesystem, privilege escalation disabled, capabilities drop `ALL` |
| Requests / limits | CPU `50m` / `200m`; memory `64Mi` / `128Mi` |
| Rollout | one replica, `Recreate`, progress deadline 60s |

No HTTPRoute or additional cluster resources were introduced. No cluster
commands were run. Conductor owns image publication, SHA substitution and Argo
sync; the operator owns namespace preparation and demo port-forwarding.

## Commands and actual outcomes

Run all commands from the repository root. The exact existing Compose checks are:

```sh
docker compose -f compose.yaml run --rm dev python -m unittest discover -s tests/contract -v
docker compose -f compose.yaml run --rm dev python -m unittest discover -s tests/e2e -v
```

Both were attempted and exited 1 before container startup:

```text
failed to connect to the docker API at unix:///var/run/docker.sock; check if the path is correct and if the daemon is running: dial unix /var/run/docker.sock: connect: no such file or directory
```

Docker Compose v5.7.0 is installed. These configuration commands both succeeded
(exit 0), including resolution with an empty environment apart from `PATH`:

```sh
docker compose -f compose.yaml config
env -i PATH="$PATH" docker compose -f compose.yaml config --format json
```

The resolved configuration has the current checkout mounted at `/workspace`,
working directory `/workspace`, root build context and `app/Dockerfile`.
Runtime Python version, UID/GID and bind-mount operation inside a container
remain unverified here; their configuration is as described above.

The same existing test commands were run directly using local Python 3.14.10:

```sh
python -m unittest discover -s tests/contract -v
python -m unittest discover -s tests/e2e -v
```

Both exited 0: contract `Ran 1 test ... OK`; process smoke `Ran 1 test ... OK`.
No test files were changed.

An additional separate-process check launched `python -u app/server.py` with
no CLI overrides, waited at most 10 seconds for `LISTENING=8080`, and used
`HTTPConnection("127.0.0.1", 8080, timeout=5)` to assert the status, exact
`Content-Type` header and decoded JSON for each request. It passed (exit 0):

| GET | Status | Content-Type | Exact decoded JSON |
| --- | --- | --- | --- |
| `/healthz` | 200 | `application/json` | `{"status":"ok"}` |
| `/` | 200 | `application/json` | `{"message":"seed"}` |
| `/not-a-route` | 404 | `application/json` | `{"error":"not_found"}` |

The check closed connections and terminated/reaped the server in `finally`,
with a 5-second terminate timeout and kill fallback. This explicitly verifies
the scaffold root response beyond the existing e2e string-type assertion.

## Remaining verification

Rerun the two exact Compose commands in a Docker-enabled environment to verify
the container runtime contract. T02 changes only the message; S6 tightens the
process assertion to the final literal. S7 must establish publication and a
Healthy deployment of the intended SHA with all three HTTP responses. Local
passes and static manifest validation do not establish those release results.
