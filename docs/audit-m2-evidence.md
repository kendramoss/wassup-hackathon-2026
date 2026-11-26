# Audit M2 evidence: real-Vertex family-check smoke (#276)

Run at: 2026-11-26T04:36:55Z

## Target

| Field | Value |
| --- | --- |
| Project / region | `wassup-hackathon-2026` / `us-central1` |
| Service | `backend` |
| Tag | `audit-m2` |
| Tag URL | `https://audit-m2---backend-guxax2teha-uc.a.run.app` |
| Revision | `backend-00242-vpx` (created 2026-11-25T23:06:07.169792Z) |
| Image | `us-central1-docker.pkg.dev/wassup-hackathon-2026/cloud-run-source-deploy/backend@sha256:f020a8e4abf5fb9d499b35dae120b2eb3e71f373a29f4db355325dfa56325bc4` |
| Service account | `agent-backend@wassup-hackathon-2026.iam.gserviceaccount.com` |
| Provider / model | `vertex` / `gemini-3.8-flash` (config.go default; ROUTER_MODEL unset) |
| Location / project env | `global` / `wassup-hackathon-2026` |
| Run kind | post-deploy re-check: the revision already served 100% of traffic, so this run does not count as the pre-promotion gate |

The tag URL addresses the revision directly. This run shifted no traffic; the untagged service URL remains the live route.

## Live cases against real Vertex

All inputs are synthetic. Tokens are not recorded.

| Case | HTTP | Family | Provider | Response |
| --- | ---: | --- | --- | --- |
| supported synthetic hair dryer | 200 | motorised_tools_small_appliances | vertex | `{"llm_provider":"vertex","prompt_version":"family-check-v2","result":{"family":"motorised_tools_small_appliances","name":"Motorised tools \u0026 small appliances","confidence":0.98,"reason":"A hair dryer is a classic motorized small appliance, fitting directly into the motorized_tools_small_appliances family which includes motor-fan assembly, heating elements, and housing moulding.","product_name":"Hair dryer","typed_unclear":false}} ` |
| unsupported synthetic yacht hull | 200 | none | vertex | `{"llm_provider":"vertex","prompt_version":"family-check-v2","result":{"family":"none","confidence":0.95,"reason":"Fibreglass boat and yacht hulls involve marine composite lay-up and moulding processes that do not fit into any of the available product families.","product_name":"Sailing yacht hull","typed_unclear":false}} ` |
| malformed request (blank text) | 400 | - | - | `{"code":"invalid_input","message":"invalid input: text is required, up to 2000 characters"} ` |
| malformed request (not JSON) | 400 | - | - | `{"code":"invalid_input","message":"invalid input: request body: invalid character 'e' in literal true (expecting 'r')"} ` |
| malformed request (unknown field) | 400 | - | - | `{"code":"invalid_input","message":"invalid input: request body: json: unknown field \"family\""} ` |
| malformed request (2001 characters) | 400 | - | - | `{"code":"invalid_input","message":"invalid input: text is required, up to 2000 characters"} ` |

## Offline guard evidence

Native Vertex cannot be forced to emit a bad enum, malformed JSON or a provider error on demand. Those paths stay covered by the existing loopback tests with a fake model:

```text
Skipped locally: this machine blocks execution of Go-built test binaries by application control.
Evidence: https://github.com/kendramoss/wassup-hackathon-2026/actions/runs/45754352954

Command run by the passing workflow:
GOFLAGS=-mod=readonly go test -count=1 -run 'TestSchemaSentToModelListsOnlyKnownFamiliesAndNone|TestInventedFamilyAndInvalidEnumAreRejected|TestMalformedModelOutputFailsClosed|TestProviderErrorsFailClosedWithoutRetry' ./internal/readiness/smokecheck/
```

These tests assert the schema enum contains only known template families plus `none`, invented or malformed families return `502 model_bad_response` with no result and no retry, and provider errors return `502 model_unavailable` without echoing upstream details.

## Promotion gate

Before production traffic moves to a new revision, a person must run this script against a no-traffic tag of that revision and keep the generated evidence. Promotion is blocked when any live case, guard test, or recorded revision/model/config check fails. Mocked tests alone never satisfy this gate. The script refuses a revision that already serves traffic unless `-AllowLiveRevision` is passed, and then it records the run as a post-deploy re-check. Since #845, `deploy.yml` runs the same cases automatically on a `--no-traffic` candidate (`scripts/backend-candidate-smoke.sh`) and shifts traffic only when they all pass. Each deploy's job summary holds that revision's evidence.
