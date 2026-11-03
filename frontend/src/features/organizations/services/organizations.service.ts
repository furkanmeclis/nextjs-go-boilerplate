import type { ServerListParams } from "@/components/entity";
import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";

export type Organization = components["schemas"]["Organization"];
export type OrganizationPage = components["schemas"]["OrganizationPage"];
export type OrganizationMember = components["schemas"]["OrganizationMember"];
export type OrganizationMemberPage =
  components["schemas"]["OrganizationMemberPage"];
export type CreateOrganizationRequest =
  components["schemas"]["CreateOrganizationRequest"];
export type PatchOrganizationRequest =
  components["schemas"]["PatchOrganizationRequest"];
export type SetOrganizationAccessRequest =
  components["schemas"]["SetOrganizationAccessRequest"];
export type OrganizationStatus = Organization["status"];

export type ListOrganizationsParams = ServerListParams & { status?: string };

const base = "/v1/platform/organizations";

export const organizationsService = {
  list(params: ListOrganizationsParams) {
    return platformRequest<OrganizationPage>("GET", base, {
      query: {
        limit: params.limit,
        offset: params.offset,
        q: params.q,
        sort: params.sort,
        status: params.status,
      },
    });
  },
  get(uuid: string) {
    return platformRequest<Organization>("GET", `${base}/${uuid}`);
  },
  create(body: CreateOrganizationRequest) {
    return platformRequest<Organization>("POST", base, { body });
  },
  patch(uuid: string, body: PatchOrganizationRequest) {
    return platformRequest<Organization>("PATCH", `${base}/${uuid}`, { body });
  },
  setStatus(uuid: string, status: OrganizationStatus) {
    return platformRequest<Organization>("POST", `${base}/${uuid}/status`, {
      body: { status },
    });
  },
  setAccess(uuid: string, body: SetOrganizationAccessRequest) {
    return platformRequest<Organization>("POST", `${base}/${uuid}/access`, {
      body,
    });
  },
  remove(uuid: string) {
    return platformRequest<unknown>("DELETE", `${base}/${uuid}`);
  },
  members(uuid: string, params: { limit?: number; offset?: number } = {}) {
    return platformRequest<OrganizationMemberPage>(
      "GET",
      `${base}/${uuid}/members`,
      { query: { limit: params.limit ?? 100, offset: params.offset ?? 0 } },
    );
  },
  addMember(uuid: string, body: { user_uuid: string; role_key: string }) {
    return platformRequest<OrganizationMember>(
      "POST",
      `${base}/${uuid}/members`,
      { body },
    );
  },
  changeMemberRole(uuid: string, userUuid: string, roleKey: string) {
    return platformRequest<OrganizationMember>(
      "PATCH",
      `${base}/${uuid}/members/${userUuid}`,
      { body: { role_key: roleKey } },
    );
  },
  removeMember(uuid: string, userUuid: string) {
    return platformRequest<unknown>(
      "DELETE",
      `${base}/${uuid}/members/${userUuid}`,
    );
  },
};
