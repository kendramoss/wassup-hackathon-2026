# Modelled operational costs (issue #281)

This describes what the relocation plan, the country comparison and the scenarios do with the optional operating-cost inputs. It describes the code and its limits. It is not an estimate of anyone's real costs, a legal opinion, or a claim that a plan is the best possible one. Every dollar figure is something the user typed.

## When it applies

By default a plan prices a move as it always did: the yearly duty saved, minus the yearly group costs the user entered for the moved groups. Nothing in this document changes that.

The operating-cost model is optional. It turns on only when a request carries an `economics.operations` block that has at least one value (a horizon, a maximum lead time, or an entry under `groups`). An empty block, `null`, a block with only a note or sources, and an entry that holds only a `ref` or `sources` (no value) do nothing and are not stored. A case that never uses it keeps the old path and its old results.

## What a "best" plan means

In the operating-cost mode a plan is ranked by

    modelled net per year = duty saved per year
                            - operating cost difference per year
                            - one-time costs spread over the horizon

It is the best of the moves that were searched (the template's movable groups and the plants that can perform them, up to the search limit), using only the inputs the user supplied. It is not a global optimum. Anything the model does not include is returned with each result under `not_modelled`, also for a fully priced result, and a client must show it beside any priced figure. This describes what the API returns; it does not claim that any screen displays it.

## Inputs, units and limits

All amounts are US dollars as typed; none is estimated or defaulted.

- `horizon_years` (positive, at most 30): spreads one-time costs. It is required when a one-time cost applies and is never defaulted, because a different horizon can change which plan wins.
- `max_lead_time_days` (whole days, 0 to 3650): the user's limit for how long a move may take.
- Per group: `cost_basis` (`included` or `additional`), `unit` (a label, compared exactly, including case), `annual_demand_units`, `orders_per_year` (greater than 0).
- Per group and plant: `labour_usd_per_year`, `logistics_usd_per_year`, `capex_usd`, `retooling_usd`, `lead_time_days`, `moq_units` with `moq_unit`, and a `ref` (the plant's trimmed name, or its id when it has no name, and its country) so that an operating-cost entry cannot silently attach to a different plant. This protects the new operating-cost entries only: the older `group_costs` table is still keyed by plant id with no ref, so the group-cost term of a result can still carry over to a plant that was removed and re-added with the same id (the known #372 limit, not fixed here).
- `note` and `sources` are notes. They change no figure and no hash.
- Limits: 64 groups, 32 plants per group, values up to 1e12, a request block up to 64 KiB. A value outside a limit is refused with a message naming the field.

An explicit 0 is a known zero. A field left empty is not entered, and the model never treats it as 0.

Inputs are limited to 1e12, but a computed figure or intermediate above 9e13 dollars (the arithmetic bound that keeps cents exact) becomes unknown with the reason `arithmetic_range`, so many legal entries can still turn a total unknown.

## How the numbers are built

- Duty: the annual import value times the difference between the duty rates of the current and the new origin, as before.
- Operating difference: for each moved group, the operating cost at the destination plant minus the operating cost at the current plant. It is always a difference, never one side alone.
- Cost basis and double counting: the group cost the user entered may already contain labour and logistics. `included` means the labour and logistics entered are only a breakdown of that group cost and are never added again (they must not exceed it). `additional` means they are added on top. Entering labour or logistics without saying which is refused. A user who wants labour and logistics to be the only operating cost enters an explicit 0 group cost.
- One-time costs: capex and retooling count only at the destination plant of a moved group, are added up and divided by the horizon (straight line; no discounting, residual value or tax).
- Sunk capital: a one-time cost entered at a group's current plant is baseline capital that already exists. It is never credited, never refunded and never makes a target unknown. It is listed as ignored.
- Rounding: each term (group cost, labour, logistics, the one-time share per year) is rounded to cents and the totals add those terms, so the absolute baseline and proposed sides can differ from the net by up to one cent.

## Unknown, not modelled, and zero

- A figure that is needed and was not entered makes the affected total unknown. The API returns it as unknown (`null`), never as 0, with the reason listed; a client must not show it as $0.
- A factor the user did not enter anywhere for a group is not modelled for it. It is listed under `not_modelled` and does not make the total unknown.
- A priced figure covers the modelled terms only. The following are never modelled: capacity use, discounting and time value, taxes and residual value, ramp-up and yield, subsidies and incentives, and any constraint whose input was not entered.
- In results, an unknown money amount is `null`, not 0. The plan's older `net_per_year` field is then only a ranking aid, so read `net_priced` before showing it; the authoritative figures are inside `operations`.

## Feasibility

Feasibility is separate from the money. It is `feasible`, `infeasible` or `unknown`, and it is not assessed at all when no limit was entered.

- Lead time: the move takes as long as its slowest moved group (an assumption that retooling runs in parallel, stated in every result). A known lead time above the limit is infeasible; a missing one is unknown; equal to the limit is feasible.
- Minimum order quantity: for a moved group, annual demand must cover the minimum order for the orders per year (demand at least the minimum times the orders; equal is feasible). The units must match exactly; there is no conversion. A missing input is unknown. A minimum of 0 is feasible. A minimum entered only at the current plant is ignored.

## What is recommended

Cost can only demote a plan that already passed the legal and precedent checks. It never promotes one and never changes a legal result.

- A known infeasible move is moved to the rejected rows as "operationally infeasible", with the check and the numbers.
- A fully priced move whose modelled net is negative is moved to the rejected rows as "net costlier", with the terms. A net of exactly 0 stays shown as no modelled saving.
- Unknown money never demotes a plan, and unknown feasibility never does either on its own. A known negative modelled net, or a known infeasibility, does demote it even when the other side is unknown. An unknown is never called feasible or beneficial.
- In the operating-cost mode, shown plans with a fully priced net rank first, then by net. The search itself is unchanged.

## Which money a request uses

A request that carries a positive annual import value uses its own economics, whole. In the operating-cost mode, an explicitly active `operations` block also uses its own economics, whole, even at an annual value of 0: nothing is priced, but lead time and minimum order are still assessed and can still demote a move. That body replaces the stored economics instead of being merged with them. Without an active `operations` block and without an annual value, the stored economics are used, as before. A client that wants to keep stored money omits `operations` or sends it inactive. The same rule covers plans, the country comparison and saved scenarios.

## Staleness and history

Only a plan result computed in the operating-cost mode records a hash of the raw money inputs it was priced from; plans run without `operations` do not carry one and are not marked stale by this mechanism. If the user later changes a cost, a rate, a constraint or the annual value, the API marks the stored plan stale: a client must not present its old money as current, and the plan has to be run again. Its legal result and the draft letter, which has no money, stay valid. Editing only notes or sources never changes the hash and never stales anything. A stored plan is not rewritten when it goes stale, and earlier cases are not migrated.

## What this does not claim

It does not say a plan is cost-optimal, cheapest or approved. It is not legal advice, a customs ruling or a certification. It makes no statement about real prices, real suppliers or any live result; the figures are the user's. Plant entries are matched by name and country, not by a unique identity, and a plant with no name is identified by its id.

See also decision D136 in `DECISIONS.md` and the API description in `api/openapi.yaml`.
