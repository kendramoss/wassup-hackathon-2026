# Data residency (#873)

Status: the application guard is shipped and off by default. Regional infrastructure is not built; the owner steps are below. Nothing here is a residency guarantee: the deployment's region decides where data is stored, and model calls go wherever the configured model endpoint runs.

## Where case data lives today

One region (the deploy workflow's `REGION`, `us-central1`): the Cloud Run services, the Firestore database (cases, caches, audit trail, rate counters) and the logs. Model requests go to the LiteLLM gateway or Vertex endpoint in the configuration.

## What the application does (shipped)

`internal/placement`, wired in `cmd/server` (`newInfra`):

- `DATA_REGION=<name>` (`us`, `eu`, ...) tags every case this deployment writes with `data_region`.
- A case tagged with another region is reported as not found on read, update and delete, a write over it is refused, and lists drop it. This catches a misrouted request or a database copied across regions.
- A case with no tag (written before residency was on) is served and tagged on its next write, so turning residency on needs no migration in a database that already belongs to the region.
- `DATA_REGION` unset changes nothing.

## Owner steps for an EU deployment

1. Create the database in the region: `gcloud firestore databases create --database=op-eu --location=eur3 --type=firestore-native` (eur3 is a multi-region in the EU; `europe-west1` is a single region).
2. Deploy the backend and frontend services in an EU region (`europe-west1`) from the same image, with `FIRESTORE_DATABASE=op-eu` and `DATA_REGION=eu` added to the usual variables.
3. Point the EU host name or the customer's workspace at the EU frontend. Routing customers to a region is not built: today a customer would use the EU host name.
4. Model calls: configure an EU model endpoint for the EU services (a Vertex location in the EU, or a gateway deployed in the EU). Without this, case facts still go to the existing endpoint.
5. Logs: create a Cloud Logging bucket in the EU and a sink routing the EU services' logs to it (`gcloud logging buckets create op-eu-logs --location=europe-west1`, then `gcloud logging sinks create ...` with a filter on the service names), and exclude them from the `_Default` bucket.
6. Encryption keys for the region, if CMEK is wanted: a key ring in the same location (docs/design/cmek.md).
7. Add the EU services to the deploy workflow. This is a workflow change, which this lane did not make.
8. Check: a case created on the EU site is in the `op-eu` database with `"data_region":"eu"` in its stored JSON, and `GET /v1/cases/{id}` for it on the U.S. service answers 404.

## Not covered

Choosing a region per workspace inside one deployment, moving existing cases between regions, the precedent corpus (public CBP and Japan Customs rulings, bundled in the image, not customer data), and where a model provider processes a request.
