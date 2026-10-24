# Source

`bank-holidays-england-and-wales.csv` comes from the division `england-and-wales` of
<https://www.gov.uk/bank-holidays.json>, the data of [UK bank holidays](https://www.gov.uk/bank-holidays) published by
GOV.UK, downloaded on 2026-10-23. It is under the
[Open Government Licence v3.0](https://www.nationalarchives.gov.uk/doc/open-government-licence/version/3/): contains
public sector information licensed under the Open Government Licence v3.0.

It lists the 83 bank holidays of England and Wales from 2019 to 2028, one event each, in date order: the title of the
holiday, its date, its notes ("Substitute day" when it is taken on a weekday because it falls on a weekend) and
whether it calls for bunting.

## How the case is made

The events and their values are unchanged, in the same order. Three things differ from the download:

- each event is a row of a CSV, one column per key, in the order of the download: `title`, `date`, `notes`,
  `bunting`;
- `bunting` is written `yes` or `no`, as Kanon writes a `Bool`, not `true` or `false`;
- an `id` column comes first, since every object has an id: the year and the slug of the title, without its
  apostrophe, such as `2026-good-friday` or `2026-new-years-day`.

The date could be the id, since no two rows share one, but `2026-good-friday` says what `2026-04-03` doesn't.

To check the case against the download, rebuild the CSV from it:

```
jq -r '"id,title,date,notes,bunting", (.["england-and-wales"].events[]
  | [.date[:4] + "-" + (.title | ascii_downcase | gsub("’"; "") | gsub("[^a-z0-9]+"; "-") | rtrimstr("-")),
     .title, .date, .notes, if .bunting then "yes" else "no" end] | join(","))' bank-holidays.json |
  diff - bank-holidays-england-and-wales.csv
```
