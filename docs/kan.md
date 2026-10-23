# The kan command

```
kan lint [-today YYYY-MM-DD] <schema.kanonarch> <document.kan>
kan eval [-today YYYY-MM-DD] <schema.kanonarch> <document.kan>
kan write [-n] [-today YYYY-MM-DD] [-format text|json] <schema.kanonarch> <document.kan>[=<new.txt>]...
kan import <database.sqlite|dump.sql> <out-dir>
kan sql <schema.kanonarch> <document.kan> [-o out.sql|out.sqlite|out.db]
kan sql -q <query|@query.sql> [-format table|csv] <schema.kanonarch> <document.kan>
kan lsp
kan version
```

The language the commands read is in `docs/language.md`; the examples below are cases of `cases/`.

## kan lint

```
$ kan lint cases/invoice/schema.kanonarch cases/invoice/f-2026-031.kan
ok  cases/invoice/schema.kanonarch
ok  cases/invoice/f-2026-031.kan
ok  cases/invoice/clients/dupont-batiment.kan
```

`kan lint` checks the document, every document it links to and every entry of its folders against the schema. It
reports like a compiler, `file:line:col: error: message`, and exits with 1 when there is an error, 2 when it is
misused.

```
f-2026-032.kan:8:7: error: write a date as YYYY-MM-DD (2026-10-22), not "22/10/2026"
f-2026-032.kan:20:8: error: names are ASCII: write designation, not "Désignation"
f-2026-032.kan:27:44: error: {total-tc} inserts no field; did you mean {total-ttc}?
```

It checks structure (fields, values, ids, links, files and folders), then evaluates the computed fields and the
rules. A computed field written in the document must hold its computed value, and a rule that fails is reported at
the object, in the words of its `reads` sentence when it has one:

```
f-2026-033.kan:12:1: error: `total-ttc` is 9 504.00 EUR, not "9 500.00 EUR"
clio.kan: error: `vidange-a-jour` fails: "Renault Clio a roule moins de 15 000 km depuis sa derniere vidange"; 15 250 km is not less than 15 000 km
```

`today` is the local date, or the one given by `-today`, on `kan lint`, `kan eval` and `kan write` alike:
`kan lint -today 2026-10-22 schema.kanonarch clio.kan`. A case whose output depends on the date passes it, so its
output stays the same.

## kan eval

```
$ kan eval cases/invoice/schema.kanonarch cases/invoice/f-2026-031.kan
f-2026-031.kan: total-ht = 7 920.00 EUR
f-2026-031.kan: tva = 1 584.00 EUR
f-2026-031.kan: total-ttc = 9 504.00 EUR
f-2026-031.kan:23:1: montant = 7 800.00 EUR
f-2026-031.kan:24:1: montant = 120.00 EUR
```

`kan eval` lints like `kan lint`, then prints the value of every computed field, where its object is written; the
diagnostics follow.

## kan write

A program that edits documents (a server, an agent) writes them with `kan write`, so that a folder of valid
documents stays valid, as a database keeps its constraints:

```
$ kan write -n schema.kanonarch liste.kan=liste-renommee.txt
critique.kan:7:8: error: unknown id `dune` in liste.kan; did you mean `dune-messie`?
1 error
```

`kan write` lints the new text of each document in its place, with every document of the schema's folder that
links to one of them, and writes them only when that finds no error: none in the new texts, and none in a document
that links to them which it didn't have before. Each document is written through a file renamed over it, so that a
reader never sees half of it.

- `liste.kan=liste-renommee.txt` reads the new text from a file; one document at most goes without `=`, and reads
  it from stdin. Several documents are checked together: a book renamed with the review that links it.
- An entry of a folder, which has no type of its own, is checked through the document that owns the folder, with
  the new text in its place: `board/tasks/2026-10-28-release-notes.kan` is a new task of `board.kan`, even before
  the folder exists.
- `-n` checks and writes nothing: an editor's lint of an unsaved text, with what it would break elsewhere.
- `-format json` prints `{"ok", "written", "checked", "diagnostics"}` for a program, each diagnostic as
  `{"file", "line", "col", "severity", "message"}`.
- It exits with 0 when it wrote (or, with `-n`, would write), 1 when it found an error or could not write, 2 when it
  is misused. Hidden folders are not searched for documents.

