import type { TenantRoleKey } from "@/features/tenant/services/tenant.service";

/** Template tenant roles (tenant-core.md §12: no custom roles in v1). */
export const TENANT_ROLE_KEYS: readonly TenantRoleKey[] = [
  "owner",
  "admin",
  "member",
];

/** Static label keys so the i18n linter can verify each one. */
export const TENANT_ROLE_LABEL_KEYS: Record<TenantRoleKey, string> = {
  owner: "tenant.role_owner",
  admin: "tenant.role_admin",
  member: "tenant.role_member",
};

export const INVITATION_STATUS_LABEL_KEYS: Record<string, string> = {
  pending: "tenant.invitations_status_pending",
  expired: "tenant.invitations_status_expired",
};

export function isTenantRoleKey(v: string): v is TenantRoleKey {
  return (TENANT_ROLE_KEYS as readonly string[]).includes(v);
}

/**
 * Mirrors the API's owner-escalation rule (orgusecase.CheckRoleChange) so the
 * UI hides what the server would refuse: only owners hand out, take away or
 * remove the owner role.
 */
export function canChangeRole(actor: string, from: string, to: string) {
  if (actor === "owner") return true;
  return from !== "owner" && to !== "owner";
}
