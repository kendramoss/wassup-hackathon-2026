# Audit M3 evidence: Cloud Run exposure and cost (#277)

Inventory taken 26 Nov 2026 with `gcloud` signed in as the project owner against project `wassup-hackathon-2026`, region `us-central1`. All commands were read-only. No IAM, scaling or service deletion was applied.

## Current services

| Service | Ingress | Invoker IAM | Traffic | Min / max instances | Service account | Last deployed by |
| --- | --- | --- | --- | --- | --- | --- |
| `backend` | `all` | `roles/run.invoker` only for `serviceAccount:frontend@wassup-hackathon-2026.iam.gserviceaccount.com` | `backend-00242-vpx` 100% (plus no-traffic `audit-m2` tag) | 0 (default) / 3 | `agent-backend@…` | project owner (manual tag for #276) |
| `frontend` | `all` | `roles/run.invoker` for `allUsers` | `frontend-00255-6nk` 100% | 0 (default) / 3 | `frontend@…` | `github-deployer@…` |
| `origin-proof-api-staging` | `all` | no service-level bindings | `origin-proof-api-staging-00018-cxd` 100% | 0 (default) / 3 | `agent-backend@…` | collaborator (October staging) |
| `origin-proof-frontend-staging` | `all` | no service-level bindings | `origin-proof-frontend-staging-00001-wkp` 100% | 0 (default) / 1 | `frontend@…` | collaborator (October staging) |

Commands used:

```powershell
gcloud run services list --project wassup-hackathon-2026 --region us-central1 --format=json
gcloud run services describe SERVICE --project wassup-hackathon-2026 --region us-central1 --format=json
gcloud run services get-iam-policy SERVICE --project wassup-hackathon-2026 --region us-central1 --format=json
```

`minScale` was absent on every service, which Cloud Run treats as 0: no always-on instance remains. `.github/workflows/deploy.yml` also sets `--min-instances 0` for the backend since #822.

## Unauthenticated reachability

| URL | Unauthenticated GET | Reading |
| --- | ---: | --- |
| `https://backend-guxax2teha-uc.a.run.app/health` | 403 | private at the Google front door |
| `https://frontend-guxax2teha-uc.a.run.app/` | 308 | redirects to the custom domain; public by IAM |
| `https://undelabs.dev/` | 200 | intended public judged frontend |
| `https://origin-proof-api-staging-guxax2teha-uc.a.run.app/` | 403 | private at the Google front door |
| `https://origin-proof-frontend-staging-guxax2teha-uc.a.run.app/` | 403 | private at the Google front door |

Ingress is `all` on all four services, but ingress is not the access decision. The IAM policies and probes show that only `frontend` is publicly invocable. The two staging services have no service-level bindings and are not public. The backend is invocable by the frontend service account; project owners can still reach it with an identity token.

## Scale and cost

- No service has a minimum instance. The D59 warm backend is gone in runtime and in deploy configuration; `docs/POST-PRESENTATION-REVERT.md` has been corrected to DONE.
- The backend still carries the `audit-m2` tag from the #276 run. It points at `backend-00242-vpx`, which is live today. After the next deploy, the tag will keep that old revision reachable at its own URL to anyone with invoker access. Remove it with `gcloud run services update-traffic backend --remove-tags=audit-m2` once #844 merges.
- Artifact Registry repo `cloud-run-source-deploy` has no cleanup policy and holds about 9.2 GB: 238 backend, 254 frontend, 13 staging-backend and 1 staging-frontend images. That costs under $1 a month today, but it grows with every deploy and is the only cost that does while the services are idle. A cleanup policy is proposed in #846.
- Backend and frontend allow at most 3 instances; staging frontend allows 1; staging backend allows 3.
- Backend env keeps `DAILY_SPEND_CAP_USD=25`, `STRICT_AUTH=on`, `REQUIRE_BEARER=on` and `API_KEYS=on`.
- Billing account `013F2A-AD8D07-FFCDBA` (`Wassup hackathon`) has budget `wassup-hackathon-2026 usage`: MYR 100 per calendar month, scoped to project `88903435351`, with current-spend alerts at 50%, 90% and 100%, plus a forecasted-spend alert at 100%. The in-app $25/day cap is a separate model guard, not this billing alert.

Read-only budget command:

```powershell
gcloud beta billing budgets list --billing-account=013F2A-AD8D07-FFCDBA --format=json
```

## Proposed changes, not applied

These need owner approval because they change live Cloud Run configuration:

1. Delete `origin-proof-api-staging` and `origin-proof-frontend-staging`. Both are private, unused since October, and already scale to zero, so deletion saves retained images and removes accidental invocation by project principals; it does not reduce idle compute. The live demo uses only `frontend` and `backend`.
2. If the staging services are kept instead, leave the IAM as-is (private) and optionally tighten ingress from `all` to `internal-and-cloud-load-balancing`. This is defence in depth, not a fix for a current public exposure.
3. Add an Artifact Registry cleanup policy (#846): keep the 10 newest versions per image, try it with `--dry-run` first, and make sure images behind live or tagged revisions are kept.
4. Remove the `audit-m2` tag from `backend` after #844 merges.
5. Change the backend deploy to `--no-traffic` plus an explicit promote step so the #276 gate can actually run before production (#845).
6. Keep the existing MYR 100/month budget; no new alert is required. Review the currency and threshold once the account is no longer a hackathon billing account.

Nothing here changes traffic, IAM, scaling or the live demo.
