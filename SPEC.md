# SPEC

## Обзор

Release smoke существующего `sandbox-release`: один HTTP-процесс Python 3.14,
стандартная библиотека, один контейнер и ClusterIP Service. Итоговый клиентский
сценарий: `/healthz` сообщает о здоровье, `/` возвращает точное сообщение
`sandbox-release smoke`, неизвестный путь возвращает JSON с ошибкой. Источники:
`PLAN.md`, `brief/`, `DECISIONS.md`, `ASSUMPTIONS.md`.

Последовательность: S3 спецификация → T01/S4 scaffold → T02/S5 сообщение →
штатный S6 точная процессная приёмка → S7 штатный релиз. В `tasks.yaml` ровно
две Solo-задачи; S6/S7 запускает Conductor. Единственная stack area — `backend`.
Новые зависимости, сервисы, UI, БД, внешние API и AI не требуются.
Все продуктовые решения зафиксированы; `questions.yaml` остаётся `questions: []`.

## Модули и границы

Файлы каждого модуля перечислены явно. S4 формально владеет `**`, но сохраняет
готовый пример и системные документы; это не разрешение расширять объём brief.
После scaffold владение приложения и приёмки разделяется по стадиям.

| Модуль | Принадлежащие файлы | Контракт и границы изменения |
|---|---|---|
| HTTP-приложение | `app/server.py` | `ThreadingHTTPServer`, `Handler`, CLI; T02/S5 меняет только значение `MESSAGE`. HTTP-контракт: `contracts/openapi.yaml`. |
| Контрактные проверки | `tests/contract/test_http.py` (область `tests/contract/**`) | S4 сохраняет существующие проверки здоровья и неизвестного пути. После S4 файлы заморожены; T02 их не меняет. |
| Процессная приёмка | `tests/e2e/test_smoke.py` (область `tests/e2e/**`) | S4/S5 используют существующий запуск процесса; S6 добавляет точное сравнение финального сообщения. |
| Упаковка | `app/Dockerfile`, `compose.yaml`, `.dockerignore` | S4 проверяет Python 3.14, UID 10001, порт 8080, корневой build context и checkout в Compose `dev`. Без новых зависимостей и секретов. |
| Метаданные сборки | `deploy/images.yaml` | S4 описывает единственный образ `api`, context `.`, dockerfile `app/Dockerfile` (путь от корня). |
| Развёртывание | `deploy/base/deployment.yaml`, `deploy/base/service.yaml`, `deploy/base/kustomization.yaml`, `deploy/overlays/demo/kustomization.yaml` | S4 сохраняет согласованные имена, селекторы, порты, пробы и безопасность; дальнейший релиз выполняет Conductor. |
| Свидетельство scaffold | `docs/scaffold-smoke.md` | T01/S4 записывает проверенные файлы, необходимые исправления, команды и фактические результаты. |
| Спецификация и контракты | `SPEC.md`, `tasks.yaml`, `contracts/README.md`, `contracts/openapi.yaml`, `contracts/schema.sql`, `contracts/types.py` | S3 фиксирует контракт и DAG. Последующие стадии используют их как источник истины. |

## Контракты

Машиночитаемое описание: [contracts/openapi.yaml](contracts/openapi.yaml).
Общие типы Python: [contracts/types.py](contracts/types.py). Отсутствие хранилища
зафиксировано в [contracts/schema.sql](contracts/schema.sql); SQL-файл содержит
только комментарии, таблицы и миграции не создаются. Порядок проверки и границы
применения описаны в [contracts/README.md](contracts/README.md).

### HTTP

Все ответы ниже имеют `Content-Type: application/json`; JSON сравнивается как
объект, независимо от пробелов и порядка ключей. Дополнительных полей нет.

| GET request target | Статус | Точный JSON после S5 |
|---|---:|---|
| `/healthz` | 200 | `{"status":"ok"}` |
| `/` | 200 | `{"message":"sandbox-release smoke"}` |
| Любой другой, например `/not-a-route` | 404 | `{"error":"not_found"}` |

До T02 константа `MESSAGE` равна `seed`; это ожидаемое промежуточное состояние
S4. Корневой ответ в OpenAPI и общих типах описывает итог после S5. Существующие
контрактные проверки не требуют финального сообщения; точное итоговое значение
проверяет S6. Нельзя менять сообщение раньше T02 или ослаблять итоговый контракт.
Маршрутизация сохраняет сравнение `self.path` с точными строками; query string
также делает request target отличным от `/` и `/healthz`. Иные HTTP-методы
остаются поведением стандартного сервера и не входят в smoke-контракт.

### Процесс

`python app/server.py` слушает `0.0.0.0:8080`. Сохраняются `--host`, `--port`,
поддержка порта `0` и строка готовности `LISTENING=<фактический порт>` с flush.
E2E запускает отдельный процесс на `127.0.0.1` с динамическим портом, ждёт
готовности не более 10 секунд, использует HTTP timeout 5 секунд и всегда
завершает процесс (terminate, затем kill при необходимости), закрывает stdout.
Импорт модуля не запускает сервер.

