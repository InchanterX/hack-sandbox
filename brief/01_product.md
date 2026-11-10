# Продукт

Это инфраструктурный smoke проекта sandbox-release, а не новая разработка продукта.
В репозитории уже есть небольшой HTTP-сервис. Сохранить его и минимально проверить конвейер.

Сценарий: клиент GET /healthz получает 200 и JSON {"status":"ok"}; GET / получает
200 и JSON {"message":"sandbox-release smoke"}; неизвестный путь возвращает 404.
Демо смотрим через port-forward, без DNS, TLS и HTTPRoute.

Критерии: контрактные и e2e проверки проходят, Build Job публикует один образ api в Harbor,
Argo развёртывает Deployment smoke-api в hack-sandbox-release, приложение Healthy.
Нельзя создавать новые сервисы, БД, AI-интеграции и публичные маршруты.
