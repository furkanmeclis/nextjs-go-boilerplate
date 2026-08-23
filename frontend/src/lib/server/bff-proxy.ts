import {
  clearAuthSessionCookie,
  getApiTokens,
  persistApiTokens,
} from "@/lib/server/auth-tokens";
import { fetchUpstream, fetchUpstreamStream } from "@/lib/server/upstream";

type TokensData = {
  access_token?: string;
  refresh_token?: string;
  expires_in?: number;
  access_expires_at?: string;
};

type Envelope = {
  success?: boolean;
  data?: TokensData & Record<string, unknown>;
  meta?: unknown;
  error?: unknown;
};

type SessionSwitchData = {
  user_uuid?: string;
  email?: string;
  impersonator_uuid?: string | null;
};

/** Paths whose successful response updates the NextAuth JWT token pair. */
function isAuthTokenPath(path: string) {
  if (path === "auth/refresh") return true;
  if (path === "auth/impersonation/stop") return true;
  return path.startsWith("platform/users/") && path.endsWith("/impersonate");
}

/** Public auth endpoints that must not send Bearer. */
const AUTH_PUBLIC_TOKEN_PATHS = new Set(["auth/refresh", "auth/register"]);

function isAuthPublicTokenPath(path: string) {
  return AUTH_PUBLIC_TOKEN_PATHS.has(path);
}

function isLogoutPath(path: string) {
  return path === "auth/logout";
}

function isSessionInvalidatePath(path: string) {
  return path === "auth/password/change" || path === "auth/password/reset";
}

function isRefreshPath(path: string) {
  return path === "auth/refresh";
}

function wantsBinaryStream(path: string) {
  return (
    path.includes("platform/storage/objects/download") ||
    path.includes("platform/storage/objects/preview") ||
    path.startsWith("public/storage/")
  );
}

function wantsEventStream(request: Request) {
  const accept = request.headers.get("accept") ?? "";
  return accept.includes("text/event-stream");
}

function encodeJson(value: unknown): string {
  return JSON.stringify(value);
}

function parseJson(buffer: ArrayBuffer): Envelope | null {
  try {
    const text = new TextDecoder().decode(buffer);
    if (!text) return null;
    return JSON.parse(text) as Envelope;
  } catch {
    return null;
  }
}

function stripTokens(envelope: Envelope): Envelope {
  if (!envelope.data || typeof envelope.data !== "object") return envelope;
  const data = { ...envelope.data };
  delete data.access_token;
  delete data.refresh_token;
  return {
    ...envelope,
    data: {
      ...data,
      authenticated: true,
    },
  };
}

function resolveAccessMaxAge(data: TokensData | undefined): number | undefined {
  if (typeof data?.expires_in === "number" && data.expires_in > 0) {
    return data.expires_in;
  }
  return undefined;
}

async function persistTokensFromEnvelope(envelope: Envelope | null) {
  const access = envelope?.data?.access_token;
  const refresh = envelope?.data?.refresh_token;
  if (!access || !refresh) return false;

  const sessionPayload = envelope?.data?.session as SessionSwitchData | undefined;
  const existing = await getApiTokens();
  const userId = sessionPayload?.user_uuid ?? existing.userId;
  if (!userId) return false;

  const impersonatorUuid =
    sessionPayload !== undefined
      ? (sessionPayload.impersonator_uuid ?? null)
      : existing.impersonatorUuid;

  await persistApiTokens({
    accessToken: access,
    refreshToken: refresh,
    userId,
    email: sessionPayload?.email ?? existing.email,
    impersonatorUuid,
    expiresIn: resolveAccessMaxAge(envelope?.data),
  });
  return true;
}

async function refreshViaUpstream(): Promise<boolean> {
  const { refreshToken, userId } = await getApiTokens();
  if (!refreshToken || !userId) return false;

  const result = await fetchUpstream("auth/refresh", {
    method: "POST",
    headers: {
      Accept: "application/json",
      "Content-Type": "application/json",
    },
    body: encodeJson({ refresh_token: refreshToken }),
  });

  if (result.status >= 400) {
    await clearAuthSessionCookie();
    return false;
  }

  const envelope = parseJson(result.body);
  const ok = await persistTokensFromEnvelope(envelope);
  if (!ok) await clearAuthSessionCookie();
  return ok;
}

function buildUpstreamBody(
  path: string,
  method: string,
  incoming: ArrayBuffer,
  refreshToken: string | null,
): BodyInit | null {
  if (method === "GET" || method === "HEAD") return null;

  if (isRefreshPath(path)) {
    return encodeJson({ refresh_token: refreshToken ?? "" });
  }

  if (isLogoutPath(path)) {
    let clientBody: Record<string, unknown> = {};
    try {
      const text = new TextDecoder().decode(incoming);
      if (text) clientBody = JSON.parse(text) as Record<string, unknown>;
    } catch {
      clientBody = {};
    }
    return encodeJson({
      ...clientBody,
      refresh_token: refreshToken ?? clientBody.refresh_token ?? "",
    });
  }

  return incoming.byteLength > 0 ? incoming : null;
}

