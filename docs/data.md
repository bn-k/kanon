# Files, relations, spreadsheets and SQL

Design of the second stage of Kanon (2026-10-22): accept any kind of data (images, binaries, code,
spreadsheets), support relations, and turn Kanon into SQL and back. Choices made: **SQLite first**, a
**pure Go driver** (`modernc.org/sqlite`) allowed in the SQL packages only, **spreadsheets are validated data**, and
**Northwind** is the first database imported.

The principle stays the same: a `.kan` file is text anyone edits; whatever is not text lives in a file next to it.

## 1. New native types

| Type | Written as | Checked |
|---|---|---|
| `File` | a link to a file next to the document: `plan: [[lilas-12/plan.pdf]]` | the file exists and is a regular file |
| `Image` | the same: `photo: [[davolio/photo.jpg]]` | the same, and its content is an image |
| `Code` | a fenced block under the field's heading, or a link to a source file | a single block |
| `DateTime` | `2026-10-22 14:30`, `2026-10-22 14:30:05`, optionally ` UTC` or ` +02:00` | a real date and time |

- A **file link** is a link whose last segment has an extension other than `.kan` and `.kanonarch`. Its path is
  relative to the document's folder, like any link; every segment is an id and the extension is lowercase ASCII
  letters and digits (`photo.jpg`, `plan-rdc.pdf`, `rapport-2026.xlsx`). A file named `Plan RDC.pdf` is reported with
  the name to give it: `name the file plan-rdc.pdf, not "Plan RDC.pdf"`.
- The content decides what a file is, not its extension: `net/http.DetectContentType` on its first 512 bytes, plus
  SVG (an `.svg` whose content holds `<svg`). An `Image` that holds something else is an error
  (`photo.jpg is not an image: it holds application/pdf`). A file whose extension doesn't match its content is a
  warning (`photo.png holds a JPEG image; name it photo.jpg`).
- By convention the files of `davolio.kan` live in its sibling folder `davolio/`, like its folder arrays. The linter
  doesn't require it. Any file may sit in that folder, except a `.kan` file that is no entry of a list: it is an
  error, since no field holds it. So is a subfolder that names no array field of `davolio`.
