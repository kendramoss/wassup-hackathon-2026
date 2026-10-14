// Server-only helpers for calling the Go backend from route handlers.

export class BackendError extends Error {
  constructor(
    message: string,
    public status: number,
  ) {
    super(message);
  }
}

// Backend statuses that describe the user's request; their message is shown
// as-is. Anything else (auth failures, crashes) is our problem, not theirs.
const userErrors = new Set([400, 409, 413, 429]);

// POSTs body to path on the backend. On Cloud Run (K_SERVICE is set) the
// backend is private, so this authenticates with an ID token for this
// service's identity; locally it calls the backend as-is.
//
// The token's audience must be the backend's service URL. BACKEND_AUDIENCE
// overrides it when BACKEND_URL is a revision tag URL (previews), which Cloud
// Run doesn't accept as an audience.
export async function callBackend(path: string, body: string, signal?: AbortSignal): Promise<Response> {
  const backendUrl = process.env.BACKEND_URL?.replace(/\/$/, "");
  if (!backendUrl) {
    throw new BackendError("BACKEND_URL is not set", 500);
  }

  const headers: Record<string, string> = { "Content-Type": "application/json" };
  if (process.env.K_SERVICE) {
    headers.Authorization = `Bearer ${await idToken(process.env.BACKEND_AUDIENCE || backendUrl)}`;
  }

  const res = await fetch(`${backendUrl}${path}`, { method: "POST", headers, body, signal });
  if (!res.ok) {
    const text = (await res.text()).trim();
    if (userErrors.has(res.status)) {
      throw new BackendError(text, res.status);
    }
    console.error(`backend ${path} returned ${res.status}: ${text.slice(0, 500)}`);
    throw new BackendError("The agent is unavailable right now. Try again in a moment.", 502);
  }
  return res;
}

// Turns any error from callBackend into a JSON error response.
export function errorResponse(err: unknown): Response {
  if (err instanceof BackendError) {
    return Response.json({ error: err.message }, { status: err.status });
  }
  console.error("chat proxy:", err);
  return Response.json({ error: "Could not reach the agent. Try again in a moment." }, { status: 502 });
}

// Fetches a Google-signed ID token for audience from the metadata server.
async function idToken(audience: string): Promise<string> {
  const res = await fetch(
    "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/identity?audience=" +
      encodeURIComponent(audience),
    { headers: { "Metadata-Flavor": "Google" }, cache: "no-store" },
  );
  if (!res.ok) {
    throw new Error(`metadata server returned ${res.status}`);
  }
  return res.text();
}
