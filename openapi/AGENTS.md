# OpenAPI contracts

The hand-written HTTP contract of the whole service, in OpenAPI 3.0.3. Every surface the processes
serve and every type the clients use is projected from these files; nothing in the Go or TypeScript
code decides a shape on its own. Repository-wide rules live in the root `AGENTS.md`.

## Commands

From the repository root, after `npm ci`, `npm --prefix frontend ci` and `npm --prefix tools/openapi ci`:

- `npm --prefix tools/openapi run generate` — rebuild every generated projection of these sources.
- `npm --prefix tools/openapi run check` — the gate: deterministic generation, then Go, setup and
  frontend checks. Run it before a pull request.

An edit here is not finished until `generate` has run and the regenerated files are committed.

## The sources

- `public.yaml` — the browser API: health, accounts, catalog, both SSE streams, reservations, the ride
  commands and notifications. The large one; its size is the contract, not duplication.
- `internal.yaml` — the internal API: its own authentication, a larger body limit than the public one,
  and no route through the public proxy, which closes `/internal` with a 404.
- `mailstub.yaml` — the mail stub, still planned.
- `components/common.yaml` (Timestamps, ExactInteger, EnergyDecimal, Cursor),
  `components/identifiers.yaml` (ResourceId, CommandId, RequestId) and `components/errors.yaml`
  (ApiError and the domain errors) hold what the three contracts share.

`*.codegen.yaml` is **not** contract: each one is an oapi-codegen configuration naming the Go package
and output file for one projection. Adding a contract means adding both files.

A reference to a shared schema is a relative local one —
`$ref: ./components/common.yaml#/components/schemas/Timestamp` — and the bundler inlines them into a
single document. Anything with a scheme or a host is refused: the build must not depend on a document
this repository does not own.

## `x-implementation-status` is the contract's honesty

Every operation states `implemented` or `planned`. A planned operation is not routed and answers
`404 RESOURCE_NOT_FOUND` exactly as an unknown path does, so writing it down costs nothing and claiming
it works is impossible. Two consequences to keep in step:

- `openapi/served.codegen.yaml` lists the operation ids the production boundary registers
  (`include-operation-ids`). It is the served set, and it is derived from the same public source so a
  planned operation cannot be registered by accident.
- The Go tests hold the two halves together: `TestImplementedRoutesAreServed` requires every
  `implemented` operation to answer, and `TestPlannedRoutesRemainAbsentFromProduction` requires every
  `planned` one to stay an unknown resource. Flipping the status without routing the operation fails
  the build.

## Extensions this repository checks

OpenAPI 3.0 cannot state everything these payloads mean, so the contract carries extensions and the Go
tests enforce them:

- `x-error-codes` — the error codes an operation answers with; the inventory test holds every operation
  to the ones its responses declare.
- `x-body-limit` — the byte cap of a request body. The proxy restates 65536 as its own
  `client_max_body_size`, because nginx cannot import this file; keep the two in step.
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
