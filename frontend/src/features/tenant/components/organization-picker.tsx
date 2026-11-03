"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect } from "react";
import { ChevronRight } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { routes } from "@/config/routes";
import { isAccessExpired } from "@/features/tenant/lib/access";
import { date } from "@/lib/utils/format";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

/** `/t` — lists the caller's organizations (several memberships). */
export function OrganizationPicker() {
  const { t, locale } = useLocale();
  const router = useRouter();
  const { user } = useAuth();
  const orgs = user?.organizations ?? [];

  // A single membership needs no choice.
  const only = orgs.length === 1 ? orgs[0] : undefined;
  useEffect(() => {
    if (only) router.replace(routes.tenant.home(only.slug));
  }, [only, router]);

  if (only) return null;

  return (
    <div className="flex min-h-svh items-center justify-center p-6">
      <Card className="w-full max-w-lg">
        <CardHeader>
          <CardTitle>
            {orgs.length
              ? t("tenant.picker_title")
              : t("tenant.picker_empty_title")}
          </CardTitle>
          <CardDescription>
            {orgs.length
              ? t("tenant.picker_description")
              : t("tenant.picker_empty_description")}
          </CardDescription>
        </CardHeader>
        {orgs.length ? (
          <CardContent>
            <ul className="divide-border divide-y rounded-md border">
              {orgs.map((org) => {
                const expired = isAccessExpired(org.accessEndsAt);
                return (
                  <li key={org.uuid}>
                    <Link
                      href={routes.tenant.home(org.slug)}
                      className="hover:bg-muted/60 flex items-center gap-3 px-4 py-3 text-sm"
                    >
                      <div className="min-w-0 flex-1">
                        <div className="truncate font-medium">{org.name}</div>
                        <div className="text-muted-foreground truncate text-xs">
                          {t("tenant.role", { role: org.role.name })}
                          {org.accessEndsAt && !expired
                            ? ` · ${t("tenant.access_until", {
                                date: date(
                                  org.accessEndsAt,
                                  "dd.MM.yyyy",
                                  locale,
                                ),
                              })}`
                            : ""}
                        </div>
                      </div>
                      {org.status === "suspended" ? (
                        <Badge variant="warning">
                          {t("tenant.status_suspended")}
                        </Badge>
                      ) : expired ? (
                        <Badge variant="danger">
                          {t("tenant.status_expired")}
                        </Badge>
                      ) : null}
                      <ChevronRight className="text-muted-foreground size-4 rtl:rotate-180" />
                    </Link>
                  </li>
                );
              })}
            </ul>
          </CardContent>
        ) : null}
      </Card>
    </div>
  );
}
