### Bars fitted on dev (target 95%, at least 20 cases; else global 0.92)

| Subfamily | dev n (in scope) | bar | dev at bar | dev at global bar |
|---|---:|---:|---|---|
| apparel_textile_goods | 1 | 0.92 (global: fewer than 20 in-scope dev cases) | 1 / 1 = 100.0% | 1 / 1 = 100.0% |
| battery_solar | 4 | 0.92 (global: fewer than 20 in-scope dev cases) | 4 / 4 = 100.0% | 4 / 4 = 100.0% |
| electronics | 120 | 0.78 | 115 / 120 = 95.8% | 107 / 108 = 99.1% |
| filters | 38 | 0.95 | 22 / 23 = 95.7% | 24 / 26 = 92.3% |
| furniture_fixtures | 15 | 0.92 (global: fewer than 20 in-scope dev cases) | 14 / 15 = 93.3% | 14 / 15 = 93.3% |
| medical | 12 | 0.92 (global: fewer than 20 in-scope dev cases) | 8 / 9 = 88.9% | 8 / 9 = 88.9% |
| metal_tools_parts | 85 | 0.78 | 80 / 84 = 95.2% | 73 / 74 = 98.6% |
| motorised | 49 | 0.75 | 47 / 49 = 95.9% | 36 / 36 = 100.0% |
| other | 131 | 0.96 | 61 / 64 = 95.3% | 88 / 95 = 92.6% |

### dev (the bars were fitted here), n = 463

| Policy | auto-accepted (coverage, 95% CI) | accuracy of auto (95% CI) | wrong auto-accepted |
|---|---|---|---:|
| global 0.92 | 368 / 463 = 79.5% (75.6–82.9) | 355 / 368 = 96.5% (94.1–97.9) | 13 |
| per-family bars | 369 / 463 = 79.7% (75.8–83.1) | 352 / 369 = 95.4% (92.7–97.1) | 17 |

Paired (exact McNemar on the cases the policies treat differently): auto only under family bars 35, only under global 34, p = 1.000; wrong answers auto-accepted only under family bars 9, only under global 5, p = 0.424.

| Subfamily | n | bar | global: auto correct / auto | family: auto correct / auto |
|---|---:|---:|---|---|
| apparel_textile_goods | 4 | 0.92 | 1 / 1 = 100.0% | 1 / 1 = 100.0% |
| battery_solar | 4 | 0.92 | 4 / 4 = 100.0% | 4 / 4 = 100.0% |
| electronics | 121 | 0.78 | 107 / 108 = 99.1% | 115 / 120 = 95.8% |
| filters | 38 | 0.95 | 24 / 26 = 92.3% | 22 / 23 = 95.7% |
| furniture_fixtures | 16 | 0.92 | 14 / 15 = 93.3% | 14 / 15 = 93.3% |
| medical | 12 | 0.92 | 8 / 9 = 88.9% | 8 / 9 = 88.9% |
| metal_tools_parts | 85 | 0.78 | 73 / 74 = 98.6% | 80 / 84 = 95.2% |
| motorised | 50 | 0.75 | 36 / 36 = 100.0% | 47 / 49 = 95.9% |
| other | 133 | 0.96 | 88 / 95 = 92.6% | 61 / 64 = 95.3% |

### final (held out; bars frozen from dev, applied once), n = 377

| Policy | auto-accepted (coverage, 95% CI) | accuracy of auto (95% CI) | wrong auto-accepted |
|---|---|---|---:|
| global 0.92 | 300 / 377 = 79.6% (75.2–83.3) | 288 / 300 = 96.0% (93.1–97.7) | 12 |
| per-family bars | 287 / 377 = 76.1% (71.6–80.2) | 270 / 287 = 94.1% (90.7–96.3) | 17 |

Paired (exact McNemar on the cases the policies treat differently): auto only under family bars 25, only under global 38, p = 0.130; wrong answers auto-accepted only under family bars 8, only under global 3, p = 0.227.

| Subfamily | n | bar | global: auto correct / auto | family: auto correct / auto |
|---|---:|---:|---|---|
| apparel_textile_goods | 6 | 0.92 | 3 / 3 = 100.0% | 3 / 3 = 100.0% |
| battery_solar | 5 | 0.92 | 5 / 5 = 100.0% | 5 / 5 = 100.0% |
| electronics | 76 | 0.78 | 72 / 72 = 100.0% | 74 / 74 = 100.0% |
| filters | 1 | 0.95 | 1 / 1 = 100.0% | 1 / 1 = 100.0% |
| furniture_fixtures | 13 | 0.92 | 4 / 6 = 66.7% | 4 / 6 = 66.7% |
| medical | 8 | 0.92 | 6 / 6 = 100.0% | 6 / 6 = 100.0% |
| metal_tools_parts | 82 | 0.78 | 64 / 66 = 97.0% | 73 / 81 = 90.1% |
| motorised | 49 | 0.75 | 38 / 40 = 95.0% | 44 / 48 = 91.7% |
| other | 137 | 0.96 | 95 / 101 = 94.1% | 60 / 63 = 95.2% |
