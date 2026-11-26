# Customer-managed encryption keys (#874)

Status: application-layer envelope encryption of the cache blobs is shipped and off by default (`BLOB_ENCRYPTION`). Customer-managed keys for Firestore itself are infrastructure, not built; the owner steps are below.

## Two layers

| Layer | What it protects | Who builds it |
|---|---|---|
| Firestore CMEK | Every stored document: cases, caches, audit trail | Owner, when creating the database (below) |
| Envelope encryption in the application | The engine prediction and precedent-check cache blobs, keyed by case content | Shipped, `internal/envelope` |

Firestore CMEK is the primary control: revoking the key makes the whole database unreadable. The application layer adds a second key over the caches, which hold content derived from case facts and are not removed when a case is deleted (docs/design/data-handling.md, item 7): revoking that key makes them unreadable without touching the cases.

## What the application does (shipped)

`internal/envelope`, wired in `cmd/server` (`newInfra`, verifier and engine caches):

- Each process makes one random 256-bit data key, wraps it with the key-encryption key, and seals every cache blob with AES-256-GCM under it. The blob's key is bound in as associated data, so a sealed blob copied under another key does not open.
- The stored blob records the wrapped data key, so any later process can open it. Unwrapped data keys are cached in memory (at most 64).
- `BLOB_ENCRYPTION=kms` with `BLOB_ENCRYPTION_KMS_KEY=projects/P/locations/L/keyRings/R/cryptoKeys/K` wraps data keys with Cloud KMS (REST `cryptoKeys.encrypt` and `decrypt`, application-default credentials): one KMS call per process start, and one per older data key read.
- `BLOB_ENCRYPTION=local` with `BLOB_ENCRYPTION_KEY` (32 bytes, base64) is for development and tests. It is refused on Cloud Run, since a key in the environment is not customer-managed.
- A blob that cannot be opened (key disabled or revoked, KMS unreachable, blob copied under another key) is a cache miss: the prediction or precedent check is recomputed, never failed.
- A blob written before encryption was on is read as stored, so turning it on needs no migration. Turning it off again leaves sealed blobs unreadable as cache entries; the caches are disposable, so clear the `verifier_cache` and `engine_cache` collections when doing that.
- `BLOB_ENCRYPTION` unset (`off`) stores blobs exactly as before.

## Owner steps

1. Create a key ring and key in the database's location:
   `gcloud kms keyrings create op-keys --location=us-central1`
   `gcloud kms keys create op-cache --keyring=op-keys --location=us-central1 --purpose=encryption --rotation-period=90d --next-rotation-time=...`
2. Grant the backend's service account `roles/cloudkms.cryptoKeyEncrypterDecrypter` on that key only.
3. Set `BLOB_ENCRYPTION=kms` and `BLOB_ENCRYPTION_KMS_KEY=projects/PROJECT/locations/us-central1/keyRings/op-keys/cryptoKeys/op-cache` on the backend service.
4. Firestore CMEK (a new database; an existing database cannot be switched): grant the Firestore service agent `roles/cloudkms.cryptoKeyEncrypterDecrypter` on a key in the database's location, then `gcloud firestore databases create --database=op-cmek --location=us-central1 --kms-key-name=projects/PROJECT/locations/us-central1/keyRings/op-keys/cryptoKeys/op-db`, copy the data across (export and import), and switch `FIRESTORE_DATABASE`.
5. Check: a stored document in `verifier_cache` holds `"openv":"openv1"` and no readable facts; after disabling the key version, a repeated prediction still answers (recomputed, at model cost) and `GET /health?deep=1` is still `ok`.

## Not covered

Encrypting case documents field by field in the application (Firestore CMEK covers them), per-tenant keys inside one deployment, and key rotation of already-sealed blobs (KMS rotation applies to new data keys; old blobs keep opening with their key version until it is destroyed).
