import type { ReactNode } from "react";

import { TenantShell } from "@/features/tenant";

type LayoutProps = {
  children: ReactNode;
  params: Promise<{ slug: string }>;
};

/** Next may hand over the raw segment; a malformed escape must not 500. */
function safeDecode(segment: string) {
  try {
    return decodeURIComponent(segment);
  } catch {
    return segment;
  }
}

export default async function TenantSlugLayout({
  children,
  params,
}: LayoutProps) {
  const { slug } = await params;
  return <TenantShell slug={safeDecode(slug)}>{children}</TenantShell>;
}
