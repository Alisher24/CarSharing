# Backend

One Go module: `github.com/Alisher24/CarSharing/backend`, Go 1.27.1. `cmd/` holds the processes,
`internal/` holds the rules, `db/migrations/` holds the schema. Repository-wide rules — one concern per
file, naming, comments, the review gate — live in the root `AGENTS.md`.

## Commands

From `backend/`:

- `gofmt -l .` — what CI checks; `gofmt -w <files>` to fix.
- `go build ./... && go vet ./... && go test ./...` — the whole gate for a change here.
- `go tool oapi-codegen` — the generator is a `tool` directive in `go.mod`, not a global install.

The tests are hermetic: no database, no environment beyond what a test sets itself, so `go test ./...`
runs anywhere. Everything that needs the assembled stack — migrations against PostGIS, the seed, the
outbox, the streams — lives in `scripts/smoke.mjs` and `scripts/acceptance/`, and CI runs it in the
`integration` job rather than here. Adding a check that needs a live service to this module is the
wrong home for it.

## The commands

Nine commands, each one process:

- `api` — serves the HTTP API and holds the database connection the streams read. It serves two
  surfaces from one listener: the public application, and the internal operations under `/internal`,
  which the external proxy answers as an unknown resource.
- `worker` — outbox delivery, the reservation deadline sweep and retention, in a process of its own so
  that either it or the API can be restarted alone, over the same modules.
- `simulator` — the clock of the modelled fleet: it calls the internal tick operation once a second
  and changes nothing itself. `-once [-tick-id]` advances the fleet exactly once. It belongs to the
  demonstration profile, because starting the demonstration is a deliberate act.
- `democontrol` — one set-to-value command of the demonstration, carried to the internal API under the
  token of its own capability.
- `migrate` — goose `up` or `status` under a lock and a two-minute deadline; it refuses any other word.
- `seed` — installs the demonstration, and refuses to run outside `APP_ENV=demo`.
- `demoscenario` — puts the prepared scenario back, and refuses to run outside `APP_ENV=demo`.
- `healthcheck` — the container's readiness probe; it takes its port from
  `config.HTTPAddrFromEnvironment` and its path from `httpapi.ReadyPath`, so it follows the listener
  rather than a copy of it.
- `contracts` — bundles the OpenAPI sources for the generators; build-time only, deliberately not built
  into the image.

Every `main` sets up JSON logging, calls `run() error` and exits 1 on the error; the assembly lives in
`run` and in the functions it calls.

## Wiring

`cmd/api/main.go` is the composition root: `run` loads the configuration, opens the pool, and
`assemble(cfg, pool, hub)` builds the public `httpapi.Dependencies` and the internal
`httpapi.InternalDependencies` field by field and serves both through `httpapi.NewSurfaceRouter`.
`NewHandler` and `NewInternalHandler` then derive their handler groups from those and fail at
construction when something is missing — `router_dependencies_test.go` removes each dependency in turn
to prove it. A new feature is wired in `assemble` and nowhere else.

`internal/platform/config` is the one place the environment is read: a `Config` struct and a single
`Load`. The rate limits show the expected shape for a new setting — `rateLimitSetting` pairs the
identifier with the environment name it is read from, its default, and where the value lands in
`ratelimit.Limits` — so a limit cannot be added with its configuration read nowhere. Secrets are read
from files named by `*_FILE` variables, never from the environment itself.

`cfg.Environment` is `APP_ENV`, defaulting to production. The process that must refuse owns that
decision: `seed`, `demoscenario` and the scenario restore each reject a non-demo environment
themselves. The API uses it only to decide whether to run the demonstration telemetry.

## The HTTP layer

- `boundary.go` validates every request against the served specification before a handler sees it, so a
  handler only ever receives a request the contract accepted and only has to answer.
- The error contract is three files: `errorcontract.go` names the codes from the generated constants,
  `errorstatus.go` maps each code to the one status the specification declares for it, and
  `errorrespond.go` writes the envelope. A new transport failure is added to those three; a new domain
  failure is a generated response type, not a code here.
- One file per subject — `reservations.go`, `rides.go`, `notifications.go`, `catalog.go` — each with a
  `newXHandlers` constructor and a struct holding what it was given. Registration is one line per group
  in `NewHandler`; nothing else has to be edited to add one.
- Command operations declare their answers as a table in `operations.go` (`status → shape`) rather than
  as a growing conditional, so a replayed answer is spelled as the operation that was asked.
- An operation is served when its id is in `openapi/served.codegen.yaml`; `dropUnimplementedPaths`
  removes the rest from the embedded specification, which is how a `planned` operation answers as an
  unknown resource.

`internal/contracts/*/generated.go` is generated and never edited: change the contract and regenerate
it (`npm --prefix tools/openapi run generate`), never the Go file.

## Modules and the database

- A module owns its rules and its SQL: the `store.go` of `fleet`, `events`, `notifications`, `rentals`
  or `outbox` holds the statements for its own tables. A handler moves a request between the contract
  and the module and writes no SQL itself.
- What two modules both need is joined at the composition root, not by one module importing the other's
  records. `notificationOperations` in `cmd/api/main.go` is the example: the collection belongs to
  `rentals`, the read of one notification to `notifications`, and neither learns about the other.
- A value written in the same transaction as the change it describes stays consistent by construction:
  a signal, an outbox task and an idempotent result are all inserted through the request's transaction,
  so a rolled-back change leaves none of them behind.
- Nothing is stored twice if it can be derived on read: the state of a vehicle comes from the live
  rental on every read. A second stored copy is the defect, not an optimisation.
- `db/migrations/0000N_name.sql` with `embed.go`; goose applies them, one statement set per numbered
  file, and the migrator role owns the schema. The API connects as `carsharing_app`, which has no DDL —
  a migration that needs a privilege the app role lacks is a migration, not a reason to widen the role.
