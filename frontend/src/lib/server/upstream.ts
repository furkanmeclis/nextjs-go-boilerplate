import { upstreamConfig } from "@/config/api";

export type UpstreamResult = {
  status: number;
  headers: Headers;
  body: ArrayBuffer;
};

export type UpstreamStreamResult = {
  status: number;
  headers: Headers;
  body: ReadableStream<Uint8Array> | null;
};

function buildUpstreamUrl(pathWithQuery: string): string {
  return `${upstreamConfig.baseUrl.replace(/\/$/, "")}/${pathWithQuery.replace(/^\//, "")}`;
}

/**
 * X-Forwarded-For chain to send to the Go API (per-IP rate limits, sessions,
 * audit trail). The BFF does not pick a hop itself: it forwards the chain it
 * received, and the API walks it right-to-left, believing only hops appended
 * by trusted proxies (TRUSTED_PROXIES; platform/clientip). X-Real-IP is used
 * only when there is no X-Forwarded-For at all: appending it would make it the
 * right-most hop and let a client-supplied value outrank the edge's real one.
 *
 * Left-most entries are client-controlled, so this relies on the BFF sitting
 * behind an edge proxy that appends the real client address (as in
 * compose.prod.yml / Dokploy). Returns null when neither header is present.
 */
export function forwardedForFromHeaders(headers: Headers): string | null {
  const hops = (headers.get("x-forwarded-for") ?? "")
    .split(",")
    .map((hop) => hop.trim())
    .filter(Boolean);
  const real = headers.get("x-real-ip")?.trim();
  if (hops.length === 0 && real) hops.push(real);
  return hops.length > 0 ? hops.join(", ") : null;
}

function scrubUpstreamHeaders(init?: HeadersInit): Headers {
  const headers = new Headers(init);
  headers.delete("host");
  headers.delete("connection");
  headers.delete("content-length");
  // Browser Origin/Cookie must not leak to Go — auth is Bearer only
  headers.delete("cookie");
  headers.delete("origin");
  return headers;
}

/**
 * Forward a request to the Go API and buffer the body.
 * Caller attaches Authorization when needed.
 */
export async function fetchUpstream(
  pathWithQuery: string,
  init: {
    method: string;
    headers?: HeadersInit;
    body?: BodyInit | null;
  },
): Promise<UpstreamResult> {
  const response = await fetch(buildUpstreamUrl(pathWithQuery), {
    method: init.method,
    headers: scrubUpstreamHeaders(init.headers),
    body:
      init.body && init.method !== "GET" && init.method !== "HEAD"
        ? init.body
        : undefined,
    cache: "no-store",
  });

  return {
    status: response.status,
    headers: response.headers,
    body: await response.arrayBuffer(),
  };
}

/**
 * Forward a request and keep the body as a live stream (SSE / long-poll).
 * Do not call arrayBuffer() — that would buffer and break event streams.
 */
export async function fetchUpstreamStream(
  pathWithQuery: string,
  init: {
    method: string;
    headers?: HeadersInit;
    body?: BodyInit | null;
    signal?: AbortSignal;
  },
): Promise<UpstreamStreamResult> {
  const response = await fetch(buildUpstreamUrl(pathWithQuery), {
    method: init.method,
    headers: scrubUpstreamHeaders(init.headers),
    body:
      init.body && init.method !== "GET" && init.method !== "HEAD"
        ? init.body
        : undefined,
    cache: "no-store",
    signal: init.signal,
  });

  return {
    status: response.status,
    headers: response.headers,
    body: response.body,
  };
}
