# Contributing

Thank you for looking. A bug report, a case that kan reads wrong, or a message that could say better what to write
are as welcome as code.

- **A bug**: open an issue with the output of `kan version`, the command, what it printed and what you expected.
  The smallest schema and document that show it are the best report.
- **An idea or a change to the language**: open an issue first. The language is small on purpose; the reasons
  behind each choice are in `docs/decisions.md`, and a change starts by saying which one it revisits.
- **A security problem**: not in an issue, see `SECURITY.md`.

## Building and testing

Go, standard library only, except the pure Go SQLite driver `modernc.org/sqlite` in the SQL packages
(`internal/sqlimport`, `internal/sqlexport`).

```
$ go build ./cmd/kan
$ gofmt -l . && go vet ./... && go test -cover ./...
```

`gofmt -l .` prints nothing, `go vet ./...` is clean, and every package keeps its coverage at 95% or more. The CI
runs the same checks on Linux and macOS, with `-race`; Windows is not supported yet (issue #1).

**Test first.** A new use case is a folder of `cases/` with its schema, its documents and an `expected.txt` written by
hand before running the linter: the command on its first line, after `$ `, then what kan prints, and `[exit N]` when
it exits with another code than 0. `go test ./internal/cli` runs every case. `go test ./internal/cli -update`
rewrites the expected outputs, only after a deliberate change of output, and the diff is read. `cases/README.md`
lists the cases.

Everything is English, the content of cases and test fixtures included, with fictional people and places. Only
third-party data stays as published, with its source and licence.

## Messages

Diagnostics are written for someone who edits documents, not code:

- lowercase, no trailing period; names in backticks, written values in double quotes;
- say what to write: `write 650.00 EUR, not "650,00 €"`, and `did you mean …?` when a name is close;
- one mistake, one message: a misspelled key that plainly stands for a field also counts as that field written;
- build a message once and pass it as `Errorf(pos, "%s", msg)`.

## The grammar

`editors/tree-sitter-kanonarch` follows `internal/schema`: a change to the syntax of schemas goes to both, with a
test in `test/corpus/`. After a change to the grammar:

```
$ cd editors/tree-sitter-kanonarch
$ tree-sitter generate && tree-sitter test
$ for f in $(find ../../cases -name '*.kanonarch'); do tree-sitter parse -q "$f"; done
```

The loop reports `cases/errors-schema-label/broken.kanonarch` only: it holds labels on purpose, which the grammar
refuses as the parser does. Zed builds the grammar from the repository named in `editors/zed/extension.toml`, at
the commit `rev`, never from the working tree: commit and push the grammar first, then set `rev` to that commit,
and rebuild the extension from Zed's extensions page.

## Code

| Package | Role |
|---|---|
| `cmd/kan` | the command |
| `internal/cli` | arguments, output, exit codes; the golden tests over `cases/` |
| `internal/lint` | binds documents and their files to their schema and reports what doesn't fit |
| `internal/model` | the bound model of a document, as lint read it: its objects, values, links, files and text |
| `internal/eval` | `kan eval`: computed fields and rules, evaluated over the model; a computed field is a field of the model whose schema field has an expression |
| `internal/sqlimport` | `kan import`: a SQLite database or dump read into a schema and a tree of documents |
| `internal/sqlexport` | `kan sql`: the tables of a schema and the rows of a document, as a script, a database, or the rows of a query |
| `internal/lsp` | `kan lsp`: the language server of editors, with diagnostics, definition, references, hover and symbols |
| `internal/schema` | lexer, parser and checker of `.kanonarch` files |
| `internal/doc` | parser of `.kan` files: front matter, sections, key lines, lists, tables, paragraphs |
| `internal/sheet` | CSV files and xlsx sheets, read as rows of cells in the canonical format |
| `internal/pattern` | `reads` patterns such as `"{quantity} [of] {name}"` |
| `internal/value` | literals: numbers, money, percentages, quantities, dates and times, durations, versions |
| `internal/names` | slugs, the folding of a heading to a name, and "did you mean" suggestions |
| `internal/diag` | diagnostics |

## Releases

A release is a tag `vX.Y.Z` on `main`. The workflow `release.yml` tests, builds the binaries for Linux and macOS
with GoReleaser and takes its notes from the tag's section of `CHANGELOG.md`, which is written before the tag.
Until 1.0, a minor version may change the language; the changelog says how to update documents.

By contributing, you agree that your work is published under the MIT licence of the project.
