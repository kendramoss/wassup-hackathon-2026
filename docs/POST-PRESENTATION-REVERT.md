# Post-presentation revert checklist

Owner-only runtime changes made for the judging/presentation window. Work through this list **after the presentation**, in order. Nothing here needs a PR unless the "Permanent" step is chosen.

Set the CLI once:

```powershell
$gcloud = "C:\Users\kendr\AppData\Local\Google\Cloud SDK\google-cloud-sdk\bin\gcloud.cmd"
$project = "wassup-hackathon-2026"
$region = "us-central1"
```

## 1. Anonymous daily model budget (#654) — DONE

- Applied 24 Nov 2026: backend env `DAILY_SPEND_CLIENT_SHARE=0.5` (default was `0.1`).
- Effect: one client/office IP may spend $12.50/day instead of $2.50/day. The global `DAILY_SPEND_CAP_USD=25` was **not** changed.

Revert:

```powershell
& $gcloud run services update backend --project $project --region $region --remove-env-vars DAILY_SPEND_CLIENT_SHARE --quiet
```

Verify (no `DAILY_SPEND_CLIENT_SHARE` line means the `0.1` default is back):

```powershell
$j = & $gcloud run services describe backend --project $project --region $region --format=json 2>$null | ConvertFrom-Json
$j.spec.template.spec.containers[0].env | Where-Object { $_.name -match 'DAILY_SPEND' } | ForEach-Object { "$($_.name)=$($_.value)" }
```

If `DAILY_SPEND_CAP_USD` was also raised for the event, restore it with `--update-env-vars DAILY_SPEND_CAP_USD=25`.

## 2. Always-on backend instance (D59) — DONE

Runtime and deploy configuration both use zero minimum instances. The live backend had no `minScale` annotation when checked on 26 Nov 2026, and `.github/workflows/deploy.yml` has set `--min-instances 0` since #822. No revert command is needed.

Verify (`minScale` should be `0` or absent):

```powershell
$j = & $gcloud run services describe backend --project $project --region $region --format=json 2>$null | ConvertFrom-Json
"minScale=" + $j.spec.template.metadata.annotations.'autoscaling.knative.dev/minScale'
```

Evidence and the full Cloud Run inventory are in `docs/audit-m3-evidence.md`.

## Gotchas

- `deploy.yml` uses `--set-env-vars`, which replaces all backend env vars. A backend deploy during the event drops the budget override — re-apply step 1 if that happens.
- Budget resets are per UTC day; removing the share override affects the rest of the current day immediately.
- After reverting, smoke-test: `/api/v1/demos` 200, anonymous `/api/v1/cases` 401, homepage 200.

## Do NOT revert (permanent)

- `STRICT_AUTH=on`, `REQUIRE_BEARER=on`, `AUDIT_ACTOR_KEY` / `AUDIT=auto` (#561).
- Frontend Cloud Run `--timeout=900` and the MCP proxy's 900 s undici timeout (#587).
- `@undelabs/mcp` publishing and the `npx -y @undelabs/mcp` docs (#562).
- The two temporary API keys from the #587 check were already disabled; there is nothing left to clean up there.
