# Broker workflow: review queue, collaboration and the eRulings filing package

Issues #879, #881 and #882; decision D924. Behind `BROKER_WORKFLOW=on` (default off: the routes are not registered and production is unchanged). API only; the screens are not built.

## What it adds

| Part | Routes | What it does |
|---|---|---|
| Review queue (#881) | `PUT /v1/cases/{id}/queue`, `POST /v1/cases/{id}/queue/transition`, `GET /v1/queue` | A case waits for a named person to review it. Priorities set a due time from when it was queued: urgent 4 h, high 24 h, normal 72 h, low 7 days. States: queued → assigned → in_review → decided (approve, changes or reject, with a required reason and whether it beat the due time). Release puts it back. The list puts overdue items first and suggests cases the model routed to review that nobody queued. |
| Collaboration (#882) | `GET /v1/cases/{id}/collab`, `POST /v1/cases/{id}/comments`, `POST /v1/cases/{id}/approvals`, `PUT /v1/cases/{id}/collaborators`, `GET` and `POST /v1/review/comments` | Threaded comments (anyone who can see the case, including a workspace viewer and the broker on a review link), sign-offs (approve, changes or reject; a reason unless approving), and per-case roles (broker, reviewer). The sign-off counts each person's latest decision; a broker's approval, in the case or as an approve or file_request answer on a review link, is what a filing needs. |
| Filing package (#879) | `POST /v1/cases/{id}/filing`, `GET /v1/cases/{id}/filing[?format=json|zip|pdf]`, `POST /v1/cases/{id}/filing/status` | The 19 CFR 177.2 letter (the `cfr177` export, or the broker's edited letter) filled from the requester's answers, checked element by element, packed with its exhibits, and tracked after it leaves. |

## Who may do what

The existing rules apply first: a hidden case is 404, a pinned demo takes none of these writes, and a workspace viewer may read the queue, the discussion and the package and may comment, but not queue, decide, sign off, set roles or file (403). In a workspace only an admin assigns a review to someone else; an editor may take one. Only the assignee decides. A per-case role (broker, reviewer) names how a person's comments and sign-offs count; it never lets anyone see a case they could not see before.

## The element check

Each required content of a ruling request is tied to a section of the generated letter. An element is present when its section is there and holds no `[FILL IN …]` placeholder the letter does not itself mark as optional ("if known", "if available", "or delete", "or none"):

| Key | Rule | Letter section |
|---|---|---|
| parties | 19 CFR 177.2(b)(1) | 1. Interested parties |
| port (if known) | 177.2(b)(1) | 2. Port of entry |
| transaction | 177.2(b)(1) | 3. The transaction |
| merchandise | 177.2(b)(1), (b)(3) | 4. The merchandise |
| origin_facts | CBP eRulings origin checklist | 5. Materials, countries and processing |
| question | 177.2(b)(1) | 6. Question presented |
| position | 177.2(b)(6) | 7. Requester's position and supporting authority |
| prior | 177.2(b)(2)(ii) | 8. Prior or current consideration |
| documents | 177.2(b)(3)-(4) | 9. Documents and samples |
| confidential | 177.2(b)(7) | 10. Confidential treatment |
| signature | 177.1(c), 177.2(c) | 11. Declaration and signature |

The requester's answers (`submitter`) fill the placeholders they answer, by exact wording; a test on the generated letter fails if that wording changes. `ready` means every required element is present; `ready_to_file` also needs a broker's approval and no sign-off asking for changes or rejecting. Moving to `submitted` needs `ready_to_file`.

The check is about form, not merit: it says the request has each content the rule asks for, not that the content is right. Counsel still reads the letter.

## The package

`format=zip` holds `package.json` (schema `undelabs.erulings-package.v1`: submitter, elements, sign-offs, letter, Exhibit A materials, Exhibit B steps, file list with SHA-256, how to submit), `letter.pdf` (a plain text PDF made without a library), `letter.md`, `exhibit-a-materials.csv` and `exhibit-b-steps.txt`. The same package gives the same bytes. Files the requester must add (photographs or drawings, and any listed attachment) are named in the file list as `to_attach`; no attachment bytes are stored.

Nothing is sent to CBP. eRulings has no public submission API; the broker files the package by hand and records `submitted` with the eRulings reference, then `acknowledged` and `ruling_issued` with the ruling number. `withdrawn` is open until a ruling.

## Storage and audit

Everything is inside the case document (no collection, no index), so it is isolated, exported and deleted with the case. Audit actions: `queue.change` (enqueue, assign, start, decide, release), `case.comment`, `case.approval`, `case.collaborator`, `filing.prepare`, `filing.status`, and `filing.download` for a json, zip or pdf download. None fails closed. No comment, reason or letter text reaches the trail. Each change is also an event in the case history.

## Not done

Screens; automatic queueing of below-bar answers; email or other notifications; tracked edits to the letter (an edited letter can be filed, but changes are not diffed); attachment upload; any submission to CBP.
