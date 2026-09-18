# Frontend

Vite 8, React 19.3, TypeScript 7.0.2, no framework beyond that. `src/app` is the shell,
`src/features/<feature>` owns one area of behaviour each, `src/shared` holds what features share.
Repository-wide rules — one concern per file, naming, no ternaries in JSX, a render records nothing —
live in the root `AGENTS.md`.

## Commands

From `frontend/`:

- `npm run dev` — Vite on port 5173 with a strict port, proxying `/api` to `http://api:8080`. That name
  is the Compose service, so a dev server started on the host cannot resolve it: the loop is
  `docker compose -f compose.yaml -f compose.dev.yaml up --build -d`, which runs this inside the
  container on 8080 with `src` bind-mounted.
- `npm run typecheck` — two projects, and both matter: `tsconfig.json` covers `src` without the test
  files, `tsconfig.test.json` covers `src/**/*.test.ts` with Node types. A test file is checked by the
  second one only.
- `npm run lint:css`, `npm test`, `npm run build` — the build runs typecheck, stylelint, the tests and
  then Vite, so it is the one command that catches everything.
- Prettier comes from the repository root: `npm run format:check` there, `npm run format` to fix.

CI (`.github/workflows/repository-checks.yml`, `TypeScript checks`) runs the root install, then
`format:check`, `lint:css`, `typecheck`, `test`, `build` and `npm audit --audit-level=low`.

## How a feature is laid out

A feature directory holds four kinds of file, and the split is the convention:

- pure logic — `filters.ts`, `countdown.ts`, `changes.ts`, `version.ts` — with a sibling `*.test.ts`;
- the Russian wording whose phrases are shared or asserted elsewhere — `fleetCopy.ts`,
  `reservationCopy.ts`, `rideCopy.ts`, `refusalText.ts`. A phrase one view shows once may sit inline in
  that view; a phrase two views share, or a suite asserts, belongs in one of these modules;
- hooks — `use*.ts` — which own effects, state and the streams;
- components — `*.tsx` — which receive what they show as props and compute nothing that outlives them.

Exactly one kind of file is unit-tested: 26 `*.test.ts` files, no `*.test.tsx`, and neither jsdom nor
a testing library is installed. Components and hooks are covered by the browser suite in `tests/e2e/`,
so a decision worth a test belongs in a pure module. A test opens with `node:test`'s `describe`/`test`
and `node:assert/strict`, and takes time as an argument (`countdownAt(held, new Date(...))`) rather than
mocking a clock — nothing in the suite installs fake timers.

## Talking to the API

- `src/shared/api/generated/` is generated from the OpenAPI contract and is never edited by hand.
  Regenerate it from the repository root with `npm --prefix tools/openapi run generate`; the tree is
  committed and digest-checked by `scripts/check-contracts.mjs`. The hand-written seam above it is one
  wrapper per operation: `catalog.ts`, `current.ts`, `session.ts`, `notifications.ts`, `rides.ts`,
  `invoices.ts`, `health.ts`. A component never calls a generated SDK function itself, and these wrappers re-export
  the generated types the rest of the application uses.
- A wrapper may add the check the wire format does not carry: `health.ts` refuses an answer that is not
  `ok` or whose `server_time` cannot be parsed, and asserts the declared time zone before it reaches
  `Intl`, so an unusable zone surfaces at the read rather than in the middle of a render.
- `Resource<T>` (`shared/api/Resource.ts`) is how server state reaches the screen: `loading`, `ready`,
  `stale`, `failed`. A failed reload keeps the last successful value and marks it stale; only the first
  failure, with nothing to show, is a failure. `useResource` loads one resource, abandons the request
  in flight when a newer one starts, and hands back `{ resource, retry }`.
- Reads are anonymous (`credentials: 'omit'`) for the public catalog and `same-origin` for anything of
  the account's. Commands go through `commands.ts`: `commandHeaders` adds `Origin`, `X-CSRF-Token` and
  `Idempotency-Key`, and `answerOf` keeps the four outcomes apart. A transport failure is `unknown`,
  never `refused` — telling a person their vehicle is free when the command may have succeeded is the
  mistake that distinction exists to prevent.
- An interval on screen is never a count of ticks. `countdown.ts` derives the time left from the
  deadline the server stored, the moment its answer was computed at and the local moment that answer
  arrived (`ServerClock`), so a tab suspended for an hour comes back to the truth rather than to where
  it stopped. Use `serverMomentAt` rather than `Date.now()` for anything the service decides.

## Signals and reconciliation

`src/features/events/` holds the whole of it: `stream.ts` reads the SSE frames, `coordinator.ts`
decides which documents one signal invalidates, `useEvents`/`usePrivateEvents` connect the two streams,
and `reconciliation.ts` re-reads every loaded resource every `RECONCILE_MILLISECONDS` to repair a
signal that was missed. The address parameter `?reconcile=off` switches that off for one page load; it
exists for the browser checks in `tests/e2e/` and changes nothing the application ships.

The stream is read with `fetch` and a body reader on purpose, not with the generated SSE client: that
client sends an event identifier back on every reconnect, and this contract declares no identifier and
no replay, so it would ask the server for something it does not have. `stream.ts` says so where the
choice is made; replacing it with the client is a regression, not a simplification.

## Wording, locale and money

- The interface is Russian. A phrase the browser checks assert, or that two views show, is a named
  constant in the copy module of the feature that owns it, so renaming one is a change to
  `tests/e2e/person.mjs` or the suite that names it in the same commit.
- `shared/locale.ts` declares `INTERFACE_LOCALE` and `SERVICE_TIME_ZONE` once for every view. The time
  zone is the backend's value restated because a browser cannot import it: a moment of the service day
  is written in `Asia/Bishkek` through `bishkekMoment`, never in the browser's zone.
- Prices cross the wire in tyiyn and are written as som by `features/fleet/money.ts`. Money and versions
  stay exact: an amount is divided as a `BigInt` and a version is compared as a decimal string, never
  through `Number`. A price is written `12,34 сома` — that unit, not `KGS` or `сом`.

## Styles

The stylesheets are global: `main.tsx` imports `src/app/styles.css` (the shell and the `:root` tokens),
`src/app/fleet.css` (the map screen), `src/app/cabinet.css` (the cabinet) and `src/app/account.css`
(the entry window and the form inside it), and no component carries a stylesheet of its own — the only
other CSS import in `src` is Leaflet's, inside `FleetMap.tsx`. Class names are kebab-case and
hyphen-chained, and state is an attribute selector rather than a modifier class:
`.fleet-row-status[data-status='available']`, `.filter-chip[aria-pressed='true']`.

Stylelint (`stylelint-config-standard` plus the repository's rules) requires one declaration per
single-line block, so a rule block is never a one-liner, a blank line before each multi-line rule and
at-rule, `@media (width <= 850px)` rather than `max-width`, and a quoted `url()`. Prettier owns the
alignment; do not hand-format what it would rewrite.
