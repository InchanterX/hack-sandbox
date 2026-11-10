# Release smoke: sandbox-repair

Тестовый сервис без БД, внешних API, DNS и секретов. Исходный ответ `/` — `seed`.
Задача S5 меняет только MESSAGE на `sandbox-repair smoke`. `/healthz` остаётся рабочим.

Локально: `python app/server.py`. Проверки:

```sh
python -m unittest discover -s tests/contract -v
python -m unittest discover -s tests/e2e -v
```

В кластере: `kubectl -n hack-sandbox-repair port-forward svc/smoke-api 18080:8080`.
Проверка: `curl --fail http://127.0.0.1:18080/healthz`.
Тег seed — заготовка: до сборки и установки SHA через Conductor overlay не развёртывать.
