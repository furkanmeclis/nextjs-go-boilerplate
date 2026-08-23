import { decode, encode } from "next-auth/jwt";
import { cookies } from "next/headers";

export function sessionCookieName() {
  return process.env.NODE_ENV === "production"
    ? "__Secure-authjs.session-token"
    : "authjs.session-token";
}

async function readSessionToken() {
  const jar = await cookies();
  const name = sessionCookieName();
  const raw = jar.get(name)?.value;
  if (!raw || !process.env.AUTH_SECRET) return null;
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

  const jar = await cookies();
  jar.set(name, payload, {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax",
    path: "/",
    maxAge,
  });
}

export async function clearAuthSessionCookie() {
  const jar = await cookies();
  jar.delete(sessionCookieName());
}
