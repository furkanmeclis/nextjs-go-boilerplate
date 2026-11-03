import type { OrganizationStatus } from "@/features/organizations/services/organizations.service";

export const ORGANIZATION_STATUS_VALUES = [
  "active",
  "suspended",
] as const satisfies readonly OrganizationStatus[];

export const ORGANIZATION_STATUS_TONE: Record<
  OrganizationStatus,
  "success" | "warning"
> = {
  active: "success",
  suspended: "warning",
};

/**
 * Template tenant roles seeded for every organization (migration 000030).
 * Org-specific custom roles are not exposed in v1 (tenant-core.md §12).
 */
export const TENANT_ROLE_KEYS = ["owner", "admin", "member"] as const;
export type TenantRoleKey = (typeof TENANT_ROLE_KEYS)[number];

/** Same shape the API allocates: lowercase letters, digits, single hyphens. */
export const SLUG_PATTERN = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;

/** Static label keys so the i18n linter can verify every one. */
export const ORGANIZATION_STATUS_LABEL_KEYS: Record<
  OrganizationStatus,
  string
> = {
  active: "organizations.status_active",
  suspended: "organizations.status_suspended",
};

export const ORGANIZATION_STATUS_TOAST_KEYS: Record<
  OrganizationStatus,
  string
> = {
  active: "organizations.toast_status_active",
  suspended: "organizations.toast_status_suspended",
};

export const TENANT_ROLE_LABEL_KEYS: Record<TenantRoleKey, string> = {
  owner: "organizations.role_owner",
  admin: "organizations.role_admin",
  member: "organizations.role_member",
};
