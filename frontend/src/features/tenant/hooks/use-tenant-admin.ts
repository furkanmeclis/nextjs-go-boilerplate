"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";

import {
  tenantAdminKeys,
  tenantQueryKeys,
  tenantService,
  type TenantRoleKey,
} from "@/features/tenant/services/tenant.service";
import { useAppMutation } from "@/lib/query/mutation";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export function useTenantMembers(slug: string, enabled = true) {
  return useQuery({
    queryKey: tenantAdminKeys.members(slug),
    queryFn: () => tenantService.listMembers(slug),
    enabled,
  });
}

export function useTenantInvitations(slug: string, enabled = true) {
  return useQuery({
    queryKey: tenantAdminKeys.invitations(slug),
    queryFn: () => tenantService.listInvitations(slug),
    enabled,
  });
}

export function useTenantSettings(slug: string) {
  return useQuery({
    queryKey: tenantAdminKeys.settings(slug),
    queryFn: () => tenantService.getSettings(slug),
  });
}

/**
 * After a membership change the caller's own scope may have changed too
 * (role, name): refresh the org context and /me so nav and switcher follow.
 */
function useRefreshScope(slug: string) {
  const queryClient = useQueryClient();
  const { user, hydrateProfile } = useAuth();
  return async () => {
    await queryClient.invalidateQueries({
      queryKey: tenantQueryKeys.context(slug, user?.uuid ?? "anonymous"),
    });
    await hydrateProfile();
  };
}

export function useChangeTenantMemberRole(slug: string) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  const refreshScope = useRefreshScope(slug);
  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: tenantAdminKeys.members(slug) });
  return useAppMutation({
    mutationFn: (vars: { userUuid: string; roleKey: TenantRoleKey }) =>
      tenantService.changeMemberRole(slug, vars.userUuid, vars.roleKey),
    onSuccess: async () => {
      await invalidate();
      await refreshScope();
      appToast.success(t("tenant.toast_role_changed"));
    },
    // Reset the optimistic select to the server value on refusal.
    onError: () => void invalidate(),
  });
}

export function useRemoveTenantMember(slug: string) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useAppMutation({
    mutationFn: (userUuid: string) =>
      tenantService.removeMember(slug, userUuid),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: tenantAdminKeys.members(slug),
      });
      appToast.success(t("tenant.toast_removed"));
    },
  });
}

export function useLeaveTenant(slug: string) {
  const { t } = useLocale();
  return useAppMutation({
    mutationFn: () => tenantService.leave(slug),
    onSuccess: () => appToast.success(t("tenant.toast_left")),
  });
}

export function useInviteTenantMember(slug: string) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useAppMutation({
    mutationFn: (body: { email: string; role_key: TenantRoleKey }) =>
      tenantService.createInvitation(slug, body),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: tenantAdminKeys.invitations(slug),
      });
      appToast.success(t("tenant.toast_invited"));
    },
  });
}

export function useRevokeTenantInvitation(slug: string) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  return useAppMutation({
    mutationFn: (uuid: string) => tenantService.revokeInvitation(slug, uuid),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: tenantAdminKeys.invitations(slug),
      });
      appToast.success(t("tenant.toast_revoked"));
    },
  });
}

export function usePatchTenantSettings(slug: string) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  const refreshScope = useRefreshScope(slug);
  return useAppMutation({
    mutationFn: (body: { name?: string }) =>
      tenantService.patchSettings(slug, body),
    onSuccess: async (settings) => {
      queryClient.setQueryData(tenantAdminKeys.settings(slug), settings);
      // The name shows in the switcher and page headers.
      await refreshScope();
      appToast.success(t("tenant.toast_settings_saved"));
    },
  });
}
