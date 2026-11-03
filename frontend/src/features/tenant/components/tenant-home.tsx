"use client";

import { Building2 } from "lucide-react";

import { PageHeader } from "@/components/layout/page-header";
import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/** `/t/{slug}` landing page; apps replace it with their own dashboard. */
export function TenantHome() {
  const { t, locale } = useLocale();
  const { organization, role, permissions } = useTenant();

  return (
    <>
      <PageHeader
        title={organization.name}
        description={t("tenant.home_description", { role: role.name })}
        icon={<Building2 className="size-6" />}
      />
      <Card>
        <CardHeader>
          <CardTitle>{t("tenant.role", { role: role.name })}</CardTitle>
          <CardDescription>
            {organization.accessEndsAt
              ? t("tenant.access_until", {
                  date: date(organization.accessEndsAt, "dd.MM.yyyy", locale),
                })
              : t("tenant.home_access_open")}
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-wrap gap-2">
          {permissions.map((p) => (
            <Badge key={p} variant="secondary" className="font-mono text-xs">
              {p}
            </Badge>
          ))}
        </CardContent>
      </Card>
    </>
  );
}
