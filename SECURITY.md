# Security

Report a vulnerability privately, through GitHub: **Security**, then **Report a vulnerability**
(<https://github.com/bn-k/kanon/security/advisories/new>). Please don't open a public issue for it. You will get an
answer within a week; a fix goes into a release, with your name in its notes unless you'd rather not.

Only the latest release is supported.

`kan` reads documents that someone else may have written: a folder from a repository, a file an agent produced, a
spreadsheet or a SQLite database to import. A vulnerability is anything such input can make `kan` do beyond
reporting on it, for example:

- read or write a file outside the folder of the schema, through a link, a path given to `kan write` or an import;
- write anything with `kan sql -q`, which only reads;
- run for ever, or use memory without bound, on a small input;
- crash `kan lsp`, which an editor runs on every keystroke.

A wrong diagnostic, or a document accepted that should not be, is a bug: open an issue.
