# Contract tooling

The pinned generators that project `openapi/` into Go and TypeScript, and the npm scripts that expose
them. This directory declares the versions; the code it produces belongs to the packages it lands in.
Repository-wide rules live in the root `AGENTS.md`.

## Commands

- `npm --prefix tools/openapi ci` — install the generators.
- `npm --prefix tools/openapi run generate` — regenerate every projection.
- `npm --prefix tools/openapi run check` — the gate CI runs.

Both scripts call `scripts/`; they are the documented spelling of the commands, not a second
implementation. `check` reads `npm_execpath`, so it needs the root `npm ci` as well, and it builds the
frontend, so `npm --prefix frontend ci` must have run too.

## What generation does

`scripts/generate-contracts.mjs` is the whole pipeline, in order:

1. For each of `public`, `internal` and `mailstub`: `go run ./cmd/contracts openapi/<name>.yaml
   .tools/contracts/<name>.json` bundles the local references into one self-contained document, then
   `go tool oapi-codegen --config openapi/<name>.codegen.yaml` writes
   `backend/internal/contracts/<name>api/generated.go` — models, a chi server and its strict interface.
2. The served interface is generated from that same `public.json` with `openapi/served.codegen.yaml`,
   which registers only the operation ids the production boundary serves.
3. `tools/openapi/generate.mjs` runs `@hey-api/openapi-ts` over `public.json` into
   `frontend/src/shared/api/generated`: types, SDK and the fetch client, including the SSE runtime the
   streams use. It changes directory into `frontend/` because the generator resolves the tsconfig the
   output is compiled with from the package it writes into.

Intermediate bundles live in the git-ignored `.tools/contracts/`. Generation is deterministic, which is
what makes the check below possible.

## What the check does

`scripts/check-contracts.mjs` digests every file of the five committed generated directories, regenerates
them all, and compares — a difference means the committed projection is stale. It then runs Go tests and
vet, the setup tests and the frontend build. It stops there: regenerating leaves the new files on disk
for review and commit rather than reverting them, so a failure that says "stale" is an instruction to
look at the diff.

## Pins

- `@hey-api/openapi-ts` 0.99.0 and `typescript` 6.0.3 (the generator's own compiler; the frontend stays
  on TypeScript 7.0.2). The fetch client is generated from the pin rather than installed: the standalone
  `@hey-api/client-fetch` release is not compatible with the types and SSE methods this generator emits.
- `js-yaml` is raised to 4.3.2 through `overrides` because the version the generator depends on carries a
  known vulnerability.
- The Go side is pinned in `backend/go.mod`: `oapi-codegen` 2.8.0 as a `tool` directive, with runtime
  1.6.0 and the validation middleware 1.2.0.

A version bump here is a deliberate edit followed by `generate`, so the regenerated diff is part of the
change — never a silent dependency update.

## Rules

- Generated files are committed and never hand-edited; they are exempt from formatting and are
  reformatted only by regenerating them.
- Which local schemas become which Go names, and which operation ids the production boundary registers,
  are decided in `openapi/*.codegen.yaml`, not in this directory.
- A new contract needs an entry in `CONTRACT_NAMES` in `scripts/generate-contracts.mjs`, its
  `openapi/<name>.codegen.yaml`, and — if a browser client consumes it — a second `createClient` call in
  `generate.mjs`. Its output directory is then added to `GENERATED_DIRECTORIES` in
  `scripts/check-contracts.mjs`, or the check would not notice it going stale.
