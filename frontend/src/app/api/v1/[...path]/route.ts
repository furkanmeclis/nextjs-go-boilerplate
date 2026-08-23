import { proxyToUpstream } from "@/lib/server/bff-proxy";

type RouteContext = {
  params: Promise<{ path: string[] }>;
};

/**
 * Long-lived SSE streams need a raised
 * function timeout. Align with API `HTTP_WRITE_TIMEOUT` (e.g. 120s).
 */
export const maxDuration = 120;

async function handle(request: Request, context: RouteContext) {
  const { path } = await context.params;
  return proxyToUpstream(path ?? [], request);
}

export const GET = handle;
export const POST = handle;
export const PUT = handle;
export const PATCH = handle;
export const DELETE = handle;
export const HEAD = handle;
export const OPTIONS = handle;
