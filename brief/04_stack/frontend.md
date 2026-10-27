# Стек: frontend (React)

> Потребители: исполнители задач с `stack: [frontend]`, их ревьюеры, scaffold.
> Визуальные требования — в `07_design.md` и дизайн-спецификации `SPEC.md#Design`; здесь — как писать код.

## Технологии и версии

| Что | Выбор | Версия | Комментарий |
|---|---|---|---|
| Фреймворк | React + TypeScript (`strict: true`) | React 19, TS 5.x | только функциональные компоненты и хуки |
| Сборка | Vite | актуальная стабильная мажорная, закреплена в `package-lock.json` | dev-сервер проксирует `/api` на `server:8080` |
| Роутинг | React Router | 7 | режим библиотеки (`createBrowserRouter`), без фреймворк-режима |
| Данные с сервера | TanStack Query | 5 | опрос списка задач: `refetchInterval: 1000` |
| API-клиент | `openapi-typescript` + `openapi-fetch` | 7 / 0.x | типы генерируются из `contracts/openapi.yaml` в `web/src/api/schema.d.ts` |
| Стили | CSS Modules + CSS-переменные (токены из `07_design.md`) | — | без UI-китов и CSS-фреймворков |
| Тесты | Vitest + Testing Library (`@testing-library/react`, `user-event`), jsdom | Vitest 3 | |
| Линтер | ESLint 9 (flat config) + `typescript-eslint`, `eslint-plugin-react-hooks` | — | |

## Структура

```
web/
  index.html
  vite.config.ts
  src/main.tsx                 # точка входа, QueryClient, роутер
  src/router.tsx               # все маршруты — общий файл scaffold
  src/api/schema.d.ts          # сгенерировано (`npm run gen:api`), руками не правится
  src/api/client.ts            # openapi-fetch + токен из хранилища сессии + 401 → на /login
  src/auth/                    # хранение токена (localStorage), хук useAuth, защищённый маршрут
  src/screens/<Screen>/        # экран = папка: <Screen>.tsx, <Screen>.module.css, <Screen>.test.tsx
  src/components/<Name>/       # переиспользуемые компоненты так же
  src/styles/tokens.css        # дизайн-токены
```

Экраны: `Login`, `Register`, `Tasks` (список + форма создания + изменение/удаление в строке).

## Как писать

- Все запросы — через `src/api/client.ts` и хуки TanStack Query (`useQuery` / `useMutation`); после мутации —
  `invalidateQueries(['tasks'])`.
- У каждого запроса три состояния в UI: загрузка, ошибка (текст из `error.message` ответа API), пусто.
- Формы — управляемые компоненты; ошибки сервера (`409`, `422`) показываются у поля, а не `alert`.
- Тексты интерфейса — на русском, константами в компоненте (без i18n).
- Статусы задачи отображаются через один компонент `StatusBadge` с картой `status → подпись/цвет`.
- Цвета, отступы, радиусы — только через CSS-переменные из `tokens.css`.

## Как не писать

- Инлайн-стили и «магические» цвета, `fetch` в обход клиента, `any`, `// @ts-ignore`.
- Состояние, дублирующее сервер (список задач в `useState`), ручные `setInterval` для опроса.
- Компоненты длиннее ~200 строк; бизнес-логика в JSX.
- Игнорирование ошибок запросов; `console.log` в коммитах.

## Доступность и адаптивность

- Семантические теги (`main`, `form`, `table` или список), `label` у каждого поля, фокус виден, всё доступно
  с клавиатуры, кнопки — `button`.
- Брейкпоинт 640 px: на телефоне список задач — карточки, на ноутбуке — таблица.

## Тесты

- Тесты рядом с экраном (`<Screen>.test.tsx`): рендер состояний, отправка формы, обработка 409/422 — с
  подменой клиента (`vi.mock('../api/client')`), без сети.
- e2e сценария демо пишет интегратор (S6) в `tests/e2e/`, по API.

## Команды проверок

Ровно эти команды выполнит Check Job для frontend-задач (в сервисе `dev`, из корня репозитория):

- `cd web && npm ci`
- `cd web && npm run lint`
- `cd web && npm run typecheck` (`tsc --noEmit`)
- `cd web && npm test -- --run`
- `cd web && npm run build`

## Зависимости

- Разрешены: перечисленные в таблице выше и `clsx`.
- Запрещены: UI-киты (MUI, Ant, Chakra), Tailwind, Redux/MobX/Zustand, axios, moment.
- Новая зависимость — правка общего `web/package.json`, только через дирижёра (HANDOFF.md).
