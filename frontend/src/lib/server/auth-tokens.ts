import { decode, encode } from "next-auth/jwt";
import { cookies } from "next/headers";

const SESSION_COOKIE_BASES = [
  "authjs.session-token",
  "__Secure-authjs.session-token",
] as const;

const SESSION_COOKIE_CHUNKS = 12;

// Auth.js picks the cookie name from the AUTH_URL scheme and salts the JWT with
// it. Follow the same rule, or a plain-HTTP deploy signs in and then loses the
// session on the first BFF call.
function usesSecureCookies() {
  const authUrl = process.env.AUTH_URL ?? process.env.NEXTAUTH_URL;
  return authUrl
    ? authUrl.startsWith("https://")
    : process.env.NODE_ENV === "production";
}

export function sessionCookieName() {
  return usesSecureCookies()
    ? "__Secure-authjs.session-token"
    : "authjs.session-token";
}

function sessionCookieOptions() {
  return {
    httpOnly: true,
    secure: usesSecureCookies(),
    sameSite: "lax" as const,
    path: "/",
  };
}

/** Expire the Auth.js session cookie and any chunked variants Auth.js may create. */
async function expireSessionCookies() {
  const jar = await cookies();
  const opts = { ...sessionCookieOptions(), maxAge: 0 };
  for (const base of SESSION_COOKIE_BASES) {
    jar.set(base, "", opts);
    for (let i = 0; i < SESSION_COOKIE_CHUNKS; i += 1) {
      jar.set(`${base}.${i}`, "", opts);
    }
  }
}

async function readSessionToken() {
  const jar = await cookies();
  const name = sessionCookieName();
  const raw = jar.get(name)?.value;
  if (!raw || !process.env.AUTH_SECRET) {
    // Auth.js may have split a large JWT across numbered chunk cookies.
    const chunks: string[] = [];
    for (let i = 0; i < SESSION_COOKIE_CHUNKS; i += 1) {
      const part = jar.get(`${name}.${i}`)?.value;
      if (!part) break;
      chunks.push(part);
    }
    if (chunks.length === 0) return null;
    return decode({
      token: chunks.join(""),
      secret: process.env.AUTH_SECRET!,
      salt: name,
    });
  }
  return decode({
    token: raw,
    secret: process.env.AUTH_SECRET,
    salt: name,
  });
}

export async function getApiTokens() {
  const token = await readSessionToken();
  if (!token) {
    return {
      accessToken: null as string | null,
      refreshToken: null as string | null,
      userId: null as string | null,
      email: null as string | null,
      impersonatorUuid: null as string | null,
    };
  }
  return {
    accessToken: (token.accessToken as string | undefined) ?? null,
    refreshToken: (token.refreshToken as string | undefined) ?? null,
    userId: (token.sub as string | undefined) ?? null,
    email: (token.email as string | undefined) ?? null,
    impersonatorUuid: (token.impersonatorUuid as string | undefined) ?? null,
  };
}

export async function persistApiTokens(input: {
  accessToken: string;
  refreshToken: string;
  userId: string;
  email?: string | null;
  impersonatorUuid?: string | null;
  expiresIn?: number;
}) {
  if (!process.env.AUTH_SECRET) {
    throw new Error("AUTH_SECRET is not configured");
  }

  const existing = await readSessionToken();
  const maxAge = input.expiresIn ?? 15 * 60;
  const name = sessionCookieName();
  const payload = await encode({
    token: {
      ...existing,
      sub: input.userId,
      email: input.email ?? existing?.email,
      accessToken: input.accessToken,
      refreshToken: input.refreshToken,
      impersonatorUuid: input.impersonatorUuid ?? undefined,
    },
    secret: process.env.AUTH_SECRET,
    salt: name,
    maxAge,
  });

  // Drop any leftover Auth.js chunks before writing a single cookie.
  await expireSessionCookies();

  const jar = await cookies();
  jar.set(name, payload, {
    ...sessionCookieOptions(),
    maxAge,
  });
}

export async function clearAuthSessionCookie() {
  await expireSessionCookies();
}
