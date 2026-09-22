# Browser acceptance

Playwright checks that drive a real Chromium against the assembled stack: what a person sees when a
change is delivered, when the stream breaks, when a reservation runs out, and what survives a reload
or a second independent browser. Repository-wide rules live in the root `AGENTS.md`; the stack's
coordinates and the HTTP helpers live in `scripts/`.

## Commands

From the repository root:

- `npm ci` once, then `npm run e2e:install` once — downloads the Chromium these checks drive.
- `docker compose up --build -d` — the stack they run against; the checks start no server of their own.
- `npm run e2e` — the whole suite through `tests/e2e/playwright.config.mjs`.
- `npm run e2e -- notifications.spec.mjs` — one suite while working on it.

The CI `integration` job runs the HTTP acceptance suites first and this suite last, so the browser
checks start from the demonstration in its prepared state.

## What the stand gives you

`playwright.config.mjs` reaches the application through `SERVICE_ORIGIN` from `scripts/service.mjs`, so
the port or origin is never restated here. One worker, no parallelism: the suites share one database
and one demonstration, and several of them restore the scenario or move a real reservation deadline
with SQL.

The 90s test timeout and 20s expect timeout are several times the 2s bound a delivery check measures,
so a failure names the frame or the state that never arrived. Keep that headroom: shrinking a timeout
turns a slow delivery into a misleading failure.

## Layout

- `*.spec.mjs` is a suite; `*.mjs` without `.spec` is a helper for the suites and is never collected.
- `person.mjs` is what a person does — register, find a free vehicle, book it, cancel, sign out — and
  the words the interface uses for those controls. Two suites drive the same interface, so a control
  they both press is described once, here.
- `scenario.mjs` is the prepared demonstration: the vehicles it holds, their identifiers read by the
  model the interface displays, and the change that makes one reservation expire. It declares nothing
  the acceptance helpers already do — `restoreScenario` comes from `scripts/acceptance/fleet.mjs` and
  the expiry is `moveDeadline`, so the transition stays the rentals module's rather than a second
  statement of it in SQL.
- Anything about HTTP, SSE frames or the database that a browser check also needs already exists under
  `scripts/acceptance/` (`events.mjs`, `reservations.mjs`) and `scripts/service.mjs`. Import it instead
  of writing a second probe.

## Measuring a signal, not a poll

The client repairs a missed change by reconciling on a timer, so a check that measures delivery would
pass on the poll alone. The address `/?reconcile=off` switches that timer off for the page a check
loads — it exists only for these checks and the shipped interval stays untouched. Use it where the
check is about a delivered signal, and leave it out where the check is about reading as the repair.

## Words the checks assert

The interface is Russian, and a suite asserts its words: `BOOK_ACTION`, `CANCEL_ACTION`, the notice
that updates are delayed, the action that marks a warning read. Those constants are the interface's own
copy, so a wording change in `frontend/src` is a change to the helper that names it — the alternative
is a suite that fails on a screen nobody broke.
