# The Kanon language

A `.kanonarch` file holds the schema: types, units, enums, computed fields and rules, written once by someone
technical. A `.kan` file is the document anyone edits, like Markdown: key lines, headings, lists, tables and prose.
This page defines both, rule by rule; `cases/` holds examples, with what `kan lint` says about them, and
`docs/kan.md` the commands that read them.

## Names and ids

1. **Names** are ASCII slugs: lowercase letters, digits and hyphens (`unit-price`, `vat-rate`). This
   covers fields, enum values, rules, functions and methods (`max-by`). Type names are PascalCase ASCII
   (`CheckInReport`). Because a hyphen belongs to the name, subtraction needs spaces: `net-total` is a name,
   `net - total` is a subtraction. A `.kanonarch` file is ASCII from end to end, strings and comments included:
   `reads en "the deposit covers the decor"`, not "décor". A character outside ASCII is
   reported at its position, with the letter to write.
2. **Every object has an id**, a slug of the same shape. A UUID is valid; a meaningful slug is better. Plain values
   (the steps of a recipe) are not objects and have no id.
3. An id is **always written**, never derived from a title:

   | Where the object is | How its id is written |
   |---|---|
   | a whole file | the file name: `inv-2026-031.kan` is the object `inv-2026-031` |
   | a section | `{#id}` at the end of the heading: `## Landlord {#claire-morton}` |
   | a table row | the `id` column |
   | a list item | `{#id}` at the end of the item: `- 4 eggs {#eggs}` |

   An id is unique within its file. The rows of a spreadsheet are objects of the `.kan` file that links to it:
   their ids share its namespace, and a link in a cell is relative to its folder. An entry of a folder is a file
   of its own, linked to by its path (`[[hatchback/services/2025-06-10-oil-change]]`), so `categories/1.kan` and
   `employees/1.kan` don't clash.
4. **A schema holds no label.** A field, a schema, an enum value or a unit is known by its name alone; a string
   after one is an error that says to remove it. Wherever a message names an enum value, it shows the name.
5. In a document, **a heading finds its field by name**: its text is folded (lowercase, accents removed, spaces,
   hyphens and apostrophes alike) and compared with the field's name, so "Décor and Fittings" opens
   `decor-and-fittings`. **Keys, table headers and enum values are the names themselves** (`condition: worn`), matched
   exactly, with no tolerance for case or accents.

## Files and folders

6. The front matter holds one key, `type:`, the schema of the document. The schema file is given to `kan lint`.
7. **An array can be written inline** (a table, a list, sections) **or as a folder** holding one `.kan` file per
   entry. The folder is a sibling of the object's file and is named after the field: the entries of `hatchback.kan`'s
   field `services` are `hatchback/services/*.kan`. An array of objects can also be **a spreadsheet**, linked to
   on a key line (`holidays: [[bank-holidays-england-and-wales.csv]]`, `produits: [[catalogue.xlsx#produits]]` for one sheet
   of a workbook, its first sheet without `#`) or alone under the field's heading. Its first row holds the `id`
   column and field names; every other row is an entry, its cells in the canonical format, checked like a table.
8. A file inside such a folder needs no front matter: its position gives its type (`Vehicle.services` is
   `[Service]`). Its id is its file name.
9. Entries of a folder are ordered by id, in lexical order, unless the object's file gives the order: **an order
   list**, a list of links alone under the field's heading, one link per entry of the folder
   (`- [[training/modules/welcome]]`). It links to whole files, lists each entry once and leaves none out.
   Without one, prefix ids with a date (`2025-06-10-oil-change`) or a number (`-001`) to control the order.
10. A field is written in one place only: inline, as a folder or as a spreadsheet, never two of them.

## How a document is read

11. A `key: value` line fills a field of the current object.
12. A heading whose folded text equals a field's name (rule 5) opens that field: a sub-object (with its own key
    lines and an id), a list, a table or prose. A field that holds a single value is always a key line.
13. Under a field heading, a table fills an array of objects (one column per field, plus `id`); a list fills an
    array of values, or of objects when their schema has a `reads` pattern; paragraphs fill a `Prose` field. The
    value of a `Prose` field is its text as written: the lines of a paragraph stay separate lines, and paragraphs
    are separated by a blank line. A renderer may reflow it; the data doesn't. An array of values may instead be
    written on a key line, separated by commas: `norms: iso-9001, iso-14001`. An enum value, a number or a date
    never holds a comma (rule 19), but a text may: an array of `Text` or `Prose` is always a list under its
    heading. Among numbers, a comma between two digits is no separator: `1,250` is an error, `1 250` one number,
    `1, 250` two.
