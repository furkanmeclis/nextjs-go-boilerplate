"use client";

import { Building2 } from "lucide-react";
import { useMemo } from "react";
import { z } from "zod";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityPage, EntitySectionCard } from "@/components/entity";
import { AppForm, AppInput } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  usePatchTenantSettings,
  useTenantSettings,
} from "@/features/tenant/hooks/use-tenant-admin";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/**
 * `/t/{slug}/settings` — organization name. The app-defined `settings`
 * object is left to apps (PATCH /v1/tenant/settings accepts it); the slug is
 * managed by the platform because every /t/{slug} link depends on it.
 */
export function TenantSettingsPage() {
  const { t } = useLocale();
  const tenant = useTenant();
  const { can } = usePermission();
  const canWrite = can(permissions.tenant.settings.write);
  const query = useTenantSettings(tenant.slug);
  const patch = usePatchTenantSettings(tenant.slug);
  const schema = useMemo(
    () =>
      z.object({
        name: z
          .string()
          .trim()
          .min(1, t("tenant.settings_validation_name"))
          .max(255),
      }),
    [t],
  );
  const settings = query.data;

  return (
    <EntityPage
      title={t("tenant.settings_title")}
      description={t("tenant.settings_description")}
      permission={permissions.tenant.settings.read}
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
        { label: t("tenant.settings_title") },
      ]}
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {settings ? (
        <EntitySectionCard title={t("tenant.settings_title")}>
          <AppForm
            key={settings.name}
            id="tenant-settings-form"
            schema={schema}
            defaultValues={{ name: settings.name }}
            onSubmit={async (values: { name: string }) => {
              await patch.mutateAsync({ name: values.name.trim() });
            }}
          >
            {(form) => (
              <div className="space-y-4">
                <div className="grid gap-4 sm:grid-cols-2">
                  <AppInput
                    name="name"
                    label={t("tenant.settings_name")}
                    startIcon={Building2}
                    disabled={!canWrite}
                  />
                  <div className="space-y-2">
                    <div className="text-sm font-medium">
                      {t("tenant.settings_slug")}
                    </div>
                    <code className="bg-muted block rounded-md px-3 py-2 text-sm">
                      /t/{settings.slug}
                    </code>
                  </div>
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
                      {t("tenant.settings_save")}
                    </Button>
                  </div>
                ) : null}
              </div>
            )}
          </AppForm>
        </EntitySectionCard>
      ) : null}
    </EntityPage>
  );
}
