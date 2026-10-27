# Стек: devops

> Потребители: scaffold (S4: Dockerfile, compose, `deploy/`), Stagehand (S7), задачи с `stack: [devops]`.
> Что разворачиваем — в `08_deploy.md`; здесь — как это оформлять.
>
> Рамки оркестра, которые менять нельзя: namespace `hack-sandbox`, Argo CD AppProject `hackathon`;
> образы собирает дирижёр в кластере по `deploy/images.yaml` и сам подставляет реестр и тег (SHA коммита).

## Технологии и версии

| Что | Выбор | Версия | Комментарий |
|---|---|---|---|
| Сборка server | `golang:1.26` → `gcr.io/distroless/static-debian12:nonroot` | закреплённые теги | `CGO_ENABLED=0`, бинарник `/server` |
| Сборка web | `node:24-bookworm-slim` → `nginxinc/nginx-unprivileged:1.29-alpine` | закреплённые теги | SPA fallback на `index.html`, порт 8080 |
| Среда проверок | `Dockerfile.dev` на `golang:1.26` | см. `00_overview.md` | Go, golangci-lint, Node 24, kustomize, hadolint |
| БД в кластере | `postgres:17-alpine`, StatefulSet на 1 реплику | 17 | PVC 1Gi, StorageClass `local-path` |
| Kustomize | встроенный в Argo CD / kustomize 5.x | 5 | |

## Структура

```
server/Dockerfile
web/Dockerfile
web/nginx.conf               # SPA fallback, кэш статики, без проксирования /api (это делает HTTPRoute)
Dockerfile.dev               # образ среды проверок
compose.yaml                 # db, server, web (vite dev), dev — локальная среда и среда проверок
deploy/images.yaml           # server (context: server), web (context: web) — общий файл scaffold
deploy/secrets.yaml          # имена секретов (только имена): db / POSTGRES_PASSWORD
deploy/base/                 # db (StatefulSet, Service), server, web (Deployment, Service), HTTPRoute, ConfigMap
deploy/overlays/demo/        # то, что синхронизирует Argo: namespace hack-sandbox, images
```

## Как писать

- Многостадийные Dockerfile; в рантайм-образе — непривилегированный пользователь, только нужные файлы.
- У `server`: readiness — `GET /api/healthz` (проверяет БД), liveness — тот же путь с `failureThreshold: 6`;
  startupProbe — до 60 с (миграции).
- У `web`: readiness и liveness — `GET /` на 8080.
- requests/limits у каждого контейнера (`08_deploy.md`), конфигурация — env и ConfigMap.
- Пароль БД — только `secretKeyRef` на Secret `db` (ключ `POSTGRES_PASSWORD`); `DATABASE_URL` собирается в
  контейнере из `POSTGRES_PASSWORD` и несекретных `DB_HOST`, `DB_NAME`, `DB_USER` (из ConfigMap). Секрет
  создаёт человек; scaffold объявляет его в `deploy/secrets.yaml`, в манифестах сам Secret не создаётся.
- Манифесты ссылаются на образы по именам из `deploy/images.yaml` (`server`, `web`) — реестр и тег подставит
  дирижёр в `deploy/overlays/demo/kustomization.yaml`.
- HTTPRoute: `parentRefs: [{name: home-gateway, namespace: default}]`, `hostnames: [sandbox.hack.faberge-eggs.ru]`,
  правило `/api` → `server:8080` (PathPrefix), правило `/` → `web:8080`.

## Как не писать

- `latest`, `privileged`, `hostPath`, `NodePort`/`LoadBalancer`, `imagePullPolicy: Always`.
- Секреты в манифестах, образах, compose для кластера; объект `Secret` в `deploy/`.
- Ресурсы без лимитов.
- Объекты, которые запрещает AppProject `hackathon`: любые cluster-scoped (Namespace, ClusterRole),
  `ServiceAccount`, `Role`, `RoleBinding`, `NetworkPolicy`, `CiliumNetworkPolicy`, `ResourceQuota`,
  `LimitRange`. Под использует ServiceAccount `default`; ограду namespace ставит человек.

## Локальная среда (compose)

- `db` — `postgres:17-alpine`, пароль для локальной среды задаётся прямо в compose (только локально),
  healthcheck `pg_isready`.
- `server` — сборка `server/Dockerfile`, порт 8080, `DATABASE_URL` на `db`, `OP_DELAY_MS=700`, `WORKERS=2`.
- `web` — Vite dev-сервер на 5173 с прокси `/api` → `server:8080`.
- `dev` — `Dockerfile.dev`, репозиторий в `/workspace`, `DATABASE_URL` на `db`, `depends_on: db (healthy)`;
  на нём идут все проверки дирижёра.
- `docker compose up` поднимает всё одной командой.

## Команды проверок

Ровно эти команды выполнит Check Job для devops-задач (в сервисе `dev`, из корня репозитория):

- `kustomize build deploy/overlays/demo > /dev/null`
- `hadolint server/Dockerfile web/Dockerfile Dockerfile.dev`
