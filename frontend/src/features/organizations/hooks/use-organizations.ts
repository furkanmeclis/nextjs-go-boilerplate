"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";

import { ORGANIZATION_STATUS_TOAST_KEYS } from "@/features/organizations/constants";
import { organizationsKeys } from "@/features/organizations/hooks/query-keys";
import {
  organizationsService,
  type CreateOrganizationRequest,
  type ListOrganizationsParams,
  type Organization,
  type OrganizationStatus,
  type PatchOrganizationRequest,
  type SetOrganizationAccessRequest,
} from "@/features/organizations/services/organizations.service";
import { useAppMutation } from "@/lib/query/mutation";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export function useOrganizationsList(params: ListOrganizationsParams) {
  return useQuery({
    queryKey: organizationsKeys.list(params),
    queryFn: () => organizationsService.list(params),
    placeholderData: (previous) => previous,
  });
}

export function useOrganization(uuid: string) {
  return useQuery({
    queryKey: organizationsKeys.detail(uuid),
    queryFn: () => organizationsService.get(uuid),
    enabled: Boolean(uuid),
  });
}

export function useOrganizationMembers(uuid: string) {
  return useQuery({
    queryKey: organizationsKeys.members(uuid),
    queryFn: () => organizationsService.members(uuid),
    enabled: Boolean(uuid),
  });
}

/** Writes the fresh organization into detail + list caches. */
function useStoreOrganization() {
  const queryClient = useQueryClient();
  return (org: Organization) => {
    queryClient.setQueryData(organizationsKeys.detail(org.uuid), org);
    void queryClient.invalidateQueries({
      queryKey: organizationsKeys.lists(),
    });
  };
}

export function useCreateOrganization() {
  const { t } = useLocale();
  const store = useStoreOrganization();
  return useAppMutation({
    mutationFn: (body: CreateOrganizationRequest) =>
      organizationsService.create(body),
    onSuccess: (org) => {
      store(org);
      appToast.success(t("organizations.toast_created"));
    },
  });
}

export function usePatchOrganization(uuid: string) {
  const { t } = useLocale();
  const store = useStoreOrganization();
  return useAppMutation({
    mutationFn: (body: PatchOrganizationRequest) =>
      organizationsService.patch(uuid, body),
    onSuccess: (org) => {
      store(org);
      appToast.success(t("organizations.toast_updated"));
    },
  });
}

export function useSetOrganizationStatus(uuid: string) {
  const { t } = useLocale();
  const store = useStoreOrganization();
  return useAppMutation({
    mutationFn: (status: OrganizationStatus) =>
      organizationsService.setStatus(uuid, status),
    onSuccess: (org) => {
      store(org);
      appToast.success(t(ORGANIZATION_STATUS_TOAST_KEYS[org.status]));
    },
  });
}

export function useSetOrganizationAccess(uuid: string) {
  const { t } = useLocale();
  const store = useStoreOrganization();
  return useAppMutation({
    mutationFn: (body: SetOrganizationAccessRequest) =>
      organizationsService.setAccess(uuid, body),
    onSuccess: (org) => {
      store(org);
      appToast.success(t("organizations.toast_access_updated"));
    },
  });
}

export function useDeleteOrganization(uuid: string) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useAppMutation({
    mutationFn: () => organizationsService.remove(uuid),
    onSuccess: () => {
      queryClient.removeQueries({ queryKey: organizationsKeys.detail(uuid) });
      void queryClient.invalidateQueries({
        queryKey: organizationsKeys.lists(),
      });
      appToast.success(t("organizations.toast_deleted"));
    },
  });
}

function useInvalidateMembers(uuid: string) {
  const queryClient = useQueryClient();
  return () =>
    queryClient.invalidateQueries({
      queryKey: organizationsKeys.members(uuid),
    });
}

export function useAddOrganizationMember(uuid: string) {
  const { t } = useLocale();
  const invalidate = useInvalidateMembers(uuid);
  return useAppMutation({
    mutationFn: (body: { user_uuid: string; role_key: string }) =>
      organizationsService.addMember(uuid, body),
    onSuccess: () => {
      void invalidate();
      appToast.success(t("organizations.toast_member_added"));
    },
  });
}

export function useChangeOrganizationMemberRole(uuid: string) {
  const { t } = useLocale();
  const invalidate = useInvalidateMembers(uuid);
  return useAppMutation({
    mutationFn: (vars: { userUuid: string; roleKey: string }) =>
      organizationsService.changeMemberRole(uuid, vars.userUuid, vars.roleKey),
    onSuccess: () => {
      void invalidate();
      appToast.success(t("organizations.toast_member_role_changed"));
    },
    // A rejected change (e.g. LAST_ORGANIZATION_OWNER) must also reset the
    // optimistic select back to the server value.
    onError: () => void invalidate(),
  });
}

export function useRemoveOrganizationMember(uuid: string) {
  const { t } = useLocale();
  const invalidate = useInvalidateMembers(uuid);
  return useAppMutation({
    mutationFn: (userUuid: string) =>
      organizationsService.removeMember(uuid, userUuid),
    onSuccess: () => {
      void invalidate();
      appToast.success(t("organizations.toast_member_removed"));
    },
  });
}
