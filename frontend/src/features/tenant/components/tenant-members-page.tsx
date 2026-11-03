"use client";

import { LogOut, MailPlus, UsersRound } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";
import { z } from "zod";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import { EntityPage, EntitySectionCard } from "@/components/entity";
import { AppForm, AppInput, AppSelect } from "@/components/forms";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  useChangeTenantMemberRole,
  useInviteTenantMember,
  useLeaveTenant,
  useRemoveTenantMember,
  useRevokeTenantInvitation,
  useTenantInvitations,
  useTenantMembers,
} from "@/features/tenant/hooks/use-tenant-admin";
import {
  INVITATION_STATUS_LABEL_KEYS,
  TENANT_ROLE_KEYS,
  TENANT_ROLE_LABEL_KEYS,
  canChangeRole,
  isTenantRoleKey,
} from "@/features/tenant/lib/roles";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import type {
  TenantMember,
  TenantRoleKey,
} from "@/features/tenant/services/tenant.service";
import { defaultHomeForUser } from "@/lib/auth/types";
import { date } from "@/lib/utils/format";
import { useAuth } from "@/providers/auth-provider";
import { useDialogs } from "@/providers/dialog-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

function roleLabel(t: (k: string) => string, key: string, fallback: string) {
  return isTenantRoleKey(key) ? t(TENANT_ROLE_LABEL_KEYS[key]) : fallback;
}

