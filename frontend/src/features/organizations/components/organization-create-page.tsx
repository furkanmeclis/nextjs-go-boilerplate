"use client";

import { Building2, UserRound } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo } from "react";

import { ErrorState } from "@/components/common/error-state";
import { EntityForm, EntityPage } from "@/components/entity";
import {
  AppForm,
  AppInput,
  FormActions,
  FormLayout,
  FormSection,
  createFormSections,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useCreateOrganization } from "@/features/organizations/hooks/use-organizations";
import {
  createOrganizationSchema,
  type CreateOrganizationValues,
} from "@/features/organizations/lib/schemas";
import type { CreateOrganizationRequest } from "@/features/organizations/services/organizations.service";
import { UserAsyncPicker } from "@/features/users/components/user-async-picker";
import { useLocale } from "@/providers/locale-provider";

function toBody(values: CreateOrganizationValues): CreateOrganizationRequest {
  return {
    name: values.name.trim(),
    ...(values.slug.trim() ? { slug: values.slug.trim() } : {}),
    ...(values.owner_user_uuid
      ? { owner_user_uuid: values.owner_user_uuid }
      : {}),
  };
}

export function OrganizationCreatePage() {
  const { t } = useLocale();
  const router = useRouter();
  const create = useCreateOrganization();
  const schema = useMemo(() => createOrganizationSchema(t), [t]);
  const sections = useMemo(
    () =>
      createFormSections([
        {
          key: "profile",
          label: t("organizations.section_profile"),
          description: t("organizations.section_profile_desc"),
          icon: Building2,
        },
        {
          key: "owner",
          label: t("organizations.section_owner"),
          description: t("organizations.section_owner_desc"),
          icon: UserRound,
        },
      ]),
    [t],
  );

  const onSubmit = async (values: CreateOrganizationValues) => {
    const org = await create.mutateAsync(toBody(values));
    router.push(routes.platform.organizations.detail(org.uuid));
  };

  return (
    <EntityPage
      title={t("organizations.create_title")}
      description={t("organizations.create_description")}
      permission={permissions.organizations.write}
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
        { label: t("organizations.create_title") },
      ]}
    >
      <EntityForm>
        <AppForm
          id="organization-create-form"
          schema={schema}
          defaultValues={{ name: "", slug: "", owner_user_uuid: "" }}
          onSubmit={onSubmit}
        >
          {(form) => (
            <FormLayout navItems={sections.navItems}>
              <FormSection
                id={sections.id("profile")}
                title={t("organizations.section_profile")}
                description={t("organizations.section_profile_desc")}
                columns={2}
              >
                <AppInput
                  name="name"
                  label={t("organizations.name")}
                  startIcon={Building2}
                />
                <AppInput
                  name="slug"
                  label={t("organizations.slug")}
                  description={t("organizations.fields_slug_hint")}
                />
              </FormSection>
              <FormSection
                id={sections.id("owner")}
                title={t("organizations.section_owner")}
                description={t("organizations.section_owner_desc")}
              >
                <UserAsyncPicker
                  name="owner_user_uuid"
                  label={t("organizations.fields_owner")}
                  description={t("organizations.fields_owner_hint")}
                />
              </FormSection>
              <FormActions>
                <Button
                  type="button"
                  variant="outline"
                  onClick={() =>
                    router.push(routes.platform.organizations.root)
                  }
                  disabled={create.isPending || form.formState.isSubmitting}
                >
                  {t("form.cancel")}
                </Button>
                <Button
                  type="submit"
                  disabled={create.isPending || form.formState.isSubmitting}
                >
                  {t("organizations.actions_create")}
                </Button>
              </FormActions>
            </FormLayout>
          )}
        </AppForm>
      </EntityForm>
    </EntityPage>
  );
}
