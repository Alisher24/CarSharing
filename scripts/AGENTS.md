# Repository scripts

The repository's own tooling: setup, contract generation and checking, the pre-commit formatter, the
secret scan, and the checks that drive the assembled stack. Node 24 ESM (`.mjs`), run from the
repository root, with no dependencies beyond the root `package.json`. Repository-wide rules live in
the root `AGENTS.md`.

## Commands

- `node scripts/setup.mjs` (or `scripts/setup.ps1` / `scripts/setup.sh`, which run it in the pinned
  Node image so no host Node is needed) — create `.env` and `.secrets/`, and install the pre-commit
  gate of the working copy.
- `node scripts/smoke.mjs [origin]` — check the assembled stack after `docker compose up --build -d`.
- `npm test` from the repository root — every declaration the repository holds in step, which is the
  `scripts/*.test.mjs` files below, under one name CI can call. The individual files, each named after
  the fact it holds:
  - `node --test scripts/setup.test.mjs`
  - `node --test scripts/published-port.test.mjs` — the port the stack publishes, against the origins
    counted from it and the listener's own default.
  - `node --test scripts/idempotency-retention.test.mjs scripts/rate-limit-settings.test.mjs` — the
    idempotency window and rate limits shared by the contract, processes and acceptance checks.
  - `node --test scripts/shutdown-grace-period.test.mjs scripts/stream-proxy-bounds.test.mjs` — the
    independent timing bounds shared by processes and stack configuration.
  - `node --test scripts/money-declaration.test.mjs` — the locale and the unit an amount is written
    with, held between `frontend/src/shared/money.ts` and the oracle `scripts/acceptance/money.mjs`
    restates on purpose.
- `node --env-file=.env --test --test-concurrency=1 "scripts/acceptance/*.test.mjs"` — the HTTP
  acceptance suites with the same public settings Compose receives.
- `node scripts/generate-contracts.mjs` / `node scripts/check-contracts.mjs` — the entry points behind
  `npm --prefix tools/openapi run generate|check`.

## `contracts-manifest.mjs` is the one declaration of the contract set

`openapi/contracts.json` names every contract, which one the production boundary serves, and the
committed directory each of its projections is written into. `generate-contracts.mjs`,
`check-contracts.mjs` and `tools/openapi/generate.mjs` read it through this module rather than each
holding a list, so a new contract is an entry there and the two files it names. The Go tests read the
same manifest and hold their own map of generated packages to it.

## `service.mjs` is the one declaration of the stack

The smoke check, the HTTP suites and the browser suites under `tests/e2e` reach the same Compose
project, so its coordinates are stated once here and nowhere else: `SERVICE_ORIGIN` (from
`ACCEPTANCE_BASE`, defaulting to `http://127.0.0.1:8080`), `READINESS_PATH`, `POSTGRES_SERVICE`,
`POSTGRES_ROLE`, `POSTGRES_DATABASE`, `SESSION_COOKIE_NAME`, and the `compose`, `composeWith`, `sql` and
`waitForReady` helpers. `sql` runs `psql` inside the container as the exported role with
`ON_ERROR_STOP=1`, so a failing query stops the caller instead of returning partial output. A changed
port, database name or cookie name is one edit in this file; a second copy of one of these values in a
suite is the defect.

`composeWith` exists to pass environment into the Compose substitution, which is how a suite recreates
a service with another limit and proves the running service reads its configuration rather than a
constant. `waitForReady` is the one bounded wait for the API to answer, with the attempts, the delay
and the per-attempt timeout declared beside it: the smoke check and every suite that restarts the API
wait the same way.

## Setup owns the secrets and the gate

`setup.mjs` creates `.env` from the template and one random credential per capability.
`DATABASE_SECRETS`, `LEGACY_DATABASE_SECRETS` and `CAPABILITY_SECRETS` are the exported lists their
tests consume: adding a role or capability changes the generated and migrated sets together. Values are never
printed or logged; a second run keeps what exists. It also installs the gate of the working copy it is
given — `core.hooksPath` pointing at `.githooks` — because that setting belongs to the copy rather than
to the repository, and a clone that does not name it commits unformatted. A directory Git will not read
is refused before anything is written, so the copy is never left half set up. Source-reading and setup
tests run without the stack, and `npm test` runs all of them.

## The gates

- The shell gates are POSIX `sh`: `/bin/sh` is dash on Debian and Ubuntu, and dash refuses
  `set -o pipefail`, so the scripts state `set -eu` and keep no pipeline whose earlier command could
  fail unseen — the one pipeline they had, the checksum comparison in `install-gitleaks.sh`, reads a
  file instead. A pipeline that comes back needs its failure named rather than an option the shell
  does not have.
- `format-staged.mjs` is what `.githooks/pre-commit` runs: staged files (`--diff-filter=ACMR`, so a
  deletion is never reformatted) through the Prettier resolved from the repository's own
  `node_modules`, never a globally installed one, honouring `.prettierignore`.
- `check-secrets.sh [staged|history]` first refuses an ignored path that Git tracks — `.gitignore`
  protects nothing already in the index — then runs Gitleaks. `install-gitleaks.sh` fetches the pinned
  version into `.tools/` and verifies its checksum per platform.
- `check-contracts.mjs` regenerates through `generate-contracts.mjs` and diffs the digests of every
  committed directory `openapi/contracts.json` projects into: one fact, one check. Run it as
  `npm --prefix tools/openapi run check`. The Go checks it used to chain run in the backend job, the
  declaration tests in `npm test`, and the frontend build in the frontend job, each named where it
  lives rather than repeated here.

## The acceptance suites

`acceptance/<subject>.mjs` holds what a suite knows about one subject — paths, seed facts, reference
values, SQL, and the Compose commands that install or restore the scenario — and
`acceptance/<subject>.test.mjs` uses it. Several `.mjs` helpers are shared with the browser suites, so
they import nothing from `tests/`. Node's own runner with `node:assert/strict`, no test framework.

These suites run against a live stack and deliberately wait real time, restart services, stop the
database and withdraw a privilege to observe a refusal. They share one database, which is why CI runs
them with `--test-concurrency=1` and why the browser checks run after them: a concurrent suite would
read another suite's outage as its own failure.