function MemberRow({
  member,
  isSelf,
  actorRole,
  canWrite,
}: {
  member: TenantMember;
  isSelf: boolean;
  actorRole: string;
  canWrite: boolean;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { confirmDelete } = useDialogs();
  const changeRole = useChangeTenantMemberRole(slug);
  const remove = useRemoveTenantMember(slug);
  const name = `${member.user.name} ${member.user.surname}`.trim();
  const current = member.role.key;
  // Options the caller may move this member to (the server enforces the same).
  const options = TENANT_ROLE_KEYS.filter((k) =>
    canChangeRole(actorRole, current, k),
  );
  const editable = canWrite && !isSelf && options.length > 1;
  const removable =
    canWrite && !isSelf && canChangeRole(actorRole, current, "");

  const onRemove = async () => {
    const ok = await confirmDelete({
      title: t("tenant.members_remove_title", { name }),
      description: t("tenant.members_remove_description"),
      confirmLabel: t("tenant.members_remove"),
    });
    if (ok) remove.mutate(member.user.uuid);
  };

  return (
    <tr className="border-b last:border-0">
      <td className="py-2 pe-3">
        <div className="font-medium">
          {name}
          {isSelf ? (
            <span className="text-muted-foreground ms-1 text-xs font-normal">
              ({t("tenant.members_you")})
            </span>
          ) : null}
        </div>
        <div className="text-muted-foreground text-xs">{member.user.email}</div>
      </td>
      <td className="py-2 pe-3">
        {editable ? (
          <Select
            value={current}
            disabled={changeRole.isPending}
            onValueChange={(roleKey) =>
              changeRole.mutate({
                userUuid: member.user.uuid,
                roleKey: roleKey as TenantRoleKey,
              })
            }
          >
            <SelectTrigger
              className="h-8 w-36"
              aria-label={t("tenant.members_role")}
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {options.map((key) => (
                <SelectItem key={key} value={key}>
                  {t(TENANT_ROLE_LABEL_KEYS[key])}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        ) : (
          roleLabel(t, current, member.role.name)
        )}
      </td>
      <td className="text-muted-foreground py-2 pe-3 text-sm">
        {date(member.created_at, "dd.MM.yyyy", locale)}
      </td>
      <td className="py-2 text-end">
        {removable ? (
          <Button
            type="button"
            size="sm"
            variant="ghost"
            disabled={remove.isPending}
            onClick={() => void onRemove()}
          >
            {t("tenant.members_remove")}
          </Button>
        ) : null}
      </td>
    </tr>
  );
}

function InviteForm({ actorRole }: { actorRole: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const invite = useInviteTenantMember(slug);
  const [formKey, setFormKey] = useState(0);
  const schema = useMemo(
    () =>
      z.object({
        email: z.email(t("tenant.invite_validation_email")),
        role_key: z.enum(["owner", "admin", "member"]),
      }),
    [t],
  );
  const roleOptions = TENANT_ROLE_KEYS.filter((k) =>
    canChangeRole(actorRole, "", k),
  ).map((key) => ({ value: key, label: t(TENANT_ROLE_LABEL_KEYS[key]) }));

  return (
    <AppForm
      key={formKey}
      id="tenant-invite-form"
      schema={schema}
      defaultValues={{ email: "", role_key: "member" }}
      onSubmit={async (values: { email: string; role_key: TenantRoleKey }) => {
        await invite.mutateAsync({
          email: values.email.trim(),
          role_key: values.role_key,
        });
        setFormKey((k) => k + 1);
      }}
    >
      {(form) => (
        <div className="mb-6 grid items-end gap-3 sm:grid-cols-[1fr_12rem_auto]">
          <AppInput
            name="email"
            type="email"
            label={t("tenant.invite_email")}
            autoComplete="off"
          />
          <AppSelect
            name="role_key"
            label={t("tenant.invite_role")}
            options={roleOptions}
          />
          <Button
            type="submit"
            disabled={invite.isPending || form.formState.isSubmitting}
          >
            <MailPlus className="size-4" />
            {t("tenant.invite_submit")}
          </Button>
        </div>
      )}
    </AppForm>
  );
}

function InvitationsSection({ canWrite }: { canWrite: boolean }) {
  const { t, locale } = useLocale();
  const { slug, role } = useTenant();
  const query = useTenantInvitations(slug);
  const revoke = useRevokeTenantInvitation(slug);
  const items = query.data?.items ?? [];

  return (
    <EntitySectionCard
      title={t("tenant.invitations_title")}
      badge={query.data?.total}
    >
      <p className="text-muted-foreground mb-4 text-sm">
        {t("tenant.invitations_description")}
      </p>
      {canWrite ? <InviteForm actorRole={role.key} /> : null}
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {query.data && items.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          {t("tenant.invitations_empty")}
        </p>
      ) : null}
      {items.length ? (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-muted-foreground border-b text-xs">
                <th className="py-2 pe-3 text-start font-medium">
                  {t("tenant.invitations_email")}
                </th>
                <th className="py-2 pe-3 text-start font-medium">
                  {t("tenant.members_role")}
                </th>
                <th className="py-2 pe-3 text-start font-medium">
                  {t("tenant.invitations_expires")}
                </th>
                <th className="py-2" />
              </tr>
            </thead>
            <tbody>
              {items.map((inv) => (
                <tr key={inv.uuid} className="border-b last:border-0">
                  <td className="py-2 pe-3">
                    <div className="font-medium">{inv.email}</div>
                    <StatusChip
                      label={t(
                        INVITATION_STATUS_LABEL_KEYS[inv.status] ??
                          INVITATION_STATUS_LABEL_KEYS.pending!,
                      )}
                      tone={inv.status === "expired" ? "danger" : "warning"}
                    />
                  </td>
                  <td className="py-2 pe-3">
                    {roleLabel(t, inv.role.key, inv.role.name)}
                  </td>
                  <td className="text-muted-foreground py-2 pe-3">
                    {date(inv.expires_at, "dd.MM.yyyy", locale)}
                  </td>
                  <td className="py-2 text-end">
                    {/* Only owners may cancel an owner-role invitation. */}
                    {canWrite &&
                    (inv.role.key !== "owner" || role.key === "owner") ? (
                      <Button
                        type="button"
                        size="sm"
                        variant="ghost"
                        disabled={revoke.isPending}
                        onClick={() => revoke.mutate(inv.uuid)}
                      >
                        {t("tenant.invitations_revoke")}
                      </Button>
                    ) : null}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
    </EntitySectionCard>
  );
}

/** `/t/{slug}/members` — members, invitations, leave. */
export function TenantMembersPage() {
  const { t } = useLocale();
  const router = useRouter();
  const tenant = useTenant();
  const { user, hydrateProfile } = useAuth();
  const { can } = usePermission();
  const { confirm } = useDialogs();
  const canWrite = can(permissions.tenant.members.write);
  const membersQuery = useTenantMembers(tenant.slug);
  const leave = useLeaveTenant(tenant.slug);
  const members = membersQuery.data?.items ?? [];

  const onLeave = async () => {
    const ok = await confirm({
      title: t("tenant.members_leave_title", {
        name: tenant.organization.name,
      }),
      description: t("tenant.members_leave_description"),
      confirmLabel: t("tenant.members_leave"),
    });
    if (!ok) return;
    try {
      await leave.mutateAsync();
    } catch {
      return; // shown as a toast (e.g. LAST_ORGANIZATION_OWNER)
    }
    // Membership is gone: go to the shell /me now points to.
    const refreshed = await hydrateProfile();
    router.replace(refreshed ? defaultHomeForUser(refreshed) : routes.cms.home);
  };

  return (
    <EntityPage
      title={t("tenant.members_title")}
      description={t("tenant.members_description")}
      permission={permissions.tenant.members.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("errors.forbidden_body")}
        />
      }
      breadcrumbs={[
        {
          label: tenant.organization.name,
          href: routes.tenant.home(tenant.slug),
        },
        { label: t("tenant.members_title") },
      ]}
      actions={
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={leave.isPending}
          onClick={() => void onLeave()}
        >
          <LogOut className="size-4" />
          {t("tenant.members_leave")}
        </Button>
      }
    >
      <div className="space-y-6">
        <EntitySectionCard
          title={t("tenant.members_title")}
          badge={membersQuery.data?.total}
        >
          {membersQuery.isLoading ? (
            <Loading label={t("common.loading")} />
          ) : null}
          {membersQuery.isError ? (
            <ErrorState
              title={t("common.error_generic")}
              onRetry={() => void membersQuery.refetch()}
              retryLabel={t("common.retry")}
            />
          ) : null}
          {membersQuery.data && members.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              {t("tenant.members_empty")}
            </p>
          ) : null}
          {members.length ? (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="text-muted-foreground border-b text-xs">
                    <th className="py-2 pe-3 text-start font-medium">
                      <UsersRound className="me-1 inline size-3.5" />
                      {t("tenant.members_user")}
                    </th>
                    <th className="py-2 pe-3 text-start font-medium">
                      {t("tenant.members_role")}
                    </th>
                    <th className="py-2 pe-3 text-start font-medium">
                      {t("tenant.members_since")}
                    </th>
                    <th className="py-2" />
                  </tr>
                </thead>
                <tbody>
                  {members.map((m) => (
                    <MemberRow
                      key={m.user.uuid}
                      member={m}
                      isSelf={m.user.uuid === user?.uuid}
                      actorRole={tenant.role.key}
                      canWrite={canWrite}
                    />
                  ))}
                </tbody>
              </table>
            </div>
          ) : null}
        </EntitySectionCard>

        <InvitationsSection canWrite={canWrite} />
      </div>
    </EntityPage>
  );
}
