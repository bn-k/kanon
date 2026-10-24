# A Kanon folder as a database: `kan serve`

Design of the third stage of Kanon (2026-10-23): a folder of Kanon documents is a database that three kinds of
users share.

| Who | Reads and writes through | Checked by |
|---|---|---|
| people | an editor: Obsidian, Zed, any IDE | `kan lint`, run by the editor after each change |
| language models | the files themselves, as Markdown | `kan lint`, run by the agent after each change |
| programs | SQL, over the protocol of PostgreSQL | `kan serve`, which lints every write before it lands |

The files stay the only source of truth. Nothing else is stored: no database file, no cache to keep in step.
Whoever writes, the folder stays one that `kan lint` accepts.

## 1. `kan serve`

```
$ kan serve [-addr 127.0.0.1:5433] [-today YYYY-MM-DD] <schema.kanonarch> <document.kan>
kan serve: 6 tables, 412 rows, on 127.0.0.1:5433
$ psql -h 127.0.0.1 -p 5433 -c 'SELECT name FROM person ORDER BY _position'
```

- It lints the document first; on an error it prints the diagnostics and does not start (exit 1).
- It speaks the frontend/backend protocol of PostgreSQL (version 3), so `psql`, psycopg, DBeaver, Metabase and the
  drivers of most languages connect to it. Any user and database name are accepted. It listens on the loopback
  address by default; with `-password-file`, a client must give the password the file holds (cleartext over the
  connection: keep it on the loopback, or behind TLS that something else ends).
- The tables are those of `kan sql` (`docs/data.md` §4), held in memory, with one column more (§3).
- Queries run on SQLite, so they are written in **SQLite's dialect**, not PostgreSQL's: `$1` parameters work,
  `ILIKE` doesn't. The few catalogue queries clients send on connecting (`SELECT version()`, `SHOW …`, `SET …`) are
  answered so that they connect.
- It reads the folder again when a file changes on disk (§5).

## 2. Reading

What a query sees is what `kan sql -q` sees, with the types a PostgreSQL client expects. A column's type comes from
its field:

| Kanon | Column | PostgreSQL type (OID) | A client reads |
|---|---|---|---|
| `Text`, `Prose`, `Code`, `Duration`, `Version`, an enum, an id, a link | `TEXT` | `text` (25) | `"ana"` |
| `Int` | `INTEGER` | `int8` (20) | `3` |
| `Number`, `Percent`, `Money`, `Quantity` | `NUMERIC` | `numeric` (1700) | `Decimal("1250.50")` |
| `Bool` | `INTEGER`, 0 or 1 | `bool` (16) | `True` |
| `Date` | `TEXT` | `date` (1082) | `date(2026, 10, 22)` |
| `DateTime` with a zone | `TEXT` | `timestamptz` (1184) | an aware datetime |
| `DateTime` without a zone | `TEXT` | `timestamp` (1114) | a naive datetime |
| `File`, `Image` | `BLOB` | `bytea` (17) | the bytes of the file |

A computed column (`count(*)`, `a || b`) takes the type of its first value. Values are sent as text, as
PostgreSQL's text format writes them (`t`, `2026-10-22 14:30:05+02`), so a client reads them with no conversion of
its own.

## 3. Order: `_position`

The rows of an array keep the order of the document. Every table whose rows are held by an array has one more
column, `_position INTEGER`: the place of the row in the array that holds it, from 0, or `NULL` for an object no
array holds (a document of its own). Its name starts with an underscore, which no Kanon name can, so it never
clashes with a field.

```sql
SELECT name FROM step WHERE phase_id = 'design' ORDER BY _position;
INSERT INTO step (id, phase_id, _position, name, kind) VALUES ('review', 'design', 2, 'Review', 'milestone');
UPDATE step SET _position = 0 WHERE id = 'review';          -- move it first
UPDATE step SET phase_id = NULL, step_id = 'mock-ups' WHERE id = 'review';   -- move it into another step
```

