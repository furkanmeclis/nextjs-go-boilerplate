"use client";

import { useMemo, type ReactNode } from "react";

import { AppSidebar } from "@/components/layout/app-sidebar";
import { Footer } from "@/components/layout/footer";
import { Header } from "@/components/layout/header";
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar";
import { shellNav, type ShellNav } from "@/config/nav";
import {
  CommandPalette,
  CommandPaletteProvider,
  SearchTrigger,
} from "@/features/search-engine";
import { ImpersonationBanner } from "@/features/auth/components/impersonation-banner";
import { NotificationLiveBridge } from "@/features/notifications/components/notification-live-bridge";
import { useWebPushSync } from "@/hooks/use-web-push-sync";

export type AppLayoutVariant = "platform" | "cms" | "tenant";

export function AppLayout({
  children,
  variant = "platform",
  nav,
  sidebarHeader,
}: {
  children: ReactNode;
  variant?: AppLayoutVariant;
  /** Required for `tenant` (its catalog depends on the org slug). */
  nav?: ShellNav;
  /** Extra content under the sidebar logo (e.g. the organization switcher). */
  sidebarHeader?: ReactNode;
}) {
  useWebPushSync(true);
  const resolvedNav = useMemo(
    () =>
      nav ??
      shellNav(
        variant === "cms" ? { variant: "cms" } : { variant: "platform" },
      ),
    [nav, variant],
  );

  return (
    <CommandPaletteProvider>
      <SidebarProvider defaultOpen>
        <AppSidebar nav={resolvedNav} header={sidebarHeader} />
        <SidebarInset
          className={
            "@container/content has-data-[layout=fixed]:h-svh peer-data-[variant=inset]:has-data-[layout=fixed]:h-[calc(100svh-(var(--spacing)*4))]"
          }
        >
          <Header fixed>
            <SearchTrigger />
          </Header>
          <div className="flex min-w-0 flex-1 flex-col gap-4 px-4 pt-2 pb-6 md:px-6">
            <ImpersonationBanner />
            {children}
          </div>
          <Footer />
        </SidebarInset>
        <NotificationLiveBridge />
        <CommandPalette catalogs={resolvedNav.paletteCatalogs} />
      </SidebarProvider>
    </CommandPaletteProvider>
  );
}
