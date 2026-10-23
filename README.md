# Kanon

[![CI](https://github.com/bn-k/kanon/actions/workflows/ci.yml/badge.svg)](https://github.com/bn-k/kanon/actions/workflows/ci.yml)

A folder of Markdown documents that people, language models and programs edit together, and that stays as valid as
a database.

A schema, `.kanonarch`, says once what the documents hold: types, enums, links, computed fields and rules. The
documents, `.kan`, are Markdown: a person edits them in any editor, an agent writes them as text, git versions
them. `kan` checks them like a compiler, refuses a write that would break them, and queries them as SQL.

## One board, three writers

The board of a small team ([`cases/board`](cases/board)): `board.kan` lists the people, `board/tasks/` holds one
file per task.

```
enum Status { todo, doing, done }

schema Person {
  name:  Text
  email: Text
  tasks: [Task] <- owner
}

schema Task {
  title:  Text @heading
  status: Status
  owner:  -> Person
  due:    Date
  notes:  Prose?
}

rule one-at-a-time(p: Person) -> Bool
  reads en "{p.name} works on one task at a time"
  = count(p.tasks, t => t.status == doing) <= 1
```

**A person** writes a task as a note, in Zed, Obsidian or any editor:

```
# Pricing page

status: doing
owner: [[/board#ana]]
due: 2026-10-30
```

**An agent** adds a task through `kan write`, which writes it only if the whole folder stays valid, and otherwise
says what to write instead:

```
$ kan write schema.kanonarch board/tasks/2026-10-28-release-notes.kan=release-notes.txt
board/tasks/2026-10-28-release-notes.kan:3:9: error: "in progress" is not a `Status`: write todo, doing or done
board/tasks/2026-10-28-release-notes.kan:4:8: error: unknown id `anna` in board.kan; did you mean `ana`?
board/tasks/2026-10-28-release-notes.kan:5:6: error: write a date as YYYY-MM-DD (2026-10-22), not "30/10/2026"
3 errors
```

Once fixed, the task is still refused: Ana is already on the pricing page, and the rule fails in another document.

```
board.kan:11:1: error: `one-at-a-time` fails: "Ana Silva works on one task at a time"; 2 is more than 1
```

**A program** reads the board as SQL tables, with no server and no copy to keep in sync:

```
$ kan sql -q "SELECT t.title, p.name AS owner, t.due FROM task t JOIN person p ON p.id = t.owner_id
              WHERE t.status != 'done' ORDER BY t.due" schema.kanonarch board.kan
title         owner      due
------------  ---------  ----------
Pricing page  Ana Silva  2026-10-30
Launch email  Sam Lee    2026-11-06
2 rows
```

## Install

```
$ go install github.com/bn-k/kanon/cmd/kan@latest
```

or take a binary for Linux or macOS from the [releases](https://github.com/bn-k/kanon/releases). Windows is not
supported yet ([#1](https://github.com/bn-k/kanon/issues/1)); the Linux binary runs in WSL. Then try the board:

```
$ git clone https://github.com/bn-k/kanon && cd kanon/cases/board
$ kan lint schema.kanonarch board.kan
ok  schema.kanonarch
ok  board.kan
ok  board/tasks/2026-10-20-brief.kan
ok  board/tasks/2026-10-30-pricing-page.kan
ok  board/tasks/2026-11-06-launch-email.kan
```

For an editor that checks as you type, see [Zed and Obsidian](docs/kan.md#kan-lsp-and-the-editors).

## Status

Kanon is young: version 0.x, tested on the cases of `cases/`, and the language may still change before 1.0. Each
change is in `CHANGELOG.md`, with how to update documents. A server that holds a folder for many writers at once,
`kan serve`, is designed in `docs/serve.md`, not built.

## Documentation

- [`docs/language.md`](docs/language.md): the language, rule by rule
- [`docs/kan.md`](docs/kan.md): the commands, `lint`, `eval`, `write`, `import`, `sql`, and the editors (Zed,
  Obsidian)
- [`cases/`](cases): real use cases, each with the exact output of `kan`
- [`docs/data.md`](docs/data.md): files, spreadsheets and SQL
- [`docs/decisions.md`](docs/decisions.md): why it is this way, and the open questions
- [`CONTRIBUTING.md`](CONTRIBUTING.md): building, testing, and how to propose a change

## Licence

MIT, in `LICENSE`. The data of a case keeps its own licence, given in its `source/README.md`.
