# Search

Design of `kan search` (2026-10-22): lexical, semantic and hybrid search over the objects of a document. Modelled
on a search engine built earlier for another project, not public: one narrow engine interface, interchangeable
engines, a rebuildable mirror, a golden corpus with hand-labelled queries. Nothing here is built yet.

A document is already searched two ways, its files (grep) and SQL (`kan sql -q`). Both answer exactly: grep finds
the words, SQL finds the values. Neither finds "the clause about the deposit" when the text says "caution", nor
"a house like this one". `kan search` is the third way: it ranks objects by how well they match what is meant. It
is for agents with a shell (one command, ids and lines out) as much as for people.

What we keep from that engine: the `Engine` interface and its contract tests; bm25 with the title above the body;
accent folding; Reciprocal Rank Fusion (k = 60) over parts asked for four times the limit; L2-normalised int8
vectors scanned by brute force, no ANN and no extension; a content hash beside every vector so only what changed is
embedded again; exact filters applied during the scan; a golden corpus with recall, nDCG and MRR.

What we do not: a silent fallback to a fake embedder, no minimum score, one vector for a long text, a full re-scan on
a timer, every vector loaded at boot, a provider that can only be remote.

## 1. What is indexed

The unit is an **object**: a file, a section, a table row, a list item, a row of a spreadsheet, an entry of a
folder. Its key is `file#id` (the file alone for a whole-file object), the same address a link uses. Linting
already binds every object to its schema, so the index is built from `internal/model`, never from the text again.
Search lints first, as `kan sql` does: on an error it prints the diagnostics and indexes nothing.

An object gives four texts, each its own column in the lexical index:

| Column | Holds | bm25 weight |
|---|---|---|
| `title` | the `@heading` field (heading or H1), else the id with hyphens turned into spaces | 5 |
| `id` | the id as a slug (`depot-de-garantie` finds `depot`, `garantie`) | 3 |
| `keys` | the object's own key lines, tables cells and links: `loyer-hc 850.00 EUR`; a link adds the **title of its target** (`client: Dupont Batiment`) and a computed field its computed value | 2 |
| `body` | the `@body` field and every `Prose` or `Code` field, with `{field}` inserts expanded (`un dépôt de garantie de 850.00 EUR`) | 1 |

- **Own text only.** A sub-object or a section is an object of its own and is not repeated in its parent. A hit on a
  clause points at the clause, not at the whole lease.
- **Context for meaning.** The text sent to the embedder is `title of the file / title of the parent / title`,
  then keys and body: "Garantie" alone means little, "Bail d'habitation, 12 rue des Lilas / Garantie" does.
- **Chunking by section, then by paragraph.** A section is already a chunk. A `Prose` longer than 1 500 characters
  is cut at paragraph breaks (a blank line) into chunks of at most that size, each with the context line, each its
  own vector; the result collapses to the best chunk of the object and reports its line. No overlap, no cutting
  inside a paragraph, a paragraph longer than the limit is cut at a sentence end.
- **Not indexed.** Images, PDFs and other binaries (their file name and alt text are, as `keys` of the object that
  links them), inverses (derived), and the words of the language itself. A `File` of text and a `Code` block are.
- **Semantic only where there is meaning.** A table row of numbers (`order_detail`) is indexed lexically and as a
  facet but not embedded: an object is embedded when its title, keys and body together hold at least 40 characters
  of `Text` or `Prose`. This keeps Northwind (about 14 000 objects) to the few thousand that say something. A
  schema could say otherwise with an annotation; open question 2.

**Facets** are the typed fields, stored in a table `facet(key, name, value, num)` and filtered exactly:

| Field | Facet |
|---|---|
| the schema | `type` (`Facture`), always; `file` (the path) |
| an enum, `Bool`, `Int`, `Version` | `etat=use`, `zone-tendue=yes`, `pieces=2` |
| `Date`, `DateTime` | `date=2026-09-01`, compared as text (canonical form sorts) |
| `Number`, `Percent`, `Money`, `Quantity` | `num` holds the amount in its base unit; `currency` / `unit` is a second facet |
| `-> T`, `[-> T]` | the target's key, found by its last segment too: `client=dupont-batiment` |
| `Text`, `Prose`, `Code` | none: free text is for the lexical index |

A facet name is a field name. Two schemas with the same field name share it, `type:` narrows.

## 2. Where the index lives, and how it stays fresh

`.kanon/index-<document>.db` in the folder of the root document, one SQLite file per root. It is a mirror, never the
source of truth: deleting it loses time, not data. `kan search` creates `.kanon/.gitignore` (`index-*.db`) when it
makes the folder, so the index is not committed and the config next to it can be.

There is no daemon, so freshness is checked at each run, and it is cheap because linting already read everything:

1. Lint and bind (needed anyway). Build the indexed text of every object.
2. `hash = sha256(schema + title + keys + body + facets)[:12]` per object, compared with the table `object(key,
   hash)`. New and changed objects are upserted into the lexical index and the facet table; gone ones are deleted.
   One transaction.
3. Vectors: per chunk, `sha256(model + "\x00" + text)[:12]` is stored beside the vector. Only a different hash is
   embedded again, so a change of model re-embeds everything by itself, and a change of a facet re-embeds nothing.
   Only the rows of the current model are scanned.
4. A change in the schema alone (a new facet, a new column in the text) changes the hashes of the objects it
   touches, and a `format` number in the table `meta` empties the index when the code's layout changes.

Embedding is the only expensive step and is never a surprise. The lexical side refreshes at every run. If more than
50 chunks need a vector, `kan search` does not embed them: the semantic part is skipped with a line that says why
(§4) and `kan index schema doc` does the work, printing the count, the estimate and the cost (§6). Until then
`-no-refresh` searches the index as it is, without linting, for a quick query.

## 3. Embedders

```go
package embed // internal/embed

type Embedder interface {
	Model() string                  // the name stored with every vector
	Dims() int                      // the size after truncation
	Embed(ctx context.Context, texts []string, kind Kind) ([][]float32, Usage, error) // Kind: Doc or Query
}
```

One interface, three providers, **chosen by explicit configuration** in `.kanon/embed`, written as key lines like
any Kanon file. No config, no semantic part: `-mode lexical` is the default and `-mode semantic` is an error that
says how to configure. A missing key, an unreachable server or a wrong model is an error that names the file and
the line; it never turns into another embedder.

```
provider: ollama
url: http://localhost:11434
model: qwen3-embedding:0.6b
dims: 256
query-prefix: Instruct: Given a search query, retrieve the passages that answer it\nQuery:
min-score: 0.35
```

| Provider | Wire | Serves |
|---|---|---|
| `ollama` | `POST /api/embed` | Ollama, local |
| `openai` | `POST /v1/embeddings`, OpenAI shape | llama.cpp `llama-server --embedding`, vLLM (local), OpenRouter, OpenAI (remote) |
| `test` | none | tests only (§5): not registered in `cmd/kan`, `cli.Run` receives the providers it may use |

- Two wire formats cover everything; `net/http` and `encoding/json` are enough, so the standard-library rule holds.
- **Local or remote is decided by the URL.** A host that is not loopback (`localhost`, `127.0.0.1`, `::1`) needs
  `remote: yes` in the config, so a typo or a copied config never sends a document out. A key is read from the
  environment variable named by `key-env: OPENROUTER_API_KEY`, never written in the file.
