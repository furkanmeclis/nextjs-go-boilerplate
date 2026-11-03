"use client";

import { useMemo } from "react";
import type { ColumnDef } from "@tanstack/react-table";

import { StatusChip } from "@/components/common/status-chip";
import { createColumn } from "@/components/tables";
import {
  ORGANIZATION_STATUS_LABEL_KEYS,
  ORGANIZATION_STATUS_TONE,
  ORGANIZATION_STATUS_VALUES,
} from "@/features/organizations/constants";
import type { Organization } from "@/features/organizations/services/organizations.service";
import { date, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

export function useOrganizationsColumns() {
  const { t, locale } = useLocale();

  return useMemo(
    () =>
      [
        createColumn<Organization>({
          accessorKey: "name",
          labelKey: "organizations.name",
          enableSorting: true,
          filterVariant: "text",
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.name}</span>
          ),
        }),
        createColumn<Organization>({
          accessorKey: "slug",
          labelKey: "organizations.slug",
          enableSorting: true,
          filterVariant: "text",
          gridSecondary: true,
          cell: ({ row }) => (
            <code className="text-muted-foreground text-xs">
              {row.original.slug}
            </code>
          ),
        }),
        createColumn<Organization>({
          accessorKey: "status",
          labelKey: "organizations.status",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: ORGANIZATION_STATUS_VALUES.map((value) => ({
            value,
            labelKey: ORGANIZATION_STATUS_LABEL_KEYS[value],
            label: value,
          })),
          cell: ({ row }) => (
            <StatusChip
              label={t(ORGANIZATION_STATUS_LABEL_KEYS[row.original.status])}
              tone={ORGANIZATION_STATUS_TONE[row.original.status]}
            />
          ),
        }),
        createColumn<Organization>({
          accessorKey: "access_ends_at",
          labelKey: "organizations.access_ends_at",
          enableSorting: false,
          enableColumnFilter: false,
          cell: ({ row }) =>
            row.original.access_ends_at ? (
              date(row.original.access_ends_at, "dd.MM.yyyy", locale)
            ) : (
              <span className="text-muted-foreground text-sm">
                {t("organizations.access_open")}
              </span>
            ),
        }),
        createColumn<Organization>({
          accessorKey: "created_at",
          labelKey: "organizations.created_at",
          enableSorting: true,
          enableColumnFilter: false,
          cell: ({ row }) =>
            datetime(row.original.created_at, "dd.MM.yyyy HH:mm", locale),
        }),
      ] as ColumnDef<Organization, unknown>[],
    [locale, t],
  );
}
