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

### Code style

Readability is a requirement, not a preference. Every change must satisfy the rules below.

**Names**

- A name states what the thing is. No abbreviations that are not already domain words
  (`cfg`, `pc`, `pool` over `p`; `status` over `s`). The idiomatic Go short names
  `ctx`, `err`, `w`, `r`, `i`, `ok` stay.
- Name length follows scope length: one letter only inside a few-line block.
- A file is named after what it contains. A file carrying its package's name is acceptable only
  while it holds that package's single concern, and is split the moment it holds more: `httpapi.go`
  holding a router, a handler and a probe was the defect, `config.go` holding only the config
  loader is not.
- One file holds one concern. If the name cannot describe the contents without "and", split the file.
- A type alias is justified only when it hides a real dependency; never to rename a generated type.

**Functions**

- A function does one thing at one level of abstraction. Target under 40 lines; over 60 is a defect.
- Prefer early returns to nested conditions. Never invert a check to `if err == nil { ... }`.
- Go `main` packages use `func run() error`; `main` only logs the error and sets the exit code.
- A nested closure that needs more than a few lines becomes a named function or method.

**Values**

- No magic numbers or repeated string literals. Use `http.Status*`, generated contract
  constants, or a named constant declared next to its meaning.
- A limit, timeout or size gets a named constant stating its unit (`maxPublicBodyBytes`).
- A value enumerated in more than one place becomes a single source and is derived elsewhere.

**Comments**

- Code answers *what*. A comment exists only to answer *why*, when the reason is not derivable
  from the code: a non-obvious ordering constraint, a workaround with its cause, a domain rule.
- Never restate the code, the log message next to it, the README, or the roadmap.
- Every package has a package doc comment. Every exported identifier whose purpose is not
  obvious from its name has a doc comment.
- A doc comment on a function describes that function's contract, not a detail beside it.
- Delete a comment that a rename or an extracted function makes unnecessary.

**Layout**

- Lines stay under 120 characters. Wrap struct literals, table-test rows and JSX one field per line.
- No conditional logic inside a long inline expression: no chained ternaries in JSX, no map
  literal inside an `if` condition.

**Review gate**

Before proposing a change: reread the diff as a stranger would. Anything that needs a comment to
be understood, or that cannot be named clearly, is a signal to restructure rather than annotate.
