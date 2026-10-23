# Kanon

Kanon describes structured documents as code. A `.kanonarch` file holds the schema: types, units, enums, computed
fields and rules, written once by someone technical. A `.kan` file is the document anyone edits, like Markdown:
key lines, headings, lists, tables and prose.

```
$ go build ./cmd/kan
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

## Computed fields and rules

```
$ kan eval cases/invoice/schema.kanonarch cases/invoice/f-2026-031.kan
f-2026-031.kan: total-ht = 7 920.00 EUR
f-2026-031.kan: tva = 1 584.00 EUR
f-2026-031.kan: total-ttc = 9 504.00 EUR
f-2026-031.kan:23:1: montant = 7 800.00 EUR
f-2026-031.kan:24:1: montant = 120.00 EUR
```

`kan eval` lints like `kan lint`, then prints the value of every computed field, where its object is written; the
diagnostics follow. A type error in an expression is found when the schema is checked, before any document is
read: ``cannot add `Money` and `Int` ``.

`today` is the local date, or the one given by `-today`, on `kan lint` and `kan eval` alike:
`kan lint -today 2026-10-22 schema.kanonarch clio.kan`. A case whose output depends on the date passes it, so its
output stays the same.

- A literal is written as in a document: `1 250.50 EUR`, `15 000 km`, `20 %`, `3 years`; `1250.50 EUR` reads too,
  and a number badly grouped is an error that says what to write: `write 125, not "1 25"`. A number followed by a
  currency code is an amount (`total > 1 EUR`), unless the schema declares a unit of that name.
- Arithmetic keeps units. Money adds to money of the same currency only; a quantity converts between the built-in
  units of one dimension (`500 g + 1 kg` is `1 500 g`), and a unit a schema declares converts to none. A sum of
  nothing is a zero without a currency or a unit, which adds to and compares with any amount; a percentage
  applies to what it multiplies. A date moves by a duration (`le + 3 months`); a date and time moves by whole
  months only.
- `if c then a else b`, `x ?? y` (`y` when `x` is none), `o?.f` (none when `o` is), `o with { f: v }` (a copy of
  `o`), `and`, `or`, `not` and comparisons.
- Builtins: `today`, `none`, `yes`, `no`; `sum(xs)` and `sum(xs, x => …)`, `count(xs)` and
  `count(xs, x => …)`, `round(x, n)`, `abs(x)`, `min(…)`, `max(…)`. Methods on arrays: `xs.filter(x => …)`,
  `xs.map(x => …)`, `xs.max-by(x => …)`.
- A function (`fn`) may give its parameters defaults. A field computed from itself, through the objects it links
  to, is an error, and so is a division by zero.

## Writing a document

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
- `-n` checks and writes nothing: an editor's lint of an unsaved text, with what it would break elsewhere.
- `-format json` prints `{"ok", "written", "checked", "diagnostics"}` for a program, each diagnostic as
  `{"file", "line", "col", "severity", "message"}`.
- It exits with 0 when it wrote (or, with `-n`, would write), 1 when it found an error or could not write, 2 when it
  is misused. Hidden folders are not searched for documents.

## SQL

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

## Editors

`editors/zed` is an extension for the Zed editor: it colours `.kanonarch` schemas, outlines their declarations
and fields, and indents them; `.kan` documents are read as Markdown. Its grammar is
`editors/tree-sitter-kanonarch`, a tree-sitter grammar that follows the Go parser of `internal/schema`. To install
it, open the command palette in Zed, run `zed: install dev extension` and choose the folder `editors/zed`.

`kan lsp` is the language server the extension runs: install `kan` on the `PATH` first
(`go install ./cmd/kan`), or name it in Zed's settings under `lsp.kan.binary.path`. It checks the open documents
and schemas as they are typed, with the diagnostics of `kan lint`, and answers:

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

Zed builds the grammar from the repository named in `editors/zed/extension.toml`, at the commit `rev`, never
from the working tree. After a change to the grammar:

```
$ cd editors/tree-sitter-kanonarch
$ tree-sitter generate && tree-sitter test
$ for f in $(find ../../cases -name '*.kanonarch'); do tree-sitter parse -q "$f"; done
```

The loop reports `cases/errors-schema-label/broken.kanonarch` only: it holds labels on purpose, which the grammar
refuses as the parser does. Then commit and push, set `rev` to that commit, and rebuild the extension from Zed's
extensions page.

Obsidian reads `.kan` documents through the plugin in `editors/obsidian`: it opens them as Markdown and follows a
`[[link]]` from the document's folder, with or without `.kan`, showing only its name: `[[../customers/hanar]]`
reads "hanar". To install it in a vault, link the folder into the
vault's plugins and turn on community plugins:

```
$ ln -s ../../../editors/obsidian cases/.obsidian/plugins/kanon
```

## Cases

`cases/` holds real use cases, one folder each: a schema, documents, and `expected.txt`, the exact output of the
command on its first line (an argument with spaces is in quotes). `go test ./internal/cli` runs them all; `go test ./internal/cli -update` rewrites the
expected outputs after a deliberate change. The content of a case is in its own language (French here); everything
that belongs to the language is English.

| Case | What it shows |
|---|---|
| `recipe/` | key lines, units, a list read through a `reads` pattern, numbered steps |
| `invoice/` | a table of objects, a sub-object, a link to another document, written computed fields |
| `lease/` | clauses as `@sections`, `{field}` inside prose, links across documents and back |
| `inventory/` | nested sections, enums, versions, a link to a rule of the schema |
| `car/` | an array as a folder: one file per entry |
| `annonce/` | links and lists of links to other documents, inverses, photos as a folder, a PDF, code, a date and time |
| `northwind/` | the Northwind database, as `kan import` writes it from `create.sql` |
| `northwind-query/`, `northwind-query-join/` | `kan sql -q` over Northwind: a revenue per customer read from a file, a join through a link column |
| `jours-feries/` | an array as a CSV file: the public holidays of France, Etalab's open data |
| `write-dependent/` | `kan write -n` refusing a reading list whose new text breaks the review that links it |
| `errors-*/` | the same schemas with mistakes, and what `kan lint` says about them; `errors-eval/` and `errors-schema/`, mistakes in computations, when a document is evaluated and when its schema is checked; `errors-northwind-query*/`, mistakes in a query |

## Names and ids

1. **Names** are ASCII slugs: lowercase letters, digits and hyphens (`prix-unitaire`, `controle-technique`). This
   covers fields, enum values, rules, functions and methods (`max-by`). Type names are PascalCase ASCII
   (`EtatDesLieux`). Because a hyphen belongs to the name, subtraction needs spaces: `total-ht` is a name,
   `total - ht` is a subtraction. A `.kanonarch` file is ASCII from end to end, strings and comments included:
   `reads fr "le depot de garantie ne depasse pas un mois de loyer"`, not "dépôt". A character outside ASCII is
   reported at its position, with the letter to write.
2. **Every object has an id**, a slug of the same shape. A UUID is valid; a meaningful slug is better. Plain values
   (the steps of a recipe) are not objects and have no id.
3. An id is **always written**, never derived from a title:

   | Where the object is | How its id is written |
   |---|---|
   | a whole file | the file name: `f-2026-031.kan` is the object `f-2026-031` |
   | a section | `{#id}` at the end of the heading: `## Bailleur {#claire-martin}` |
   | a table row | the `id` column |
   | a list item | `{#id}` at the end of the item: `- 4 œufs {#oeufs}` |

   An id is unique within its file. The rows of a spreadsheet are objects of the `.kan` file that links to it:
   their ids share its namespace, and a link in a cell is relative to its folder. An entry of a folder is a file
   of its own, linked to by its path (`[[clio/entretiens/2025-06-10-vidange]]`), so `categories/1.kan` and
   `employees/1.kan` don't clash.
4. **A schema holds no label.** A field, a schema, an enum value or a unit is known by its name alone; a string
   after one is an error that says to remove it. Wherever a message names an enum value, it shows the name.
5. In a document, **a heading finds its field by name**: its text is folded (lowercase, accents removed, spaces,
   hyphens and apostrophes alike) and compared with the field's name, so "Dépôt de garantie" opens
   `depot-de-garantie`. **Keys, table headers and enum values are the names themselves** (`etat: use`), matched
   exactly, with no tolerance for case or accents.

## Files and folders

6. The front matter holds one key, `type:`, the schema of the document. The schema file is given to `kan lint`.
7. **An array can be written inline** (a table, a list, sections) **or as a folder** holding one `.kan` file per
   entry. The folder is a sibling of the object's file and is named after the field: the entries of `clio.kan`'s
   field `entretiens` are `clio/entretiens/*.kan`. An array of objects can also be **a spreadsheet**, linked to
   on a key line (`jours: [[jours-feries-metropole.csv]]`, `produits: [[catalogue.xlsx#produits]]` for one sheet
   of a workbook, its first sheet without `#`) or alone under the field's heading. Its first row holds the `id`
   column and field names; every other row is an entry, its cells in the canonical format, checked like a table.
8. A file inside such a folder needs no front matter: its position gives its type (`Vehicule.entretiens` is
   `[Entretien]`). Its id is its file name.
9. Entries of a folder are ordered by id, in lexical order: prefix ids with a date (`2025-06-10-vidange`) or a
   number (`-001`) to control the order.
10. A field is written in one place only: inline, as a folder or as a spreadsheet, never two of them.

## How a document is read

11. A `key: value` line fills a field of the current object.
12. A heading whose folded text equals a field's name (rule 5) opens that field: a sub-object (with its own key lines and an
    id), a list, a table or prose. A field that holds a single value is always a key line.
