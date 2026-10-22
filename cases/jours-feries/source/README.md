# Source

`jours-feries-metropole.csv` comes from `jours_feries_metropole.csv`, of the dataset
[Jours fériés en France](https://www.data.gouv.fr/datasets/jours-feries-en-france/) published by Etalab on
data.gouv.fr, downloaded on 2026-10-22 from
<https://etalab.github.io/jours-feries-france-data/csv/jours_feries_metropole.csv> (last modified 2026-10-13). The
dataset is under the [Licence Ouverte v2.0](https://www.etalab.gouv.fr/licence-ouverte-open-licence/).

It lists the 286 public holidays of metropolitan France from 2006 to 2031, one row each, in date order: the date,
the year, the zone (always "Métropole") and the name of the holiday.

## How the case is made

The rows and their values are unchanged. Three things differ from the download:

- the file is renamed with hyphens, `jours-feries-metropole.csv`, as Kanon names files;
- the column `nom_jour_ferie` is renamed `nom`;
- an `id` column comes first, since every object has an id: the year and the slug of the name, such as
  `2026-lundi-de-paques`.

The date can't be the id: 2008-05-01 is both Ascension and 1er mai, two rows.

To check the case against the download, drop the `id` column and the header:

```
diff <(tail -n +2 jours_feries_metropole.csv) <(tail -n +2 jours-feries-metropole.csv | cut -d, -f2-)
```