- An `INSERT` without `_position` adds the row at the end of its array; with one, before the row that held that
  place. The other rows of the array shift; `_position` is always 0, 1, 2… with no gap.
- Changing `_position`, or the column of the parent (`phase_id`, `step_id`), moves the object, with what it holds.
- A table of links (`[-> T]`) and a table of values (`[Text]`) have it too: the place of the link or the value in
  its list.

## 4. Writing

`INSERT`, `UPDATE` and `DELETE` (with `RETURNING`, `ON CONFLICT`, in a transaction or not) are run on the tables in
memory, then turned into edits of the files:

1. The statement runs in a SQLite transaction. Triggers record every row it inserts, changes or deletes.
2. At the end of the statement, or at `COMMIT` in a transaction, the recorded rows become changes to the objects:
   a value set or cleared, an object added, moved or removed, a link added or removed.
3. The changes become edits of the files, each one as small as the change: the value of a key line, a cell, a
   row, a section, a file. What a person wrote around it, the prose, the order of the lines, the alignment of a
   table, stays as it was.
4. The edited files are linted, from memory, before anything is written. If lint finds an error, the transaction
   is rolled back and nothing is written; the client gets an error (§6) with every diagnostic, `file:line:col` in
   the edited files.
5. Otherwise the files are written (each to a temporary file, then renamed over the old one), and the tables are
   built again from them.

So a program cannot leave the folder in a state a person would have to repair: a value that doesn't parse, a
missing required field, a link to nothing, a rule that fails, are refused as they would be in an editor.

### How each change is written

An object is written where its array already is, in the same form; an array written nowhere yet takes the form the
same field has in another object, else a table when every field of its schema fits in a cell, else sections, else a
folder.

| Change | Key lines, `@heading`, `@body` | Table | Sections | Folder |
|---|---|---|---|---|
| set a value | the value of its key line, or a new key line after the others | the cell; a new column when the table has none | its key line or the section of its field | its key line or the section of its field |
| clear a value | the key line goes | the cell is emptied | the key line or section goes | the key line or section goes |
| add an object | | a row, at its place | a section, at its place, its id in `{#id}` | a file, `<id>.kan`, and a link in the order list (§4.1) |
| remove an object | | its row | its section, with what it holds | its file and its folder |
| move an object | | its row moves | its section moves | its link moves in the order list; to another parent, the file and its folder move |
| change an id | | the `id` cell | the `{#id}` | the file and its folder are renamed |

- A value is written in the canonical format (`1 250.50 EUR`, `yes`, `2026-10-22`), from what the column holds: a
  `Money` from its two columns, a link from the id of its target, written as a path from the document's folder
  (`[[#ana]]` in the same file, `[[clients/harbour-builders]]`, or `[[/board#ana]]` from the root folder when the path
  would climb out of it, §7).
- A `Prose` value is written as its text, under its heading; a `Code` value as a fenced block.
- A list item read through a `reads` pattern is written again through the first pattern that fits its values.
- An object that holds others written as files moves with its folder.
- A file that a value set to `NULL` linked to (`File`, `Image`) stays where it is: `kan serve` never deletes a file
  that is not a `.kan` document. Writing a `BLOB` is refused: a file is added on disk, then linked to.
- What a form cannot hold is refused with `feature_not_supported` and says how to write it by hand: a row of a
  spreadsheet (`.csv`, `.xlsx`), which `kan serve` does not edit, or a section deeper than `######`.

### 4.1 Order lists

A folder is in the order of its file names (rule 9 of `docs/language.md`). An array that `kan serve` must keep in another
order gets an **order list**: under the field's heading, in the document that owns the folder, a list of links to
its entries.

```
# Website launch

## Phases

- [[launch/phases/brief]]
- [[launch/phases/design]]
- [[launch/phases/build]]
```