- **Dims.** `dims` truncates a Matryoshka model (qwen3-embedding, nomic-embed-text, OpenAI's text-embedding-3) and the
  vector is L2-normalised again; 256 is the default (the earlier engine's choice: small, and recall is close to full size). A model
  without Matryoshka keeps its size. A document and a query must use the same `dims` and `model`; the hash covers both.
- **Prefixes.** `query-prefix` and `doc-prefix` are the model's instruction convention (asymmetric search); they
  differ per model, so they are config, not code.
- **Quantisation.** The unit vector is stored as `int8` (`round(x * 127)`), one BLOB of `dims` bytes: 256 bytes per
  chunk, 25 MB for 100 000 chunks. A cosine is `dot / 127²`; the error is about 0.01, below the noise of any model.
- **Scan.** `SELECT key, vec FROM vec WHERE model = ? AND key IN (filtered keys)`, streamed through a top-k heap. No
  boot load, memory is the heap. 100 000 chunks at 256 dims is 25 million multiply-adds, tens of milliseconds in
  plain Go even on a CPU without AVX. A query vector is cached in `qcache(model, hash, vec)` (last 1 000).
- **Suggested local model.** `qwen3-embedding:0.6b` through Ollama: multilingual (the content is French), Matryoshka,
  runs on a CPU. Check the French recall with the live eval (§5) before trusting any other.

**SQLite and FTS5.** `modernc.org/sqlite` v1.61.0 (go.mod) is built with `SQLITE_ENABLE_FTS5` (the flag is in the header
of `lib/sqlite_linux_amd64.go`), and a throwaway program confirmed it: `CREATE VIRTUAL TABLE … USING
fts5(title, body, tokenize='unicode61 remove_diacritics 2')` works, `MATCH 'depot'` finds "Dépôt", and `bm25(t, 5, 1)`
ranks. So the lexical engine is FTS5, as in the earlier engine, and no pure-Go BM25 is needed. Not available: a French stemmer
(`porter` is English); stop words are removed from the query in Go (a short French and English list), not from the
index, since bm25 already lowers their weight. Tokens such as `c++` or `f-2026-031` are split by FTS5 on punctuation;
a query keeps a quoted phrase whole. The driver is allowed today in the SQL packages only: `internal/search` joins
them (decisions to amend).

## 4. Query

```
kan search [-mode lexical|semantic|hybrid] [-n 20] [-offset 0] [-min-score 0.35] [-where name<op>value]…
           [-like key] [-format lines|json|ids] [-no-refresh] [-v] schema.kanonarch document.kan [query]
```

- The mode defaults to `hybrid` when `.kanon/embed` exists, else `lexical`. `-n` is capped at 50, `-offset` pages.
- **Filters.** `-where` is repeated and AND-ed; operators `= != < <= > >=`, values in the canonical format
  (`date>=2026-01-01`, `loyer-hc<900 EUR`). The same name twice is OR. A filter written inside the query also
  counts when its name is a known facet: `kan search … "relance type:Facture client:dupont-batiment"`. An unknown
  name or a value that is no member of the enum is a diagnostic in the project's words: `a facet is a field name;
  did you mean client?`, `write use, not "usé"`. Filters apply before ranking, in both parts.
- **Lexical.** The terms are AND-ed, the last one as a prefix; a quoted phrase is kept; `-any` ORs them. No result is an
  empty list, not a fallback to OR.
- **Semantic.** Cosine against the chunks that pass the filters, **dropped below `min-score`** (from the config, or the
  flag): a query with nothing close returns nothing, not the least bad. The best chunk gives the object's score.
- **Hybrid.** Both parts are asked for `4 × n`, fused by RRF with k = 60, the fused score divided by the best so the
  top is 1.00. Lenient, as the earlier engine is: if one part fails or times out (10 s) the other answers, **and the output says
  so**. `-strict` makes it exit 1.
- **`-like key`** replaces the query by the vector of that object (its stored vectors averaged and normalised), and
  leaves it out of the results. It needs the semantic part; with `-mode lexical` it uses the object's `title` and
  `keys` terms. This is "similar to this document".

Output follows `kan lint`: one line per object, `file:line: id  score  excerpt`, the line of the object (of the
chunk that matched), the score from 0 to 1, an excerpt of about 100 characters, one line, around the first match.

```
$ kan search cases/search-annonce/schema.kanonarch cases/search-annonce/agence.kan "caution du locataire"
annonces/cal-118.kan:31: garantie  1.00  … le locataire verse une caution équivalente à un mois …
bail.kan:48: garantie  0.71  Le locataire verse un dépôt de garantie de 850.00 EUR. Il lui est restitué …
2 results
parts: lexical 1 (bm25), semantic 14 (ollama qwen3-embedding:0.6b, 256 dims, min-score 0.35), fused by RRF
```

The last line says which parts answered and with how many candidates; a part that did not says why:
`parts: lexical 1; semantic skipped: 412 of 1380 chunks have no vector, run kan index`. `-v` adds each part's rank and
raw score after the excerpt. `-format json` is one object per line for programs (key, file, line, score, ranks,
excerpt, facets); `-format ids` prints only keys, for a pipe.

**With `kan sql -q`.** Search finds, SQL counts and joins. Two bridges, both after the first phases:

- `kan search -format ids … | kan sql -q "SELECT … WHERE id IN (…)"` works by hand today, since a key's `#id` is the
  `id` column (for a strong schema; a weak one needs its parent, the key says it).
- `kan sql -q 'SELECT … FROM facture f JOIN hit h ON h.id = f.id' -search "relance impayé" …` gives the query a table
  `hit(schema, id, file, rank, score)`, filled by a hybrid search before it runs, read only like the rest. The
  other direction, `kan search -sql 'SELECT id FROM facture WHERE total_ttc > 5000 EUR'`, restricts the candidates to the
  ids a query returns. Facets cover most filters without SQL; this one covers the rest.

## 5. Evaluation

Two layers, both in the existing harness.

**Golden cases (`go test ./internal/cli`).** A case `cases/search-annonce/`: the annonce schema as a collection
(`Agence`), about 30 fictional objects (10 annonces with prose, their agents, sellers, a few visit notes) written
before any search runs. Its `expected.txt` is written by hand like every other: the first line is the command, the
rest the lines expected. Cases: `search-annonce` (lexical), `search-annonce-semantic`, `search-annonce-hybrid`,
`search-annonce-like`, `errors-search` (unknown facet, enum written wrong, semantic with no config, remote without
`remote: yes`). `-update` rewrites them after a deliberate change, and the diff is read.

**The offline embedder.** Golden tests cannot call a model, and a hash of the words proves nothing about meaning. The
`test` provider is feature hashing of folded tokens into 256 dims, plus a **synonym file of the case**
(`synonyms.txt`: `caution = depot de garantie`, `pavillon = maison`) that sends the words of a line to one bucket. It is
deterministic, offline, and says exactly what a case means: semantics is whatever the case declares. It is built in
`internal/search/searchtest`, passed to `cli.Run` by the test only, so `cmd/kan` cannot reach it. The index goes
to `t.TempDir()` (`-index path` exists for this). What this tests is everything around the model: chunking, filters,
the minimum score, fusion, `-like`, staleness, the parts line. Not the quality of a real model.

**Quality eval (`go test ./internal/search -run Eval`).** `cases/search-annonce/judgments.txt`: about 15 queries, each
with the keys judged relevant, labelled by hand, in the categories the earlier engine found worth keeping: an exact term, a
synonym, an acronym (`DPE`), no accents (`Caluire`), a long natural sentence, a filter alone, an object used as the
query (`-like`), and a **nothing relevant** query that must return no result. Per mode it computes recall@10, nDCG@10 and MRR,
printed as a table. Asserted: lexical finds every exact, acronym and no-accent query; semantic finds the synonym ones
(with the `test` embedder, by construction); **hybrid ≥ best part − 0.05** on each metric; the empty query returns
nothing. Run with `KAN_EMBED_LIVE=.kanon/embed` the same judgments go through the configured model; thresholds are then
reported, not asserted (a model changes, a test must not).

**Contract tests.** `Engine` has one suite every engine passes: an upsert twice is one object, a delete removes it, a filter
excludes before ranking, the limit and the offset hold, an empty query and an unknown key are errors, a score is in
[0, 1]. The lexical, semantic and hybrid engines run it; so would a later one.

## 6. Cost and privacy

- **Local by default.** Without `.kanon/embed` nothing is embedded and nothing leaves the machine. With a local server,
  the text goes to loopback only. A remote host needs `remote: yes`, written in the config of that folder.
- **What leaves, if remote:** the chunks to index (context line, keys and body) and the query. Never a file, never a
  binary. Personal documents belong with a local model; this is said once, where `kan search` is documented.
- **The index holds the text** (FTS5 stores it, to build excerpts): it is as private as the documents, gitignored, and
  deleted with `.kanon/`. No secret belongs in a document anyway (the project rule).
- **Cost is printed before and after** every embedding run, on stderr: `embedding 412 chunks (about 21 000 tokens,
  about 0.002 USD) with openai qwen/qwen3-embedding-8b`, then the actual tokens the API reports. `price:` in the config
  (USD per million tokens) feeds the estimate; without it only tokens are shown. A local model costs time, shown the
  same way. Totals per model are in the table `meta` (`kan index -stats`).
- **A query costs one embedding**, cached: asking twice is free.

## 7. Plan

1. **Lexical.** `internal/search`: objects from the model, the four columns, facets, the hash table, the FTS5
   engine, `kan search -mode lexical`, `-where`, the lines output, `-format ids|json`. The contract suite. Case
   `search-annonce` with its hand-written `expected.txt`, then the `errors-search` case. No network, no embedder.
2. **Semantic.** `internal/embed` (interface, `ollama`, `openai`, `test`), config, quantised vectors, the scan,
   chunking, `kan index`, `-like`, `min-score`, the query cache, cost lines. Judgments and the eval table.
3. **Hybrid.** RRF, the parts line, leniency and `-strict`. The "hybrid ≥ best part − 0.05" assertion.
4. **Bridges.** `kan sql -q -search`, `kan search -sql`. A live eval on a real model and the recall numbers in this file.

Each phase ends with `gofmt`, `go vet`, the cases, and coverage at 95% for the new packages.

## Open questions

1. **Names and the language of the index.** Lexical search folds accents and drops stop words but has no stemmer:
   "locataires" does not find "locataire". A light French suffix stripper in Go (fifty lines) is possible; do we
   accept a stemmer for one language in a language-neutral tool, or leave plurals to the semantic part?
2. **What is embedded.** The 40-character rule is a guess. A schema annotation (`@search`, `@nosearch` on a field or a
   schema) would let the author decide, and fits the language, but it is a change to the schema syntax being
   rewritten now.
3. **`remote: yes` is a guard, not a consent.** It lives in a file an agent can write. Is that enough, or should a
   remote provider also need an environment variable the human sets?
4. **Several roots in a folder, or a root that links a folder far away.** One index per root document is simple, but a
   link out of the folder (`../shared/clients`) puts foreign objects in the index. Index them, or only the root's tree?
5. **Index location when the folder is read only** (a mounted Drive). `-index path`, or fall back to the user cache
   directory, which breaks the "next to the document" rule.
6. **Score meaning.** Hybrid scores are rank-based and normalised to the best, so 0.71 says "less good than the top", not
   "71% relevant". Is that honest enough for an agent that wants to cut at a threshold, or should it be given the
   cosine as well (`-v` does)?
7. **`min-score` is per model**, and so is the right default. Calibrate it from the live eval and write it in the
   config of the suggested model, or compute it from the corpus (a percentile of the cosines of unrelated pairs)?
8. **Computed fields in the index** need `kan eval` to have run; they are cheap, but a rule that fails makes the whole
   run a lint error. Index despite errors, with a warning, so search works on a draft?
