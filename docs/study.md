# Kanon — initial study (2026-10-22)

The study that opened the project, kept as written but for its links. What has been decided since is in
`docs/decisions.md`; the language as it stands is in `docs/language.md`.

**Kanon** (`.kan`) is a language for documents as code. It describes any structured document (a contract, a
company profile, a quality system, an equipment inventory, a balance sheet, a database) as typed code: schema,
data, prose, references and rules in a single source.

It is code, but **human-readable first**: someone who has never programmed must be able to edit a Kanon document
the way they edit Markdown.

Name: Greek κανών, "the rule", and the canon, the body of texts that holds authority. Extension `.kan` for documents
(only known use: annotations from Kwalitan, a Dutch text-analysis tool; no programming language). Extension
`.kanonarch` for schemas and rules: in Byzantine liturgy, the canonarch (κανονάρχης) intones each verse of the canon
before the choir sings it; the schema sets the tone, documents follow (no known use; a Go tool named `kanonarion`
exists). To check before freezing: GitHub, npm, Go modules, registered trademarks.

## Language

Kanon is universal, so **everything about the language is in English**: keywords, native types, standard library,
documentation, tooling. Content stays in its author's language: a French tender is written in French, with English
keywords around it.

The domains listed here (French public tenders, ISO 9001, accounting…) are **working material**, not part of the
language. The core knows nothing about any domain.

## The request

- A document has a schema, like OpenAPI or GraphQL, and can contain sub-documents.
- References must work inside the code itself.
- Functional concepts: `less-than(a, b) -> Bool`, "amount a is less than b", with its implementation; used in
  business rules.
- Generated tooling: parse the code, convert it to TS, Python and Go objects, find references, etc.
- Target examples: a complete tender file, a company profile, ISO 9001, an equipment inventory, a balance sheet,
  a database.

## Prior art

Every piece exists; nobody combines prose, typed data, checked references and pure rules in documents a non-developer
can edit.

| Family | Languages | What they do | What's missing |
|---|---|---|---|
| Typed data | CUE, Pkl (Apple), Nickel, Dhall, KCL | unified schema and data, internal references, pure functions; Pkl generates Go, Swift, Kotlin | no prose: configuration, not documents |
| Schemas / IDL | TypeSpec, Smithy, GraphQL SDL, LinkML | schemas and type generation (TS, Python, SQL) | no data, no prose, no rules |
| Contracts and law | Accord Project (Concerto, TemplateMark, Ergo), Catala (Inria, French tax law), L4, Lexon | legal text tied to executable code | Accord: contracts only, Ergo "early in development"; Catala: statutes, not structured documents |
| Business rules | DMN/FEEL, CEL (Google), Rego (OPA), Drools | evaluable business expressions | no document model |
| Documents | XML + XSD + ID/IDREF, DITA, Markdoc (Stripe), MDX, Typst | structured text, cross-references | weak typing, no business functions; XML and DITA are not editable by non-developers |
| Structured notes | Obsidian (properties, `[[wikilinks]]`, Dataview), Org-mode | Markdown-like text with fields and links, edited by non-developers | no schema, no type checking, links not verified |
| Domain standards | XBRL, Beancount (accounting), OSCAL (compliance), Structurizr (architecture), DBML, Prisma (databases) | established standards | each in its own silo |

Closest neighbours: Accord Project, CUE/Pkl, Catala for the semantics; Markdoc and Obsidian for the editing
experience.

## Principles

1. **Human-readable first.** A document reads like Markdown: headings, paragraphs, `key: value` lines. A file
   without a schema is still valid.
2. **Composable schemas**, GraphQL-style: a document contains sub-documents, each of which can live in its own file.
3. **Prose as a first-class type** (`Prose`), with references and expressions inside the text.
4. **Typed references, checked at compile time**: stable ids (`#penalites`), not positions. A broken reference is
   an error; backlinks are computed (who cites what).
5. **Native business types**: `Money` as a decimal (never a float) with currency and tax basis, `Date`, `Duration`,
   `Percent`, `Quantity` with a unit.
6. **Pure functions with their sentence**: the same rule reads in the PDF and runs in the code. The sentence can
   exist in several languages.
7. **Tooling**: TS / Python / Go generation, `kanon check` (types, references, rules), `kanon refs` (backlinks),
   PDF/DOCX rendering (via Typst), language server (LSP), MCP server for agents.

## Two layers, two kinds of authors

| Layer | Who writes it | Looks like |
|---|---|---|
| **Documents** | anyone | Markdown with fields and links |
| **Schemas and rules** | someone technical, once | real code, short and explicit |

The relationship is that of a Word template to its content: the schema says what a document must contain, the
document fills it in.

The compiler speaks to both. For a document author, an error reads "the field *cap* expects a percentage, you wrote
'ten'", pointing at the line, not a parser message.

## Names, ids and files

Decided on 2026-10-22; the reading rules are in `docs/language.md`, the examples in `cases/`.

- **Names are ASCII slugs in kebab-case**: fields, enum values, rules, functions, units and standard methods
  (`prix-unitaire`, `controle-technique`, `less-than`, `max-by`). Type names are PascalCase ASCII (`EtatDesLieux`).
  Subtraction needs spaces: `total-ht` is a name, `total - ht` a subtraction.
- **Labels carry the Unicode**: `echeance: Date "Échéance"`. Without a label, the name is shown with spaces and a
  capital (`zone-tendue` reads "Zone tendue"). Headings match labels; keys and table headers use names.
