# OpenAPI contracts

The hand-written HTTP contract of the whole service, in OpenAPI 3.0.3. Every surface the processes
serve and every type the clients use is projected from these files; nothing in the Go or TypeScript
code decides a shape on its own. Repository-wide rules live in the root `AGENTS.md`.

## Commands

From the repository root, after `npm ci`, `npm --prefix frontend ci` and `npm --prefix tools/openapi ci`:

- `npm --prefix tools/openapi run generate` — rebuild every generated projection of these sources.
- `npm --prefix tools/openapi run check` — the gate: every committed projection is digested, the
  pipeline runs again, and a difference says the committed tree is stale. Run it before a pull request.
  The other checks a contract edit can break — the Go tests that read the documents, `npm test` for the
  declarations shared with the rest of the repository, and the frontend build — are steps of their own
  elsewhere in the pipeline.

An edit here is not finished until `generate` has run and the regenerated files are committed.

## The sources

- `public.yaml` — the browser API: health, accounts, catalog, both SSE streams, reservations, the ride
  commands and notifications. The large one; its size is the contract, not duplication.
- `internal.yaml` — the internal API: its own authentication, a larger body limit than the public one,
  and no route through the public proxy, which closes `/internal` with a 404.
- `mailstub.yaml` — the mail stub: its own delivery key, its own two capabilities, and the two
  listeners the `x-listener` extension splits its paths between.
- `components/common.yaml` (Timestamps, ExactInteger, EnergyDecimal, Cursor),
  `components/identifiers.yaml` (ResourceId, CommandId, RequestId) and `components/errors.yaml`
  (ApiError and the domain errors) hold what the three contracts share.

`contracts.json` is the one declaration of the contract set: every name, which contract the production
boundary serves, and the committed directory each projection is written into. `generate-contracts.mjs`,
`check-contracts.mjs`, `tools/openapi/generate.mjs` and `openapi`'s own Go tests all read it, so adding
a contract is a new `<name>.yaml` beside its `<name>.codegen.yaml` and one entry there.

`*.codegen.yaml` is **not** contract: each one is an oapi-codegen configuration naming the Go package
and output file for one projection.

A reference to a shared schema is a relative local one —
`$ref: ./components/common.yaml#/components/schemas/Timestamp` — and the bundler inlines them into a
single document. Anything with a scheme or a host is refused: the build must not depend on a document
this repository does not own.

A response body more than one operation answers with is declared once under that contract's
`components.responses` and referenced as `$ref: '#/components/responses/<Name>'`. Naming a response
after the codes it carries keeps the name and the body saying the same thing, and the contract tests
fail on a body two operations declare in full.

## `x-implementation-status` is the contract's honesty

Every operation states `implemented` or `planned`. A planned operation is not routed and answers
`404 RESOURCE_NOT_FOUND` exactly as an unknown path does, so writing it down costs nothing and claiming
it works is impossible. Three consequences to keep in step:

- `openapi/served.codegen.yaml` lists the operation ids the production boundary registers
  (`include-operation-ids`), and generates `servedapi` from the same public bundle, so a planned
  operation cannot be registered by accident.
- The contract tests compare the two directly: every operation the source marks `implemented` must be
  one the served document registers, and every operation it registers must be marked `implemented`.
  Deriving the served set from the status is what makes the status the only list.
- The router tests hold the routed surface to the same statuses: `TestImplementedRoutesAreServed`
  requires every `implemented` operation to answer, and `TestPlannedRoutesRemainAbsentFromProduction`
  requires every `planned` one to stay an unknown resource. Flipping the status without routing the
  operation fails the build.

## Extensions this repository checks

OpenAPI 3.0 cannot state everything these payloads mean, so the contract carries extensions and the Go
tests enforce them:

- `x-error-codes` — the error codes an operation answers with; the inventory test holds every operation
  to the ones its responses declare.
- `x-body-limit` — the byte cap of a request body, declared on every operation of a surface. The tests
  require every operation that accepts a body to declare it and every such operation of one surface to
  declare the same value, so the limit cannot drift between the operations that read a body. The proxy
  restates the public 65536 as its own `client_max_body_size`, because nginx cannot import this file;
  the acceptance suite keeps the two in step.
- `x-idempotency-retention-seconds` — how long an idempotent result remains repeatable. The repository
  check keeps every operation, the service retention and the browser repeat window equal.
- `x-event-schemas` — the events a stream carries; `x-listener` — which listener of the mail stub a path
  belongs to.
- `x-coordinate-order` (a GeoJSON position is longitude then latitude) and `x-mode-order` (invoice lines
  are ordered driving then paused) state positional rules OpenAPI 3.0 cannot express as tuples.
  `backend/internal/contracts/formats` validates them against examples and live payloads, together with
  a stricter `date-time` check that rejects calendar dates like February 30.
- `x-go-type` and `x-maximum-decimal` keep a wire string a Go string and state the largest exact
  integer it may carry.

## Editing rules

- A schema carries an `example`; the contract tests validate every example against its schema, so an
  example that no longer fits fails the build rather than the documentation.
- Timestamps are UTC RFC3339 with exactly six fractional digits, identifiers are UUIDv7 (`ResourceId`)
  or UUIDv4 (`CommandId`). Do not widen those formats here; a shape that needs another one is a new
  named schema in `components/`.
- `x-implementation-status` and the operation id are the two things a client sees as behaviour, so they
  change in the same commit as the code that serves them.

Generated output is never edited here or there: `backend/internal/contracts/*/generated.go` and
`frontend/src/shared/api/generated/` are projections, and `tools/openapi` owns how they are produced.
