# Brief: как читать этот каталог

> Дирижёр читает каталог целиком на S1 и раздаёт выдержки исполнителям, поэтому
> каждый файл понятен без остальных.

## Проект

- **Название:** Calc Queue
- **Slug:** `sandbox` (репозиторий `InchanterX/hack-sandbox`, namespace `hack-sandbox`,
  демо на `https://sandbox.hack.faberge-eggs.ru`)
- **Команда и контакты:** InchanterX — отвечает на вопросы дирижёра и решает на гейтах.
- **Назначение:** тестовый прогон оркестра. Продукт маленький, но затрагивает все части конвейера: три области
  стека (backend, frontend, devops), БД, очередь, авторизацию, секрет в кластере, релиз по URL.

## Приоритеты при конфликте

1. `02_scope.md`: что входит в MVP.
2. `01_product.md`: сценарий демо и критерии приёмки.
3. `06_contracts.md`: API и модель данных.
4. `04_stack/`: стек и запреты; `00_overview.md` — общее для всех, остальные файлы — по областям.
5. Остальное.

## Чего не делать

- Никаких внешних SaaS и платных API, никакой AI/ML-части.
- Не усложнять: это тест конвейера, а не продукт. Всё, что не в **Must** (`02_scope.md`), — только если
  останется время.
- Не коммитить секреты: значения паролей только в переменных окружения; в репозитории — имена.

## Статус заполнения

| Файл | Готов? | Кто писал |
|---|---|---|
| 01_product.md | да | InchanterX |
| 02_scope.md | да | InchanterX |
| 03_architecture.md | да | InchanterX |
| 04_stack/00_overview.md | да | InchanterX |
| 04_stack/backend.md | да | InchanterX |
| 04_stack/frontend.md | да | InchanterX |
| 04_stack/devops.md | да | InchanterX |
| 05_conventions.md | да | InchanterX |
| 06_contracts.md | да | InchanterX |
| 07_design.md | да | InchanterX |
| 08_deploy.md | да | InchanterX |
