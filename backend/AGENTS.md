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

Ten commands, each one process:

- `api` — serves the HTTP API and holds the database connection the streams read. It serves two
  surfaces from one listener: the public application, and the internal operations under `/internal`,
  which the external proxy answers as an unknown resource.
- `worker` — outbox delivery, the reservation deadline sweep and retention, in a process of its own so
  that either it or the API can be restarted alone, over the same modules.
- `mailstub` — the mail stub: it accepts one letter per delivery key into a schema of its own, arms the
  loss of an answer when a demonstration asks for it, and serves the read-only inbox. Two listeners in
  one process: the internal one, which nothing publishes, and the inbox, which the container publishes
  on the loopback address.
- `simulator` — the clock of the modelled fleet: it calls the internal tick operation once a second
  and changes nothing itself. `-once [-tick-id]` advances the fleet exactly once. It belongs to the
  demonstration profile, because starting the demonstration is a deliberate act.
- `democontrol` — one set-to-value command of the demonstration, carried to the internal API under the
  token of its own capability. `internal/demoaction` names the changes once — the identifiers the
  contract publishes, which the rentals module, the mail stub, the terminal and the internal surface
  all name — and `internal/democontrol` declares what each change carries: the fields of the request,
  the terminal's flags and their hints. The usage line, the flags, the request the terminal writes and
  the surface's reading of that request all follow from that declaration, and a test holds the surface's
  reading to the kinds the module knows rather than a second list doing it.
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

`cmd/api/` is the composition root: `run` in `main.go` loads the configuration and opens the pool;
`assemble(cfg, pool, hub)` in `assembly.go` builds the public `httpapi.Dependencies` and the internal
`httpapi.InternalDependencies` field by field and serves both through `httpapi.NewSurfaceRouter`.
`NewHandler` and `NewInternalHandler` then derive their handler groups from those and fail at
construction when something is missing — `router_dependencies_test.go` removes each dependency in turn
to prove it. A new feature is wired in `assemble` and nowhere else.

`internal/platform/config` is the one place the environment is read. There is one loader per process
shape, and all six live in that package: `Load`, `MailstubServerFromEnvironment`, `SimulatorClient`,
`DemoControlClient`, `MailstubClient` and `MailstubDemoClient`. The rate limits show the expected
shape for a new setting — `rateLimitSetting` pairs the
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
  as a growing conditional. Public and internal commands share the decoder and header metadata in
  `answershape.go`; each response constructor declares the replay and retry headers its contract allows.
  The refusal dictionaries in `refusalcontract.go` select each surface's vocabulary.
- An operation is served when its id is in `openapi/served.codegen.yaml`; `dropUnimplementedPaths`
  removes the rest from the embedded specification, which is how a `planned` operation answers as an
  unknown resource.

`internal/contracts/*/generated.go` is generated and never edited: change the contract and regenerate
it (`npm --prefix tools/openapi run generate`), never the Go file.

## Modules and the database

- A module owns its rules and its SQL. Rental row statements and the shared rental projection live in
  `rentals/store.go`; focused owner operations for another aggregate live beside that aggregate, such
  as fleet installation and telemetry confirmation. A handler or a demonstration moves values between
  owners and writes no SQL for their tables.
- The commands that act on one of the caller's rentals — starting, pausing, continuing and giving a
  reservation back — are one `rentals.RentalCommand` that names its action, dispatched by one table in
  `rentals/rentalcommand.go`. A new action is a row there rather than a method per verb, and the action
  spellings are the operation names the HTTP layer uses.
- The reservation deadline pass is one sweep over the reservations a statement finds due. It supplies
  the locking and the per-reservation transaction; the expiry of a reservation past its deadline and
  the warning of one inside its last minute are the two bodies it runs, each in its own file.
- A rental that moves and a rental that ends announce the same two changes — the rental and the vehicle
  it holds — through one function. The announcement of an ending adds what only an ending has, which is
  the invoice and the work it owes.
