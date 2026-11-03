"use client";

import { Building2, Pause, Play, Trash2, UserPlus } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PermissionGuard } from "@/components/common/permission-guard";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityActions,
  EntityHeader,
  EntityPage,
  EntitySectionCard,
} from "@/components/entity";
import {
  AppDatePicker,
  AppForm,
  AppInput,
  AppSelect,
} from "@/components/forms";
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
  ORGANIZATION_STATUS_LABEL_KEYS,
  ORGANIZATION_STATUS_TONE,
  TENANT_ROLE_KEYS,
  TENANT_ROLE_LABEL_KEYS,
  type TenantRoleKey,
} from "@/features/organizations/constants";
import {
  useAddOrganizationMember,
  useChangeOrganizationMemberRole,
  useDeleteOrganization,
  useOrganization,
  useOrganizationMembers,
  usePatchOrganization,
  useRemoveOrganizationMember,
  useSetOrganizationAccess,
  useSetOrganizationStatus,
} from "@/features/organizations/hooks/use-organizations";
import {
  dayToEndISO,
  dayToStartISO,
  isoToDay,
} from "@/features/organizations/lib/access-dates";
import {
  accessWindowSchema,
  addMemberSchema,
  editOrganizationSchema,
  type AccessWindowValues,
  type AddMemberValues,
  type EditOrganizationValues,
} from "@/features/organizations/lib/schemas";
import type {
  Organization,
  OrganizationMember,
} from "@/features/organizations/services/organizations.service";
import { UserAsyncPicker } from "@/features/users/components/user-async-picker";
import { date } from "@/lib/utils/format";
import { useDialogs } from "@/providers/dialog-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

function roleLabel(t: (key: string) => string, key: string, name: string) {
  return key in TENANT_ROLE_LABEL_KEYS
    ? t(TENANT_ROLE_LABEL_KEYS[key as TenantRoleKey])
    : name;
}

function DetailActions({ org }: { org: Organization }) {
  const { t } = useLocale();
  const router = useRouter();
  const { confirm, confirmDelete } = useDialogs();
  const setStatus = useSetOrganizationStatus(org.uuid);
  const remove = useDeleteOrganization(org.uuid);

  const toggleStatus = async () => {
    if (org.status === "active") {
      const ok = await confirm({
        title: t("organizations.suspend_title", { name: org.name }),
        description: t("organizations.suspend_description"),
        confirmLabel: t("organizations.actions_suspend"),
      });
      if (!ok) return;
      setStatus.mutate("suspended");
    } else {
      setStatus.mutate("active");
    }
  };

  const onDelete = async () => {
    const ok = await confirmDelete({
      title: t("organizations.delete_title", { name: org.name }),
      description: t("organizations.delete_description"),
      confirmLabel: t("organizations.actions_delete"),
    });
    if (!ok) return;
    try {
      await remove.mutateAsync();
    } catch {
      return; // the API error is already shown as a toast
    }
    router.push(routes.platform.organizations.root);
  };

  return (
    <PermissionGuard permission={permissions.organizations.write}>
      <EntityActions>
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={setStatus.isPending}
          onClick={() => void toggleStatus()}
        >
          {org.status === "active" ? (
            <Pause className="size-4" />
          ) : (
            <Play className="size-4" />
          )}
          {org.status === "active"
            ? t("organizations.actions_suspend")
            : t("organizations.actions_activate")}
        </Button>
        <Button
          type="button"
          size="sm"
          variant="destructive"
          disabled={remove.isPending}
          onClick={() => void onDelete()}
        >
          <Trash2 className="size-4" />
          {t("organizations.actions_delete")}
        </Button>
      </EntityActions>
    </PermissionGuard>
  );
}

