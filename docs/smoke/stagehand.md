# Диагностика hack-sandbox

Дата: 2026-11-10. Sandbox и namespace: `hack-sandbox`.

Попытки запуска команд:

- `orch-ops app_status hack-sandbox` — ошибка оболочки `orch-ops: not found` (код 127).
- `orch-ops get pods -n hack-sandbox` — ошибка оболочки `orch-ops: not found` (код 127).
- `orch-ops events -n hack-sandbox` — ошибка оболочки `orch-ops: not found` (код 127).

CLI orch-ops отсутствует в PATH, а инструменты MCP orch-ops в этой сессии недоступны. Поэтому запросы до кластера не дошли: наличие pods, событий и ошибок в namespace неизвестно. Нельзя сделать вывод, что окружение пустое. Диагностику необходимо повторить при доступном orch-ops.