13. Under a field heading, a table fills an array of objects (one column per field, plus `id`); a list fills an
    array of values, or of objects when their schema has a `reads` pattern; paragraphs fill a `Prose` field. The
    value of a `Prose` field is its text as written: the lines of a paragraph stay separate lines, and paragraphs
    are separated by a blank line. A renderer may reflow it; the data doesn't.
14. Any other heading becomes an object of the field marked `@sections`: the heading text fills the field marked
    `@heading`, the content fills the field marked `@body` or the object's own fields.
15. The H1 is the document's title; it fills the root field marked `@heading`, if there is one. Only the front
    matter comes before it.
16. `{field}` inserts a value into prose, looked up in the current object, then in the objects that hold it.
    `[[path#id]]` links to a document, an object or a declaration of a schema; `[[path.ext]]` links to a file and
    `![[path.ext]]` embeds one. The path is relative to the document's folder, without `.kan`. Markdown links
    with a relative path, `[text](path)` and `![alt](path)`, are checked the same way, and `![alt](path)` must be
    an image; links to another site (`https:`, `mailto:`) or within the page (`#top`) are not checked.
17. Computed fields (`= …` in the schema) need not be written. When one is, it must hold its computed
    value. An inverse (`<- f`) is never written: it is computed from the documents that link here.
