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

## What lives where

`src/app` is the shell: the addresses, the header, the route table, the one application hook and the
global stylesheets. It names every feature it shows — the fleet is handed the map as a value rather
than importing it — and it is the only place that reaches into a feature from outside one.

`src/features/<feature>` owns one area of behaviour: `account` the session, `fleet` the public
catalog and the screens drawn from it, `map` the basemap and everything painted on it, `reservation`
the rental in force and its commands, `notifications` the collection addressed to the account,
`cabinet` the account's own history, `connection` the reader's link to the service, and `live` the two
change streams.

`src/shared` is what more than one of them uses, and a feature imports nothing else:
`app/featureBoundaries.test.ts` walks `src` and fails on an import of one feature from another, and it
is in the shell because the shell is the one place allowed to know the features. The shared tree is:

- `api` — the seam above the generated client (`request.ts` holds the one `sameOriginRequest` and
  `originHeader`, `commands.ts` the command headers and the four outcomes), the wrapper per operation,
  and `generated/`;
- `read` — how server state reaches the screen: `Resource.ts`, `presence.ts` (`Found`), `coordinator.ts`,
  `changes.ts`, `version.ts`, `observedResource.ts`, `documentAnswers.ts`, `answerStore.ts`,
  `useReadCycle.ts` (the one cycle: a change, a handshake, the interval), `request.ts` (what one window
  of changes asks for), `useResource.ts`, `useRequestedReads.ts`, the SSE reader (`frames.ts`,
  `stream.ts`, `streamEvents.ts`, `useEventStream.ts`) and `reconciliation.ts`;
- `command` — the command lifecycle: `unfinishedCommand.ts` (the record a reload repeats),
  `commandSender.ts`, `commandPhase.ts` (what is reported), `usePayment.ts`, `useReadAfterPayment.ts`;
- `ride` — the ride a rental is having: `pace.ts`, `countdown.ts`, `serverClock.ts`, `fares.ts` (what a
  rental costs, and the one mark for a value the answer does not carry), `spell.ts` (its wording),
  `paymentCopy.ts`, `TariffRates.tsx` and the two hooks;
- `vehicle` — how one vehicle is named and narrowed: `spell.ts` and `filters.ts`;
- `account` — what one session states: `session.ts`, `identity.ts`, `notifications.ts`, `currentWarning.ts`;
- the leaves: `copy.ts` (a phrase two features show), `refusal.ts`, `money.ts`, `time.ts`, `locale.ts`,
  and `components/` for the absence notice a view of any resource shows.

## How a feature is laid out

A feature directory holds four kinds of file, and the split is the convention:

- pure logic — `filters.ts`, `countdown.ts`, `changes.ts`, `version.ts` — with a sibling `*.test.ts`;
- the Russian wording whose phrases are shared or asserted elsewhere — `reservationCopy.ts`,
  `cabinetCopy.ts`, `accountCopy.ts`. A phrase one view shows once may sit inline in that view; a
  phrase two views of the same feature share, or a suite asserts, belongs in one of these modules, and
  a phrase two *features* show, or a refusal of an operation, belongs in `shared/`;
- hooks — `use*.ts` — which own effects, state and the streams;
- components — `*.tsx` — which receive what they show as props and compute nothing that outlives them.

Exactly one kind of file is unit-tested: the `*.test.ts` files, no `*.test.tsx`, and neither jsdom nor
a testing library is installed. Components and hooks are covered by the browser suite in `tests/e2e/`,
so a decision worth a test belongs in a pure module. A test opens with `node:test`'s `describe`/`test`
and `node:assert/strict`, and takes time as an argument (`countdownAt(held, new Date(...))`) rather than
mocking a clock — nothing in the suite installs fake timers.

State that belongs to a session is keyed by the session rather than cleared by an effect: see
`shared/command/commandSender.ts` and `features/cabinet/useFeed.ts`. An effect that resets state on a
changed account leaves one render showing the previous account, which is the defect that idiom exists
to prevent.

## Talking to the API

