"use client";

import type { ReactNode } from "react";

import { RouteGuard } from "@/components/common/route-guard";

/**
 * Organization (tenant) shell — `/t`, `/t/{slug}/…`. Needs a session only;
 * access to each organization is checked per membership by `TenantShell`.
 */
export default function TenantGroupLayout({
  children,
}: {
  children: ReactNode;
}) {
  return <RouteGuard mode="tenant">{children}</RouteGuard>;
}