14. Any other heading becomes an object of the field marked `@sections`: the heading text fills the field marked
    `@heading`, the content fills the field marked `@body` or the object's own fields.
15. The H1 is the document's title; it fills the root field marked `@heading`, if there is one. Only the front
    matter comes before it.
16. `{field}` inserts a value into prose, looked up in the current object, then in the objects that hold it.
    `[[path#id]]` links to a document, an object or a declaration of a schema; `[[path.ext]]` links to a file and
    `![[path.ext]]` embeds one. The path is relative to the document's folder, without `.kan`; a path that starts
    with `/` starts from the schema's folder (`[[/training#dawson]]`), the same from every document. Markdown links
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

20. A field whose type is a schema **holds its own object** (`location: Location`): it is written here,
    under the field's heading with its id (`## Location {#orchard-close}`), and `[T]` as a table, a list,
    sections or a folder. A link there is an error, except in the order list of a folder (rule 9).
21. `-> T` is a **link** to an object written elsewhere, as a key line: `agent: [[agents/helen-carter]]`, or
    `[[lease#deposit]]` for an object inside a document. The target must be a `T`. `[-> T]` is a list of links:
    one per item under the field's heading (`- [[agents/tom-ashby]]`, with no id), or separated by commas on a
    key line. A table cell may hold a link too.
22. `[S] <- f` and `S? <- f` are **inverses**: the `S` objects whose field `f` links here. Rule 17: never written.

## Files and code

23. `File` and `Image` are **links to files** next to the document: `plan: [[house-elm-grove/ground-floor.pdf]]`. Every
    segment of the path is an id and the extension is lowercase: `Ground Floor.PDF` is reported with the name to
    give it, `ground-floor.pdf`. The content decides what a file is, not its extension: an `Image` that holds a PDF is an
    error, a `.png` that holds a JPEG image a warning.
24. An array of files (`photos: [Image]`) is a list of file links under its heading, file links separated by
    commas on a key line, or **a folder** holding the files themselves, named after the field:
    `house-elm-grove/photos/*.jpg`, in lexical order. It holds no folders; hidden files are skipped.
    An object's folder (`house-elm-grove/` next to `house-elm-grove.kan`) holds the folders of its arrays and its
    own files, which it links to (`house-elm-grove/ground-floor.pdf`). A folder there that names no array field is an
    error, and so is a `.kan` file that is no entry of a list.
25. `Code` is **one fenced block** under the field's heading, its language as the info string (```` ```html ````),
    or a link to a source file, which must hold text.

## Computed fields and rules

A field computed in the schema (`gross-total: Money = net-total + vat`) and a `rule` are expressions. A type error in
one is found when the schema is checked, before any document is read: ``cannot add `Money` and `Int` ``.

- A literal is written as in a document: `1 250.50 EUR`, `15 000 km`, `20 %`, `3 years`; `1250.50 EUR` reads too,
  and a number badly grouped is an error that says what to write: `write 125, not "1 25"`. A number followed by a
  currency code is an amount (`total > 1 EUR`), unless the schema declares a unit of that name.
- Arithmetic keeps units. Money adds to money of the same currency only; a quantity converts between the built-in
  units of one dimension (`500 g + 1 kg` is `1 500 g`), and a unit a schema declares converts to none. A sum of
  nothing is a zero without a currency or a unit, which adds to and compares with any amount; a percentage
  applies to what it multiplies. A date moves by a duration (`start + 3 months`); a date and time moves by whole
  months only. A date compared with a date and time by `<`, `<=`, `>` or `>=` stands for the start of its day, as
  in SQL: `at >= today` holds for any time today, and `==` between them is an error. Against a date and time with
  a zone, the day starts at 00:00 UTC, as a date and time without a zone is read as UTC against one with a zone.
- `if c then a else b`, `x ?? y` (`y` when `x` is none), `o?.f` (none when `o` is), `o with { f: v }` (a copy of
  `o`), `and`, `or`, `not` and comparisons.
- Builtins: `today`, `none`, `yes`, `no`; `sum(xs)` and `sum(xs, x => …)`, `count(xs)` and
  `count(xs, x => …)`, `round(x, n)`, `abs(x)`, `min(…)`, `max(…)`. Methods on arrays: `xs.filter(x => …)`,
  `xs.map(x => …)`, `xs.max-by(x => …)`.
- A function (`fn`) may give its parameters defaults. A field computed from itself, through the objects it links
  to, is an error, and so is a division by zero.
- A rule that fails is reported at its object, in the words of its `reads` sentence when it has one.
