# CLAUDE.md

The Kanon toolchain: `kan lint`, `kan eval`, `kan import` and `kan sql`. The language, the reading rules and the
layout of the code are in `README.md`; the project's decisions and open questions in `docs/decisions.md`; the
study that opened it in `docs/study.md`. Kanon is open source: every document of the project lives in this
repository, none in a shared drive. A new decision goes into `docs/decisions.md`.

The repository is local for now: no public repository and no push without approval.

## Working here

- Go, standard library only, except the pure Go SQLite driver `modernc.org/sqlite` in the SQL packages
  (`internal/sqlimport`, `internal/sqlexport`). `gofmt -l .`, `go vet ./...` and
  `go test -cover ./...` stay clean; every package keeps its coverage at 95% or more.
- The design of files, relations, spreadsheets and SQL is in `docs/data.md`.
- Test first. A new use case is a folder of `cases/` with its schema, its documents and an `expected.txt` written by
  hand before running the linter. `go test ./internal/cli -update` only after a deliberate change of output, and
  the diff is read.
- Everything is English, except the content of a case.

## Messages

Diagnostics are written for someone who edits documents, not code:

- lowercase, no trailing period; names in backticks, written values in double quotes;
- say what to write: `write 650.00 EUR, not "650,00 €"`, and `did you mean …?` when a name is close;
- one mistake, one message: a misspelled key that plainly stands for a field also counts as that field written;
- build a message once and pass it as `Errorf(pos, "%s", msg)`.
