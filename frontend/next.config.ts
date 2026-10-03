import { withSentryConfig } from "@sentry/nextjs/config";
import type { NextConfig } from "next";

/**
 * Baseline security headers for every route. A full script CSP is not set
 * here (Next inline bootstrap scripts would need nonces); `frame-ancestors`
 * and `base-uri` restrictions are safe to enforce globally.
 */
const securityHeaders = [
  {
    key: "Content-Security-Policy",
    value: "frame-ancestors 'self'; base-uri 'self'",
  },
  { key: "X-Frame-Options", value: "SAMEORIGIN" },
  { key: "X-Content-Type-Options", value: "nosniff" },
  { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
  {
    key: "Permissions-Policy",
    value: "camera=(self), microphone=(self), geolocation=(), payment=()",
  },
  ...(process.env.NODE_ENV === "production"
    ? [
        {
          key: "Strict-Transport-Security",
          value: "max-age=31536000; includeSubDomains",
        },
      ]
    : []),
];

const nextConfig: NextConfig = {
  output: "standalone",
  async headers() {
    return [{ source: "/:path*", headers: securityHeaders }];
  },
};

/**
 * The error tracker is a self-hosted Sentry-protocol endpoint without the
 * sentry-cli API, so nothing is uploaded at build time: no source maps, no
 * release creation, no telemetry. The build needs no Sentry env or token;
 * DSNs are read at runtime (SENTRY_DSN).
 */
export default withSentryConfig(nextConfig, {
  silent: true,
  telemetry: false,
  authToken: undefined,
  sourcemaps: { disable: true },
  release: { create: false, finalize: false },
});
