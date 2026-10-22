# Decisions

What has been decided about Kanon, and when. The language as it stands is in `README.md`, the design of data and SQL
in `docs/data.md`, the study that opened the project in `docs/study.md`.

## Project

- **Open source**, kept in this repository only: no shared drive, every document of the project lives here
  (2026-10-22). A private repository for now, `bn-k/kanon` on GitHub, and the module path
  `github.com/bn-k/kanon` (2026-10-22); nothing public yet.
- **The cases are fictional** or public data with their licence (`source/README.md`): no real person, machine or
  address (2026-10-22).
- **Workflow**: import a real use case (a database, a document, any format), write its schema, write the document
  (one file or a tree), write the expected output by hand, then run `kan lint` (2026-10-22). The cases live in
  `cases/`, their sources included.
- **Compiler, parser and linter in Go**, for now (2026-10-22). Standard library only, except the pure Go SQLite
  driver `modernc.org/sqlite`, allowed in the SQL packages only.

## Language

- Name **Kanon**, extension **`.kan`** for documents (2026-10-22).
- Extension **`.kanonarch`** for schemas and rules, after the Byzantine canonarch who intones the canon before the
  choir (2026-10-22).
- **Universal, so in English**: keywords, native types, documentation, project files. No domain (tenders…) in the
  core of the language: those are working material, and only their content stays in its own language (2026-10-22).
- **Code, but human-readable**: a non-developer must be able to edit a `.kan` file like Markdown (2026-10-22).
- **Literals in canonical English format only**, for now: `2026-09-01`, `1 250.50 EUR`, `no`, `3 years`. No `lang:`
  and no localized literals (2026-10-22). Expressions write them the same way: `total > 1 250.50 EUR`.
- **Schemas in Kanon's own syntax**, GraphQL-style, not YAML: one parser covers schemas, computed fields and rules.
  Kanon exports JSON Schema and OpenAPI for interoperability (2026-10-22).

## Names, ids and files

- **Names are ASCII alphanumeric slugs** only; no accents, no Unicode in names. Free text (content) stays Unicode
  (2026-10-22).
- **A schema holds no labels** (2026-10-22). No string after a field type, a schema name, an enum value or a unit:
  the parser reports it and says to remove it. A field, an enum value and a unit are known by their name alone, and
  a message that names an enum value shows the name. `kan import` writes no labels. This drops the "label already
  used" check: two names that differ cannot fold to the same heading.
- **A `.kanonarch` file is ASCII** (2026-10-22), strings and comments included, so that a schema reads the same in
  every editor and diff. The checker reports any other character at its position, one diagnostic per line, with the
  letter to write when a plain one exists (`write "e", not "é"`). A `reads fr` sentence is written without accents,
  « » become plain quotes or go, `n°` becomes `no`. The content of a case stays in its own language.
- **A heading finds its field by name** (2026-10-22). The heading is folded (lowercase, accents removed, spaces,
  hyphens and apostrophes alike) and compared with the field's name: "Dépôt de garantie" opens `depot-de-garantie`.
  The folding is one function, `names.Slugify`. Keys, table headers and enum values stay exact names. A document
  keeps its real French headings.
- **Hyphen everywhere** (kebab-case) for ids, fields, enum values, rules and functions; subtraction needs spaces
  (2026-10-22).
- **Every object has an `id`**, an ASCII alphanumeric slug. A UUID is valid, but meaningful slugs are encouraged
  (2026-10-22).
- **Ids are always written** (file name, `{#id}` on a heading or list item, `id` column), never derived from a title
  (2026-10-22).
- **Ids are unique per file**: an entry of a folder array is not registered in its parent; the rows of a
  spreadsheet share the namespace of the `.kan` that links to it (2026-10-22).
- **An object can be a file**, named after its id: `un-fichier-001.kan`. A file with child objects can have them as
  files in the tree; **an array is a folder** holding one `.kan` file per entry (2026-10-22).
- **Children of a file object go in a sibling folder** named after the object, then the field: `clio.kan` +
  `clio/entretiens/` (2026-10-22).
- **Both forms for an array**: inline (table, list, sections) or a folder (2026-10-22).

## Data, relations and SQL

- **Any kind of data**: images, binaries, code and spreadsheets are accepted; whatever is not text is a file next
  to the document. **Relations** are supported. **Kanon turns into SQL and back** (2026-10-22). Design in
  `docs/data.md`.
- **SQLite first**, through the pure Go driver (2026-10-22).
- **Spreadsheets (csv, xlsx) are validated data**: a source of an array of objects, every row checked against the
  schema; a cell link is relative to the `.kan` (2026-10-22).
- **Northwind** (SQLite version) is the first database imported (2026-10-22).
- **A `DateTime` without a zone is a floating local time** (2026-10-22).
- **`kan import` names a table's field in the plural**, whether the table's name is or not (2026-10-22).

- **`kan sql -q` queries a document** (2026-10-22): `kan sql -q 'SELECT …' schema.kanonarch doc.kan`, or
  `-q @query.sql`. A document is searched two ways, its files (grep) and SQL; the query runs over the tables of
  `kan sql`, built in memory (no file) and read only (one `SELECT`/`WITH`/`VALUES`/`EXPLAIN` statement, first word
  checked, then `PRAGMA query_only`). It prints an aligned table and the number of rows, or `-format csv`; a SQL
  error is a diagnostic at its position (exit 1), a misuse (`-q` with `-o`) exit 2. The in-memory database declares
  `DATE` and `DATETIME` as `TEXT`, so dates come out as written. Values are printed as SQLite gives them: a `Money`
  stays in two columns, no canonical regrouping.