function passthroughHeaders(upstream: Headers): Headers {
  const out = new Headers();
  const allow = [
	"content-type",
    "content-disposition",
    "cache-control",
    "location",
    "x-request-id",
    "content-length",
  ] as const;
  for (const key of allow) {
    const value = upstream.get(key);
    if (value) out.set(key, value);
  }
  return out;
}

function streamPassthroughHeaders(upstream: Headers): Headers {
  const out = passthroughHeaders(upstream);
  if (!out.has("content-type")) {
    out.set("Content-Type", "text/event-stream");
  }
  out.set("Cache-Control", "no-cache, no-transform");
  out.set("Connection", "keep-alive");
  out.set("X-Accel-Buffering", "no");
  return out;
}

async function proxyBuffered(
  path: string,
  pathWithQuery: string,
  request: Request,
  headers: Headers,
  body: BodyInit | null,
  refreshToken: string | null,
): Promise<Response> {
  let result = await fetchUpstream(pathWithQuery, {
    method: request.method,
    headers,
    body,
  });

  if (
    result.status === 401 &&
    Boolean(refreshToken) &&
    !isAuthPublicTokenPath(path) &&
    !isLogoutPath(path)
  ) {
    const refreshed = await refreshViaUpstream();
    if (refreshed) {
      const { accessToken: nextAccess } = await getApiTokens();
      if (nextAccess) headers.set("Authorization", `Bearer ${nextAccess}`);
      result = await fetchUpstream(pathWithQuery, {
        method: request.method,
        headers,
        body,
      });
    }
  }

  if (
    isLogoutPath(path) ||
    (result.status < 400 && isSessionInvalidatePath(path))
  ) {
    await clearAuthSessionCookie();
  }

  let responseBody: ArrayBuffer | string = result.body;
  const envelope = parseJson(result.body);

  if (result.status < 400 && isAuthTokenPath(path)) {
    await persistTokensFromEnvelope(envelope);
    if (envelope) {
      responseBody = JSON.stringify(stripTokens(envelope));
    }
  }

  return new Response(responseBody, {
    status: result.status,
    headers: passthroughHeaders(result.headers),
  });
}

async function proxyStream(
  path: string,
  pathWithQuery: string,
  request: Request,
  headers: Headers,
  body: BodyInit | null,
  refreshToken: string | null,
): Promise<Response> {
  const binary = wantsBinaryStream(path);
  if (!binary && !headers.get("Accept")?.includes("text/event-stream")) {
    headers.set("Accept", "text/event-stream");
  }

  let result = await fetchUpstreamStream(pathWithQuery, {
    method: request.method,
    headers,
    body,
    signal: request.signal,
  });

  if (
    result.status === 401 &&
    Boolean(refreshToken) &&
    !isAuthPublicTokenPath(path)
  ) {
    await result.body?.cancel().catch(() => undefined);
    const refreshed = await refreshViaUpstream();
    if (refreshed) {
      const { accessToken: nextAccess } = await getApiTokens();
      if (nextAccess) headers.set("Authorization", `Bearer ${nextAccess}`);
      result = await fetchUpstreamStream(pathWithQuery, {
        method: request.method,
        headers,
        body,
        signal: request.signal,
      });
    }
  }

  const contentType = result.headers.get("content-type") ?? "";
  if (binary || !contentType.includes("text/event-stream")) {
    if (binary) {
      return new Response(result.body, {
        status: result.status,
        headers: passthroughHeaders(result.headers),
      });
    }
    const chunks: Uint8Array[] = [];
    if (result.body) {
      const reader = result.body.getReader();
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        if (value) chunks.push(value);
      }
    }
    const total = chunks.reduce((n, c) => n + c.byteLength, 0);
    const merged = new Uint8Array(total);
    let offset = 0;
    for (const chunk of chunks) {
      merged.set(chunk, offset);
      offset += chunk.byteLength;
    }
    return new Response(merged, {
      status: result.status,
      headers: passthroughHeaders(result.headers),
    });
  }

  return new Response(result.body, {
    status: result.status,
    headers: streamPassthroughHeaders(result.headers),
  });
}

export async function proxyToUpstream(
  pathSegments: string[],
  request: Request,
): Promise<Response> {
  const path = pathSegments.join("/");
  const url = new URL(request.url);
  const pathWithQuery = `${path}${url.search}`;

  const incomingBody =
    request.method === "GET" || request.method === "HEAD"
      ? new ArrayBuffer(0)
      : await request.arrayBuffer();

  const { accessToken, refreshToken } = await getApiTokens();

  const headers = new Headers();
  const accept = request.headers.get("accept");
  const contentType = request.headers.get("content-type");
  if (accept) headers.set("Accept", accept);
  else headers.set("Accept", "application/json");
  if (contentType) headers.set("Content-Type", contentType);

  if (accessToken && !isAuthPublicTokenPath(path)) {
    headers.set("Authorization", `Bearer ${accessToken}`);
  }

  const body = buildUpstreamBody(
    path,
    request.method,
    incomingBody,
    refreshToken,
  );

  if (wantsEventStream(request) || wantsBinaryStream(path)) {
    return proxyStream(
      path,
      pathWithQuery,
      request,
      headers,
      body,
      refreshToken,
    );
  }

  return proxyBuffered(
    path,
    pathWithQuery,
    request,
    headers,
    body,
    refreshToken,
  );
}
