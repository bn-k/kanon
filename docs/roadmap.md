# Roadmap

2026-10-23. Kanon is for a folder of documents that people, language models and programs share as one database:
people edit it in an editor, agents write it as text, programs query it, and it stays valid. The order below puts
credibility first: a stranger installs `kan`, runs the example of the README, and it does what the README says.
Then each kind of writer gets what it lacks, one at a time.

## 0.1 — tried in five minutes

Done: `kan version`, `CHANGELOG.md`, CI on Linux and macOS (the checks of `CONTRIBUTING.md`, the grammar), releases
built by GoReleaser from a tag, a README that is a front page and runs as cases (`cases/board*`), the language in
`docs/language.md`, `CONTRIBUTING.md`, `SECURITY.md`, issue templates.

Left: the tag `v0.1.0`, once the name is checked.

## 0.2 — agents

The writer the project exists for, and the one that makes the most mistakes.

- `-format json` on `kan lint` and `kan eval`, as `kan write` has it: a program reads diagnostics without parsing
  text.
- A stable code on each diagnostic, in the JSON only, so that an agent can act on a kind of mistake; the wording
  stays free to improve.
- `docs/agents.md`: how an agent reads a schema, writes through `kan write` and repairs from the diagnostics, tested
  on the board with a real model.

## 0.3 — programs

- `kan serve`, designed in `docs/serve.md`: the PostgreSQL protocol over a folder, every write linted before it
  lands, the changes made on disk read again.
- `kan search`, designed in `docs/search.md`: lexical, semantic and hybrid search over the objects.

## Before 1.0

- The open questions of `docs/decisions.md` decided.
- Windows (issue #1).
- Someone other than the author depends on Kanon, and has said what broke.

## Deferred, on purpose

Each of these was weighed on 2026-10-23 and waits for the need that would justify it:

- **a separate specification** (`spec/`, a grammar in EBNF, MUST and SHOULD): `docs/language.md` is the one
  definition, until a second implementation needs more than it says;
- **a conformance corpus** and a runner for it: `cases/` is the corpus while there is one implementation;
- **a second implementation** (a reader in Python): when someone asks for one;
- **governance** (an RFC process, `GOVERNANCE.md`, a code of conduct, sign-off on commits): when there is a second
  maintainer;
- **publishing the editors** to Zed's and Obsidian's registries, a GitHub Action, a website, a playground, an
  export to JSON Schema: after 0.2, one at a time.

## Rules

- No 1.0 for the optics: it would cost the freedom to fix the language.
- A rule of the language is defined once, in `docs/language.md`; everything else links to it.
- The wording of a message is not a contract; its code, position and severity will be.
- An example in the documentation comes from a case, so that it can't drift from what `kan` does.
