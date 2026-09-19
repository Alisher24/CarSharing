# Infrastructure

The images and the reverse proxy the assembled stack is built from. `compose.yaml` at the repository
root names the services and their wiring; this directory holds what they run. Repository-wide rules
live in the root `AGENTS.md`.

## Commands

From the repository root:

- `docker compose config --quiet` — validate a Compose edit.
- `docker compose up --build -d` — rebuild and start the stack; the CI `integration` job runs this exact command.
- `docker compose -f compose.yaml -f compose.dev.yaml up --build -d` — the React hot-reload profile.
- `docker compose logs --no-color --tail=200` — what CI prints when the integration job fails.

`infra/` is listed in `.prettierignore`: these files are reviewed as prose and never reformatted.

## The Dockerfiles

Both are built with the **repository root** as context (`build.context: .` in `compose.yaml`), so every
`COPY` source is stated from the root (`backend/go.mod`, `frontend/package.json`). A file an image needs
is added to its Dockerfile, not by widening the context.

- `backend.Dockerfile` compiles the eight runnable commands into `/out` in one stage and copies them
  into `/usr/local/bin/` of a `scratch` image running as `10001:10001`. Each service then picks one
  with `entrypoint:`. A new command under `backend/cmd/` must be added to the `go build` list here
  **and** given an entrypoint by the service that runs it, or that service starts into "not found".
  `cmd/contracts` is deliberately absent: it runs under `go run` while contracts are generated, never
  in a container.
- `frontend.Dockerfile` has five stages: `extractor` (the pinned `protomaps/go-pmtiles` image, taken as
  an image rather than built from source because its module graph takes minutes to fetch), `basemap`
  (the tools in `tools/basemap` and the declaration in the frontend — it cuts the archive, fetches the
  glyphs and the sprite, and checks what it produced), `dev` (what `compose.dev.yaml` runs with
  `frontend/src` bind-mounted), `build` (`npm run build` — typecheck, stylelint, unit tests, then
  Vite), and `runtime` (the Vite output, basemap included, served by `nginx-unprivileged`). The nginx
  files are copied by name, so a new one is added to the `COPY` list in the same change; the same is
  true of a document the unit tests read, which is why `openapi/public.yaml` and `infra/nginx.conf`
  are copied into `dev` as well.
- A worker a bundler has to emit is imported for its URL (`?worker&url`), never copied as an asset: a
  plain copy cannot import the chunk it shares with its library, fails to start, and the failure shows
  up as nothing at all rather than as an error. `basemapWorker.ts` says so where the choice is made.
- Base images carry a tag **and** a digest. Bumping either is a deliberate edit in this directory, never
  a side effect of another change.

## The proxy

`nginx.conf` is the only service published to the host; it listens on 8080.

- `/api/` is proxied to `api:8080` with `proxy_buffering off` and a 60s read timeout, because two of
  those responses are SSE streams that must arrive frame by frame. The resolver `127.0.0.11` keeps the
  upstream a per-request lookup, so a restarted API is found again without reloading nginx.
- `/health` and `/internal` are closed here with an explicit JSON 404, and 413/502/503/504 answer the
  API's error body shape — the one part of the contract a proxy cannot import. Those bodies and their
  headers live once in `error-json.conf`; a new one belongs there rather than inline in a `location`.
- `/basemap/` serves the archive, the glyphs and the sprite the map is drawn from, by the prefix the
  frontend's declaration names. It answers 404 for what is not there rather than the application, and
  it declares two types of its own — `.pbf` and `.pmtiles` — beside the ones the server already knows,
  because a `types` block replaces the whole map rather than adding to it. gzip is on for text and for
  glyphs; the archive is deliberately left out, since it is already compressed and compressing it
  would break the range requests it is read with.
- `security-headers.conf` carries the headers for both the static site and the JSON errors. Its CSP
  allows `'self'` and `data:` images only, so a change that needs another origin is a change to this
  file. MapLibre's worker needs no directive of its own: it is a file of this installation rather than
  a Blob URL, so `'self'` covers it, and that was measured against the assembled stack rather than
  assumed.

## The database

`postgres/20-app-role.sql` runs once, when the volume is first initialized: it creates the
`carsharing_app` role without DDL rights, reads its password from the mounted secret, and revokes
`CREATE ON SCHEMA public`. Password files are read once, so an existing volume keeps its password.
Schema changes belong to the migrations the `migrate` service applies — this script runs before any
migration exists and cannot carry them.

## Health checks

A service's health check is the contract its dependents wait on, so it moves with the code it probes:

- `api` runs the built `/usr/local/bin/healthcheck`, which takes the port from the same setting the API
  listens on and the path from `httpapi.ReadyPath`.
- `frontend` fetches `/api/v1/health/live`; `postgres` runs `pg_isready` as `carsharing_migrator`.

Renaming a route, a binary or a port means editing the check in `compose.yaml` in the same change.