## kan import and kan sql

```
$ kan import northwind.sqlite northwind
$ kan sql northwind/schema.kanonarch northwind/northwind.kan -o northwind.db
```

`kan import` writes a schema and a tree of documents from a SQLite database or a SQL dump, into a new folder, and
prints what it changed on the way (keys turned into ids, empty texts left out). `kan sql` lints a document first:
on an error it prints the diagnostics and writes nothing. Otherwise it prints a SQL script, or writes it to the new
file named by `-o`: a script for `.sql`, a SQLite database for `.sqlite` or `.db`. It never overwrites a file. The
mapping both ways is in `docs/data.md`.

There are two ways to search a document: its files, with grep, and SQL. `kan sql -q` runs a query over the same
tables, without writing any file:

```
$ kan sql -q 'SELECT c.company_name AS customer, count(*) AS orders
              FROM customer c JOIN "order" o ON o.customer_id = c.id
              GROUP BY c.id ORDER BY orders DESC LIMIT 3' northwind/schema.kanonarch northwind/northwind.kan
customer            orders
------------------  ------
Save-a-lot Markets      31
Ernst Handel            30
QUICK-Stop              28
3 rows
```

- The query is an argument, or `-q @query.sql` reads it from a file. It lints the document first, as `kan sql`
  does: on an error it prints the diagnostics and runs nothing.
- The database is built in memory and only read: a query is one `SELECT`, `WITH`, `VALUES` or `EXPLAIN` statement,
  and anything that writes is refused. The table names are those of `kan sql`; one that is an SQL keyword is
  written with double quotes (`"order"`).
- The rows are an aligned table, numbers to the right, `NULL` for none, a BLOB as its size (`<blob 10151 bytes>`),
  then their number. `-format csv` prints them as CSV instead, with no count. Values come as SQLite stores them:
  dates and times as written in the document, a `Money` in two columns (`revenue`, `revenue_currency`).
- A mistake in the query is a diagnostic at its position, in the file of `@query.sql` or in `query`, with exit
  code 1: ``query:1:8: error: no such column `company_nam`; did you mean `company_name`?``.

## kan lsp and the editors

### Zed

`editors/zed` is an extension for the Zed editor: it colours `.kanonarch` schemas, outlines their declarations
and fields, and indents them; `.kan` documents are read as Markdown. Its grammar is
`editors/tree-sitter-kanonarch`, a tree-sitter grammar that follows the Go parser of `internal/schema`. To install
it, open the command palette in Zed, run `zed: install dev extension` and choose the folder `editors/zed`.

`kan lsp` is the language server the extension runs: install `kan` on the `PATH` first, or name it in Zed's
settings under `lsp.kan.binary.path`. It checks the open documents and schemas as they are typed, with the
diagnostics of `kan lint`, and answers:

- go to definition and declaration: from the `type` of a document, a field, a heading, an enum value or an insert
  `{field}` to where the schema declares it; from a link `[[…]]` or `[…](…)` to its file, or to the object of its
  `#id`; in a schema, from a type, a field, a function or a rule to its declaration;
- find references and highlights: every place that names the same declaration, in the schema and the documents;
- hover: the line that declares it, and where;
- symbols: the outline of a file, and a search over the declarations of every open file.

A document finds its schema in the nearest folder, its own or above, that holds `.kanonarch` files: the one that
declares the `type` of its front matter, or the first. A document without a `type` is checked as part of the
document that owns its folder (`clio/entretiens/…` through `clio.kan`). An open schema also checks the documents of
its folder whose schema it is.

### Obsidian

Obsidian reads `.kan` documents through the plugin in `editors/obsidian`: it opens them as Markdown and follows a
`[[link]]` from the document's folder, with or without `.kan`, showing only its name: `[[../customers/hanar]]`
reads "hanar". To install it in a vault, link the folder into the vault's plugins and turn on community plugins:

```
$ ln -s ../../../editors/obsidian cases/.obsidian/plugins/kanon
```

## kan version

```
$ kan version
kan 0.1.0 (commit e5fa13e, go1.26.8, darwin/arm64)
```

A binary of a release, or one installed with `go install github.com/bn-k/kanon/cmd/kan@v0.1.0`, names its
version; any other build says `dev`. A bug report gives this line.
