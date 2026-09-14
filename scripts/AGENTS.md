# Repository scripts

The repository's own tooling: setup, contract generation and checking, the pre-commit formatter, the
secret scan, and the checks that drive the assembled stack. Node 24 ESM (`.mjs`), run from the
repository root, with no dependencies beyond the root `package.json`. Repository-wide rules live in
the root `AGENTS.md`.

## Commands

- `node scripts/setup.mjs` (or `scripts/setup.ps1` / `scripts/setup.sh`, which run it in the pinned
  Node image so no host Node is needed) — create `.env` and `.secrets/`.
- `node scripts/smoke.mjs [origin]` — check the assembled stack after `docker compose up --build -d`.
- `node --test scripts/setup.test.mjs`
- `node --test --test-concurrency=1 "scripts/acceptance/*.test.mjs"` — the HTTP acceptance suites.
- `node scripts/generate-contracts.mjs` / `node scripts/check-contracts.mjs` — the entry points behind
  `npm --prefix tools/openapi run generate|check`.

## `service.mjs` is the one declaration of the stack

The smoke check, the HTTP suites and the browser suites under `tests/e2e` reach the same Compose
project, so its coordinates are stated once here and nowhere else: `SERVICE_ORIGIN` (from
`ACCEPTANCE_BASE`, defaulting to `http://127.0.0.1:8080`), `POSTGRES_DATABASE`, `SESSION_COOKIE_NAME`,
and the `compose`, `composeWith` and `sql` helpers. `sql` runs `psql` inside the container as
`carsharing_migrator` with `ON_ERROR_STOP=1`, so a failing query stops the caller instead of returning
partial output. A changed port, database name or cookie name is one edit in this file; a second copy of
one of these values in a suite is the defect.

`composeWith` exists to pass environment into the Compose substitution, which is how a suite recreates
a service with another limit and proves the running service reads its configuration rather than a
constant.

## Setup owns the secrets

`setup.mjs` creates `.env` from the template and one random credential per capability. `CAPABILITY_SECRETS`
is the single list: adding a capability means adding its secret name there, and both the file creation
and the validation that a legacy installation is not silently repaired follow from it. Values are never
printed or logged; a second run keeps what exists. `setup.test.mjs` is the only test that runs without
the stack, and it is chained into `check-contracts.mjs`.

## The gates

- `format-staged.mjs` is what `.githooks/pre-commit` runs: staged files (`--diff-filter=ACMR`, so a
  deletion is never reformatted) through the Prettier resolved from the repository's own
  `node_modules`, never a globally installed one, honouring `.prettierignore`.
- `check-secrets.sh [staged|history]` first refuses an ignored path that Git tracks — `.gitignore`
  protects nothing already in the index — then runs Gitleaks. `install-gitleaks.sh` fetches the pinned
  version into `.tools/` and verifies its checksum per platform.
- `check-contracts.mjs` regenerates, diffs the digests of every committed generated directory, and then
  chains Go tests and vet, the setup tests and the frontend build. It requires the root `npm ci`,
  because it invokes npm through `npm_execpath`; run it as `npm --prefix tools/openapi run check`.

## The acceptance suites

`acceptance/<subject>.mjs` holds what a suite knows about one subject — paths, seed facts, reference
values, SQL, and the Compose commands that install or restore the scenario — and
`acceptance/<subject>.test.mjs` uses it. Several `.mjs` helpers are shared with the browser suites, so
they import nothing from `tests/`. Node's own runner with `node:assert/strict`, no test framework.

These suites run against a live stack and deliberately wait real time, restart services, stop the
database and withdraw a privilege to observe a refusal. They share one database, which is why CI runs
them with `--test-concurrency=1` and why the browser checks run after them: a concurrent suite would
read another suite's outage as its own failure.
