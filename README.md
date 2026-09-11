# CarSharing

Сервис каршеринга для Бишкека: Go backend, React frontend, расчёты в кыргызских сомах (KGS).
Планируются электрические, бензиновые, дизельные, гибридные и газовые автомобили.

Репозиторий: [Alisher24/CarSharing](https://github.com/Alisher24/CarSharing) (public).

Работает начальный каркас: React получает данные от Go API, API проверяет PostgreSQL,
схему и PostGIS. Вход, карта, аренды, расчёты, worker, симулятор и почта пока не реализованы.

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
токен в `.secrets/`. Значения не выводятся; повторный запуск сохраняет их и настройки.
Внутренний токен зарезервирован для будущего симулятора и пока не подключён к сервисам.
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

- `GET /api/health` через frontend возвращает готовность, город, валюту, часовой пояс и UTC-время сервера.
- Внутри API: `/health/live` проверяет процесс; `/health/ready` — доступ к схеме и PostGIS.
- Логи: `docker compose logs --tail=100 api migrate postgres frontend`.

После запуска, при наличии Node 24.21 на хосте:

```sh
node --test scripts/setup.test.mjs
node scripts/smoke.mjs
```

Smoke проверяет реальные API/PostGIS, права роли, повтор миграций/seed и сохранность
данных. Он кратко останавливает БД и запускает её снова, проверяя ответ 503 и восстановление
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
cd frontend
npm ci
npm run build
npm audit
```

Версии Go/Node и образы закреплены в Dockerfiles/Compose, зависимости — в
`go.mod`/`go.sum` и `package-lock.json`. Использованы React 19.3, TypeScript 7.0,
Vite 8.3, chi 5.3, pgx 5.11 и goose 3.28. [Vite поддерживает Node 24](https://vite.dev/guide/),
[PostGIS использует путь volume PostgreSQL 18](https://github.com/postgis/docker-postgis).
Полный CI приложения и OpenAPI будут добавлены отдельно; текущий обязательный
Repository checks проверяет гигиену репозитория и секреты.

Это локальная версия для разработки. Конфигурация публичного развёртывания с HTTPS,
резервным копированием и реальными провайдерами в этот этап не входит.
Проверка Trivy от 12.09.2026 обнаружила известные CVE в базовом PostGIS и инструментах
сборки Go/Node; обновление системных пакетов и анализ применимости ещё нужны.
Отсутствие находок `npm audit`/`govulncheck` в коде приложения не означает, что все образы
свободны от уязвимостей.
