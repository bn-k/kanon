# Changelog

The changes of `kan` and of the language, newest first, in the format of [Keep a Changelog](https://keepachangelog.com).
Versions follow [Semantic Versioning](https://semver.org); before 1.0, a minor version may break documents and
schemas, and its entry says how.

## [Unreleased]

### Added

- `kan set schema.kanonarch task.kan status=done #ana.email=…`, for a program that changes a field without
  rewriting the text: each value changes in place (a key line, a table cell, a heading, the part of a list item its
  field is read from), a field not written yet gets its key line where a person would write it, an empty value
  removes an optional field, and the rest of the text stays byte for byte; an aligned table stays aligned. The new
  text is checked and written as `kan write` does, with `-n` and `-format json`, which adds the new `text`. Inverses,
  computed fields not written, objects, prose, code, lists and folders are refused, with what to do instead. See
  `docs/kan.md`; cases `set-*`.
- `kan lint -format json` and `kan eval -format json`, for a program: `{"ok", "checked", "diagnostics"}` as
  `kan write` prints them, and for `kan eval` the `documents`, every object of the document and of those it links
  to, with every field of its schema: its value in canonical text and typed (numbers, `{"amount", "currency"}`, ISO
  dates and durations, links as `{"file", "id"}`), whether it is computed or an inverse, and where it is written
  (`line`, `col`, `end_line`, `end_col`). See `docs/kan.md`; cases `invoice-json`, `board-json`, `meetings-json`,
  `recipe-json`, `errors-invoice-json`.
- A `DateTime` is ordered against a `Date`, which stands for the start of its day, as in SQL: `m.at >= today` holds
  for a meeting held earlier today (`cases/meetings`). `==` and `!=` between them stay errors, with a message that
  says to compare with `>=` and `<`, or to compare two dates.
- An array of values is written on a key line, separated by commas, when no value of its type holds a comma:
  `norms: iso-9001, iso-14001`, `audits: 2025-03-12, 2026-03-10`. Each item is checked as on its own key line; an
  empty item and a trailing comma are reported; a required array left out is shown both ways, under its heading or
  on a key line. An array of `Text` or `Prose` stays a list under its heading, and the message says why. Among
  numbers, a comma between two digits stays in its item: `1,250` is reported.

### Changed

- The JSON of `kan write -format json` keeps `<`, `>` and `&` as they are, instead of `<`, `>` and
  `&`; JSON readers read both the same.
- The cases, the examples of the documentation and the test fixtures are in English, with fictional people and
  places: `annonce/` is `listing/`, `jours-feries/` is `public-holidays/` (the bank holidays of England and Wales,
  from GOV.UK), and the other cases keep their folder with English documents. Northwind and the workbooks of
  `internal/sheet/testdata` stay as published.
- A table header or key written with capitals is reported as `names are lowercase ASCII`, as one with accents is.
- A heading, a key, a table header or a folder close to a field's name (`## Revenue` for `revenues`, `foundd:` for
  `founded`) is reported once, with its `did you mean`, and counts as that field written: the field is no longer
  reported missing as well.

### Security

- `kan` is built with Go 1.26.10, which fixes vulnerabilities of the standard library of Go 1.26.8 in `net/http`,
  `net/textproto`, `crypto/tls` and `crypto/x509`. `kan` opens no network connection, but its binaries held that
  code.

## [0.1.0] - 2026-10-23

The first release: a folder of `.kan` documents, checked against a `.kanonarch` schema, that people, language
models and programs can share.

### Added

- `kan lint`: the structure of a document, of every document it links to and of its folders, then its computed
  fields and rules, reported as `file:line:col: error: message` in words that say what to write.
- `kan eval`: the value of every computed field, where its object is written.
- `kan write`: new texts of documents linted in place, with the documents that link to them, and written only when
  nothing breaks; a new entry of a folder through the document that owns it; `-n` to check only, `-format json`
  for a program.
- `kan import`: a schema and a tree of documents from a SQLite database or a SQL dump.
- `kan sql`: a document as a SQL script or a SQLite database; `kan sql -q` runs a read-only query over it.
- `kan lsp`: a language server with diagnostics, definition, references, hover and symbols.
- `kan version`.
- Spreadsheets as arrays (CSV, xlsx), folders as arrays with an optional order list, links from the schema's
  folder (`[[/board#ana]]`), files and images, code blocks.
- The language defined rule by rule in `docs/language.md`, the commands in `docs/kan.md`.
- An extension for Zed, its tree-sitter grammar of `.kanonarch`, and a plugin for Obsidian, all in `editors/`.

[Unreleased]: https://github.com/bn-k/kanon/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/bn-k/kanon/releases/tag/v0.1.0
