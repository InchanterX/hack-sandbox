# Контракты

GET /healthz: 200 application/json, объект status=ok.
GET /: 200 application/json, строка message=sandbox-repair smoke после S5.
Остальные маршруты: 404 application/json, error=not_found.
Тесты фиксируют контракт здоровья до реализации и остаются неизменными в S5.
