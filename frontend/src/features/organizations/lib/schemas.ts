import { z } from "zod";

import {
  SLUG_PATTERN,
  TENANT_ROLE_KEYS,
} from "@/features/organizations/constants";

type Translate = (key: string) => string;

/** Aligns with CreateOrganizationRequest (slug and owner optional). */
export function createOrganizationSchema(t: Translate) {
  return z.object({
    name: z.string().trim().min(1, t("organizations.validation_name_required")),
    slug: z
      .string()
      .trim()
      .refine((v) => v === "" || SLUG_PATTERN.test(v), {
        message: t("organizations.validation_slug"),
      }),
    owner_user_uuid: z.string(),
  });
}
export type CreateOrganizationValues = z.infer<
  ReturnType<typeof createOrganizationSchema>
>;

/** Aligns with PatchOrganizationRequest (name + slug). */
export function editOrganizationSchema(t: Translate) {
  return z.object({
    name: z.string().trim().min(1, t("organizations.validation_name_required")),
    slug: z
      .string()
      .trim()
      .regex(SLUG_PATTERN, t("organizations.validation_slug")),
  });
}
export type EditOrganizationValues = z.infer<
  ReturnType<typeof editOrganizationSchema>
>;

/** Whole days (`yyyy-MM-dd`); empty end = open-ended. */
export function accessWindowSchema(t: Translate) {
  return z
    .object({ starts: z.string(), ends: z.string() })
    .refine((v) => !v.starts || !v.ends || v.ends >= v.starts, {
      message: t("organizations.validation_access_order"),
      path: ["ends"],
    });
}
export type AccessWindowValues = z.infer<ReturnType<typeof accessWindowSchema>>;

export function addMemberSchema(t: Translate) {
  return z.object({
    user_uuid: z.string().min(1, t("organizations.validation_user_required")),
    role_key: z.enum(TENANT_ROLE_KEYS),
  });
}
export type AddMemberValues = z.infer<ReturnType<typeof addMemberSchema>>;
