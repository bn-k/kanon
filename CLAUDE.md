# CLAUDE.md

The Kanon toolchain: `kan lint`, `kan eval`, `kan write`, `kan import`, `kan sql` and `kan lsp`. `README.md` is the
front page, kept short; the language and its reading rules are in `docs/language.md`, the commands in
`docs/kan.md`, the layout of the code and the checks in `CONTRIBUTING.md`; the project's decisions, its prior art
and its open questions in `docs/decisions.md`. Kanon is open source: every document of the project lives in this
repository, none in a shared drive. A new decision goes into `docs/decisions.md`.

The repository is public, on GitHub at `bn-k/kanon`, under the MIT licence (`LICENSE`): whatever is pushed is
published, so no real company, person or tender goes in a commit, even one removed afterwards. Several
sessions work on it at once: commit each finished step, `git pull --rebase origin main`, then push, and do it
often, without waiting to be asked.

## Working here

- Go, standard library only, except the pure Go SQLite driver `modernc.org/sqlite` in the SQL packages
  (`internal/sqlimport`, `internal/sqlexport`). `gofmt -l .`, `go vet ./...` and
  `go test -cover ./...` stay clean; every package keeps its coverage at 95% or more.
- The design of files, relations, spreadsheets and SQL is in `docs/data.md`.
- Test first. A new use case is a folder of `cases/` with its schema, its documents and an `expected.txt` written by
  hand before running the linter. `go test ./internal/cli -update` only after a deliberate change of output, and
  the diff is read.
- Everything is English, the content of cases and test fixtures included. Only third-party data stays as
  published, with its source and licence: Northwind, the bank holidays of `cases/public-holidays`, the workbooks of
  `internal/sheet/testdata`.
- `editors/tree-sitter-kanonarch` follows `internal/schema`: a change to the syntax of schemas goes to both, with a
  test in `test/corpus/`. `tree-sitter generate` and `tree-sitter test` stay clean, and no case schema but
  `cases/errors-schema-label/broken.kanonarch` parses with an error. Zed fetches the grammar from GitHub at the
  commit named by `rev` in `editors/zed/extension.toml`: commit and push the grammar first, then update `rev`.

## Messages

Diagnostics are written for someone who edits documents, not code:

- lowercase, no trailing period; names in backticks, written values in double quotes;
- say what to write: `write 650.00 EUR, not "650,00 €"`, and `did you mean …?` when a name is close;
- one mistake, one message: a misspelled key that plainly stands for a field also counts as that field written;
- build a message once and pass it as `Errorf(pos, "%s", msg)`.
