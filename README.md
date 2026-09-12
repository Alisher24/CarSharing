# CarSharing

Сервис каршеринга для Бишкека: Go backend, React frontend, расчёты в кыргызских сомах (KGS).
Планируются электрические, бензиновые, дизельные, гибридные и газовые автомобили.

Репозиторий: [Alisher24/CarSharing](https://github.com/Alisher24/CarSharing) (public).

Работает начальный каркас: React получает данные от Go API, API проверяет PostgreSQL,
схему и PostGIS через сгенерированный OpenAPI-интерфейс. Контракты остальных функций
описаны как planned; вход, карта, аренды, расчёты, worker, симулятор и почта пока не реализованы.

## Локальный запуск

Нужны Git, Docker с Linux containers и Docker Compose v2.29 или новее.
На Windows используйте Docker Desktop с WSL2 и PowerShell. Go и Node на хосте
для основного запуска не нужны. Первая сборка требует интернета.
Образ PostGIS закреплён для `linux/amd64`; ARM потребует эмуляции и отдельно не проверен.

```sh
git clone https://github.com/Alisher24/CarSharing.git
cd CarSharing
```

Подготовьте настройки из корня проекта — в PowerShell:

```powershell
powershell -ExecutionPolicy Bypass -File scripts/setup.ps1
```

Либо в Linux/macOS:

```sh
sh scripts/setup.sh
```

Если Node 24.21 уже установлен, на любой платформе можно выполнить
`node scripts/setup.mjs`. Setup создаёт `.env` из шаблона и случайные пароли/внутренний
ключи в `.secrets/`. Значения не выводятся; повторный запуск сохраняет их и настройки.
Отдельно создаются `simulator_token`, `demo_control_token`, `mailstub_delivery_token`,
`mailstub_demo_token`, `cursor_hmac_key` и `mailstub_cursor_hmac_key`. Они подготовлены
для будущих функций и пока не подключены к сервисам. При обновлении полной старой
установки `internal_token` сохраняется как `simulator_token`; прежний файл остаётся
локальной копией. Частичная миграция или потеря существующего ключа вызывает ошибку,
а не незаметную замену оставшихся значений. Session authentication key появится вместе с входом.
При утрате секрета восстановите его из своей локальной копии: новый пароль не подходит
к существующему volume. На Unix каталог секретов имеет права 0700; на Windows доступ
определяется правами папки пользователя. Не размещайте локальные секреты в общедоступной папке.

```sh
docker compose up --build -d
docker compose ps
```

Откройте [приложение](http://127.0.0.1:8080). Сообщение «Сервис на связи» появляется
после реального ответа API; время последней проверки берётся с сервера и отображается
по Бишкеку. Если порт занят, измените `APP_PORT` в `.env` и повторите `up`.
Наружу опубликован только frontend, на `127.0.0.1`. БД и API доступны внутри Compose.

БД хранится в named volume. `docker compose restart`, `docker compose stop` и
`docker compose down` сохраняют данные. **`docker compose down -v` удаляет данные**;
для обычной остановки используйте `docker compose down` без `-v`.

## Миграции, seed и проверки

Одноразовый сервис `migrate` автоматически применяет SQL-миграции после готовности
БД и перед запуском API. Повторный `up` безопасен. API использует отдельную роль без
superuser/DDL-прав; административный секрет подключён только к БД, миграциям и seed.
Соединение с БД без TLS предназначено только для локальной сети Compose.

```sh
docker compose run --rm migrate status
docker compose run --rm migrate up
docker compose --profile demo run --rm seed
```

Seed пока записывает только маркер `bootstrap-v1`, повторно не изменяя его дату.
Демопользователи и автомобили появятся вместе с соответствующими схемами.
Обычный запуск не выполняет seed. Запуск команды без `APP_ENV=demo` отклоняется.
Автоматический откат схемы намеренно не предлагается; перед опасными миграциями
потребуется резервная копия.

- `GET /api/v1/health/ready` через frontend возвращает готовность, город, валюту, часовой пояс и UTC-время сервера с шестью дробными цифрами.
- `GET /api/v1/health/live` проверяет только процесс; readiness проверяет БД, схему и PostGIS.
- При недоступной БД readiness возвращает безопасный `503 SERVICE_UNAVAILABLE` в общем JSON-формате ошибок. Все API-ответы имеют `X-Request-ID` и `Cache-Control: no-store`.
- Старые health URL удалены; planned endpoints и внешний `/internal` возвращают JSON `404 RESOURCE_NOT_FOUND`.
- Логи: `docker compose logs --tail=100 api migrate postgres frontend`.

После запуска, при наличии Node 24.21 на хосте:

```sh
node --test scripts/setup.test.mjs
node scripts/smoke.mjs
```

Smoke проверяет реальные API/PostGIS, права роли, повтор миграций/seed и сохранность
данных, отсутствие старых/planned маршрутов и закрытый внешний internal API.
Он кратко останавливает БД и запускает её снова, проверяя ответ 503 и восстановление
API, поэтому выполняйте его в свободном локальном окружении. Для другого порта:
`node scripts/smoke.mjs http://127.0.0.1:8181`.

## Разработка

React с hot reload, с тем же API и БД:

```sh
docker compose -f compose.yaml -f compose.dev.yaml up --build -d
```

Адрес остаётся [127.0.0.1:8080](http://127.0.0.1:8080); изменения в `frontend/src`
подхватываются автоматически. Изменения зависимостей требуют пересборки. Возврат
к собранному frontend: `docker compose up --build -d`. Backend пересобирается той же
командой; автоматический Go hot reload пока не включён.

Для проверок без контейнерной сборки нужны Go 1.27.1 и Node 24.21.0 с npm:

```sh
go -C backend test ./...
go -C backend vet ./...
npm ci
npm run format:check
npm --prefix frontend ci
npm --prefix frontend run build
```

Форматирование и линт стилей обязательны для каждого изменения; правила чтения кода
описаны в `AGENTS.md`. Prettier выравнивает TypeScript, JavaScript, CSS и JSON,
Stylelint проверяет CSS, `gofmt` — Go. Хук `pre-commit` форматирует файлы из индекса
той же конфигурацией, а `npm run format` выравнивает репозиторий целиком.
Сгенерированный код исключён из форматирования: он обновляется только генерацией.

Версии Go/Node и образы закреплены в Dockerfiles/Compose, зависимости — в
`go.mod`/`go.sum` и `package-lock.json`. Использованы React 19.3, TypeScript 7.0,
Vite 8.3, chi 5.3, pgx 5.11 и goose 3.28. [Vite поддерживает Node 24](https://vite.dev/guide/),
[PostGIS использует путь volume PostgreSQL 18](https://github.com/postgis/docker-postgis).
Обязательный Repository checks параллельно проверяет форматирование и стили,
Go, TypeScript-сборку, воспроизводимость контрактов, npm/Go-зависимости, историю
секретов и полный Compose smoke с тестовой PostGIS. Итоговый check проходит только
при успехе всех этих задач.

## Учётные записи и сессии

Вход по email и паролю. Сессия хранится в PostgreSQL, браузер получает только непрозрачный
токен в cookie `carsharing_session` (`HttpOnly`, `Path=/`, `SameSite=Lax`, абсолютные 12 часов
без продления); в БД лежит его SHA-256. `Secure` включается через `SESSION_COOKIE_SECURE=true`
и выключен только в документированном локальном HTTP-профиле. Создание пользователя и сохранение
сессии выполняются одной транзакцией: откат не оставляет ни пользователя, ни сессии.

Мутации учётной записи проверяют `Origin` по списку `ALLOWED_ORIGINS`; мутации с действующей
сессией дополнительно проверяют `X-CSRF-Token` из снимка сессии. CSRF-токен выдаётся на каждую
новую сессию и не действует дольше неё.

### Параметры Argon2id и измерения

Пароли хэшируются Argon2id с индивидуальной солью. Параметры — конфигурация, а не константы:

| Настройка | Переменная | Значение |
| --- | --- | --- |
| Память | `AUTH_ARGON2_MEMORY_KIB` | 19456 KiB (19 MiB) |
| Проходы | `AUTH_ARGON2_PASSES` | 2 |
| Потоки | `AUTH_ARGON2_PARALLELISM` | 1 |
| Одновременных вычислений на экземпляр | `AUTH_ARGON2_CONCURRENT` | 2 |

Измерение в целевом Docker-окружении (образ сборки `golang:1.27.1-alpine`, `linux/amd64`,
11th Gen Intel Core i7-11800H @ 2.30GHz), команда
`go test ./internal/auth/ -run NONE -bench BenchmarkDeployedHashing -benchtime 20x -cpu 1`:

| Величина | Результат |
| --- | --- |
| Время одного хэша | 28.4 мс |
| Память на один хэш | 19.00 MiB |

При занятости обоих мест вычисления запрос получает `503 SERVICE_UNAVAILABLE` без постановки
в неограниченную очередь. Неизвестный email проверяется против фиктивного хэша, чтобы быстрый
ответ не раскрывал отсутствие аккаунта.

### Ограничения частоты

Счётчики хранятся в PostgreSQL и переживают restart. Применимое ограничение проверяется до
вычисления хэша, поэтому отклонённая попытка не оплачивает Argon2id. Превышение даёт
`429 RATE_LIMITED` с `Retry-After`; доступ восстанавливается сам по истечении окна.

| Операция | Область счётчика | Порог | Окно | Переменные |
| --- | --- | --- | --- | --- |
| Вход | email + IP | 10 неудач | 15 минут | `RATE_LIMIT_SIGNIN_EMAIL_ADDRESS_ATTEMPTS` / `_WINDOW` |
| Вход | email | 30 неудач | 15 минут | `RATE_LIMIT_SIGNIN_EMAIL_ATTEMPTS` / `_WINDOW` |
| Вход | IP | 100 попыток | 15 минут | `RATE_LIMIT_SIGNIN_ADDRESS_ATTEMPTS` / `_WINDOW` |
| Регистрация | IP | 10 попыток | 1 час | `RATE_LIMIT_REGISTRATION_ADDRESS_ATTEMPTS` / `_WINDOW` |

## Контракт API и генерация

Источник HTTP-схем — `openapi/public.yaml`, `openapi/internal.yaml` и
`openapi/mailstub.yaml` (OpenAPI 3.0.3), с общими схемами в `openapi/components/`.
Подключены только health operations. Полные Go DTO/strict interfaces и public
TypeScript SDK описывают будущие функции; их наличие не означает работающий endpoint.
Для production health отдельно генерируется интерфейс из того же public-контракта.

Нужны Go 1.27.1 и Node 24.21.0 с npm. Из корня проекта:

```sh
npm --prefix frontend ci
npm --prefix tools/openapi ci
npm --prefix tools/openapi run generate
npm --prefix tools/openapi run check
```

Закреплены `oapi-codegen 2.8.0`, Go runtime `1.6.0`, validation middleware `1.2.0`
и `@hey-api/openapi-ts 0.99.0`. Генератор TypeScript работает из отдельного
`tools/openapi` с compiler API TypeScript 6.0.3; frontend продолжает использовать
TypeScript 7.0.2. Fetch-клиент копируется из закреплённого генератора 0.99.0:
первоначально указанный standalone `@hey-api/client-fetch 0.13.1` несовместим с
генерируемыми им типами и SSE-методами. Это осознанное изменение инструментария T03.
Транзитивный `js-yaml` поднят через `overrides` до 4.3.2: версия из генератора
содержала известную уязвимость, найденную `npm audit`.

Generated-код в `backend/internal/contracts/*/generated.go` и
`frontend/src/shared/api/generated/` хранится в Git и не редактируется вручную.
Промежуточные локальные bundles находятся в игнорируемом `.tools/contracts/`.
Удалённые `$ref` не разрешаются. Команда `check` повторяет генерацию и требует
побайтово одинаковый результат, затем запускает Go tests/vet, проверки setup и
frontend typecheck/build. Если generated-файлы устарели, команда обновит их и
завершится ошибкой, чтобы изменения можно было просмотреть и закоммитить.

Проверки охватывают inventory endpoints, закрытые unions, примеры payloads, точные
числа, UUID, календарные даты, пагинацию и HTTP-ошибки. Позиционные ограничения
`x-coordinate-order` и `x-mode-order`, которые OpenAPI 3.0 не выражает через tuple,
проверяются дополнительным валидатором. Planned requests проверяются изолированным
test router; транзакции аренды, платежи, HMAC cursors и доставка остаются будущими задачами.
После локальных проверок запустите отдельный Compose smoke из раздела выше:
он проверяет настоящую БД и не заменяется тестовой dependency check.

Это локальная версия для разработки. Конфигурация публичного развёртывания с HTTPS,
резервным копированием и реальными провайдерами в этот этап не входит.
Проверка Trivy от 12.09.2026 обнаружила известные CVE в базовом PostGIS и инструментах
сборки Go/Node. CI запускает `npm audit` и `govulncheck` как блокирующие проверки; разбор и
исправление системных пакетов контейнеров остаётся частью T18. Отсутствие находок этих сканеров не означает,
что всё окружение свободно от уязвимостей.
