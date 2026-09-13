## Agent skills

### Issue tracker

Issues are tracked in this repository's GitHub Issues. See `docs/agents/issue-tracker.md`.

### Triage labels

Use the five canonical triage labels. See `docs/agents/triage-labels.md`.

### Domain docs

This is a single-context repository. See `docs/agents/domain.md`.

### Commit messages

Write the entire commit message in English as one concise sentence using the format
`T<task number>: <what was done>`, for example `T05: add the vehicle availability endpoint`.
Use only that single subject line: do not add a body, bullet list, pull request reference, author credit,
`Co-authored-by` trailer, or any other metadata.

### Merging

`master` is protected and takes no merge commits, so a change reaches it in one way only: a branch off
`master`, pushed, and merged through a pull request with squash merge. `master` stays linear, and one
task is one commit on it.

- Never merge into `master` locally and never push to it directly. A local merge commit is rejected by
  the branch protection rule, which leaves the work stranded outside `master` with a history nobody
  asked for.
- A branch is named `T<task number>-<short-name>`. One task is one branch and one pull request; a
  follow-up to a merged pull request is a new branch.
- A squash merge discards the branch's own commits, so the pull request is where the reasoning, the
  checks and the alternatives belong. The commit that lands carries only the one-sentence subject.
- Run the formatting gates before the pull request rather than relying on the pipeline to report them;
  the required `Repository checks` gate blocks the merge until every job passes.

### Architecture and wiring

A process has one composition root that names every concrete implementation it runs and hands them
to the code that uses them. Everything below it receives what it needs as an argument or a field.

- Configuration is read at the composition root or by the loader it calls. A package deeper down
  never reads the environment, a global, or a package-level variable to learn how it is configured.
- A feature declares what it must be told (`ratelimit.Limits`), and the loader fills that shape in.
  The table pairing an identifier with the setting it is read from lives beside the identifier, not
  in the loader, so a new case cannot be added with its configuration wired up nowhere.
- Inject the concrete type and wire it at the composition root. Introduce an interface when a second
  real implementation exists or a test needs a seam; an interface with one implementation and no
  second caller is speculation. Narrowing a dependency to an interface it already satisfies is not
  speculation, and neither is a function type for one behaviour.
- Validate dependencies where they are assembled, and fail at startup with a message naming what is
  missing. Never default a missing dependency and never dereference it inside a handler, where the
  failure reaches a client as a crash.
- A nil receiver or a nil field that means "not configured" is a second way of saying the same
  thing. Say it once, where the value is built.
- A flag, an environment name or a mode decision belongs to the layer that owns the decision: the
  command that must refuse, or the feature that must behave differently — never to the handler that
  happens to see it.

### Separation and extension

- One file holds one concern, at every level. If it needs "and" to be described, split it: a file
  that declares the errors of a contract, the statuses they carry and the code that writes them is
  three files.
- A file that declares a container declares only the container. The shape and the assembly of a
  feature's handlers live beside those handlers, so the file does not grow a second concern every
  time a feature is added to it.
- A handler moves a request between the contract and the feature that owns the behaviour. It does
  not write SQL, hold a domain rule, or build a second copy of a value the contract already states.
- A file named after a thing holds only that thing. A module named after the HTTP client does not
  also own process control and database access.
- Everything the codebase maintains has exactly one declaration. A second list of the same facts —
  an inventory, a status table, a set of implemented operations — is derived from the first or it is
  a defect waiting to disagree.
- A document that maps the code — the package tree, the list of commands, the environment variables
  a process reads — belongs to the change that moves the code. It is corrected in that change, not
  left for a later one.
- Adding a feature is a new file and one line at the composition root. If it is an edit to a growing
  conditional, a growing type switch, or a list that everything must be added to, the seam is in the
  wrong place.
- Take what the request carries as an argument and pass it down. A request-scoped value — the
  session, the transaction — is established once at the boundary and read from there; reaching for
  the request again further down is how two parts of one request come to disagree.

### Configuration

- A setting has one owner: one default, one environment name, one shape, declared together. Nothing
  outside the loader names the environment variable, and nothing outside the feature names the
  default.
- Every value that is configuration in more than one layer — a port, a path, a timeout, a limit, a
  cookie name, a pair of credentials — is declared once in code and named by everything else, or
  passed as configuration to the thing that needs it.
- The loader owns the shape it produces and hands it over whole. A consumer reads the fields it
  needs from that value; it does not rebuild the same shape from the same environment.
- A value two processes must agree on — a hash cost, a cookie name, a session lifetime — has one
  shape, declared below both, filled in by the loader and consumed by the feature that needs it. A
  process does not assemble that shape from the loader's fields: the second assembly is how the two
  come to disagree.
- Where a tool cannot import the program's value — a container health check, a proxy directive, a
  workflow output, a document — the copy is that tool's own configuration. Keep it in one place in
  our code and derive it from the same declaration wherever the tool allows, such as an exported
  path constant or an environment variable the entrypoint sets.
- Generated code is the projection of its source, so it is not a second declaration. So is a value
  the contract restates in order to be self-contained.

### Code style

Readable, extensible code is the deliverable; working code that nobody can read is not finished.
These rules are enforced by the checks named at the end of this section, so a change that breaks
them fails the pipeline rather than waiting for a reviewer.

