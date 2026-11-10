# Стек

Python 3.14, только стандартная библиотека: http.server, json, unittest.
Один существующий Dockerfile app/Dockerfile, build context — корень репозитория.
Сервис dev в compose.yaml имеет доступ к checkout и выполняет команды проверок без секретов.
Без pip/npm-зависимостей, frontend и внешних API. Контейнер приложения работает как UID 10001.
