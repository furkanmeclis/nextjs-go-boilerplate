"use client";

import { useCallback, useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityPage,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { ResourceIOToolbar } from "@/features/io";
import type { ResourceMeta } from "@/features/io/types";
import { NotificationDetailDrawer } from "@/features/notifications/components/notification-detail-drawer";
import { useNotificationsColumns } from "@/features/notifications/components/notifications-columns";
import type { NotificationRowActionHandlers } from "@/features/notifications/components/notification-row-actions";
import { NOTIFICATION_INBOX_CHANNEL } from "@/features/notifications/constants";
import { useNotificationRealtimeInvalidate } from "@/features/notifications/hooks/use-notification-realtime";
import {
  usePlatformNotificationsList,
  usePlatformNotificationsMeta,
} from "@/features/notifications/hooks/use-notifications-query";
import type {
  ListPlatformNotificationsParams,
  Notification,
} from "@/features/notifications/services/notifications.service";
import { useLocale } from "@/providers/locale-provider";

function firstString(value: string | string[] | undefined) {
  if (Array.isArray(value)) return value[0];
  return value;
}

export function NotificationsPage() {
  const { t } = useLocale();

  useNotificationRealtimeInvalidate();

  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 20,
  });

  const [selected, setSelected] = useState<Notification | null>(null);

  const listParams = useMemo<ListPlatformNotificationsParams>(() => {
    const columnValue = (id: string) =>
      firstString(
        listState.columnFilters.find((filter) => filter.id === id)?.value as
          string | string[] | undefined,
      );

    const q = listState.params.q || columnValue("title")?.trim() || undefined;

    return {
      ...listState.params,
      q,
      status: columnValue("status"),
      channel: NOTIFICATION_INBOX_CHANNEL,
    };
  }, [listState.columnFilters, listState.params]);

  const listQuery = usePlatformNotificationsList(listParams);
  const metaQuery = usePlatformNotificationsMeta(true);

  const openDetail = useCallback((notification: Notification) => {
    setSelected(notification);
  }, []);

  const rowHandlers = useMemo<NotificationRowActionHandlers>(
    () => ({
      onView: openDetail,
    }),
    [openDetail],
  );

  const columns = useNotificationsColumns(rowHandlers);

  const pageCount = useMemo(() => {
    const total = listQuery.data?.total ?? 0;
    const size = listState.pagination.pageSize || 20;
    return Math.max(1, Math.ceil(total / size));
  }, [listQuery.data?.total, listState.pagination.pageSize]);

  return (
    <EntityPage
      title={t("notifications.title")}
      description={t("notifications.description")}
      permission={permissions.notifications.platformRead}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("notifications.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        { label: t("notifications.title") },
      ]}
    >
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={openDetail}
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        onRetry={() => void listQuery.refetch()}
        emptyTitle={t("notifications.empty_title")}
        emptyDescription={t("notifications.empty_description")}
        pageCount={pageCount}
        state={listState.tableState}
        features={{
          persistKey: "platform-notifications-v2",
          rowSelection: false,
        }}
        toolbarExtra={
          <>
            <ResourceIOToolbar
              resource="platform.notifications"
              query={{
                q: listParams.q,
                status: listParams.status,
                channel: listParams.channel,
                sort: listParams.sort,
              }}
              capabilities={
                (metaQuery.data as ResourceMeta | undefined)?.capabilities
              }
            />
            <EntityToolbar
              onRefresh={() => void listQuery.refetch()}
              refreshDisabled={listQuery.isFetching}
            />
          </>
        }
      />

      <NotificationDetailDrawer
        notification={selected}
        open={Boolean(selected)}
        onOpenChange={(next) => {
          if (!next) setSelected(null);
        }}
      />
    </EntityPage>
  );
}