- An operation used inside another module's transaction accepts the concrete `pgx.Tx`. The caller keeps
  ownership of commit and rollback, while the callee keeps ownership of its table and statement. Rental
  transactions acquire their participants through the one `users`, `vehicles`, `rentals` lock plan.
- What two modules both need is joined at the composition root, not by one module importing the other's
  records. `notificationOperations` in `cmd/api/notificationoperations.go` supplies notification-owned callbacks to the
  rental transaction and adapts the collection result; neither module imports the other's record types.
- The fleet owner alone changes `vehicles.version`. Callers describe a `fleet.VehicleChange`; the owner
  increments the stored value and returns the resulting version, so restoration cannot put it back.
- A value written in the same transaction as the change it describes stays consistent by construction:
  a signal, an outbox task and an idempotent result are all inserted through the request's transaction,
  so a rolled-back change leaves none of them behind.
- Nothing is stored twice if it can be derived on read: the state of a vehicle comes from the live
  rental on every read. A second stored copy is the defect, not an optimisation.
- `db/migrations/0000N_name.sql` with `embed.go`; goose applies them, one statement set per numbered
  file, and the migrator role owns the schema. The API connects as `carsharing_app`, which has no DDL —
  a migration that needs a privilege the app role lacks is a migration, not a reason to widen the role.

## Shared platform mechanisms

- `platform/database.Settings` is the connection shape filled by the loader. Database code does not
  import configuration. `Moment` reads the authoritative clock; `ReadOne` verifies row cardinality for
  reads and transition results; `InitialVersion` is the version a stored row carries when it is
  created, so no module states that number again. Idempotency's `ClaimKey` and `Complete` require the
  transaction directly.
- `platform/cursor` owns the page size, optional `Position`, descending keyset SQL and page slicing.
  Collection owners provide their SQL column names and the record's sort position. The cursor payload
  has one declaration; `compatibility_test.go` preserves the previously issued wire format.
- Each retention owner exposes `DeleteExpired`. `cmd/worker/retention.go` registers one `retention.Sweep`
  per owner; `platform/retention` supplies the common interval and batch size. `periodic.RunAll` joins
  all worker tasks before the process closes the pool.
- `platform/httpserver` supplies listener bounds and shutdown for API and mailstub. `httpheader` owns
  their shared protocol names. Internal clients receive their HTTP transport from command assembly.
- Stream timing is passed by API assembly and validated when handlers are built. Stream and internal
  client timeout tests use `testing/synctest`, so their deadlines advance without wall-clock waits.

## The modelled fleet's routes

`internal/simulation/route.go` is the only place the geometry of a vehicle's journey lives. A route is
a ring of four sides along named streets of central Bishkek, declared as the crossings it turns at.

- A `crossing` carries both street names and the point, because a coordinate on an avenue is only a
  corner if the street it names reaches that longitude. `ring` names each side from the street the two
  crossings have in common, so a ring cannot be named after a street it never drove.
- The crossings are taken from OpenStreetMap rather than placed by hand, so a vehicle is seen driving
  along a road instead of across the blocks. The corpus is deliberately small: `x` and `y` are not
  street names, and a made-up crossing shows itself as a car in a courtyard.
- The demo fleet no longer declares where its vehicles stand. `demo.Fleet()` takes the first vertex of
  the route as the place, so the same twenty-five coordinates are stated once.
- Every ring is named by a declared `simulation.RouteID` constant, and the demonstration fleet names
  its circuits by those constants, so a misspelt circuit is a build error rather than a vehicle
  standing nowhere. `Routes()` answers a copy of its own — corpus, streets and points — so a package
  that drives the fleet can neither add a circuit nor rewrite the geometry of one.
- `route_test.go` holds the declaration to what it claims: every ring is closed, stays inside the
  demonstration service area (except the one scenario ring, which exists to leave it), has no side
  shorter than `ShortestSideMetres`, is named by the streets it follows, and no two rings are the same
  circuit.
- The vehicle still follows an announced ring rather than searching a road graph, so one moved by
  `set-position` drives back to its route in a straight line. That is a named limitation, not a defect
  to fix here.