- `Code`: the language is the info string of the fence (```` ```sql ````) or the extension of the linked file. A
  `Code` field holds one block; two blocks under its heading are an error. A linked source file must hold text.
- An **array of files** (`photos: [Image]`) is written as a list of file links under the field's heading, as file
  links separated by commas on a key line, or **as a folder**: `maison/photos/` holding the files themselves, in
  lexical order of their names.
- In prose, `![[path]]` embeds a file and `[[path.ext]]` links to one; Markdown's `![alt](path)` and `[text](path)`
  with a relative path (no scheme, not `#…`) are checked the same way: the file must exist, and `![alt](path)` must
  hold an image. A query or a fragment after a file (`plan.pdf#page=2`) is not checked. `[text](path)` to a
  document, with or without `.kan`, is checked like `[[path#id]]`.

## 2. Relations

A field that holds a schema is either **owned** (the object is written here) or a **link** (the object is written
elsewhere). The schema says which:

```
schema Employee {
  last-name:   Text
  photo:       Image?
  reports-to:  -> Employee?          // a link to one object, or none
  territories: [-> Territory]        // links to many
  orders:      [Order] <- employee   // the inverse of Order.employee: computed, never written
}
```

- `T` is owned: written under the field's heading (a sub-object), or as a table, a list, sections or a folder for
  `[T]`. Writing a link there is an error that says how to write it.
- `-> T` is a link: `reports-to: [[fuller]]`. `[-> T]` is a list of links under the heading, one per item, or links
  separated by commas on a key line. Writing the object itself there is an error. The target must be a `T`.
- `[S] <- f` (or `S? <- f` for one to one) is the **inverse** of the field `f` of `S`, which must be `-> Self`,
  `-> Self?` or `[-> Self]`. It is computed from the documents, so writing it is an error, like a computed field.
- `->` applies to schemas only: `-> Text` is an error in the schema.
- Existing cases change accordingly: `Facture.client: -> Partie`, `EtatDesLieux.bail: -> Bail`.

## 3. Spreadsheets

An array of objects can live in a CSV or a spreadsheet: write the field as a link to it.

```
mesures: [[maison/mesures.csv]]
produits: [[catalogue.xlsx#produits]]     // #sheet; without it, the first sheet
```

- The first row holds field names (the names themselves, exactly: the same rule as a table's header) and the `id` column; every
  other row is an object, checked like a row of a table.
- Cells are written in the canonical format. An xlsx cell typed by Excel is first turned into its canonical text: a
  number into a decimal (`1250.5`), a date into `2026-10-22` (or a `DateTime`), a boolean into `yes`/`no`, a
  percentage into `20 %`, an amount with a currency format into `1250.50 EUR`.
- A diagnostic points at the cell: `mesures.csv:4:12: error: …` (line, column in characters) and
  `catalogue.xlsx#produits:4:3: error: …` (row, column number) with the cell's name in the message when it helps
  (`C4`).

## 4. SQL (SQLite)

### `kan sql <schema.kanonarch> <document.kan> [-o out.sql|out.sqlite|out.db]`

Lints first; on an error it prints the diagnostics and writes nothing. Otherwise it prints SQL (`BEGIN;`,
`CREATE TABLE …`, `INSERT …`, `COMMIT;`), or writes a SQLite database when `-o` names a `.sqlite` or `.db` file.

| Kanon | SQLite |
|---|---|
| a schema | a table, its name in snake_case: `EtatDesLieux` → `etat_des_lieux` |
| a field | a column in snake_case: `prix-unitaire` → `prix_unitaire` |
| `id` | `id TEXT NOT NULL`, the primary key (see weak schemas) |
| `Text`, `Prose`, `Code`, `Duration`, `Version` | `TEXT`, in canonical form |
| `Date`, `DateTime` | `DATE`, `DATETIME`: the text in canonical form (`2026-10-22 14:30:05 +02:00`), which SQLite stores as `TEXT` |
| `Int` | `INTEGER` |
| `Number`, `Percent` | `NUMERIC` (the number as written: `20 %` is 20) |
| `Money` | `x NUMERIC` and `x_currency TEXT` |
| `Quantity` | `x NUMERIC` and `x_unit TEXT` (NULL without a unit) |
| `Bool` | `INTEGER CHECK (x IN (0, 1))` |
| an enum | `TEXT CHECK (x IN ('neuf', 'bon', 'use'))` |
| `File`, `Image` | `BLOB`, the file's content |
| `T` (owned) or `-> T` | `x_id TEXT REFERENCES t (id)` |
| `[T]` (owned) | rows of `t` with a column `<parent>_id` (`ligne.facture_id`); `<parent>_<field>_id` when the parent has two such fields |
| `[-> T]` | a join table `<parent>_<field>` (`employee_territories`) with `<parent>_id` and `<t>_id` |
| `[Text]`, `[Image]`, … (values) | a table `<parent>_<field>` with `<parent>_id` and `value` |
| `S <- f`, a computed field | nothing: they are derived |

- A required field is `NOT NULL`. Every foreign key is `DEFERRABLE INITIALLY DEFERRED`, so rows are inserted in
  document order within one transaction, and SQLite keeps that order (`ORDER BY rowid`).
- A `Prose` value is its text as written: a newline between the lines of a paragraph, a blank line between
  paragraphs. `Coventry House\nMiner Rd.` reads back with its newline. The language of a `Code` or `Prose` value is
  lost: its column holds the text only.
- `DATE` and `DATETIME` tell SQLite tools what a column holds. They give the column NUMERIC affinity, but a date
  written `2026-10-22` is not a number, so SQLite keeps it as `TEXT`, zone and seconds included.
- A **weak schema** is one only ever held by one owned array field and never linked to nor held by a single field
  (`Ligne`, only in `Facture.lignes`). Its ids are unique within its parent only, so its primary key is
  `(<parent>_id, id)`. Every other schema's primary key is `id`, and `kan sql` reports two objects of the same schema
  with the same id.
- A **collection** is a schema whose fields are all arrays of objects (the root of an imported database). It has no
  table, and its entries no parent column.

### `kan sql -q <query|@query.sql> [-format table|csv] <schema.kanonarch> <document.kan>`

Lints like `kan sql`, builds the same tables in memory and runs one query over them, read only. The mapping above is
what the query sees, with one difference: `DATE` and `DATETIME` columns are declared `TEXT` there, since the
driver would give a `DATE` as a time and lose the text as written (`2026-10-22 14:30:05 +02:00`); the stored values
are the same. So a query reads values as they are stored: a `Money` is `x` and `x_currency`, a `Quantity` `x` and
`x_unit`, a `Bool` 0 or 1, a link the id of its target (`order.customer_id`, `order_detail.product_id`).

- A query is one statement starting with `SELECT`, `WITH`, `VALUES` or `EXPLAIN`; another first word, a second
  statement after a `;`, or a `WITH … DELETE` is refused. The check of the first word comes first because a
  `PRAGMA query_only = OFF` would undo the read-only mode of the connection.
- The table is aligned on the characters of each cell; a column whose values are all numbers is aligned to the
  right, with its header. A float is written in full (`0.30000000000000004`, never `1e+20`), so `round()` what you
  show. A newline in a text is written `\n`, a BLOB is `<blob N bytes>`, NULL is `NULL`.
- SQLite's errors are said in the project's words, at the position of the name: `no such column` and
  `no such table` with the closest name of the schema (`names.Closest`), `ambiguous column name` with the table to
  write before it, `syntax error near` with the table name that is a keyword (`order`) told to be quoted.

### `kan import <database.sqlite|dump.sql> <out-dir>`

Writes a schema and a tree of documents from a database. `out-dir` must not exist or be empty; its base name is the
name of the result (`northwind/` → `Northwind`).

```
northwind/
├── schema.kanonarch
├── northwind.kan                  type: Northwind, a collection: one array field per entity table
└── northwind/
    ├── customers/alfki.kan
    ├── orders/10248.kan           its order details as a table
    └── employees/1.kan + employees/1/photo.jpg
```

- **Tables.** A table whose primary key is all its columns, two foreign keys, is a **join table**: a `[-> B]` field
  of `A`, named after `B`'s table (`territories`). A table whose primary key holds a foreign key to `P` plus other
  columns is **weak**: an owned `[T]` field of `P` named after the table (`Order Details` → `order-details`), written
  as a table in `P`'s documents; its id is made of the other key columns. Every other table is an **entity**: an
  array field of the collection, written as a folder.
- **Names.** Tables become singular PascalCase schemas (`Order Details` → `OrderDetail`, `Categories` →
  `Category`: `-ies` → `-y`, a final `-s` dropped), and the field of a table is plural whether the table is or not
  (`Categories` and `category` both give `categories`); columns become kebab-case fields (`CompanyName` →
  `company-name`); a foreign key column loses its `ID` suffix (`CustomerID` → `customer: -> Customer`). Each foreign
  key gets its inverse on its target (`Customer.orders: [Order] <- customer`); when two would share a name, the
  second is `<table>-by-<field>`.
- **Ids.** A single-column primary key gives the id, lowercased and slugged (`ALFKI` → `alfki`, `10248` →
  `10248`); a composite key joins its slugs with `-`; a table without a key uses its rowid. Two rows giving the same
  id are an error.
- **Types.** `INTEGER` → `Int` (`Bool` when the column is declared `BOOLEAN`/`BIT` or checked `IN (0, 1)`); `REAL`,
  `NUMERIC`, `DECIMAL` → `Number`; `DATE`/`DATETIME` → `Date` when every value is at midnight, else `DateTime`;
  `BLOB` → `Image` when every value is an image, else `File`, each value written to a file named after its field,
  its extension from its content; text → `Text`, or `Prose` when a value spans several lines. A nullable column is
  optional (`?`).
- **Round trip.** `kan import` then `kan sql` gives back the same data, but for what the import changes and notes:
  keys lowercased into ids, Windows line breaks written as `\n`, the spaces around a text and at the ends of its
  lines, extra blank lines, and an empty text, which is no value. Importing that result gives back the same Kanon
  files, byte for byte: the schema's header, `// Generated by kan import.`, names no source (a case says where its
  data comes from in `source/README.md`). `TestRoundTrip` (`internal/sqlexport`) checks both on Northwind.
- **Fidelity.** That exact round trip holds for the schemas `kan import` writes: `Text`, `Prose`, `Int`, `Number`,
  `Bool`, `Date`, `DateTime`, `Image`, `File` and relations. Any other schema exports, but `Duration`, `Money`,
  `Quantity`, enums and `Code` come back from SQL as plain columns (`Text`, `Number`), and the language of `Code`
  and `Prose` is lost. Embedding the schema in the database would make the round trip exact for any schema; it is a
  later step.

## 5. Order of the work

1. Language: `->`, `[-> T]`, `<-`, `File`, `Image`, `Code`, `DateTime`, file links (schema, value, doc, lint).
2. In parallel: `internal/sheet` (CSV and xlsx reading) and `internal/sqlimport` (`kan import`).
3. The bound model of a document, then `kan sql`; spreadsheets as array sources in the linter.
4. The round-trip test over Northwind.
