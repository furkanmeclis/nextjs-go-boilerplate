"use client";

import Link from "next/link";
import { Check, ChevronsUpDown, LayoutGrid } from "lucide-react";

import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

function OrgMark({ name }: { name: string }) {
  return (
    <div className="bg-sidebar-primary text-sidebar-primary-foreground flex aspect-square size-8 shrink-0 items-center justify-center rounded-lg text-sm font-semibold">
      {name.trim().charAt(0).toUpperCase() || "?"}
    </div>
  );
}

/**
 * Sidebar organization switcher. Switching is plain navigation to another
 * `/t/{slug}`: nothing in the session changes, so other tabs keep their org.
 */
export function OrganizationSwitcher() {
  const { t } = useLocale();
  const { user } = useAuth();
  const tenant = useTenant();
  const orgs = user?.organizations ?? [];
  const current = tenant.organization;

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <SidebarMenuButton
              size="lg"
              className="data-[state=open]:bg-sidebar-accent"
              aria-label={t("tenant.switcher_label")}
            >
              <OrgMark name={current.name} />
              <div className="grid flex-1 text-start text-sm leading-tight">
                <span className="truncate font-medium">{current.name}</span>
                <span className="text-muted-foreground truncate text-xs">
                  {tenant.role.name}
                </span>
              </div>
              <ChevronsUpDown className="ms-auto size-4 opacity-60" />
            </SidebarMenuButton>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" className="min-w-56">
            <DropdownMenuLabel className="text-muted-foreground text-xs">
              {t("tenant.switcher_label")}
            </DropdownMenuLabel>
            {orgs.map((org) => {
              const active =
                org.slug.toLowerCase() === current.slug.toLowerCase();
              return (
                <DropdownMenuItem key={org.uuid} asChild>
                  <Link
                    href={routes.tenant.home(org.slug)}
                    aria-current={active ? "page" : undefined}
                    className="gap-2"
                  >
                    <OrgMark name={org.name} />
                    <span className="flex-1 truncate">{org.name}</span>
                    {org.status === "suspended" ? (
                      <span className="text-muted-foreground text-xs">
                        {t("tenant.status_suspended")}
                      </span>
                    ) : null}
                    {active ? <Check className="size-4" /> : null}
                  </Link>
                </DropdownMenuItem>
              );
            })}
            {orgs.length > 1 ? (
              <>
                <DropdownMenuSeparator />
                <DropdownMenuItem asChild>
                  <Link href={routes.tenant.picker} className="gap-2">
                    <LayoutGrid className="size-4" />
                    {t("tenant.switcher_all")}
                  </Link>
                </DropdownMenuItem>
              </>
            ) : null}
          </DropdownMenuContent>
        </DropdownMenu>
      </SidebarMenuItem>
    </SidebarMenu>
  );
}
