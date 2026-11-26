# SLAs and status page (#885)

Status: service-level objectives are defined and a status endpoint is shipped, off by default (`STATUS_PAGE=on`). Monitoring, alerting and a hosted status page are infrastructure, not built; the owner steps are below. No SLA is offered: an SLA is a contract with credits, and these objectives are the measurement it would be written against.

## Objectives (`httpapi.DefaultSLOs`)

| Objective | Target | Window | Measured as |
|---|---|---|---|
| Availability | 99.5% | 30 days | Share of `/v1` requests answered without a 5xx; a 503 from a configured limit (daily model budget, rate limit) does not count against it |
| Case reads | 99% under 1 s | 30 days | `GET /v1/cases/{id}` latency at the backend |
| Predictions | 95% under 60 s | 30 days | `POST /v1/cases/predict` latency at the backend |

99.5% over 30 days is about 3.6 hours of error budget. Prediction latency depends on the model provider; a provider outage spends the availability budget.

## What the application does (shipped)

`GET /v1/status` (`internal/httpapi/status.go`, contract in `docs/api/openapi.yaml`):

- Answers `status` (`operational`, `degraded`, or `unknown` before the first snapshot), one entry per component (`case_store`, `model`) and the objectives above.
- Never runs a check on the request path. It returns the last snapshot of the deep health checks and starts one background refresh when the snapshot is older than 5 minutes, so a monitor polling every few seconds costs at most one model call every 5 minutes.
- Never returns error text; `GET /health?deep=1` (admin side) keeps the details.
- Rate-limited like other cheap routes. With `STATUS_PAGE` unset the route does not exist.

## Owner steps

1. Set `STATUS_PAGE=on` on the backend. The frontend already proxies `/api/v1/*`, so the status is at `https://undelabs.dev/api/v1/status` (the frontend route is public; no change needed unless the proxy restricts paths).
2. Uptime check: `gcloud monitoring uptime create op-status --resource-type=uptime-url --resource-labels=host=undelabs.dev,project_id=PROJECT --path=/api/v1/status --period=5 --matcher-content='"status":"operational"' --matcher-type=contains-string`.
3. SLOs in Cloud Monitoring: create a Cloud Run service in Service Monitoring for `backend`, then one request-based SLO per row above (availability: good = response code class not 5xx; latency: distribution cut on `run.googleapis.com/request_latencies` at 1000 ms for the case-read route and 60000 ms for predict, filtered by the `route` label the request log carries).
4. Alerting: a burn-rate alert per SLO (fast burn: 2% of the budget in 1 hour; slow burn: 10% in 6 hours) to the on-call channel.
5. Hosted status page: either a small public page that reads `/api/v1/status` (a frontend change, out of this lane's scope), or a third-party status page fed by the uptime check in step 2.
6. Write the SLA (availability commitment, measurement, exclusions, credits) against the availability objective once 30 days of measured data exist.

## Not covered

Multi-region failover, synthetic end-to-end prediction checks (they cost model calls), incident history on the status page, and latency measured at the user's browser.
