# SPEC

## Обзор

`sandbox-repair` — release smoke существующего HTTP-сервиса, а не новая
разработка. Источники решений: [PLAN.md](PLAN.md), [brief/](brief/),
[DECISIONS.md](DECISIONS.md) и [ASSUMPTIONS.md](ASSUMPTIONS.md). Последние два
документа пока не содержат дополнительных решений. Все продуктовые решения
зафиксированы в brief; существующий `questions.yaml` остаётся `questions: []`.

Стек: Python 3.14, только стандартная библиотека (`http.server`, `json`,
`unittest`). Один `ThreadingHTTPServer`, один контейнер, один ClusterIP Service.
Нет frontend, БД, внешних API, AI, зависимостей pip/npm или прикладных секретов.
Единственное изменение приложения на S5 — значение `MESSAGE`.

## Модули и границы

Пути в таблице — полное владение каждого модуля относительно корня репозитория.
Общий wildcard scaffold в DAG нужен по правилам Conductor; фактическая работа
S4 ограничена перечисленными файлами и сохранением готового приложения.

| Модуль | Собственные файлы | Ответственность и этап |
|---|---|---|
| Планирование и контракты | `SPEC.md`, `tasks.yaml`, `contracts/README.md`, `contracts/openapi.yaml`, `contracts/schema.sql`, `contracts/types.py` | S3: контракт HTTP, отсутствие хранения, типы ответов, владение и DAG; последующие реализации используют эти документы. |
| HTTP-сервис | `app/server.py` | T02 / S5: изменить только `MESSAGE` с `seed` на `sandbox-repair smoke`; сохранить обработчик, заголовки, CLI и жизненный цикл. |
| Упаковка и локальная среда | `app/Dockerfile`, `compose.yaml`, `.dockerignore` | T01 / S4: сохранить Python 3.14, UID/GID 10001, порт 8080 и запуск без env/secrets; `dev` работает с checkout в `/workspace`. |
| Конфигурация развёртывания | `deploy/images.yaml`, `deploy/base/deployment.yaml`, `deploy/base/service.yaml`, `deploy/base/kustomization.yaml`, `deploy/overlays/demo/kustomization.yaml` | T01 / S4: согласовать один образ `api` и существующие ресурсы. S7 Conductor обновляет release image reference. |
| Контрактная проверка | `tests/contract/**` (существующий `test_http.py`) | T01 / S4: сохранить проверки здоровья и неизвестного маршрута; после S4 файлы заморожены. |
| Процессная приёмка | `tests/e2e/**` (существующий `test_smoke.py`) | Штатный S6: проверять отдельный процесс и точное итоговое сообщение; сохранить таймауты и очистку. S4 только запускает исходную проверку. |
| Отчёт scaffold | `docs/scaffold-smoke.md` | T01 / S4: перечислить проверенные пути, исправления конфигурации и фактические результаты базовых проверок. |

Системные документы, brief, `docs/fusion/**`, исторические `docs/smoke/**`
и `.github/workflows/**` не изменяются. CI и новые инструменты не добавляются.
Контракты S3 не требуют дополнительных runtime-модулей, роутеров или генерации кода.

## Контракты

Нормативное описание: [contracts/openapi.yaml](contracts/openapi.yaml).
Пояснения и границы: [contracts/README.md](contracts/README.md).
Общие Python-типы: [contracts/types.py](contracts/types.py).
Схема хранения: [contracts/schema.sql](contracts/schema.sql) явно фиксирует
отсутствие БД и DDL; миграции, таблицы и сервис БД не нужны.

| GET-запрос | Статус | Content-Type | Точный JSON после S5 |
|---|---:|---|---|
| `/healthz` | 200 | `application/json` | `{"status":"ok"}` |
| `/` | 200 | `application/json` | `{"message":"sandbox-repair smoke"}` |
| Любой другой путь, например `/not-a-route` | 404 | `application/json` | `{"error":"not_found"}` |

Объекты не имеют дополнительных полей. Равенство JSON семантическое: порядок
ключей и пробелы незначимы. `Content-Length` соответствует числу байтов тела.
Аутентификация, тела запросов и параметры API отсутствуют. Другие HTTP-методы
не входят в smoke-контракт; существующее поведение stdlib сохраняется.
Маршрутизация сравнивает `Handler.self.path` буквально: строки с query,
`/healthz/` и вложенные неизвестные пути также получают 404; нормализация
и поддержка query не добавляются. Fallback описан расширением
`x-unknown-get-response` OpenAPI, без создания нового endpoint.

До T02 корень возвращает `{"message":"seed"}`: это допустимое состояние
локального scaffold, но не готовый релиз. S4 сохраняет исходные контрактные
проверки `/healthz` и `/not-a-route`; итоговый корень проверяется в S6.
OpenAPI и общие типы описывают итоговое состояние, не разрешают `seed` в релизе.

## HTTP-сервис

Сохранить `Handler`, `ThreadingHTTPServer`, `main()` и CLI `--host` / `--port`.
По умолчанию сервер слушает `0.0.0.0:8080`. `--port 0` выделяет временный порт;
строка stdout `LISTENING=<фактический порт>` с flush позволяет процессной
проверке узнать адрес. Закрытие сервера остаётся в `finally`.
T02 владеет только `app/server.py`, не меняет тесты, контракты или конфигурацию.

## Design

