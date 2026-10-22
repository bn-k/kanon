# Source

`create.sql` is `src/create.sql` of [jpwhite3/northwind-SQLite3](https://github.com/jpwhite3/northwind-SQLite3),
copied unchanged on 2026-10-22: the last commit to touch it is `16ee04352127e170d765154cb024ebd1442567c6`
(2023-03-24), its blob `c75bae5e722fd95cf34504f8e037bdeb57dbbc38`. The repository is under the MIT licence,
copyright (c) 2016 JP White.

It is Microsoft's Northwind sample database for SQLite: its tables, its views and its rows.

Each `photo.jpg` and `picture.jpg` is written byte for byte from the BLOB of its row, a plain JPEG.

## How the case is made

Everything outside `source/` and `expected.txt` is written by `kan import`, never by hand. `kan import` writes into
a new folder only, so regenerate the case through one, from the root of the repository:

```
kan import cases/northwind/source/create.sql /tmp/northwind
cp -r /tmp/northwind/. cases/northwind/
```

`cp` adds and replaces files and deletes none, so `source/` and `expected.txt` stay; a document the new import no
longer writes stays too, and `TestNorthwind` reports it.

`TestNorthwind` in `internal/sqlimport` imports `create.sql` again and checks that the case, `source/` and
`expected.txt` left out, is what it writes. `expected.txt` is what `kan lint` prints for the case, checked by the
`cli` tests.

The import notes:

- `Customers`: the keys change to make ids, such as "ALFKI" → "alfki";
- `Suppliers.ContactName` and `Suppliers.Country`: the spaces around one value each are dropped;
- `Employees.Address` (2 values) and `Suppliers.Address` (9 values): the Windows line breaks (\r\n) of
  `create.sql` are written as \n;
- the 16 views are not imported.