### Упаковка и релиз

Compose `dev` работает без env vars и секретов, монтирует checkout в
`/workspace`; команды проверок исполняются из него. Контейнер использует
`python:3.14-slim` и UID/GID `10001:10001`.

S4 исправляет существующее описание образа (`app` без Dockerfile) на:

```yaml
version: 1
images:
  - name: api
    context: .
    dockerfile: app/Dockerfile
```

Deployment и Service называются `smoke-api`. Селектор и pod label —
`app: smoke-api`; порт контейнера `http:8080`, Service port `8080`,
`targetPort: http`. Readiness и liveness вызывают `/healthz` на `http`.
Сохраняются `automountServiceAccountToken: false`, non-root UID/GID 10001,
read-only root filesystem, запрет privilege escalation, drop ALL capabilities,
существующие limits/requests, strategy и параметры проб.

Demo overlay использует `hack-sandbox-release` и image name `api`, совпадающий
с `deploy/images.yaml`. Namespace и ограничивающие ресурсы подготовлены
оператором. Согласно `brief/08_deploy.md`, в этом smoke нет HTTPRoute, DNS, TLS,
публичного ingress, `deploy/secrets.yaml` или прикладных секретов; не добавлять
Namespace, Secret, RBAC, ServiceAccount, NetworkPolicy, PVC и CI workflows.

Conductor собирает и публикует один SHA-тегированный образ `api` в Harbor,
подставляет образ и синхронизирует Argo. Кодовые задачи не меняют SHA-теги
вручную и не запускают `kubectl`, `helm`, `argocd`. Свидетельство успешного
релиза: публикация нужного SHA, соответствующая развёрнутая ревизия, Healthy
Deployment в `hack-sandbox-release` и три ответа из таблицы через операторский
port-forward. Только локальные тесты или здоровая старая ревизия недостаточны.

## Design

### Токены

Неприменимо: интерфейс — JSON HTTP, без цветов, шрифтов, отступов и изображений.

### Экраны

Неприменимо: страниц и frontend нет. Демо — три GET-запроса из таблицы HTTP.

### Компоненты

Единственный компонент представления — JSON-ответ HTTP-обработчика; схемы и
примеры определены в `contracts/openapi.yaml`. HTML-страницы ошибок не нужны.

### Состояния

Успех: здоровье `status=ok` и финальное `message=sandbox-release smoke`.
Ошибка неизвестного GET request target: 404 и `error=not_found`.
Пустые и loading-состояния UI неприменимы. До старта процесса клиент ждёт строку
готовности с ограниченным timeout; отдельного API состояния загрузки нет.

## Проверки и DAG

T01: Solo, T1, S4, `owned_files: ["**"]`, 8 минут. Проверяет существующий
scaffold, исправляет метаданные образа, сохраняет `seed`, пишет
`docs/scaffold-smoke.md`. T02: Solo, T3, S5, только `app/server.py`, 2 минуты;
зависит от T01 как от готового scaffold и меняет одну константу. Дополнительных
Fusion-задач и искусственных последовательных зависимостей нет.

`defaults.checks` выполняет существующие контрактные проверки. `stack_checks`
содержит `backend: []`: отдельных дополнительных проверок одного stack нет,
дублировать общую команду не требуется. `defaults.e2e` — штатная команда S6;
S4 и S5 дополнительно запускают её вручную, пока она принимает строковое
сообщение. Проверки выполняются в Compose `dev` без секретов:

```sh
docker compose -f compose.yaml run --rm dev python -m unittest discover -s tests/contract -v
docker compose -f compose.yaml run --rm dev python -m unittest discover -s tests/e2e -v
```

S6 владеет `tests/e2e/**` и заменяет проверку типа корневого сообщения на
`self.assertEqual(json.load(response), {"message": "sandbox-release smoke"})`.
Ожидаемое значение задаётся литералом, независимо от `app.server.MESSAGE`.
Сохраняются динамический порт, готовность, таймауты и очистка процесса.
S6/S7 не добавляются в DAG как задачи S5.

Конфигурация DAG проверяется установленным валидатором (CLI может отсутствовать):

```sh
/opt/venv/bin/python -c 'from orchestra_contracts.tasks_dag import load_tasks_file; load_tasks_file("tasks.yaml")'
```

## Общие файлы

Точный список `shared_files` из `tasks.yaml`:

- `compose.yaml`
- `app/Dockerfile`
- `deploy/**` (включает `deploy/images.yaml`, base и demo overlay)

Эти файлы создаёт/проверяет только S4; T02 ими не владеет. Контрактные тесты
`tests/contract/**` дополнительно защищены правилами Conductor после S4.
Исторические отчёты, brief и системные решения сохраняются.
