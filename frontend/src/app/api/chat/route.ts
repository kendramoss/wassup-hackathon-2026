// Proxies chat messages to the Go agent backend. BACKEND_URL is read at
// request time, so the same image works against any backend.
export async function POST(request: Request) {
  const backendUrl = process.env.BACKEND_URL;
  if (!backendUrl) {
    return Response.json({ error: "BACKEND_URL is not set" }, { status: 500 });
  }

  const res = await fetch(`${backendUrl.replace(/\/$/, "")}/chat`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: await request.text(),
  });

  if (!res.ok) {
    return Response.json(
      { error: `backend returned ${res.status}: ${await res.text()}` },
      { status: 502 },
    );
  }
  return Response.json(await res.json());
}