Представление — только JSON по HTTP; экранов, визуальных токенов, компонентов,
шрифтов, изображений и HTML-страниц ошибок нет. Состояния клиента:

| Состояние | Наблюдаемое представление |
|---|---|
| Здоровье | 200 и объект `status=ok`. |
| Успешное демо | 200 и точный объект `message=sandbox-repair smoke`. |
| Неизвестный путь | 404 и объект `error=not_found`. |
| Сервер ещё не слушает / недоступен | Ошибка соединения на стороне клиента; дополнительных экранов и endpoints нет. |

## Упаковка и развёртывание

S4 сохраняет `app/Dockerfile`: `python:3.14-slim`, `USER 10001:10001`,
`PYTHONDONTWRITEBYTECODE=1`, запуск существующего сервера на 8080.
`compose.yaml` предоставляет `dev`, build context `.`, Dockerfile
`app/Dockerfile`, bind mount checkout в `/workspace` и соответствующий working
directory. Команды проверок работают без env vars, credentials и секретов.

`deploy/images.yaml` имеет `version: 1` и ровно один элемент:
`{name: api, context: '.', dockerfile: app/Dockerfile}`. Deployment и overlay
ссылаются на логическое имя образа `api`; registry и SHA назначает Conductor.
Существующий `seed`-тег — заглушка, до S7 его не развёртывать и вручную не менять.

Сохранить Deployment/Service `smoke-api`, совпадающие селекторы `app: smoke-api`,
именованный порт `http` / 8080, readiness и liveness GET `/healthz`.
Сохранить ограничения ресурсов, Recreate, `runAsNonRoot`, UID/GID 10001,
`readOnlyRootFilesystem`, запрет privilege escalation, drop ALL capabilities
и `automountServiceAccountToken: false`.
Overlay задаёт `hack-sandbox-repair`; namespace предоставляет оператор.

Специальные исключения этого smoke из общего шаблона S3 зафиксированы в
`brief/08_deploy.md` и PLAN: **доступ только через port-forward**, без DNS,
TLS, HTTPRoute, Namespace, Secret, RBAC, ServiceAccount, NetworkPolicy, PVC
и `deploy/secrets.yaml`. Прикладных секретов нет. Кодовые задачи не запускают
`kubectl`, `helm`, `argocd`, не публикуют образы и не меняют SHA-теги.

## Проверки и этапы

`tasks.yaml` содержит только `T01 (S4, solo, T1) → T02 (S5, solo, T3)`.
T02 использует готовый сервер, среду и замороженные контракты scaffold,
поэтому зависит только от T01. Дополнительные Fusion и искусственные цепочки
не нужны. S6 интеграции и S7 релиза запускает Conductor после этого DAG;
их не дублируем в S5-задачах.

`defaults.check_env` — `{kind: compose, file: compose.yaml, service: dev}`.
Общая проверка — `python -m unittest discover -s tests/contract -v`.
`defaults.e2e` — `python -m unittest discover -s tests/e2e -v`.
`defaults.stack_checks.backend` пуст: единственная область backend уже
покрыта общей проверкой; нет дополнительных lint, frontend или ML-команд.
Scaffold и интеграция получают все области проверок по правилам Conductor.

- **S4 / T01:** исправить описание образа, проверить перечисленные файлы,
  выполнить обе исходные проверки с сообщением `seed` через Compose `dev`,
  записать реальные результаты в `docs/scaffold-smoke.md`. Не усиливать
  проверку итогового сообщения раньше S6. Заморозить контрактные тесты.
- **S5 / T02:** изменить только константу, выполнить общую контрактную проверку,
  создать отдельный осмысленный коммит. Корень должен выдавать итоговый JSON.
- **S6:** в `tests/e2e/test_smoke.py` проверять точные JSON, статус 200 и
  `application/json` для `/healthz` и `/`. Сохранить отдельный процесс,
  временный порт, ожидание старта не более 10 секунд, HTTP timeout 5 секунд,
  terminate/wait/kill и закрытие stdout. Подтвердить, что точное сравнение
  сообщения отвергает seed-ответ, не меняя приложение и замороженные тесты.
  Выполнить обе команды через Compose `dev` и закоммитить изменение теста.
- **S7:** Conductor собирает и публикует один `api` в Harbor с release SHA,
  обновляет ссылку и синхронизирует Argo. Уполномоченный оператор получает
  свежие подтверждения Available Deployment, Healthy Argo и развёрнутого SHA
  в `hack-sandbox-repair`, затем через port-forward проверяет все три строки
  таблицы HTTP. Исторические отчёты другого namespace не являются приёмкой.

Если инфраструктура недоступна, записать непройденный gate и доступные
локальные результаты; успешный локальный smoke не означает успешный релиз.

## Общие файлы

`shared_files` в `tasks.yaml` совпадает с этим списком. Только scaffold
создаёт или согласует их в кодовых задачах; T02 их не изменяет:

- `app/Dockerfile`
- `.dockerignore`
- `compose.yaml`
- `deploy/images.yaml`
- `deploy/base/deployment.yaml`
- `deploy/base/service.yaml`
- `deploy/base/kustomization.yaml`
- `deploy/overlays/demo/kustomization.yaml`
- `tests/contract/**`
- `docs/scaffold-smoke.md`

Штатное обновление image reference в S7 выполняет Conductor.
`tests/e2e/**` не shared: эти файлы меняет штатный интегратор S6.
