# Деплой

> Проект живёт в namespace `hack-sandbox` и доступен на `https://sandbox.hack.faberge-eggs.ru`.
> Образы собирает дирижёр в кластере по `deploy/images.yaml` и сам подставляет реестр и тег.

## Что разворачиваем

| Сервис | Образ (`deploy/images.yaml`) | Порт | Публичный |
|---|---|---|---|
| `web` | `web` (context `web`) | 8080 | да: `/` через HTTPRoute |
| `server` | `server` (context `server`) | 8080 | да: `/api` через HTTPRoute |
| `db` | `postgres:17-alpine` (внешний образ, не собирается) | 5432 | нет |

Один HTTPRoute на `sandbox.hack.faberge-eggs.ru` (Gateway `default/home-gateway`, TLS уже настроен на шлюзе):
`/api` → `server`, `/` → `web`.

## Зависимости в кластере

- Postgres — StatefulSet `db` на 1 реплику с PVC 1Gi (StorageClass `local-path`) и Service `db`.
- Ограду namespace (квота, сетевые политики, pull-секрет реестра) ставит человек до релиза — в манифестах
  проекта её нет.

## Переменные окружения

| Имя | Кому | Откуда |
|---|---|---|
| `POSTGRES_PASSWORD` | `db`, `server` | **Secret `db`**, ключ `POSTGRES_PASSWORD` — создаёт человек; объявить в `deploy/secrets.yaml` |
| `POSTGRES_DB`, `POSTGRES_USER` / `DB_NAME`, `DB_USER` | `db`, `server` | ConfigMap: `calc`, `calc` |
| `DB_HOST` | `server` | ConfigMap: `db` |
| `WORKERS`, `OP_DELAY_MS`, `POLL_INTERVAL_MS`, `SESSION_TTL_HOURS` | `server` | ConfigMap: `2`, `700`, `300`, `24` |

`deploy/secrets.yaml`:
```yaml
secrets:
  - name: db
    keys: [POSTGRES_PASSWORD]
    description: пароль Postgres демо
```

## Ресурсы

Квота namespace: requests 2 CPU / 4Gi, limits 4 CPU / 8Gi, до 4 PVC.

| Контейнер | requests | limits |
|---|---|---|
| `server` | 100m / 128Mi | 500m / 256Mi |
| `web` | 50m / 64Mi | 200m / 128Mi |
| `db` | 100m / 256Mi | 500m / 512Mi |
