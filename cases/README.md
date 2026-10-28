# Cases

`cases/` holds use cases, one folder each: a schema, documents, and `expected.txt`, the exact output of the
command on its first line (an argument with spaces is in quotes). `go test ./internal/cli` runs them all;
`go test ./internal/cli -update` rewrites the expected outputs after a deliberate change. Their people, places and
companies are fictional, and their content is English, except data published by others, kept as published.

| Case | What it shows |
|---|---|
| `board/` | the example of the README: a board of tasks as a folder, and `kan write -n` refusing a task an agent got wrong |
| `board-rule/` | `kan write -n` refusing a valid task that breaks a rule of another document: one task at a time |
| `board-query/` | `kan sql -q` over the board: the open tasks with their owner, by due date |
| `recipe/` | key lines, units, a list read through a `reads` pattern, numbered steps |
| `invoice/` | a table of objects, a sub-object, a link to another document, written computed fields |
| `lease/` | clauses as `@sections`, `{field}` inside prose, links across documents and back |
| `inventory/` | nested sections, enums, versions, a link to a rule of the schema |
| `car/` | an array as a folder: one file per entry |
| `ordered-folder/` | a folder whose order is given by an order list, and links from the schema's folder (`[[/training#dawson]]`) |
| `listing/` | links and lists of links to other documents, inverses, photos as a folder, a PDF, code, a date and time |
| `meetings/` | dates and times ordered against `today`, a date that stands for the start of its day: a meeting held yesterday fails a rule |
| `northwind/` | the Northwind database, as `kan import` writes it from `create.sql` |
| `northwind-query/`, `northwind-query-join/` | `kan sql -q` over Northwind: a revenue per customer read from a file, a join through a link column |
| `public-holidays/` | an array as a CSV file: the bank holidays of England and Wales, GOV.UK's open data |
| `suppliers/` | arrays of enum values and of dates on a key line, separated by commas, read by computed fields and a rule; a list of texts under its heading |
| `suppliers-query/` | `kan sql -q` over the suppliers: the last audit of those that hold a norm, through the table of an array of values |
| `write-dependent/` | `kan write -n` refusing a reading list whose new text breaks the review that links it |
| `invoice-json/`, `board-json/`, `recipe-json/` | `kan eval -format json` over the invoice, the board and the recipe: every object with its values, canonical and typed, and where each is written; table rows, a sub-object, list items, folder entries, inverses |
| `meetings-json/` | `kan eval -format json` on a document whose rule fails: the values still come, with `"ok": false` and exit code 1 |
| `errors-invoice-json/` | `kan lint -format json` over the invoice with mistakes |
| `set-board/`, `set-invoice/`, `set-recipe/` | `kan set -n -format json`: key lines changed and added in a task of the board's folder; a table cell, a key line added to a sub-object and a written computed field in the invoice; parts of list items read through a `reads` pattern, and a key line removed, in the recipe |
| `set-garden/` | `kan set` in a table aligned by hand: the column widened on every line, a cell emptied |
| `set-board-rule/`, `set-wrong-value/` | `kan set -n` whose new text breaks a rule of the board, and one whose value is not a date |
| `set-refused/`, `set-refused-task/`, `set-refused-invoice/` | what `kan set` refuses, with what to do instead: an unknown id or field, a required field removed, an inverse, objects, a folder, prose, computed fields |
| `errors-*/` | the same schemas with mistakes, and what `kan lint` says about them; `errors-eval/` and `errors-schema/`, mistakes in computations, when a document is evaluated and when its schema is checked; `errors-northwind-query*/`, mistakes in a query; `errors-heading-close/`, headings a letter off a field's name, each reported once as the field it stands for, and one far from every name, reported with the field it leaves out |

The data of a case keeps its own licence, given in its `source/README.md`.

`cases/` is also an Obsidian vault: see [Obsidian](../docs/kan.md#obsidian).
