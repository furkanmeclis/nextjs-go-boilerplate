"use client";

import { useQuery } from "@tanstack/react-query";
import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useRef,
  type ReactNode,
} from "react";

import { TenantAccessCard } from "@/features/tenant/components/tenant-access-card";
import { denyReasonFromError } from "@/features/tenant/lib/access";
import {
  fetchTenantContext,
  tenantQueryKeys,
  type TenantContextResponse,
} from "@/features/tenant/services/tenant.service";
import { findOrganization, type AuthOrganization } from "@/lib/auth/types";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";
import { ScopedPermissionProvider } from "@/providers/permission-provider";

export type TenantScope = {
  /** Slug from the URL; sent as `X-Organization` by `tenantRequest`. */
  slug: string;
  organization: {
    uuid: string;
    slug: string;
    name: string;
    logoUrl?: string;
    status: string;
    accessEndsAt: string | null;
  };
  role: { key: string; name: string };
  /** `tenant.*` permissions of the membership in this organization. */
  permissions: string[];
  /** False while the API check is still in flight (cached membership shown). */
  verified: boolean;
};

const TenantContext = createContext<TenantScope | null>(null);

function scopeFromMembership(slug: string, m: AuthOrganization): TenantScope {
  return {
    slug,
    organization: {
      uuid: m.uuid,
      slug: m.slug,
      name: m.name,
      logoUrl: m.logoUrl,
      status: m.status,
      accessEndsAt: m.accessEndsAt,
    },
    role: m.role,
    permissions: m.permissions,
    verified: false,
  };
}

function scopeFromContext(
  slug: string,
  ctx: TenantContextResponse,
  m: AuthOrganization | undefined,
): TenantScope {
  return {
    slug,
    organization: {
      uuid: ctx.organization.uuid,
      slug: ctx.organization.slug,
      name: ctx.organization.name,
      logoUrl: m?.logoUrl,
      status: ctx.organization.status,
      accessEndsAt: ctx.organization.access_ends_at ?? null,
    },
    role: {
      key: ctx.role,
      // /tenant/context only carries the key; reuse the display name from
      // /me when it still matches.
      name: m && m.role.key === ctx.role ? m.role.name : ctx.role,
    },
    permissions: ctx.permissions ?? [],
    verified: true,
  };
}

function TenantLoader() {
  const { t } = useLocale();
  return (
    <div className="text-muted-foreground flex min-h-svh items-center justify-center text-sm">
      {t("common.loading")}
    </div>
  );
}

/**
 * Resolves the organization of a `/t/{slug}` page and gates it.
 *
 * Renders at once from the cached `/me` membership (no loader on navigation),
 * while `GET /v1/tenant/context` re-checks membership, status and access
 * window on the server. That answer wins: a 403 replaces the page with an
 * explanation card, and its permission list feeds `usePermission()` for
 * everything below (sidebar, buttons, guards).
 */
export function TenantProvider({
  slug,
  children,
}: {
  slug: string;
  children: ReactNode;
}) {
  const { user, hydrateProfile } = useAuth();
  const membership = findOrganization(user, slug);

  const query = useQuery({
    queryKey: tenantQueryKeys.context(slug, user?.uuid ?? "anonymous"),
    queryFn: () => fetchTenantContext(slug),
    // 403s are answers, not transient failures.
    retry: false,
    staleTime: 30_000,
    // The access card is the explanation; skip the global error toast.
    meta: { silent: true },
  });

  // The cached /me is stale when the server disagrees with it (membership
  // added, role changed): refresh once so the switcher and nav catch up.
  const refreshedFor = useRef<string | null>(null);
  const ctx = query.data;
  useEffect(() => {
    if (!ctx) return;
    const serverPerms = [...(ctx.permissions ?? [])].sort().join(",");
    const stale =
      !membership ||
      membership.role.key !== ctx.role ||
      [...membership.permissions].sort().join(",") !== serverPerms;
    const key = `${slug}:${ctx.role}:${serverPerms}`;
    if (stale && refreshedFor.current !== key) {
      refreshedFor.current = key;
      void hydrateProfile();
    }
  }, [ctx, membership, slug, hydrateProfile]);

  const scope = useMemo<TenantScope | null>(() => {
    if (ctx) return scopeFromContext(slug, ctx, membership);
    if (membership) return scopeFromMembership(slug, membership);
    return null;
  }, [ctx, membership, slug]);

  const roles = useMemo(() => (scope ? [scope.role.key] : []), [scope]);

  if (query.error) {
    return (
      <TenantAccessCard
        reason={denyReasonFromError(query.error)}
        slug={slug}
        onRetry={() => void query.refetch()}
      />
    );
  }

  if (!scope) return <TenantLoader />;

  return (
    <TenantContext.Provider value={scope}>
      <ScopedPermissionProvider permissions={scope.permissions} roles={roles}>
        {children}
      </ScopedPermissionProvider>
    </TenantContext.Provider>
  );
}

/** Current organization of the tenant shell. */
export function useTenant(): TenantScope {
  const ctx = useContext(TenantContext);
  if (!ctx) throw new Error("useTenant must be used within TenantProvider");
  return ctx;
}

/** Same as useTenant, but null outside the tenant shell. */
export function useOptionalTenant(): TenantScope | null {
  return useContext(TenantContext);
}
