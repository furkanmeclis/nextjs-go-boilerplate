"use client";

import Link from "next/link";
import type { ReactNode } from "react";

import { AppSidebarLogo } from "@/components/brand";
import {
  Sidebar,
  SidebarFooter,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
} from "@/components/ui/sidebar";
import type { ShellNav } from "@/config/nav";
import { NavEngine } from "@/features/nav-engine";

export function AppSidebar({
  nav,
  header,
}: {
  nav: ShellNav;
  header?: ReactNode;
}) {
  const { catalog, homeHref } = nav;

  return (
    <Sidebar collapsible="icon" variant="inset">
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton size="lg" asChild>
              <Link href={homeHref}>
                <AppSidebarLogo />
              </Link>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
        {header}
      </SidebarHeader>

      <NavEngine catalog={catalog} homeHref={homeHref} />

      <SidebarFooter />
      <SidebarRail />
    </Sidebar>
  );
}