function ProfileSection({
  org,
  canWrite,
}: {
  org: Organization;
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const patch = usePatchOrganization(org.uuid);
  const schema = useMemo(() => editOrganizationSchema(t), [t]);

  return (
    <EntitySectionCard title={t("organizations.section_profile")}>
      {/* Remount when the saved values change (not on unrelated saves). */}
      <AppForm
        key={`${org.name}|${org.slug}`}
        id="organization-profile-form"
        schema={schema}
        defaultValues={{ name: org.name, slug: org.slug }}
        onSubmit={async (values: EditOrganizationValues) => {
          await patch.mutateAsync({
            name: values.name.trim(),
            slug: values.slug.trim(),
          });
        }}
      >
        {(form) => (
          <div className="space-y-4">
            <div className="grid gap-4 sm:grid-cols-2">
              <AppInput
                name="name"
                label={t("organizations.name")}
                disabled={!canWrite}
              />
              <AppInput
                name="slug"
                label={t("organizations.slug")}
                description={t("organizations.fields_slug_hint")}
                disabled={!canWrite}
              />
            </div>
            {canWrite ? (
              <div className="flex justify-end">
                <Button
                  type="submit"
                  disabled={
                    patch.isPending ||
                    form.formState.isSubmitting ||
                    !form.formState.isDirty
                  }
                >
                  {t("organizations.actions_save")}
                </Button>
              </div>
            ) : null}
          </div>
        )}
      </AppForm>
    </EntitySectionCard>
  );
}

function AccessSection({
  org,
  canWrite,
}: {
  org: Organization;
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const setAccess = useSetOrganizationAccess(org.uuid);
  const schema = useMemo(() => accessWindowSchema(t), [t]);

  return (
    <EntitySectionCard title={t("organizations.section_access")}>
      <p className="text-muted-foreground mb-4 text-sm">
        {t("organizations.section_access_desc")}
      </p>
      <AppForm
        key={`${org.access_starts_at}|${org.access_ends_at ?? ""}`}
        id="organization-access-form"
        schema={schema}
        defaultValues={{
          starts: isoToDay(org.access_starts_at),
          ends: isoToDay(org.access_ends_at),
        }}
        onSubmit={async (values: AccessWindowValues) => {
          // An untouched bound keeps its exact stored timestamp; only an
          // edited day is snapped to local start/end of day.
          const starts =
            values.starts === isoToDay(org.access_starts_at)
              ? org.access_starts_at
              : values.starts
                ? dayToStartISO(values.starts)
                : null;
          const ends =
            values.ends === isoToDay(org.access_ends_at)
              ? (org.access_ends_at ?? null)
              : values.ends
                ? dayToEndISO(values.ends)
                : null;
          await setAccess.mutateAsync({
            access_starts_at: starts,
            access_ends_at: ends,
          });
        }}
      >
        {(form) => (
          <div className="space-y-4">
            <div className="grid gap-4 sm:grid-cols-2">
              <AppDatePicker
                name="starts"
                label={t("organizations.fields_access_start")}
                disabled={!canWrite}
              />
              <AppDatePicker
                name="ends"
                label={t("organizations.fields_access_end")}
                description={t("organizations.fields_access_end_hint")}
                disabled={!canWrite}
              />
            </div>
            {canWrite ? (
              <div className="flex justify-end">
                <Button
                  type="submit"
                  disabled={
                    setAccess.isPending ||
                    form.formState.isSubmitting ||
                    !form.formState.isDirty
                  }
                >
                  {t("organizations.actions_save")}
                </Button>
              </div>
            ) : null}
          </div>
        )}
      </AppForm>
    </EntitySectionCard>
  );
}

function MemberRow({
  orgUuid,
  member,
  canWrite,
}: {
  orgUuid: string;
  member: OrganizationMember;
  canWrite: boolean;
}) {
  const { t, locale } = useLocale();
  const { confirmDelete } = useDialogs();
  const changeRole = useChangeOrganizationMemberRole(orgUuid);
  const remove = useRemoveOrganizationMember(orgUuid);
  const fullName = `${member.user.name} ${member.user.surname}`.trim();

  const onRemove = async () => {
    const ok = await confirmDelete({
      title: t("organizations.members_remove_title", { name: fullName }),
      description: t("organizations.members_remove_description"),
      confirmLabel: t("organizations.members_remove"),
    });
    if (ok) remove.mutate(member.user.uuid);
  };

  return (
    <tr className="border-b last:border-0">
      <td className="py-2 pe-3">
        <div className="font-medium">{fullName}</div>
        <div className="text-muted-foreground text-xs">{member.user.email}</div>
      </td>
      <td className="py-2 pe-3">
        {canWrite ? (
          <Select
            value={member.role.key}
            disabled={changeRole.isPending}
            onValueChange={(roleKey) =>
              changeRole.mutate({ userUuid: member.user.uuid, roleKey })
            }
          >
            <SelectTrigger
              className="h-8 w-36"
              aria-label={t("organizations.members_role")}
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {TENANT_ROLE_KEYS.map((key) => (
                <SelectItem key={key} value={key}>
                  {t(TENANT_ROLE_LABEL_KEYS[key])}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        ) : (
          roleLabel(t, member.role.key, member.role.name)
        )}
      </td>
      <td className="text-muted-foreground py-2 pe-3 text-sm">
        {date(member.created_at, "dd.MM.yyyy", locale)}
      </td>
      <td className="py-2 text-end">
        {canWrite ? (
          <Button
            type="button"
            size="sm"
            variant="ghost"
            disabled={remove.isPending}
            onClick={() => void onRemove()}
          >
            {t("organizations.members_remove")}
          </Button>
        ) : null}
      </td>
    </tr>
  );
}

function MembersSection({
  org,
  canWrite,
}: {
  org: Organization;
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const { can } = usePermission();
  const canSearchUsers = can(permissions.users.read);
  const membersQuery = useOrganizationMembers(org.uuid);
  const add = useAddOrganizationMember(org.uuid);
  const schema = useMemo(() => addMemberSchema(t), [t]);
  // Bumped after a successful add to remount (clear) the add-member form.
  const [addFormKey, setAddFormKey] = useState(0);
  const members = membersQuery.data?.items ?? [];
  const roleOptions = TENANT_ROLE_KEYS.map((key) => ({
    value: key,
    label: t(TENANT_ROLE_LABEL_KEYS[key]),
  }));

  return (
    <EntitySectionCard
      title={t("organizations.section_members")}
      badge={membersQuery.data?.total}
    >
      <p className="text-muted-foreground mb-4 text-sm">
        {t("organizations.section_members_desc")}
      </p>

      {/* The user picker searches /v1/platform/users (users read). */}
      {canWrite && canSearchUsers ? (
        <AppForm
          key={addFormKey}
          id="organization-add-member-form"
          schema={schema}
          defaultValues={{ user_uuid: "", role_key: "member" }}
          onSubmit={async (values: AddMemberValues) => {
            await add.mutateAsync(values);
            setAddFormKey((k) => k + 1);
          }}
        >
          {(form) => (
            <div className="mb-6 grid items-end gap-3 sm:grid-cols-[1fr_12rem_auto]">
              <UserAsyncPicker
                name="user_uuid"
                label={t("organizations.members_user")}
              />
              <AppSelect
                name="role_key"
                label={t("organizations.members_role")}
                options={roleOptions}
              />
              <Button
                type="submit"
                disabled={add.isPending || form.formState.isSubmitting}
              >
                <UserPlus className="size-4" />
                {t("organizations.members_add")}
              </Button>
            </div>
          )}
        </AppForm>
      ) : null}

      {membersQuery.isLoading ? <Loading label={t("common.loading")} /> : null}
      {membersQuery.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void membersQuery.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {membersQuery.data && members.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          {t("organizations.members_empty")}
        </p>
      ) : null}
      {members.length ? (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-muted-foreground border-b text-start text-xs">
                <th className="py-2 pe-3 text-start font-medium">
                  {t("organizations.members_user")}
                </th>
                <th className="py-2 pe-3 text-start font-medium">
                  {t("organizations.members_role")}
                </th>
                <th className="py-2 pe-3 text-start font-medium">
                  {t("organizations.members_since")}
                </th>
                <th className="py-2" />
              </tr>
            </thead>
            <tbody>
              {members.map((member) => (
                <MemberRow
                  key={member.user.uuid}
                  orgUuid={org.uuid}
                  member={member}
                  canWrite={canWrite}
                />
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
    </EntitySectionCard>
  );
}

/** `/platform/organizations/{uuid}` — profile, access window, members. */
export function OrganizationDetailPage({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const orgQuery = useOrganization(uuid);
  const org = orgQuery.data;
  const canWrite = can(permissions.organizations.write);
  const title = org?.name ?? t("organizations.title");

  return (
    <EntityPage
      title={title}
      description={t("organizations.detail_description")}
      permission={permissions.organizations.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("organizations.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        {
          label: t("organizations.title"),
          href: routes.platform.organizations.root,
        },
        { label: title },
      ]}
      actions={org ? <DetailActions org={org} /> : null}
    >
      {orgQuery.isLoading ? <Loading label={t("common.loading")} /> : null}
      {orgQuery.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("organizations.detail_not_found")}
          onRetry={() => void orgQuery.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}

      {org ? (
        <div className="space-y-6">
          <EntityHeader
            title={org.name}
            badges={
              <StatusChip
                label={t(ORGANIZATION_STATUS_LABEL_KEYS[org.status])}
                tone={ORGANIZATION_STATUS_TONE[org.status]}
              />
            }
            leading={
              <div className="bg-muted flex size-12 items-center justify-center rounded-lg">
                <Building2 className="text-muted-foreground size-6" />
              </div>
            }
          >
            <dl className="grid gap-3 sm:grid-cols-3">
              <div className="space-y-1">
                <dt className="text-muted-foreground text-xs font-medium">
                  {t("organizations.slug")}
                </dt>
                <dd className="text-sm">
                  <code>{org.slug}</code>
                </dd>
              </div>
              <div className="space-y-1">
                <dt className="text-muted-foreground text-xs font-medium">
                  {t("organizations.access_ends_at")}
                </dt>
                <dd className="text-sm">
                  {org.access_ends_at
                    ? date(org.access_ends_at, "dd.MM.yyyy", locale)
                    : t("organizations.access_open")}
                </dd>
              </div>
              <div className="space-y-1">
                <dt className="text-muted-foreground text-xs font-medium">
                  {t("organizations.created_at")}
                </dt>
                <dd className="text-sm">
                  {date(org.created_at, "dd.MM.yyyy", locale)}
                </dd>
              </div>
            </dl>
          </EntityHeader>

          <ProfileSection org={org} canWrite={canWrite} />
          <AccessSection org={org} canWrite={canWrite} />
          <MembersSection org={org} canWrite={canWrite} />
        </div>
      ) : null}
    </EntityPage>
  );
}
