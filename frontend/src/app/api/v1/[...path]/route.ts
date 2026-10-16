import { proxyBackend } from "@/lib/backend";

// Proxies /api/v1/* to the backend's /v1/* (the backend is private on Cloud
// Run; this service authenticates as frontend@).
async function handle(request: Request, ctx: RouteContext<"/api/v1/[...path]">) {
  const { path } = await ctx.params;
  return proxyBackend(request, path.map(encodeURIComponent).join("/"));
}

export { handle as GET, handle as POST };
