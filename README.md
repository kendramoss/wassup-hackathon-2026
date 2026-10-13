# wassup-hackathon-2026

Monorepo for the hackathon.

| Path        | What                                                                   | Cloud Run service |
|-------------|------------------------------------------------------------------------|-------------------|
| `backend/`  | Go hello-world agent (ADK Go) using Gemini on Vertex AI, with one `get_time` tool | `backend`         |
| `frontend/` | Next.js page with a text box; `/api/chat` proxies to the backend        | `frontend`        |

GCP project: `wassup-hackathon-2026`. Region: `us-central1`.
The backend runs as `agent-backend@wassup-hackathon-2026.iam.gserviceaccount.com` (Vertex AI User).
**No JSON keys:** locally, use `gcloud auth application-default login`.

## Backend

`POST /chat` takes `{"message": "...", "session_id": "optional"}` and returns `{"reply", "session_id", "tool_calls"}`. `GET /healthz` is the liveness check.

| Env var                 | Default            |
|-------------------------|--------------------|
| `GOOGLE_CLOUD_PROJECT`  | (required)         |
| `GOOGLE_CLOUD_LOCATION` | `us-central1`      |
| `GEMINI_MODEL`          | `gemini-3.8-flash` |
| `PORT`                  | `8080`             |

Run locally:

```sh
cd backend
GOOGLE_CLOUD_PROJECT=wassup-hackathon-2026 go run .
curl -X POST localhost:8080/chat -d '{"message":"what time is it in Singapore?"}'
```

## Frontend

Run locally:

```sh
cd frontend
BACKEND_URL=http://localhost:8080 npm run dev
```

## Deploy

```sh
gcloud run deploy backend --source backend --region us-central1 \
  --service-account agent-backend@wassup-hackathon-2026.iam.gserviceaccount.com \
  --set-env-vars GOOGLE_CLOUD_PROJECT=wassup-hackathon-2026,GOOGLE_CLOUD_LOCATION=us-central1 \
  --allow-unauthenticated

gcloud run deploy frontend --source frontend --region us-central1 \
  --set-env-vars BACKEND_URL=$(gcloud run services describe backend --region us-central1 --format='value(status.url)') \
  --allow-unauthenticated
```
