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
