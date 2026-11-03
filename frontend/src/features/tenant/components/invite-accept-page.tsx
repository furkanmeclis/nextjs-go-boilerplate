"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { useRouter } from "next/navigation";

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
  TENANT_ROLE_LABEL_KEYS,
  isTenantRoleKey,
} from "@/features/tenant/lib/roles";
import { invitationService } from "@/features/tenant/services/tenant.service";
import { ApiError } from "@/lib/api/errors";
import { useAppMutation } from "@/lib/query/mutation";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

const STATUS_KEYS: Record<string, string> = {
  expired: "tenant.accept_status_expired",
  accepted: "tenant.accept_status_accepted",
  revoked: "tenant.accept_status_revoked",
};

function Shell({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex min-h-svh items-center justify-center p-6">
      <Card className="w-full max-w-md">{children}</Card>
    </div>
  );
}

/**
 * `/invite/{token}` — shows an organization invitation and lets the invited
 * account accept it. Works signed out (sign in, then come back via ?next=).
 */
export function InviteAcceptPage({ token }: { token: string }) {
  const { t } = useLocale();
  const router = useRouter();
  const { user, isAuthenticated, bootstrapped, logout, hydrateProfile } =
    useAuth();

  const preview = useQuery({
    queryKey: ["invitation", "preview", token],
    queryFn: () => invitationService.preview(token),
    retry: false,
    staleTime: 30_000,
    // The card explains failures; no global toast on top.
    meta: { silent: true },
  });

  const accept = useAppMutation({
    mutationFn: () => invitationService.accept(token),
  });

  const onAccept = async () => {
    let accepted;
    try {
      accepted = await accept.mutateAsync();
    } catch {
      void preview.refetch(); // shown as a toast; refresh the status
      return;
    }
    await hydrateProfile(); // the new membership appears in /me
    appToast.success(
      t("tenant.accept_done", { name: preview.data?.organization.name ?? "" }),
    );
    router.replace(routes.tenant.home(accepted.organization_slug));
  };

  if (preview.isLoading || (!bootstrapped && !user)) {
    return (
      <div className="text-muted-foreground flex min-h-svh items-center justify-center text-sm">
        {t("common.loading")}
      </div>
    );
  }

  if (preview.isError || !preview.data) {
    const notFound =
      preview.error instanceof ApiError &&
      (preview.error.isNotFound || preview.error.isValidation);
    return (
      <Shell>
        <CardHeader>
          <CardTitle>
            {notFound
              ? t("tenant.accept_not_found_title")
              : t("tenant.gate_error_title")}
          </CardTitle>
          <CardDescription>
            {notFound
              ? t("tenant.accept_not_found_description")
              : t("tenant.gate_error_description")}
          </CardDescription>
        </CardHeader>
        {!notFound ? (
          <CardContent>
            <Button onClick={() => void preview.refetch()}>
              {t("tenant.gate_retry")}
            </Button>
          </CardContent>
        ) : null}
      </Shell>
    );
  }

  const inv = preview.data;
  const role = isTenantRoleKey(inv.role.key)
    ? t(TENANT_ROLE_LABEL_KEYS[inv.role.key])
    : inv.role.name;

  if (inv.status !== "pending") {
    return (
      <Shell>
        <CardHeader>
          <CardTitle>{t("tenant.accept_unavailable_title")}</CardTitle>
          <CardDescription>
            {t(STATUS_KEYS[inv.status] ?? "tenant.accept_status_expired")}
          </CardDescription>
        </CardHeader>
      </Shell>
    );
  }

  const description = inv.invited_by
    ? t("tenant.accept_description", {
        inviter: inv.invited_by,
        email: inv.email,
        role,
      })
    : t("tenant.accept_description_no_inviter", { email: inv.email, role });
  const here = routes.invite(token);
  const sameAccount =
    user && user.email.trim().toLowerCase() === inv.email.toLowerCase();

  return (
    <Shell>
      <CardHeader>
        <CardTitle>
          {t("tenant.accept_title", { name: inv.organization.name })}
        </CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {!isAuthenticated || !user ? (
          <Button asChild>
            <Link
              href={`${routes.guest.login}?next=${encodeURIComponent(here)}`}
            >
              {t("tenant.accept_sign_in")}
            </Link>
          </Button>
        ) : sameAccount ? (
          <Button disabled={accept.isPending} onClick={() => void onAccept()}>
            {t("tenant.accept_submit")}
          </Button>
        ) : (
          <>
            <p className="text-muted-foreground text-sm">
              {t("tenant.accept_wrong_account", {
                current: user.email,
                email: inv.email,
              })}
            </p>
            <Button
              variant="outline"
              onClick={async () => {
                await logout();
                window.location.replace(
                  `${routes.guest.login}?next=${encodeURIComponent(here)}`,
                );
              }}
            >
              {t("tenant.accept_sign_out")}
            </Button>
          </>
        )}
      </CardContent>
    </Shell>
  );
}