- **Generated code transliterates names**: `prix-unitaire` becomes `PrixUnitaire` in Go, `prix_unitaire` in Python,
  `prixUnitaire` in TypeScript, and stays `prix-unitaire` as a JSON key.
- **Every object has an id**, a slug of the same shape (a UUID is valid, a meaningful slug is better), **always
  written**: the file name for a file, `{#id}` at the end of a heading or list item, an `id` column in a table.
- **An object can be a file** named after its id (`f-2026-031.kan`). Its child objects go in a sibling folder named
  after it, then after the field: `clio.kan` and `clio/entretiens/2025-06-10-vidange.kan`.
- **An array is written inline** (table, list, sections) **or as a folder** of `.kan` files, one per entry, ordered
  by id. A file in such a folder needs no front matter: its position gives its type.

## Syntax sketch

Working material: a French public tender (DCE). Keywords and types are English; names are ASCII slugs taken from
the material, and labels and content are French.

Standard library (part of the language):

```kan
fn less-than(a: Money, b: Money) -> Bool
  reads en "{a} is less than {b}"
  reads fr "{a} est inférieur à {b}"
  = a < b
```

Schema (working material, written by someone technical), in `dce/schema.kanonarch`:

```kan
schema Article {
  numero: Text "Numéro"
  titre:  Text @heading
  corps:  Prose @body
}

schema Ccap {
  penalite-jour: Money "Pénalité par jour"
  plafond:       Percent
  articles:      [Article] @sections
}

schema Dce { rc: Rc  ae: Ae  ccap: Ccap  cctp: Cctp  bpu: Bpu }

rule penalites-plafonnees(d: Dce, jours-de-retard: Int) -> Bool
  reads fr "les pénalités de {jours-de-retard} jours de retard restent sous le plafond du CCAP"
  = less-than(d.ccap.penalite-jour * jours-de-retard, d.ccap.plafond * d.ae.montant-total)
```

Document (working material, editable by anyone), in `dce/ccap.kan`:

```kan
---
type: Ccap
---

# CCAP — Voirie 2027

penalite-jour: 150.00 EUR
plafond: 10 %

## Pénalités de retard {#penalites}

numero: 4.2

Par jour de retard, le titulaire paie {penalite-jour}, dans la limite de {plafond} du montant du marché
([[ae]]). Délais : voir [[cctp#delais]].
```

- Headings split the document into sections; `{#penalites}` gives the section's object its id.
- A `key: value` line fills a field, by its exact name.
- Paragraphs are prose.
- `{…}` inserts a value; `[[…]]` links to a document or an object, here or in another document.

If `#delais` doesn't exist in the CCTP, compilation fails; renaming the id `penalites` updates every link.

How headings and paragraphs map onto schema objects is answered by the reading rules in `docs/language.md` (`@sections`,
`@heading`, `@body`). Still open: writing `## 4.2 Pénalités de retard` and splitting it into `numero` and `titre`,
which would take a `reads` pattern on the heading.

## Recommendations

- **Stay close to Markdown.** Ideally a Kanon document renders acceptably in any Markdown viewer (GitHub, Obsidian)
  without Kanon tooling. To evaluate: a strict superset of CommonMark, or a cousin of it.
- **Schemas in Kanon's own syntax**, GraphQL-style, not YAML (decided 2026-10-22). In YAML, computed fields and
  rules would be unchecked strings and would still need an expression parser; TypeSpec was created for the same
  reason over OpenAPI YAML. Kanon exports JSON Schema and OpenAPI instead.
- **Functions in a small expression language of our own**, pure and total (no infinite loops), CEL-like, rather than
  JS/TS: it compiles to TS, Python and Go with the same result everywhere. `extern ts` remains an escape hatch,
  marked non-portable.
- **Compiler in Go** (single binary; decided 2026-10-22, for now), **tree-sitter** grammar for editors. The
  document parser is hand-written: a Markdown-like syntax is not a classic grammar.
- **AI writes, humans read and edit**: an agent converts a PDF into Kanon, the compiler corrects it (schema,
  references), people read the rendering and fix the text themselves.
- **Import and export standards rather than compete with them**: XBRL and charts of accounts for balance sheets,
  SQL/DBML for databases, OSCAL for compliance.

## Risks

- **Scope**: a language that wants to describe everything at once describes nothing well. Start from one piece of
  working material: a **French public tender (DCE)** (RC, AE, CCAP, CCTP, BPU/DPGF), concrete, structured, full of
  rules (criteria and weightings, penalties, deadlines) and cross-references. Breek (construction tender responses)
  works on these documents.
- **Domain bias**: designing the core around the first example. A second, unrelated piece of material (an
  equipment inventory, for instance) keeps the core generic.
- **Friendliness versus strictness**: Markdown is lenient, a compiler is strict. The friendlier the syntax, the
  harder it is to give precise errors and to keep the file unambiguous.
- **Adoption**: without an editor and a clean rendering, it stays a developer tool.

## Plan

1. A real DCE converted to `.kan` by hand, plus a small second example from another domain.
2. Grammar and parser.
3. `kanon check`, with references.
4. TypeScript generation.
5. PDF rendering.
6. MCP server and PDF → Kanon agent.

## Sources

- Accord Project, Concerto and Ergo: https://arxiv.org/pdf/2112.07064v1 ;
  https://www.artificiallawyer.com/2018/06/01/accord-project-releases-ergo-a-new-smart-contract-language/
- Catala: https://libraries.io/pypi/catala-runtime
- `.kan` extension: https://www.file-extensions.org/extension/kan
