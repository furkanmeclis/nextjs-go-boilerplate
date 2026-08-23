import { useCallback, useEffect, useMemo, useState } from "react";
import type { RowSelectionState, Updater } from "@tanstack/react-table";

import type { SelectionScope } from "@/features/bulk-engine/types";

type UseBulkSelectionOptions = {
  listQueryKey: unknown;
  bulkQuery: Record<string, string | undefined>;
  total: number;
};

export function useBulkSelection({
  listQueryKey,
  bulkQuery,
  total,
}: UseBulkSelectionOptions) {
  const [rowSelection, setRowSelection] = useState<RowSelectionState>({});
  const [scope, setScope] = useState<SelectionScope>({ mode: "none" });

  const onRowSelectionChange = useCallback((updater: Updater<RowSelectionState>) => {
    setRowSelection((prev) => (typeof updater === "function" ? updater(prev) : updater));
    setScope((current) => (current.mode === "all" ? { mode: "none" } : current));
  }, []);

  const pageIds = useMemo(
    () => Object.keys(rowSelection).filter((id) => rowSelection[id]),
    [rowSelection],
  );

  useEffect(() => {
    if (scope.mode === "all") return;
    if (pageIds.length === 0) {
      setScope({ mode: "none" });
      return;
    }
    setScope({ mode: "page", ids: pageIds });
  }, [pageIds, scope.mode]);

  useEffect(() => {
    setScope({ mode: "none" });
    setRowSelection({});
  }, [listQueryKey, bulkQuery]);

  const clearSelection = useCallback(() => {
    setScope({ mode: "none" });
    setRowSelection({});
  }, []);

  const selectAllMatching = useCallback(() => {
    const query: Record<string, string> = {};
    for (const [key, value] of Object.entries(bulkQuery)) {
      if (value) query[key] = value;
    }
    setScope({ mode: "all", query, total });
    setRowSelection({});
  }, [bulkQuery, total]);

  const selectedCount =
    scope.mode === "all" ? scope.total : scope.mode === "page" ? scope.ids.length : 0;

  const showSelectAllBanner =
    scope.mode === "page" && pageIds.length > 0 && total > pageIds.length;

  return {
    scope,
    rowSelection,
    onRowSelectionChange,
    selectedCount,
    showSelectAllBanner,
    clearSelection,
    selectAllMatching,
  };
}

export function buildBulkTarget(scope: SelectionScope) {
  if (scope.mode === "page") {
    return { scope: "ids" as const, ids: scope.ids };
  }
  if (scope.mode === "all") {
    return { scope: "query" as const, query: scope.query };
  }
  throw new Error("No rows selected");
}