- `src/shared/api/generated/` is generated from the OpenAPI contract and is never edited by hand.
  Regenerate it from the repository root with `npm --prefix tools/openapi run generate`; the tree is
  committed and digest-checked by `scripts/check-contracts.mjs`. The hand-written seam above it is one
  wrapper per operation: `catalog.ts`, `current.ts`, `session.ts`, `notifications.ts`, `rides.ts`,
  `invoices.ts`, `history.ts`, `health.ts`. A component never calls a generated SDK function itself, and
  these wrappers re-export the generated types the rest of the application uses. How a request is made —
  the credentials it carries, the caching, and the `Origin` a changing request states — is declared once
  in `shared/api/request.ts`.
- A wrapper may add the check the wire format does not carry: `health.ts` refuses an answer that is not
  `ok` or whose `server_time` cannot be parsed, and asserts the declared time zone before it reaches
  `Intl`, so an unusable zone surfaces at the read rather than in the middle of a render.
- `Resource<T>` (`shared/read/Resource.ts`) is how server state reaches the screen: `loading`, `ready`,
  `stale`, `failed`. A failed reload keeps the last successful value and marks it stale; only the first
  failure, with nothing to show, is a failure. `useResource` loads one resource, abandons the request
  in flight when a newer one starts, and hands back `{ resource, retry }`; `useFeed` reads one page of a
  paginated feed and merges it into the pages on screen. Both store an answer through `storedAnswer`, so
  one rule decides what reaches the screen rather than two.
- Reads are anonymous (`credentials: 'omit'`) for the public catalog and `same-origin` for anything of
  the account's. Commands go through `shared/api/commands.ts`: `commandHeaders` adds `Origin`,
  `X-CSRF-Token` and `Idempotency-Key`, and `answerOf` keeps the four outcomes apart. A transport failure
  is `unknown`, never `refused` — telling a person their vehicle is free when the command may have
  succeeded is the mistake that distinction exists to prevent.
- An interval on screen is never a count of ticks. `countdown.ts` derives the time left from the
  deadline the server stored, the moment its answer was computed at and the local moment that answer
  arrived (`ServerClock`), so a tab suspended for an hour comes back to the truth rather than to where
  it stopped. Use `serverMomentAt` rather than `Date.now()` for anything the service decides, and
  `useClockTick` for anything that has to be recomputed while a view is on screen.

## Signals and reconciliation

`src/shared/read/` holds the whole of it: `stream.ts` reads the SSE frames, `coordinator.ts` decides
which documents one signal invalidates, `useReadCycle` performs a change, a handshake and the interval
for every reader, and `useResource`/`useFeed` are the readers it wakes. `features/live` connects the two
streams to that cycle: `useEvents` for the public one and `usePrivateEvents` for the account's.
`reconciliation.ts` re-reads every loaded resource every `RECONCILE_MILLISECONDS` to repair a signal
that was missed. The address parameter `?reconcile=off` switches that off for one page load; it exists
for the browser checks in `tests/e2e/` and changes nothing the application ships.

The stream is read with `fetch` and a body reader on purpose, not with the generated SSE client: that
client sends an event identifier back on every reconnect, and this contract declares no identifier and
no replay, so it would ask the server for something it does not have. `stream.ts` says so where the
choice is made; replacing it with the client is a regression, not a simplification.

## Wording, locale and money

- The interface is Russian. A phrase the browser checks assert, or that two views show, is a named
  constant in the copy module of the feature that owns it, so renaming one is a change to
  `tests/e2e/person.mjs` or the suite that names it in the same commit. A phrase two *features* show
  lives in `shared/copy.ts`, and a refusal of an operation lives in `shared/refusal.ts`, keyed by the
  generated `ErrorCode` so a code the contract adds is a compile error rather than a silent fallback.
- `shared/locale.ts` declares `INTERFACE_LOCALE` and `SERVICE_TIME_ZONE` once for every view. The time
  zone is the backend's value restated because a browser cannot import it: a moment of the service day
  is written in `Asia/Bishkek` through `bishkekMoment`, never in the browser's zone.
