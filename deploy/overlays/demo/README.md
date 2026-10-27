# Demo deployment

Build from the repository root with `kustomize build deploy/overlays/demo`.
The overlay sets `hack-sandbox` on every resource; it does not create a Namespace.
The Conductor replaces the catalog names `server` and `web` in `images:` with
registry images tagged by commit SHA before release. Keep these names aligned
with `deploy/images.yaml`; do not substitute `latest` tags.

The render contains exactly eight objects: ConfigMap `calc-config`, Services
`db`, `server` and `web`, StatefulSet `db`, Deployments `server` and `web`, and
HTTPRoute `calc-queue`. Every Service uses the default ClusterIP type.

The route attaches to `default/home-gateway` for
`sandbox.hack.faberge-eggs.ru`. PathPrefix `/api` reaches `server:8080` with its
prefix preserved; PathPrefix `/` reaches `web:8080`. There are no rewrite
filters. nginx serves SPA deep links and does not proxy API requests.

| Container | CPU / memory requests | CPU / memory limits | Health checks |
| --- | --- | --- | --- |
| server | 100m / 128Mi | 500m / 256Mi | HTTP `/api/healthz`; readiness and liveness failure threshold 6; startup 12 attempts every 5 seconds |
| web | 50m / 64Mi | 200m / 128Mi | HTTP `/` on port 8080 for readiness and liveness |
| db | 100m / 256Mi | 500m / 512Mi | `pg_isready` for readiness and liveness |

All workloads have one replica. The server uses `Recreate` and a 30-second
termination grace period to prevent overlapping queue workers during rollout.
The database claims a 1Gi ReadWriteOnce PVC using `local-path`, mounted at
`/var/lib/postgresql/data`; `PGDATA` uses its `pgdata` subdirectory.

ConfigMap defaults are `POSTGRES_DB=calc`, `POSTGRES_USER=calc`, `DB_HOST=db`,
`DB_NAME=calc`, `DB_USER=calc`, `PORT=8080`, `WORKERS=2`, `OP_DELAY_MS=700`,
`POLL_INTERVAL_MS=300` and `SESSION_TTL_HOURS=24`. Both server and db obtain
`POSTGRES_PASSWORD` only through `secretKeyRef` to `db/POSTGRES_PASSWORD`.
The manifests contain no password values or Secret objects.

Before public release, S7 must verify the existing namespace, Secret and key,
StorageClass, Gateway, route attachment permissions and namespace policies.
These prerequisites are managed outside this overlay.

## Verification

Run the Conductor checks in compose service `dev`, from the repository root:

```sh
npm --prefix tests/contract ci
npm --prefix tests/contract test
kustomize build deploy/overlays/demo > /dev/null
hadolint server/Dockerfile web/Dockerfile Dockerfile.dev
```

Build the production images from their catalog contexts, outside `dev`:

```sh
docker build -t t07-a3-server:verify server
docker build -t t07-a3-web:verify web
```

Verified on 2026-10-27 using Linux/arm64 Docker and the compose `dev` image
(Go 1.26.10, Node 24.22.1, Kustomize 5.8.3, hadolint 2.15.1):

- All 31 contract tests passed; Kustomize render and hadolint passed.
- Additional assertions against the rendered YAML passed for the exact object
  set, namespace, defaults, resources, replicas, rollout, PVC, probes, Service
  selectors, catalog images, secret references and route without rewrites.
- Both production images built. Server image: distroless, `/server`, uid/gid
  65532, port 8080. Web image: nginx-unprivileged 1.29.8, uid/gid 101, port 8080.
- In an isolated Docker network, the production server returned HTTP 200 from
  `/api/healthz` against a fresh Postgres 17.11 running as uid/gid 70 with dropped
  capabilities. The server ran with a read-only root filesystem and dropped
  capabilities. Database credentials were generated at runtime.
- The server failed immediately without database credentials and exited with
  status 0 after SIGTERM within its 30-second termination allowance.
- nginx returned the same SPA index with `Cache-Control: no-cache` for
  `/index.html`, `/login`, `/sessions/abc` and `/x/y`. Fingerprinted CSS and JS
  assets had one immutable cache header; a missing asset returned 404. A direct
  nginx request to `/api/healthz` also returned the SPA, confirming API handling
  belongs to the HTTPRoute.

The Docker daemon could not bind-mount this checkout, so the committed tree was
copied into a separate compose `dev` container before running checks. Production
builds used the local `server` and `web` contexts. Verification did not apply
manifests to the cluster or establish public URL availability.
