// Proxies chat messages to the Go agent backend. BACKEND_URL is read at
// request time, so the same image works against any backend.
export async function POST(request: Request) {
  const backendUrl = process.env.BACKEND_URL?.replace(/\/$/, "");
  if (!backendUrl) {
    return Response.json({ error: "BACKEND_URL is not set" }, { status: 500 });
  }

  try {
    const headers: Record<string, string> = { "Content-Type": "application/json" };
    // On Cloud Run (K_SERVICE is set) the backend is private: authenticate with
    // an ID token for this service's identity. Locally, call it as-is.
    if (process.env.K_SERVICE) {
      headers.Authorization = `Bearer ${await idToken(backendUrl)}`;
    }

    const res = await fetch(`${backendUrl}/chat`, {
      method: "POST",
      headers,
      body: await request.text(),
    });

    if (!res.ok) {
      // Pass the backend's client errors (bad input) through; anything else is
      // the backend failing, which is a bad gateway from the browser's view.
      const status = res.status >= 400 && res.status < 500 ? res.status : 502;
      return Response.json({ error: (await res.text()).trim() }, { status });
    }
    return Response.json(await res.json());
  } catch (err) {
    console.error("chat proxy:", err);
    return Response.json({ error: "could not reach the backend" }, { status: 502 });
  }
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