Reach for a name before a comment, and a smaller unit before a longer one. Code answers *what*;
a comment exists only to answer *why*. When you cannot name a thing clearly, that is the
instruction: restructure it until you can.

**Names**

- A name states what the thing is. No abbreviations that are not already domain words
  (`status` over `s`, `configuration` over `cfg`, `connectionPool` over `pool`). The idiomatic Go
  short names `ctx`, `err`, `w`, `r`, `i`, `ok` stay, as does a receiver or a request parameter
  that names its own type.
- Name length follows scope length: one letter only inside a few-line block.
- A name is read far more often than it is written. `formatCheckedAt` over `fmt`, `refusalText`
  over `text`, `submission` over `data`.
- A file is named after what it contains. A file carrying its package's name is acceptable only
  while it holds that package's single concern, and is split the moment it holds more: `httpapi.go`
  holding a router, a handler and a probe was the defect, `config.go` holding only the config
  loader is not.
- One file holds one concern. If the name cannot describe the contents without "and", split the
  file.
- A type alias is justified only when it hides a real dependency; never to rename a generated type.

**Functions**

- A function does one thing at one level of abstraction. Target under 40 lines; over 60 is a defect.
- A function that config, transport and teardown all pass through is three functions. Extract
  `newServer` and `serve` rather than writing one long `run`.
- Prefer early returns to nested conditions. Never invert a check to `if err == nil { ... }`.
- Go `main` packages use `func run() error`; `main` only logs the error and sets the exit code.
- A nested closure that needs more than a few lines becomes a named function or method.

**Values**

- No magic numbers or repeated string literals. Use `http.Status*`, generated contract
  constants, or a named constant declared next to its meaning.
- A limit, timeout or size gets a named constant stating its unit (`maxPublicBodyBytes`,
  `HEALTH_REQUEST_TIMEOUT_MS`).
- A value enumerated in more than one place becomes a single source and is derived elsewhere —
  including a value that a human-readable message spells out.
- Two lists read by the same index are one declaration. State them together, or the shorter one
  silently becomes the limit of the longer.

**Comments**

- Code answers *what*. A comment exists only to answer *why*, when the reason is not derivable
  from the code: a non-obvious ordering constraint, a workaround with its cause, a domain rule.
- Never restate the code, the log message next to it, the README, or the roadmap. Delete
  commented-out code.
- A comment that a rename or an extracted function makes unnecessary is deleted, not reworded.
- Every package has a package doc comment. Every exported identifier whose purpose is not
  obvious from its name has a doc comment.
- A doc comment on a function describes that function's contract, not a detail beside it.
- Ticket numbers and specification question numbers mean nothing to a reader of the code. State
  the rule itself.

**Layout and separation**

- Lines stay under 120 characters. Wrap struct literals, object literals, argument lists,
  table-test rows and JSX one field per line.
- One statement per line, one CSS declaration per line, one intent per block.
- Separate distinct meanings with a blank line: between declarations, between the phases of a
  function, between CSS rule blocks, between `@media` blocks. A blank line is how a reader sees
  where one thought ends.
- No conditional logic inside a long inline expression: no chained ternaries, no ternaries in JSX,
  no map literal inside an `if` condition. Extract a named helper or a lookup table.
- A rule is not a licence for a one-liner: `.connection { padding: 30px; background: #fffefa; }`
  is the defect this section exists to prevent.

**Rendering**

- A render computes what to draw and returns elements; it records nothing that outlives it. A value
  a render has to remember is written by the handler of the event that produced it or by an effect,
  because a render React discards must not leave a value behind that no screen ever showed.

**Review gate**

Before proposing a change:

1. Reread the diff as a stranger would. Anything that needs a comment to be understood, or that
   cannot be named clearly, is a signal to restructure rather than annotate.
2. Reread the modified file as a whole, not only the diff. A function that grew past 40 lines, a
   file that grew a second concern and a comment that the change made redundant are visible only
   in the whole.
3. Ask what the change now knows. A new coupling — a feature reading the environment, a handler
   naming a table, a value restated in a second file, a list that must be edited in step with
   another — is a defect even when every test passes, and the fix belongs in this change rather
   than in a follow-up.
4. Run the formatting gates below and fix what they report. A check that fails is not a reviewer's
   problem to raise later.
5. Check what the change claims. A claim the code makes about its own data — a position inside the
   zone, a declared count, a value two processes share — is a check rather than a comment, and a
   check the specification lists either exists or is recorded as missing with the reason.

**Formatting gates**

- Go files: `gofmt -w <files>`, then from `backend/`: `go build ./... && go vet ./... && go test ./...`.
- JavaScript, TypeScript, CSS, JSON: from the repository root, `npm run format:check` and, from
  `frontend/`, `npm run typecheck && npm run lint:css`. Run `npm run format` at the root to fix
  what the check reports.
- The pre-commit hook formats staged files with the same Prettier configuration, so hand-written
  code arrives already aligned; a repository-wide `npm run format` is available when a change
  reorders many files at once.
- Prettier and Stylelint decide formatting, and their configuration is the single source of truth
  for it: never hand-format what the tools format, and never disable a rule to land a change.
  Write the code the rule wants.
- Generated code is exempt from every rule here. It is reformatted only by regenerating it:
  `npm --prefix tools/openapi run generate`.
