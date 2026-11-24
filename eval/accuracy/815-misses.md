# #815 miss table: U.S. engine, development split, food and no-family slices

Development rulings only (469). No held-out ruling was read. Made by `diagnose_815.py`, no model calls. Causes were assigned by hand from the ruling facts, CBP's held origin and the engine's reasoning and precedents.

- Engine rows: the official development run of the production configuration (`dev-gemini-4-argon-prec-engine-v1-vertex`, 422 of 469).
- Slice: the last family check that covers all 469 development rulings (`families-coverage-dev-all-litellm-f26-b2-g3`, 26 templates, match bar 0.6); a ruling is in the template it matched, else "none".

| Slice | Rulings | Right | Misses |
|---|---:|---:|---:|
| food | 20 | 16 | 4 |
| none | 118 | 103 | 15 |
| other | 331 | 303 | 28 |

## Causes

| Cause | Food | None | Both |
|---|---:|---:|---:|
| Rule applied (wrong side of a CBP rule) | 3 | 9 | 12 |
| Retrieval (precedents misled or lacked the analogue) | 0 | 1 | 1 |
| Missed fact | 0 | 0 | 0 |
| Label or case ambiguous (two scenarios, or the engine flips between identical runs) | 1 | 5 | 6 |

Also found: the engine answers China 92 times on development and is right 69 times; 23 of its 47 misses are China answers.

## Misses

| Ruling | Slice | CBP | Engine | Cause | Unstable | Why |
|---|---|---|---|---|---|---|
| H331375 | none | Italy | China | rule | no | House kit made in China, assembled in Italy, used, disassembled: the engine kept China; HQ held Italy, where the kit last became the house. |
| H354210 | none | United States | Brazil | rule | no | Retanning and dyeing U.S. wet blue hides in Brazil was read as a transformation to crust leather; HQ held the United States. |
| N348376 | none | Germany | China | rule | no | Sintered spinel from China, then crushed, milled and classified in Germany: engine kept China (crushing is not a transformation); CBP held Germany. Label is manual. |
| N349972 | none | Taiwan | China | label | yes | Helmet shell moulded in Taiwan, liner and assembly in China. Essential character is close; the engine flips between identical runs (right in -litellm-gw). |
| N350091 | none | India | China | rule | no | Pen with an Indian refill and Chinese barrel parts assembled in China: the engine named N345964 (refill gives character) and still chose China, at 0.65. |
| N350517 | food | France | Guatemala | rule | no | Pureeing and blending fruit, then pasteurising, was read as simple processing; CBP held France (a different food: puree). No fruit-puree precedent exists before this date, so retrieval is not the cause. |
| N351073 | none | China | Thailand | label | no | Oven lights with two scenarios; the label is the second scenario's result and the engine answered the first. The ruling has two answers. |
| N351081 | none | China | Thailand | rule | no | Air filter: CBP held China, where the filter paper is made; the engine took Thailand, where it was pleated and assembled. |
| N351174 | none | Thailand | China | label | yes | Jack stands: Thai ratchet bar, Chinese base parts, assembly in Thailand. Close; right in two other identical runs. |
| N351368 | none | China | Vietnam | rule | no | Door lock: Chinese knob assemblies, Vietnamese cylinder and latch. CBP held China; the engine held Vietnam. Retrieved precedents are ovens and ranges, not locks. |
| N352231 | food | Ireland | Spain | label | yes | Cooking and canning razor clams was read as a transformation; CBP held the harvest country. The gateway run (-litellm-gw) was right: the call is close. |
| N352389 | food | Peru | China | rule | no | Esterification and distillation of crude fish oil was read as a chemical transformation; CBP held Peru (refining an oil). Label is manual. |
| N352485 | none | Thailand | China | retrieval | no | Four retrieved sibling air filters all held China (Chinese media); here the media is composited and pleated in Thailand and CBP held Thailand. The near-duplicate precedents anchored the answer. |
| N353232 | none | United Kingdom | Canada | label | yes | Soap: UK soap base, mixed and moulded in Canada. Close; right in one other identical run. |
| N353581 | none | France | United States | rule | no | Borax pentahydrate (U.S.) made into decahydrate in France: the engine held that a hydration change is not a transformation; CBP held France (new chemical name and use). |
| N353597 | none | Cambodia | China | rule | no | Metal tiara body from China, Cambodian headband and combs, gluing, screwing and ribbon in Cambodia: engine kept China; CBP held Cambodia. |
| N353879 | none | Thailand | China | rule | no | Showerhead: about 40 parts from China, assembled and tested in Thailand. The engine called it simple assembly; CBP held Thailand. |
| N354481 | food | United States | China | rule | no | Cooking skewered chicken was read as a transformation (N345699, fully cooked, battered chicken: China); CBP held the United States, as for chargrilled chicken (N339749). Retrieved precedents point both ways. |
| N356520 | none | South Korea | China | label | yes | Deodorising filter: Korean core and mesh, Chinese carbon, filled and heat-pressed in Korea. The five retrieved water filters all held Korea; right in one other identical run. |

Unstable: another run of the same configuration answered the ruling correctly.
