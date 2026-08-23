"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityPage } from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { SettingsForm } from "@/features/io/components/settings-form";
import { ioKeys } from "@/features/io/hooks/query-keys";
import type { PatchSettingsRequest } from "@/features/io/services/settings.service";
import { settingsService } from "@/features/io/services/settings.service";
import { useAppMutation } from "@/lib/query/mutation";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

export function SettingsPage() {
  const { t } = useLocale();
  const { can } = usePermission();
  const canWrite = can(permissions.settings.write);
  const queryClient = useQueryClient();

  const settingsQuery = useQuery({
    queryKey: ioKeys.settings.all,
    queryFn: () => settingsService.get(),
  });

  const save = useAppMutation({
    mutationFn: (body: PatchSettingsRequest) => settingsService.patch(body),
    onSuccess: (data) => {
      void queryClient.setQueryData(ioKeys.settings.all, data);
      appToast.success(t("settings.toast.saved"));
    },
    onError: () => {
      appToast.error(t("settings.toast.save_failed"));
    },
  });

  const uploadLogo = useAppMutation({
    mutationFn: (file: File) => settingsService.uploadLogo(file),
    onSuccess: (data) => {
      void queryClient.setQueryData(ioKeys.settings.all, data);
      appToast.success(t("settings.toast.logo_saved"));
    },
  });

  const removeLogo = useAppMutation({
    mutationFn: () => settingsService.deleteLogo(),
    onSuccess: (data) => {
      void queryClient.setQueryData(ioKeys.settings.all, data);
      appToast.success(t("settings.toast.logo_removed"));
    },
  });

  return (
    <EntityPage
      title={t("settings.title")}
      description={t("settings.description")}
      permission={permissions.settings.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("settings.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        { label: t("layout.section_io"), href: routes.platform.settings.root },
        { label: t("settings.title") },
      ]}
    >
      {settingsQuery.isLoading ? (
        <Loading label={t("common.loading")} />
      ) : settingsQuery.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("settings.forbidden")}
          onRetry={() => void settingsQuery.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : settingsQuery.data ? (
        <SettingsForm
          key={JSON.stringify(settingsQuery.data)}
          settings={settingsQuery.data}
          canWrite={canWrite}
          isSaving={save.isPending}
          isLogoUploading={uploadLogo.isPending}
          isLogoRemoving={removeLogo.isPending}
          onSubmit={async (payload) => {
            await save.mutateAsync(payload);
          }}
          onUploadLogo={(file) => uploadLogo.mutate(file)}
          onRemoveLogo={() => removeLogo.mutate()}
        />
      ) : null}
    </EntityPage>
  );
}
