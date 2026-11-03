import { ApiError } from "@/lib/api/errors";

/** Why the tenant shell refuses to render an organization. */
export type TenantDenyReason =
  "not_member" | "suspended" | "expired" | "disabled" | "error";

/** Maps a `/v1/tenant/*` failure to a gate reason. */
export function denyReasonFromError(error: unknown): TenantDenyReason {
  if (error instanceof ApiError) {
    switch (error.code) {
      case "NO_TENANT_MEMBERSHIP":
        return "not_member";
      case "ORGANIZATION_SUSPENDED":
        return "suspended";
      case "ORGANIZATION_ACCESS_EXPIRED":
        return "expired";
    }
    // Tenant routes are not mounted when TENANCY_ENABLED=false.
    if (error.isNotFound) return "disabled";
  }
  return "error";
}

/** True once the access window has closed (`null` = open-ended). */
export function isAccessExpired(accessEndsAt: string | null, now = Date.now()) {
  if (!accessEndsAt) return false;
  const end = Date.parse(accessEndsAt);
  return Number.isFinite(end) && end <= now;
}
