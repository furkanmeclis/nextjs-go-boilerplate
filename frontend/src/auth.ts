import NextAuth from "next-auth";
import type { Provider } from "next-auth/providers";
import Apple from "next-auth/providers/apple";
import Credentials from "next-auth/providers/credentials";
import Facebook from "next-auth/providers/facebook";
import GitHub from "next-auth/providers/github";
import Google from "next-auth/providers/google";
import Passkey from "next-auth/providers/passkey";
import { NextResponse } from "next/server";

import { routes } from "@/config/routes";
import {
  InvalidMFACodeError,
  MFANotEnrolledError,
  MFARequiredError,
} from "@/lib/auth/credentials-errors";
import { goAdapter } from "@/lib/auth/go-adapter";
import {
  adapterGetOAuthConfig,
  adapterGetOAuthAccountUser,
  adapterGetUserByEmail,
  adapterIssueSession,
  loginWithPassword,
  type OAuthConfigPayload,
} from "@/lib/auth/go-adapter-client";
import { fetchAppPublicConfig } from "@/services/app-config.service";

const pendingGitHubLogins = new Map<string, string>();

const OAUTH_PROVIDERS = ["github", "google", "facebook", "apple"] as const;
type OAuthProviderId = (typeof OAUTH_PROVIDERS)[number];

const GUEST_AUTH_PREFIXES = [
  routes.guest.login,
  routes.guest.register,
  routes.guest.forgotPassword,
  routes.guest.resetPassword,
  routes.guest.verifyEmail,
];

function isGuestAuthPath(pathname: string) {
  return GUEST_AUTH_PREFIXES.some(
    (path) => pathname === path || pathname.startsWith(`${path}/`),
  );
}

function isProtectedPath(pathname: string) {
  if (pathname === routes.cms.root) return true;
  if (pathname.startsWith(`${routes.cms.profile.root}`)) return true;
  if (pathname === routes.platform.root) return true;
  if (pathname.startsWith(`${routes.platform.root}/`) && !isGuestAuthPath(pathname)) {
    return true;
  }
  return false;
}

function isOAuthProvider(provider: string | undefined): provider is OAuthProviderId {
  return (
    provider !== undefined &&
    (OAUTH_PROVIDERS as readonly string[]).includes(provider)
  );
}

async function loadOAuthConfig(provider: OAuthProviderId): Promise<OAuthConfigPayload | null> {
  try {
    const config = await adapterGetOAuthConfig(provider);
    if (config.enabled && config.client_id && config.client_secret) {
      return config;
    }
  } catch {
    // Provider stays disabled when config is unavailable.
  }
  return null;
}

async function buildProviders(): Promise<Provider[]> {
  let passwordLogin = true;
  let passkeyLogin = true;
  try {
    const appConfig = await fetchAppPublicConfig("en");
    passwordLogin = appConfig.auth_methods.password.login;
    passkeyLogin = appConfig.auth_methods.passkey.login;
  } catch {
    // Keep defaults when public config is unavailable.
  }

  const providers: Provider[] = [];

  if (passwordLogin) {
    providers.push(
      Credentials({
        credentials: {
          email: { label: "Email", type: "email" },
          password: { label: "Password", type: "password" },
          totp_code: { label: "Authenticator code", type: "text" },
        },
        async authorize(credentials) {
          const email = String(credentials?.email ?? "").trim();
          const password = String(credentials?.password ?? "");
          const totpCode = String(credentials?.totp_code ?? "").trim();
          if (!email || !password) return null;

          try {
            const tokens = await loginWithPassword(
              email,
              password,
              totpCode || undefined,
            );
            const user = await adapterGetUserByEmail(email);
            return {
              id: user.id,
              email: user.email,
              name: user.name,
              accessToken: tokens.access_token,
              refreshToken: tokens.refresh_token,
              expiresIn: tokens.expires_in,
            };
          } catch (error) {
            const code = (error as Error & { code?: string }).code;
            if (code === "MFA_REQUIRED") {
              throw new MFARequiredError();
            }
            if (code === "INVALID_MFA_CODE") {
              throw new InvalidMFACodeError();
            }
            if (code === "MFA_NOT_ENROLLED") {
              throw new MFANotEnrolledError();
            }
            return null;
          }
        },
      }),
    );
  }

  if (passkeyLogin) {
    providers.push(Passkey({}));
  }

  const github = await loadOAuthConfig("github");
  if (github) {
    providers.push(
      GitHub({
        clientId: github.client_id,
        clientSecret: github.client_secret,
      }),
    );
  }

  const google = await loadOAuthConfig("google");
  if (google) {
    providers.push(
      Google({
        clientId: google.client_id,
        clientSecret: google.client_secret,
      }),
    );
  }

  const facebook = await loadOAuthConfig("facebook");
  if (facebook) {
    providers.push(
      Facebook({
        clientId: facebook.client_id,
        clientSecret: facebook.client_secret,
      }),
    );
  }

  const apple = await loadOAuthConfig("apple");
  if (apple) {
    providers.push(
      Apple({
        clientId: apple.client_id,
        clientSecret: apple.client_secret,
      }),
    );
  }

  return providers;
}