## Computed fields and rules

- **`kan eval`** prints the computed fields; `kan lint` checks the rules. Both run only on a document without lint
  errors (2026-10-22).
- **A rule applies to every object of its first parameter's schema**; a rule that gives `none` holds (2026-10-22).
- **A written computed field that differs from its computation is an error** (2026-10-22).
- **`today` comes from the `-today YYYY-MM-DD` flag**, or the local date without it; a case that depends on the date
  writes the flag in its command (2026-10-22).

## Editors

- **Zed first, as an extension**, not a change to Zed. It lives in this repository: `editors/zed` for the
  extension, `editors/tree-sitter-kanonarch` for the grammar of schemas (2026-10-22).
- **The tree-sitter grammar follows the Go parser** of `internal/schema` and never contradicts it: what one reads,
  the other reads. It checks no types, names or units; `kan lint` does (2026-10-22).
- **The generated `src/parser.c` is committed**: Zed compiles it itself, from the commit named by `rev` (2026-10-22).
- **The language of schemas is named "Kanonarch"** in the editor, after its extension; "Kanon" is the language of
  documents (2026-10-22).
- **The grammar's `repository` is the GitHub repository**, `https://github.com/bn-k/kanon`: Zed fetches the
  commit `rev` from there, so a grammar change is seen once it is pushed. While the repository is private, the
  machine that runs Zed needs git credentials for GitHub (2026-10-22).
- **Navigation and live diagnostics come from a language server, `kan lsp`**, in `internal/lsp`: the editor gets
  what `kan lint` knows, through one protocol any editor speaks. The extension only starts it; it runs the `kan` on
  the `PATH`, or the one named by `lsp.kan.binary.path` in Zed's settings (2026-10-22).
- **Every name the checker resolves is recorded as a reference** to its declaration, and every declaration as a
  reference to itself: definition, references, highlights and hover all read that one list (2026-10-22).
- **The server lints again 300 ms after the last edit**, from the open text and the files on disk, and before it
  answers any request (2026-10-22).
- **A document's schema is found, not given**: the nearest folder, the document's own or above, that holds
  `.kanonarch` files; there, the file that declares the document's `type`, or the first. A document without a
  `type` is checked through the document that owns its folder (`clio/` belongs to `clio.kan`). An open schema checks
  the documents of its folder whose schema it is. This holds for the editor only; `kan lint` still takes the schema
  as an argument (2026-10-22).
- **A `.kan` document is Markdown in the editor**: the language "Kanon" uses the Markdown grammar that ships with
  Zed, and its queries (2026-10-22).
- **A document is read and edited laid out in Zed's Markdown preview**, as in Obsidian's live preview: a click on
  a block turns it into an editor of its source, written through to the file at every keystroke, and escape or a
  click elsewhere lays it out again. A header above the document switches between source and preview, and its
  buttons format the selection or the block being edited: headings, bold, italic, lists, tasks, quotes and
  `[[…]]` links. An extension cannot draw a view, so this is a change to Zed itself, kept in
  our fork (`bn-k/zed`, branch `karim/kanon-preview`) and meant for upstream: the preview takes any language built
  on the `markdown` grammar, reads `[[…]]` and `![[…]]`, and opens `[[agents/sophie-bernard]]` as
  `agents/sophie-bernard.kan`. A paragraph of `key: value` lines keeps one line per key (2026-10-22).

## Open questions

- **A heading that looks like a field.** A clause titled "Dépôt de garantie" folds to the name
  `depot-de-garantie` and opens that field instead of becoming a clause. The folding is looser than the old label
  match (case and accents no longer tell them apart), so a neophyte will fall into this more often. The message
  says to give the section another title.
- **A single sub-object as a file**: `clio/carte-grise.kan` would be named after a field, not an id. Allowed, or
  arrays only?
- **Where schemas live**: the command line takes the schema as an argument; the language server finds it in the
  nearest folder that holds one (see Editors). Should `kan lint` find it the same way, or is an explicit `use` or a
  project file better?
- **`reads` patterns** both parse and render a line (`250 g de farine`). How far can they go before they become a
  grammar of their own?
- **Severity**: an oil change that's due is a warning, an illegal deposit is an error. `rule … warn`?
- **An exact SQL round trip for any schema**: today it holds for the types `kan import` writes; embedding the schema
  in the database would make it hold for `Duration`, `Money`, `Quantity`, enums and `Code`.
- **Canonical values in query results**: `kan sql -q` prints a `Money` as `x` and `x_currency`, a `Quantity` as `x`
  and `x_unit`. Joining them into `1 250.50 EUR` would need the schema to follow each result column back to its
  table column, which a computed column (`sum(price)`) loses. A view per schema that joins them, or a function, is
  the way if it matters.
- **Quoting of keyword tables**: `"order"` must be quoted in a query, as in the SQL `kan sql` prints. A friendlier
  name (`orders`) would break the `kan sql` script's names.
- **The size of `cases/northwind`**: the whole database, photos included (about 1 100 files).
