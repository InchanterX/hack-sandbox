# S4 / T01 scaffold smoke

Inspected and checked on 2026-11-10, before S5. The existing server still has
`MESSAGE = "seed"`. This report covers the scaffold, not release acceptance.

## Inspected paths and repair

| Paths | Finding / action |
| --- | --- |
| `deploy/images.yaml` | Repaired the single image entry: changed `name: app` to `name: api` and added `dockerfile: app/Dockerfile`; retained `version: 1` and `context: .`. This fixes the catalog's mismatch with the deployment and its missing Dockerfile path. |
| `app/Dockerfile` | Already correct: `python:3.14-slim`, standard library only, `USER 10001:10001`, `PYTHONDONTWRITEBYTECODE=1`, port 8080, and `CMD ["python", "server.py"]` using the copied existing server. No change. |
| `compose.yaml` | Already correct: `dev` builds the root context with `app/Dockerfile`, bind-mounts the checkout at `/workspace`, sets that working directory, and runs `python app/server.py`. Python version, UID/GID and bytecode settings are inherited from the image. No supplied environment variables or secrets are required. No change. |
| `.dockerignore` | Existing exclusions inspected; the server remains in the build context. No change. |
| `app/server.py` | Preserved `Handler`, `ThreadingHTTPServer`, default `0.0.0.0:8080`, CLI host/port flags, flushed `LISTENING=<port>` announcement and cleanup. No change. |
| `deploy/base/deployment.yaml`, `deploy/base/service.yaml`, `deploy/base/kustomization.yaml`, `deploy/overlays/demo/kustomization.yaml` | Existing resource wiring, probes and security inspected as detailed below. No change. |
| `tests/contract/test_http.py`, `tests/e2e/test_smoke.py` | Both original baseline suites run unchanged. Contract tests remain frozen after S4; strengthening the release-message process assertion belongs to S6. |

Required context read: `brief/04_stack/00_overview.md`,
`brief/05_conventions.md`, `brief/04_stack/backend.md`, `brief/08_deploy.md`,
`SPEC.md`, `PLAN.md`, `contracts/README.md`, `contracts/openapi.yaml`,
`contracts/schema.sql` and `contracts/types.py`. Also inspected `tasks.yaml`
and `README.md` for check commands and operator access.

## Configuration validation

`docker compose -f compose.yaml config --format json` succeeded and resolved
`dev` to this checkout's absolute build context and bind source, with
`app/Dockerfile`, target/working directory `/workspace`, and command
`["python", "app/server.py"]`. Validation also succeeded with an empty
environment:

```sh
env -i PATH="$PATH" docker compose -f compose.yaml config --quiet
```

Static inspection of the base and demo overlay confirmed:

- Exactly two resource kinds: Deployment and Service, both `smoke-api`;
  the overlay sets namespace `hack-sandbox-repair`.
- Deployment selector, pod label and Service selector all use
  `app: smoke-api`.
- The base container image and overlay image-transform name are `api`,
  matching the repaired catalog. The overlay retains the existing registry
  reference and `seed` placeholder; Conductor assigns the release SHA in S7.
- Container port `http` is 8080; Service port `http` is 8080 with target
  `http`. The Service omits `type`, so Kubernetes defaults it to ClusterIP.
- Readiness and liveness both GET `/healthz` on named port `http`.
- Retained Recreate strategy, resource requests/limits,
  `automountServiceAccountToken: false`, `runAsNonRoot: true`, UID/GID 10001,
  `readOnlyRootFilesystem: true`, `allowPrivilegeEscalation: false`,
  and dropped `ALL` capabilities.

Standalone Kustomize is unavailable here, so the overlay was inspected rather
than rendered. No cluster commands were run and no resource kinds were added.

## Baseline results

Both prescribed Compose commands were attempted without supplying application
environment variables or secrets:

```sh
docker compose -f compose.yaml run --rm dev python -m unittest discover -s tests/contract -v
docker compose -f compose.yaml run --rm dev python -m unittest discover -s tests/e2e -v
```

Both exited 1 before tests could start: Docker could not connect to
`unix:///var/run/docker.sock` because the socket does not exist. Compose
v5.7.0 is installed, but no daemon is available. Image build/start and the
container's effective Python version, UID/GID and mounted-checkout execution
therefore remain unverified at runtime in this environment.

Fallback checks used the installed Python **3.14.10**, without installing any
dependencies:

| Command | Actual result |
| --- | --- |
| `PYTHONDONTWRITEBYTECODE=1 python -m unittest discover -s tests/contract -v` | PASS: `HttpContract.test_routes`, 1 test, 0.511 s. Exact health/404 JSON, status and Content-Type assertions passed. |
| `PYTHONDONTWRITEBYTECODE=1 python -m unittest discover -s tests/e2e -v` | PASS: `HttpProcessSmoke.test_real_process`, 1 test, 0.122 s. Existing separate-process startup, announced temporary port, requests and cleanup passed. |

The original e2e test accepts any string message. To verify the S4 seed
requirement without changing S6 tests, an additional one-off Python check
started `app/server.py --host 127.0.0.1 --port 0` in a separate process using
`subprocess.Popen(..., env={})`. It waited at most 10 seconds for stdout,
received `LISTENING=38585`, and used that announced port with five-second HTTP
timeouts. All assertions passed:

| GET | Status | Content-Type | Exact parsed JSON |
| --- | --- | --- | --- |
| `/healthz` | 200 | `application/json` | `{"status":"ok"}` |
| `/` | 200 | `application/json` | `{"message":"seed"}` |
| `/not-a-route` | 404 | `application/json` | `{"error":"not_found"}` |

The extra check also verified each Content-Length against the response bytes,
then terminated/reaped the process and closed stdout. It did not replace the
prescribed baseline suites or establish container/cluster acceptance.

## Operator continuation

Rerun both Compose baseline commands above when a Docker daemon is available.
S5/T02 changes only `MESSAGE`; S6 strengthens the process test to require the
final release message. S7 build, SHA publication and Argo synchronization
remain Conductor responsibilities. Do not deploy the `seed` placeholder.

The operator provides the namespace. Demo access remains port-forward:

```sh
kubectl -n hack-sandbox-repair port-forward svc/smoke-api 18080:8080
```

Then request `/healthz`, `/` and `/not-a-route` at `http://127.0.0.1:18080`
and verify their status, Content-Type and exact JSON (the root must contain
`sandbox-repair smoke` after S5). No DNS, TLS, HTTPRoute, Namespace, Secret,
RBAC, ServiceAccount, NetworkPolicy or PVC resources are added by this scaffold.