18. A `rule` is a statement that must hold. `today` is supplied by the tool (`-today`), so rules stay pure.
19. Values are written in the canonical English format only, whatever the language of the content: `2026-09-01`,
    `2026-10-25 10:30`, `1 250.50 EUR`, `no`, `3 years`, `48 m2`. A date and time takes seconds and a zone when it
    needs them: `2026-10-25 10:30:05 UTC`, `2026-10-25 10:30 +02:00`. Without a zone it is a floating local time,
    not UTC, and it reads back as written: `2026-10-22 14:30` stays `2026-10-22 14:30`, `+02:00` stays `+02:00`.

## Relations

20. A field whose type is a schema **holds its own object** (`localisation: Localisation`): it is written here,
    under the field's heading with its id (`## Localisation {#impasse-des-vergers}`), and `[T]` as a table, a list,
    sections or a folder. A link there is an error.
21. `-> T` is a **link** to an object written elsewhere, as a key line: `agent: [[agents/sophie-bernard]]`, or
    `[[bail#garantie]]` for an object inside a document. The target must be a `T`. `[-> T]` is a list of links:
    one per item under the field's heading (`- [[agents/marc-petit]]`, with no id), or separated by commas on a
    key line. A table cell may hold a link too.
22. `[S] <- f` and `S? <- f` are **inverses**: the `S` objects whose field `f` links here. Rule 17: never written.

## Files and code

23. `File` and `Image` are **links to files** next to the document: `plan: [[maison-caluire/plan-rdc.pdf]]`. Every
    segment of the path is an id and the extension is lowercase: `Plan RDC.PDF` is reported with the name to give
    it, `plan-rdc.pdf`. The content decides what a file is, not its extension: an `Image` that holds a PDF is an
    error, a `.png` that holds a JPEG image a warning.
24. An array of files (`photos: [Image]`) is a list of file links under its heading, file links separated by
    commas on a key line, or **a folder** holding the files themselves, named after the field:
    `maison-caluire/photos/*.jpg`, in lexical order. It holds no folders; hidden files are skipped.
    An object's folder (`maison-caluire/` next to `maison-caluire.kan`) holds the folders of its arrays and its
    own files, which it links to (`maison-caluire/plan-rdc.pdf`). A folder there that names no array field is an
    error, and so is a `.kan` file that is no entry of a list.
25. `Code` is **one fenced block** under the field's heading, its language as the info string (```` ```html ````),
    or a link to a source file, which must hold text.

## Code

Go, standard library only, except the pure Go SQLite driver `modernc.org/sqlite` in the SQL packages.

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
| `internal/pattern` | `reads` patterns such as `"{quantite} [de] {nom}"` |
| `internal/value` | literals: numbers, money, percentages, quantities, dates and times, durations, versions |
| `internal/names` | slugs, the folding of a heading to a name, and "did you mean" suggestions |
| `internal/diag` | diagnostics |

## Licence

MIT, in `LICENSE`. The data of a case keeps its own licence, given in its `source/README.md`.