The list gives the order of the entries; every entry of the folder is in it once, and it links to nothing else.
`kan lint` reports an entry left out, a link to a file that is not an entry, and a link written twice. In
Obsidian the list is a table of contents, each link opens its entry. `kan serve` writes one when an `INSERT` or a
change of `_position` asks for an order that the names don't give.

## 5. Changes made on disk

`kan serve` checks the files every half second (their size and modification time) and builds the tables again when
one changed, once the folder lints. While it doesn't, the tables stay as they were, writes are refused with the
diagnostics, and reads go on.

A transaction that began before such a change is refused at its first write after it, or at `COMMIT`, with
`serialization_failure` (`40001`): the client runs it again on the new tables. Two writes from SQL never overlap:
one runs at a time.

## 6. Errors

| SQLSTATE | When |
|---|---|
| `23514` `check_violation` | lint refused the edit; `DETAIL` holds every diagnostic, one per line |
| `23505`, `23503`, `23502` | a unique key, a foreign key, a required column, as SQLite checks them |
| `0A000` `feature_not_supported` | a change no form can hold (§4) |
| `40001` `serialization_failure` | the files changed on disk during the transaction (§5) |
| `42601`, `42P01`, `42703` | a syntax error, an unknown table, an unknown column, with "did you mean …?" |
| `XX000` `internal_error` | a file cannot be read or written |

## 7. Links from the root

A link path that starts with `/` is read from the root folder: the folder of the schema. `[[/board#ana]]`
names the object `ana` of `board.kan` next to the schema, from any document below it. `kan serve` writes it
when a relative path would need `..`; a person writes whichever reads better.

## 8. Lint after every edit

People and models edit the files directly; each of them lints after each change, so a mistake is seen where it was
made and not by the next program that reads the folder.

- **Zed** lints as one types, through `kan lsp` (`docs/kan.md`, Zed).
- **Obsidian**: the plugin in `editors/obsidian` runs `kan lint` when a `.kan` file is saved and shows the
  diagnostics in a notice and in the gutter of the line.
- **Language models**: the folder holds an `AGENTS.md` (or `CLAUDE.md`) that says how the documents are written
  and to run `kan lint` after each edit; `editors/claude/kan-lint-hook` is a hook for Claude Code that runs it
  after every edit of a `.kan` file and gives the diagnostics back to the model.
- **Programs** go through `kan serve`, which lints before writing.

## 9. Clients

Any PostgreSQL driver works. `clients/python` is a thin layer over psycopg 3, imported as `kanon` and published
under another name, since `kanon` is taken on PyPI:

```python
import kanon

with kanon.connect("postgresql://127.0.0.1:5433/board") as db:
    people = db.execute("SELECT * FROM person ORDER BY _position").fetchall()   # dicts
    try:
        db.execute("UPDATE task SET due = %s WHERE id = %s", ["next friday", "2026-10-30-pricing-page"])
    except kanon.LintError as e:
        for d in e.diagnostics:            # file, line, col, severity, message
            print(d)
```

- `kanon.connect` returns a psycopg connection whose rows are dicts and which never prepares statements on the
  server.
- `kanon.LintError` is raised for `23514` and holds the diagnostics, parsed from `DETAIL`.
- `kanon.serve(schema, document)` starts `kan serve` on a free port for the time of a `with` block: for tests.

## 10. Order of the work

1. Lint: links from the root (`/`), order lists, and reading the files through an interface, so that edits are
   linted from memory before they are written.
2. `internal/edit`: changes to objects → edits of the files, with the cases of `cases/serve-*`.
3. `internal/sqlexport`: the tables in memory with `_position`, the triggers that record writes, and the
   recorded rows turned into changes.
4. `internal/pgwire`: the protocol (start-up, simple and extended queries, errors), and `kan serve`.
5. `clients/python`, the hook for Claude Code, lint on save in Obsidian.
