# wassup-hackathon-2026

AI agent for the hackathon: a Next.js chat app talking to a Go agent (Google ADK) that uses Gemini on Vertex AI. Its tools read what's deployed on Cloud Run and how it was built (`list_services`, `list_revisions`, `list_builds`, `get_build`), plus `get_time`. So far it can only look; deploying and rolling back come next. Replies stream in as they're generated, and every tool call is shown with its arguments and result.

Find the live URL with `gcloud run services describe frontend --region us-central1 --format='value(status.url)'`.

## How it works

```
browser ──► frontend (Next.js, public) ──► backend (Go, private) ──► Gemini on Vertex AI
              POST /api/chat/stream          POST /chat/stream         (location: global)
              adds an ID token               ADK agent + get_time tool
```

1. The chat page (`/chat`) sends your message to the frontend's `/api/chat/stream` route.
2. The route forwards it to the backend. On Cloud Run it first gets an ID token for the `frontend@` service account from the metadata server, because the backend only accepts calls from that account.
3. The backend's ADK agent sends the message to Gemini. If you ask about the time, Gemini calls `get_time`, the Go code answers, and Gemini writes the reply.
4. The reply streams back as server-sent events (tool calls, tool results, text as it's written). The page sends the `session_id` from the first event with the next message so the agent remembers the conversation.

| Path        | What                                     | Cloud Run service | Runs as                                                  |
|-------------|------------------------------------------|-------------------|----------------------------------------------------------|
| `backend/`  | Go agent (ADK Go); Cloud Run and Cloud Build tools in `backend/cloudtools` | `backend` (private) | `agent-backend@wassup-hackathon-2026.iam.gserviceaccount.com` (Vertex AI User, Cloud Run Viewer, Cloud Build Viewer) |
| `frontend/` | Next.js: landing page `/`, chat app `/chat`; `/api/chat*` proxy to the backend | `frontend` (public) | `frontend@wassup-hackathon-2026.iam.gserviceaccount.com` (Cloud Run Invoker on `backend` only) |

GCP project `wassup-hackathon-2026`, Cloud Run region `us-central1`. **No JSON keys** anywhere: Cloud Run uses the service accounts above, and locally you use your own Google login.

## CI/CD

- **CI** (`.github/workflows/ci.yml`) runs on every PR: `gofmt`, `go vet` and `go test` for the backend; `eslint` and `next build` (which type-checks) for the frontend. Each side only runs when its folder changed. `ci-ok` is the single check to require before merging.
- **Deploy** (`.github/workflows/deploy.yml`) runs on every push to `main`: CI first, then it deploys whichever service changed (backend before frontend), then sends "what time is it in Tokyo?" through the live site and fails unless the reply used `get_time`. Re-deploy everything by hand from **Actions → Deploy → Run workflow**.
- Deploys authenticate with **Workload Identity Federation**, so there are no keys or GitHub secrets. Only pushes to `main` of this repo can get a token, and they act as `github-deployer@`, which can build and deploy but can't change who is allowed to call a service. One-time setup: `bash scripts/setup-github-deploy.sh`.

So the normal flow is: branch → PR (CI runs) → merge → live in a few minutes. The manual commands below still work for one-off deploys.

## Deploy

Run from the repo root, with `gcloud` pointed at the project (`gcloud config set project wassup-hackathon-2026`). Each deploy uploads the folder, builds it with its `Dockerfile` in Cloud Build, and rolls out a new revision (3–5 minutes). The URLs don't change. Only redeploy the side you changed.

### PowerShell

```powershell
gcloud run deploy backend --source backend --region us-central1 --service-account agent-backend@wassup-hackathon-2026.iam.gserviceaccount.com --set-env-vars "GOOGLE_CLOUD_PROJECT=wassup-hackathon-2026,GOOGLE_CLOUD_LOCATION=global" --no-allow-unauthenticated --max-instances 3

$backend = gcloud run services describe backend --region us-central1 --format='value(status.url)'
gcloud run deploy frontend --source frontend --region us-central1 --service-account frontend@wassup-hackathon-2026.iam.gserviceaccount.com --set-env-vars "BACKEND_URL=$backend" --allow-unauthenticated --max-instances 3
```

Keep the quotes around the `--set-env-vars` value: without them PowerShell splits it at the comma and the backend gets a broken project ID.

### Bash

```sh
gcloud run deploy backend --source backend --region us-central1 \
  --service-account agent-backend@wassup-hackathon-2026.iam.gserviceaccount.com \
  --set-env-vars GOOGLE_CLOUD_PROJECT=wassup-hackathon-2026,GOOGLE_CLOUD_LOCATION=global \
  --no-allow-unauthenticated --max-instances 3

BACKEND=$(gcloud run services describe backend --region us-central1 --format='value(status.url)')
gcloud run deploy frontend --source frontend --region us-central1 \
  --service-account frontend@wassup-hackathon-2026.iam.gserviceaccount.com \
  --set-env-vars BACKEND_URL=$BACKEND \
  --allow-unauthenticated --max-instances 3
```

`--max-instances 3` caps how far each service scales, which caps how fast the public page can spend credits.

### Cost guard

A billing budget on this project emails billing admins as spend approaches its limit. It counts usage *before* credits, so it warns while credits are being used. It only alerts; it doesn't stop anything. See or change it under **Billing → Budgets & alerts** in the console. Per-user rate limiting is still open ([#3](../../issues/3)).

### Check it

PowerShell:

```powershell
$frontend = gcloud run services describe frontend --region us-central1 --format='value(status.url)'
$backend = gcloud run services describe backend --region us-central1 --format='value(status.url)'

# Through the public frontend
Invoke-RestMethod -Method Post "$frontend/api/chat" -ContentType 'application/json' -Body '{"message":"what time is it in Tokyo?"}'

# Backend directly (needs your identity token; without one you get 403)
$t = gcloud auth print-identity-token
Invoke-RestMethod -Method Post "$backend/chat" -Headers @{Authorization="Bearer $t"} -ContentType 'application/json' -Body '{"message":"hi"}'

# Backend logs
gcloud logging read 'resource.labels.service_name="backend"' --limit 20 --format='value(textPayload)'
```

Bash:

```sh
FRONTEND=$(gcloud run services describe frontend --region us-central1 --format='value(status.url)')
BACKEND=$(gcloud run services describe backend --region us-central1 --format='value(status.url)')

curl -X POST $FRONTEND/api/chat \
  -H 'Content-Type: application/json' -d '{"message":"what time is it in Tokyo?"}'

curl -X POST $BACKEND/chat \
  -H "Authorization: Bearer $(gcloud auth print-identity-token)" \
  -H 'Content-Type: application/json' -d '{"message":"hi"}'
```

A good reply has `"tool_calls":["get_time"]`.

## Run locally

One-time: `gcloud auth application-default login` (the backend uses your login to call Vertex AI).

PowerShell, in two terminals:

```powershell
cd backend; $env:GOOGLE_CLOUD_PROJECT="wassup-hackathon-2026"; go run .
cd frontend; $env:BACKEND_URL="http://localhost:8080"; npm run dev
```

Bash:

```sh
cd backend && GOOGLE_CLOUD_PROJECT=wassup-hackathon-2026 go run .
cd frontend && BACKEND_URL=http://localhost:8080 npm run dev
```

Open http://localhost:3000. Locally the frontend calls the backend without a token.

Tests and checks:

```sh
cd backend && go test ./... && go vet ./...
cd frontend && npm run lint && npx tsc --noEmit
```

## Backend API

| Endpoint      | What |
|---------------|------|
| `POST /chat`  | `{"message": "...", "session_id": "optional"}` → `{"reply", "session_id", "tool_calls", "tools"}`. `tool_calls` is tool names; `tools` adds each call's `args` and `result`. Message max 2000 characters; `session_id` must be one the backend returned. |
| `POST /chat/stream` | Same request; the reply as server-sent events, each `data: {"type": ...}`: `session` (with `session_id`), then any of `text` (a delta to append), `tool_call`, `tool_result`, then `done` or `error`. |
| `GET /health` | Liveness check, returns `ok`. |

| Env var                 | Default            | Notes |
|-------------------------|--------------------|-------|
| `GOOGLE_CLOUD_PROJECT`  | (required)         | |
| `GOOGLE_CLOUD_LOCATION` | `global`           | Vertex AI location. Gemini 3.x flash models aren't served from `us-central1`. |
| `GEMINI_MODEL`          | `gemini-3.5-flash` | `gemini-3.8-flash` works but was heavily queued on 2026-10-14 (3–74 s per call vs 1–2 s). |
| `CLOUD_RUN_REGION`      | `us-central1`      | Region the Cloud Run and Cloud Build tools look at. |
| `PORT`                  | `8080`             | Set by Cloud Run. |

The frontend reads `BACKEND_URL` (required) and `BACKEND_AUDIENCE` (optional: the backend's service URL, needed only when `BACKEND_URL` is a revision tag URL such as `https://preview---backend-...run.app`, because Cloud Run only accepts ID tokens for the service URL).

## Frontend

- **Copy:** every word on the landing page and the example prompts live in `frontend/src/lib/site.ts`. Rewrite that file when the idea is settled; the layout doesn't need to change.
- **Design:** colours are tokens in `frontend/src/app/globals.css` (`--bg`, `--fg`, `--muted`, `--subtle`, `--border`, `--accent`, ...), defined once for light and once for dark. Components use them as Tailwind colours (`bg-subtle`, `text-muted`, `border-border`), never raw colours. One accent colour, no gradients, icons only where they label an action or a status (`frontend/src/components/icons.tsx`).
- **Theme:** follows the system; the header button cycles System, Light, Dark and remembers the choice.
- **Checking UI changes:** with `npm run dev` running, `npm run ui-check` (in `frontend/`) saves screenshots of `/` and `/chat` in light/dark and desktop/mobile to `frontend/.ui-check/`, and fails on axe violations, console errors or horizontal scroll. It uses your installed Chrome or Edge.
- **Claude Code:** `.claude/skills/frontend-ui` holds the design rules and check loop, `.claude/skills/ui-review` reviews code against detailed interface rules, and `.claude/settings.json` turns on Anthropic's `frontend-design` plugin.
- **Accessibility:** skip link, labelled controls, visible focus rings, screen-reader announcement when a reply finishes, `prefers-reduced-motion` respected. Checked with axe-core: no violations on `/` and `/chat` in either theme.

## Gotchas

- **PowerShell and commas:** quote `--set-env-vars` values, or use Bash.
- **`404` on Vertex:** the model isn't available in that location. Use `GOOGLE_CLOUD_LOCATION=global`.
- **Slow replies:** check model latency before blaming the code. Time a direct Vertex call; switching `GEMINI_MODEL` needs no code change (`--update-env-vars GEMINI_MODEL=...`).
- **Paths ending in `z`** (like `/healthz`) are reserved by Cloud Run and return 404, which is why the health check is `/health`.
- **Streaming and ADK v1.8.0:** a streamed Gemini 3 reply is saved with an empty "thought signature" part that Vertex rejects on the next turn (400 INVALID_ARGUMENT). `dropEmptyParts` in `backend/main.go` strips it before each model call; remove it once ADK fixes this.
- **Previews pin traffic.** `gcloud run deploy ... --no-traffic --tag preview` gives a private test URL, but it also pins live traffic to the current revision, so later deploys don't go live. The Deploy workflow switches traffic back to the latest revision after every deploy; by hand, run `gcloud run services update-traffic <service> --region us-central1 --to-latest --remove-tags preview`.
- **Conversation memory is in-memory** on each backend instance. It's lost when the instance restarts, and a follow-up that lands on a different instance starts fresh ([#2](../../issues/2)).
- **`frontend/AGENTS.md`** is rewritten by `next dev`; commit it as is.