- Prices cross the wire in tyiyn and are written as som by `shared/money.ts`. Money and versions
  stay exact: an amount is divided as a `BigInt` and a version is compared as a decimal string, never
  through `Number`. A price is written `12,34 сома` — that unit, not `KGS` or `сом` — and
  `scripts/money-declaration.test.mjs` holds the acceptance oracle of money to the unit and the locale
  this file declares rather than letting it state a second rule.

## Styles

The stylesheets are global: `main.tsx` imports `src/app/styles.css` (the shell and the `:root` tokens),
`src/app/fleet.css` (the map screen, vehicle markers included), `src/app/cabinet.css` (the cabinet) and
`src/app/account.css` (the entry window and the form inside it), and no component carries a stylesheet
of its own — the only other CSS import in `src` is MapLibre's, inside `FleetMap.tsx`. Class names are
kebab-case and hyphen-chained, and state is an attribute selector rather than a modifier class:
`.fleet-row-status[data-status='available']`, `.filter-chip[aria-pressed='true']`,
`.map-vehicle-marker[data-selected='true']`.

One fact is one rule. A pill a person has chosen looks the same wherever it is offered, so the rule
that says one is chosen is stated once for the filter chip, the view tab, the account tab and the
cabinet tab, and the size each is drawn at comes from a `--padding-pill*` token. A colour is a
declared token or a `color-mix` of one, never a literal: `zoneLayer.ts` reads `--color-accent` for the
boundary it paints, as the legend draws its own edge from the same property.

Stylelint (`stylelint-config-standard` plus the repository's rules) requires one declaration per
single-line block, so a rule block is never a one-liner, a blank line before each multi-line rule and
at-rule, `@media (width <= 850px)` rather than `max-width`, and a quoted `url()`. Prettier owns the
alignment; do not hand-format what it would rewrite.

## The map

`src/features/map/` is the basemap and everything drawn on it. `basemap.json` is the one declaration
of what lies underneath: the pinned Protomaps build, the bbox, the depth, the serving prefix, the font
stacks and ranges, the sprite files and the attribution. The image stages read it to cut the archive
and fetch the glyphs, `basemap.ts` reads it to build every address the map asks for, and
`basemap.test.ts` holds the two together — a second copy of any of those addresses is the defect.

- `basemap.ts` builds the style (`layers("protomaps", namedFlavor(...), { lang: "ru" })`) and every
  address the map asks for, below the declared prefix, as a whole address of the installation that
  serves it: the library refuses a path for glyphs and a sprite, and the archive is read through a
  scheme that needs one too. It imports no browser API — the origin is an argument — so it is
  unit-tested by `node:test`.
- `basemapProtocol.ts` puts `pmtiles://` in MapLibre's global protocol table once, from `main.tsx`,
  wrapping the reader so a failed request for the archive reaches whoever is listening. It must be
  installed before the first map exists, and a second registration would replace the first.
- `basemapWorker.ts` tells MapLibre where its own worker is, and imports it as a module the bundler
  emits. Copying the worker file instead is the defect that costs the most and says the least: it
  cannot import the chunk it shares with the library, it fails to start, no tile ever reaches the
  protocol, and the map stays an empty pane with no error anywhere.
- `FleetMap.tsx` creates the map, waits for the style's own `load` event before it says
  `data-basemap="ready"`, draws the zones and the markers, and says `data-basemap="unavailable"` with a
  line under the map when the archive could not be read.
- `vehicleMarkers.ts` keeps one DOM element per vehicle and moves it, rather than rebuilding it on
  every tick: the basemap is painted into the map's own canvas, so a marker has to be an element to be
  clicked and to be found by a check.
- `zoneLayer.ts` and `geometryBounds.ts` put the published boundaries into one GeoJSON source and
  measure the box they span.

The zone and the streets are painted into that canvas, which has no DOM. That is why the browser suite
counts markers, legend items and the attribution rather than asserting the geometry, and why the
geometry is left to a human eye in the demonstration.
