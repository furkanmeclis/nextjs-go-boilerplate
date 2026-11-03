import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

export type TenantContextResponse = components["schemas"]["TenantContext"];

export const tenantQueryKeys = {
  all: ["tenant"] as const,
  /**
   * Scoped to the signed-in user as well: the payload is that user's role and
   * permissions, so another account on the same tab must never reuse it.
   */
  context: (slug: string, userUuid: string) =>
    ["tenant", userUuid, slug.toLowerCase(), "context"] as const,
};

/**
 * Authoritative membership check for one organization: the API resolves the
 * slug, the caller's membership, status and access window on every call.
 */
export function fetchTenantContext(slug: string) {
  return tenantRequest<TenantContextResponse>(
    slug,
    "GET",
    "/v1/tenant/context",
    { silent: true },
  );
}
