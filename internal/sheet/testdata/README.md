# Real workbooks

Two workbooks saved by Microsoft Excel, published on [data.gouv.fr](https://www.data.gouv.fr) under the
[Licence Ouverte / Open Licence](https://www.etalab.gouv.fr/wp-content/uploads/2014/05/Licence_Ouverte.pdf)
(`fr-lo`), downloaded on 2026-10-22 and kept unchanged but for their file name. `real_test.go` reads them.

| File | Size | Source |
|---|---|---|
| `boe-budget-primitif-2026.xlsx` | 59 773 bytes | Mairie de Boé, dataset [Budget Primitif](https://www.data.gouv.fr/datasets/budget-primitif-6), file [`bp2026.xlsx`](https://static.data.gouv.fr/resources/budget-primitif-6/20261021-120107/bp2026.xlsx) |
| `lorient-dette-2015.xlsx` | 25 486 bytes | Lorient Agglomération, dataset [BUDGET PRINCIPAL : comptes administratifs, budget primitif, état de la dette](https://www.data.gouv.fr/datasets/budget-principal-comptes-administratifs-budget-primitif-etat-de-la-dette), file [`Etat_de_la_dette_2015_BUDGET_PRINCIPAL.xlsx`](https://static.data.gouv.fr/resources/budget-principal-comptes-administratifs-budget-primitif-etat-de-la-dette/20160307-172712/Etat_de_la_dette_2015_BUDGET_PRINCIPAL.xlsx) |

SHA-256:

```
f355d5762de00a2e7089bb0d01d6d1a715cd79496ab8a168ebfd081558cbf36a  boe-budget-primitif-2026.xlsx
ad60303463ca162163d10d4ed121c194ad4b41696f453b6429be9e7979014fb2  lorient-dette-2015.xlsx
```

What they cover: shared strings, currency formats written as `#\ ##0.00\ \€` and `[$€-40C]` with a red negative
section, formulas with their cached value, a date format `dd\/mm\/yyyy`, a title above the header, skipped cells.
