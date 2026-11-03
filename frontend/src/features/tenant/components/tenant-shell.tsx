"use client";

import { useMemo, type ReactNode } from "react";

import { AppLayout } from "@/components/layout";
import { shellNav } from "@/config/nav";
import { OrganizationSwitcher } from "@/features/tenant/components/organization-switcher";
import { TenantProvider } from "@/features/tenant/providers/tenant-provider";

/** `/t/{slug}` shell: org gate + per-org permissions + tenant sidebar. */
export function TenantShell({
  slug,
  children,
}: {
  slug: string;
  children: ReactNode;
}) {
  const nav = useMemo(() => shellNav({ variant: "tenant", slug }), [slug]);
  return (
    <TenantProvider slug={slug}>
      <AppLayout
        variant="tenant"
        nav={nav}
        sidebarHeader={<OrganizationSwitcher />}
      >
        {children}
      </AppLayout>
    </TenantProvider>
  );
}
