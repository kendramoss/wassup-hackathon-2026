# Tenant isolation by deployment tier (#872)

Status: the application guard is shipped and off by default. The separate infrastructure per dedicated tenant is not built; the owner steps are below.

## Tiers

| Tier | Who it serves | Where its data lives |
|---|---|---|
| `shared` (default) | Every workspace and anonymous visitors, as today | The shared Cloud Run service and Firestore database |
| `dedicated` | One team workspace | Its own Cloud Run service, Firestore database and service account |

Isolation inside the shared tier is by workspace and owner (docs/design/case-isolation.md). The dedicated tier adds a hard boundary: a separate service, database and identity, so a bug in authorization cannot reach another customer's data.

## What the application does (shipped)

`internal/placement`, wired in `cmd/server` (`newInfra`):

- `TENANT_TIER=dedicated` with `TENANT_WORKSPACE=<workspace id>` wraps the case store. A case whose workspace is not that one is reported as not found on read, update and delete, and a write of one is refused. Lists drop them. Pinned demo cases are still served.
- Anonymous cases have no workspace, so a dedicated deployment refuses them: only signed-in members of the tenant's workspace (`TEAMS=on`) can create cases.
- `TENANT_TIER` unset or `shared` returns the store unchanged. `TENANT_WORKSPACE` without `dedicated` stops the server at startup.
- The guard is defence in depth. It does not replace the separate database below: with a shared database it only hides other tenants' cases from this service.
- The deep health check probes the raw store, so it keeps working under the guard.

## Owner steps for one dedicated tenant

Names below are placeholders: `TENANT` (short id), `WS` (the workspace id), `REGION` (for example `us-central1`).

1. Create a Firestore database for the tenant:
   `gcloud firestore databases create --database=op-TENANT --location=REGION --type=firestore-native`
2. Create a service account with access to that database only:
   `gcloud iam service-accounts create op-TENANT-backend`, then grant `roles/datastore.user` with an IAM condition limited to `resource.name.startsWith("projects/PROJECT/databases/op-TENANT")`.
3. Deploy a second backend service from the same image as the shared one, with that service account and these variables added to the shared service's set: `FIRESTORE_DATABASE=op-TENANT`, `TENANT_TIER=dedicated`, `TENANT_WORKSPACE=WS`, `TEAMS=on`. Give it its own `ADMIN_TOKEN` and `AUDIT_ACTOR_KEY`.
4. Deploy a frontend for the tenant whose backend URL is the new service (or route the tenant's host name to it at the load balancer). The shared frontend must not proxy to the dedicated backend.
5. Add the new service to the deploy workflow (a matrix entry or a second job). This is a workflow change, which this lane did not make.
6. Check: a case created on the shared site is not visible on the tenant's site, and the reverse; `GET /health?deep=1` on the dedicated service is `ok`.

## Not covered

Per-tenant model keys and spend caps, per-tenant encryption keys (see docs/design/cmek.md, which works per deployment), and moving an existing workspace's cases from the shared database into a dedicated one (an export and import script is needed).
