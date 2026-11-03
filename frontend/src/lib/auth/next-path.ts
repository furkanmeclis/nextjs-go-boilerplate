/**
 * Post-login redirect target from a `?next=` value. Only same-origin paths
 * are accepted: browsers treat a backslash like "/", so a value starting
 * "/<backslash>evil.com" would become a protocol-relative redirect.
 */
export function safeNextPath(raw: string | null | undefined, fallback: string) {
  if (
    !raw ||
    !raw.startsWith("/") ||
    raw.startsWith("//") ||
    /[\\\u0000-\u001f]/.test(raw)
  ) {
    return fallback;
  }
  return raw;
}
