# Cases

`cases/` holds real use cases, one folder each: a schema, documents, and `expected.txt`, the exact output of the
command on its first line (an argument with spaces is in quotes). `go test ./internal/cli` runs them all;
`go test ./internal/cli -update` rewrites the expected outputs after a deliberate change. The content of a case is
in its own language (English or French here); everything that belongs to the language is English.

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
| `ordered-folder/` | a folder whose order is given by an order list, and links from the schema's folder (`[[/formation#durand]]`) |
| `annonce/` | links and lists of links to other documents, inverses, photos as a folder, a PDF, code, a date and time |
| `northwind/` | the Northwind database, as `kan import` writes it from `create.sql` |
| `northwind-query/`, `northwind-query-join/` | `kan sql -q` over Northwind: a revenue per customer read from a file, a join through a link column |
| `jours-feries/` | an array as a CSV file: the public holidays of France, Etalab's open data |
| `write-dependent/` | `kan write -n` refusing a reading list whose new text breaks the review that links it |
| `errors-*/` | the same schemas with mistakes, and what `kan lint` says about them; `errors-eval/` and `errors-schema/`, mistakes in computations, when a document is evaluated and when its schema is checked; `errors-northwind-query*/`, mistakes in a query |

The data of a case keeps its own licence, given in its `source/README.md`.

`cases/` is also an Obsidian vault: see [Obsidian](../docs/kan.md#obsidian).