const authHandlers = NextAuth(async () => ({
  adapter: goAdapter({ pendingGitHubLogins }),
  trustHost: process.env.AUTH_TRUST_HOST === "true",
  session: { strategy: "jwt" },
  providers: await buildProviders(),
  experimental: {
    enableWebAuthn: true,
  },
  pages: {
    signIn: routes.guest.login,
    error: routes.guest.login,
  },
  callbacks: {
    authorized({ auth, request }) {
      const { pathname } = request.nextUrl;
      if (!isProtectedPath(pathname) || auth) {
        return true;
      }

      const url = request.nextUrl.clone();
      url.pathname = routes.guest.login;
      url.searchParams.set("next", pathname);
      return NextResponse.redirect(url);
    },
    async signIn({ account, profile }) {
      if (!isOAuthProvider(account?.provider)) {
        return true;
      }

      const provider = account.provider;

      if (
        provider === "github" &&
        profile &&
        typeof profile === "object" &&
        "login" in profile &&
        typeof profile.login === "string" &&
        account.providerAccountId
      ) {
        pendingGitHubLogins.set(account.providerAccountId, profile.login);
      }

      const providerAccountId = account.providerAccountId ?? "";
      try {
        await adapterGetOAuthAccountUser(provider, providerAccountId);
        return true;
      } catch {
        // Not linked yet.
      }

      const { getApiTokens } = await import("@/lib/server/auth-tokens");
      const session = await getApiTokens();
      if (session.userId) {
        return true;
      }

      try {
        const config = await adapterGetOAuthConfig(provider);
        if (config.register_allowed) {
          return true;
        }
      } catch {
        // fall through
      }

      return `${routes.guest.login}?error=OAuthNotLinked`;
    },
    async jwt({ token, user, account, trigger, session }) {
      if (user) {
        token.sub = user.id;
        token.email = user.email;
        const extended = user as typeof user & {
          accessToken?: string;
          refreshToken?: string;
          expiresIn?: number;
        };
        if (extended.accessToken && extended.refreshToken) {
          token.accessToken = extended.accessToken;
          token.refreshToken = extended.refreshToken;
          token.expiresIn = extended.expiresIn;
        }
      }

      if (
        account &&
        (account.provider === "passkey" || isOAuthProvider(account.provider)) &&
        token.sub &&
        !token.accessToken
      ) {
        try {
          const authMethod =
            account.provider === "passkey" ? "passkey" : "oauth";
          const tokens = await adapterIssueSession(token.sub, authMethod);
          token.accessToken = tokens.access_token;
          token.refreshToken = tokens.refresh_token;
          token.expiresIn = tokens.expires_in;
        } catch {
          token.error = isOAuthProvider(account.provider)
            ? "OAuthSessionError"
            : "PasskeySessionError";
        }
      }

      if (trigger === "update" && session) {
        const patch = session as {
          accessToken?: string;
          refreshToken?: string;
          expiresIn?: number;
        };
        if (patch.accessToken) token.accessToken = patch.accessToken;
        if (patch.refreshToken) token.refreshToken = patch.refreshToken;
        if (patch.expiresIn) token.expiresIn = patch.expiresIn;
      }

      return token;
    },
    async session({ session, token }) {
      if (session.user) {
        session.user.id = token.sub ?? "";
      }
      (session as { error?: string }).error = token.error as
        | "PasskeySessionError"
        | "OAuthSessionError"
        | undefined;
      return session;
    },
  },
}));

export const { handlers, auth, signIn, signOut } = authHandlers;
