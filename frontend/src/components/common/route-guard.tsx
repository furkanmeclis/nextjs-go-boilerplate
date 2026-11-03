"use client";

import { useEffect, type ReactNode } from "react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { routes } from "@/config/routes";
import {
  defaultHomeForUser,
  isCmsUser,
  isPlatformUser,
} from "@/lib/auth/types";
import { safeNextPath } from "@/lib/auth/next-path";
import { useAuthStore } from "@/lib/auth/session-store";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

type RouteGuardProps = {
  children: ReactNode;
  /**
   * `tenant` only requires a session: organization access is decided per
   * membership by the tenant shell, so platform users who are members may
   * enter too.
   */
  mode: "guest" | "platform" | "cms" | "tenant";
};

function PageLoader({ label }: { label: string }) {
  return (
    <div className="text-muted-foreground flex min-h-svh items-center justify-center text-sm">
      {label}
    </div>
  );
}

function RoleMismatch({ expected, home }: { expected: string; home: string }) {
  const { t } = useLocale();
  return (
    <div className="flex min-h-svh items-center justify-center p-6">
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle>{t("errors.forbidden_title")}</CardTitle>
          <CardDescription>{t("errors.role_mismatch")}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          <p className="text-muted-foreground text-sm">
            {t("errors.area_requires", { area: expected })}
          </p>
          <Button asChild>
            <Link href={home}>{t("errors.continue")}</Link>
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}

const AUTHENTICATED_MODES = new Set<RouteGuardProps["mode"]>([
  "platform",
  "cms",
  "tenant",
]);

export function RouteGuard({ children, mode }: RouteGuardProps) {
  const router = useRouter();
  const pathname = usePathname();
  const { bootstrapped, isAuthenticated, user } = useAuth();
  const { t } = useLocale();
  const cachedUser = useAuthStore((s) => s.user);

  useEffect(() => {
    if (!bootstrapped) return;

    if (mode === "guest" && isAuthenticated && user) {
      // Same target as the login form: an explicit ?next= wins, otherwise
      // the user's default shell. (Without this the guard raced the form and
      // dropped deep links such as /t/{slug}.)
      const next = new URLSearchParams(window.location.search).get("next");
      router.replace(safeNextPath(next, defaultHomeForUser(user)));
      return;
    }

    if (AUTHENTICATED_MODES.has(mode) && !isAuthenticated) {
      // Tenant links are shareable: come back to the same org after login.
      router.replace(
        mode === "tenant" && pathname
          ? `${routes.guest.login}?next=${encodeURIComponent(pathname)}`
          : routes.guest.login,
      );
    }
  }, [bootstrapped, mode, isAuthenticated, user, router, pathname]);

  // Show the loader only when there is no cached user to render optimistically.
  // With a cached user, render immediately and let session verification update
  // state in the background.
  if (!bootstrapped && !cachedUser) {
    return <PageLoader label={t("common.loading")} />;
  }

  if (AUTHENTICATED_MODES.has(mode) && !isAuthenticated) {
    return null;
  }

  if (mode === "platform" && user && !isPlatformUser(user)) {
    return (
      <RoleMismatch
        expected={t("layout.nav_platform")}
        home={isCmsUser(user) ? routes.cms.home : routes.errors.forbidden}
      />
    );
  }

  if (mode === "cms" && user && !isCmsUser(user)) {
    return (
      <RoleMismatch
        expected="CMS"
        home={
          isPlatformUser(user) ? routes.platform.home : routes.errors.forbidden
        }
      />
    );
  }

  return <>{children}</>;
}
