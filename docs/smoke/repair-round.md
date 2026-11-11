# Readiness recovery release smoke

This is a prospective operator procedure for `smoke-api` in namespace
`hack-sandbox-repair`. S7 owns the intentional readiness fault injection,
recovery, and cluster verification. Do not prepare the fault by changing the
application, Dockerfile, or Kubernetes manifests.

Use fresh evidence from the target release at each stage and retain the
observation time, namespace, resource, and release SHA. The expected SHA is the
release SHA supplied by the S7 release; confirm it against the deployed image
reference. Do not substitute a local or historical smoke result.

## 1. Before injection: record Ready

Confirm the `smoke-api` Deployment is Ready/Available and record its deployed
release SHA. Confirm Argo reports the application Healthy. Capture fresh
operator evidence for all three values before injecting the fault.

## 2. Inject and observe NotReady

The S7 operator intentionally injects the readiness fault and records fresh
evidence that `smoke-api` is NotReady in `hack-sandbox-repair`. Record the
injection and observed state. Do not report recovery at this stage.

## 3. Recover and verify release state

The S7 operator restores readiness. Capture fresh evidence that the Deployment
is Ready/Available, Argo is Healthy, and the deployed image uses the expected
release SHA. If any value is missing or differs, recovery has not passed.

## 4. Verify HTTP through port-forward

With the recovered Service, forward local port 8080 to Service
`smoke-api` port `http` (8080). For example, using the operator's approved
cluster access:

```sh
kubectl -n hack-sandbox-repair port-forward svc/smoke-api 8080:8080
```

Keep the forward running and issue these requests from another terminal. Check
the HTTP status, the exact JSON object (with no extra fields), and the
`Content-Type` header on every response:

| Request | Expected status | Expected `Content-Type` | Exact JSON body |
|---|---:|---|---|
| `GET http://127.0.0.1:8080/healthz` | 200 | `application/json` | `{"status":"ok"}` |
| `GET http://127.0.0.1:8080/` | 200 | `application/json` | `{"message":"sandbox-repair smoke"}` |
| `GET http://127.0.0.1:8080/not-a-route` | 404 | `application/json` | `{"error":"not_found"}` |

For example, inspect each response with `curl -i`; verify the body as JSON
rather than relying on a successful connection alone. Record fresh HTTP evidence
after the recovery checks.

## Release gate

The release smoke passes only when the before-injection Ready/Available state,
the injected NotReady state, and the recovered Ready/Available state are all
supported by fresh evidence; Argo is Healthy; the deployed SHA matches the
expected release SHA; and all three port-forward responses match exactly.
If the cluster, Argo, port-forward, or required evidence is unavailable, record
the gate as **not passed** and state what could not be verified. A local check
or historical smoke report does not establish successful recovery.

## Repeat run after inject Git authorization

Run this procedure again only after the operator has configured Git
authorization for inject. That authorization is an operator prerequisite; this
S5 change does not alter Git configuration or handle credentials. S7 remains
responsible for the intentional readiness fault, its recovery, and all cluster
verification for `smoke-api` in `hack-sandbox-repair`.

For this repeat, collect fresh evidence before injection showing the initial
Deployment Ready/Available, Argo Healthy, and the expected release SHA; while
the injected fault is active, showing NotReady; and after recovery, showing
Ready/Available, Argo Healthy, and the expected deployed release SHA. Then
repeat all three port-forward HTTP requests above and retain their status,
`Content-Type`, and exact JSON response bodies as evidence. Previous smoke
results do not establish success for this run.

Missing inject authorization, unavailable infrastructure, or missing evidence
at any required stage leaves this repeat's release gate **not passed**. This
is a prospective procedure and does not claim that the repeat has run or
passed.
