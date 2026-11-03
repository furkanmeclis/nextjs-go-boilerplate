"use client";

import Link from "next/link";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { routes } from "@/config/routes";
import type { TenantDenyReason } from "@/features/tenant/lib/access";
import { defaultHomeForUser } from "@/lib/auth/types";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

const COPY: Record<TenantDenyReason, { title: string; description: string }> = {
  not_member: {
    title: "tenant.gate_not_member_title",
    description: "tenant.gate_not_member_description",
  },
  suspended: {
    title: "tenant.gate_suspended_title",
    description: "tenant.gate_suspended_description",
  },
  expired: {
    title: "tenant.gate_expired_title",
    description: "tenant.gate_expired_description",
  },
  disabled: {
    title: "tenant.gate_disabled_title",
    description: "tenant.gate_disabled_description",
  },
  error: {
    title: "tenant.gate_error_title",
    description: "tenant.gate_error_description",
  },
};

/** Full-page explanation when an organization cannot be opened. */
export function TenantAccessCard({
  reason,
  slug,
  onRetry,
}: {
  reason: TenantDenyReason;
  slug: string;
  onRetry?: () => void;
}) {
  const { t } = useLocale();
  const { user } = useAuth();
  const copy = COPY[reason];
  const hasOtherOrgs = (user?.organizations ?? []).some(
    (o) => o.slug.toLowerCase() !== slug.toLowerCase(),
  );
  // Never link back to this same blocked organization.
  const preferred = user ? defaultHomeForUser(user) : routes.cms.home;
  const home =
    preferred === routes.tenant.home(slug) ? routes.cms.home : preferred;

  return (
    <div className="flex min-h-svh items-center justify-center p-6">
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle>{t(copy.title)}</CardTitle>
          <CardDescription>{t(copy.description, { slug })}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          {reason === "error" && onRetry ? (
            <Button onClick={onRetry}>{t("tenant.gate_retry")}</Button>
          ) : null}
          {hasOtherOrgs ? (
            <Button
              asChild
              variant={reason === "error" ? "outline" : "default"}
            >
              <Link href={routes.tenant.picker}>
                {t("tenant.gate_choose_other")}
              </Link>
            </Button>
          ) : null}
          <Button asChild variant="outline">
            <Link href={home}>{t("tenant.gate_go_home")}</Link>
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}
