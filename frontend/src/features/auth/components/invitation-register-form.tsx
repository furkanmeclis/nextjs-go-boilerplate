"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { signIn } from "next-auth/react";
import { useQuery } from "@tanstack/react-query";
import { Mail } from "lucide-react";
import { useMemo, useState } from "react";

import { AppForm, AppInput, AppPassword } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { Field, FieldError, FieldGroup } from "@/components/ui/field";
import { routes } from "@/config/routes";
import {
  createEmailCodeSchema,
  createRegisterSchema,
  type EmailCodeFormValues,
  type RegisterFormValues,
} from "@/features/auth/schemas";
import { invitationService } from "@/features/tenant/services/tenant.service";
import { isApiError } from "@/lib/api";
import {
  resolveCredentialErrorCode,
  type CredentialSignInResult,
} from "@/lib/auth/credentials-errors";
import { safeNextPath, withNextPath } from "@/lib/auth/next-path";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";
import { fetchAppPublicConfig } from "@/services/app-config.service";
import { authService } from "@/services/auth.service";

import { AuthCard } from "./auth-card";

/**
 * Sign-up for the address an organization invited. Works while public
 * registration is closed: the emailed token is the proof, and the Go API binds
 * it to that one address. Accepting happens back on the invitation page.
 */
export function InvitationRegisterForm({
  token,
  next,
}: {
  token: string;
  next: string | null;
}) {
  const { t, locale } = useLocale();
  const router = useRouter();
  const { markAuthenticated, hydrateProfile } = useAuth();
  const [formError, setFormError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);
  const [codeSent, setCodeSent] = useState(false);
  const schema = useMemo(() => createRegisterSchema(t), [t]);
  const codeSchema = useMemo(() => createEmailCodeSchema(t), [t]);

  const here = routes.invite(token);
  const target = safeNextPath(next, here);
  const loginHref = withNextPath(routes.guest.login, target);

  const preview = useQuery({
    queryKey: ["invitation", "preview", token],
    queryFn: () => invitationService.preview(token),
    retry: false,
    staleTime: 30_000,
    // The card explains failures; no global toast on top.
    meta: { silent: true },
  });
  const { data: appConfig } = useQuery({
    queryKey: ["app", "config", locale],
    queryFn: () => fetchAppPublicConfig(locale),
  });

  const finish = async () => {
    markAuthenticated();
    await hydrateProfile();
    router.replace(target);
  };

  if (preview.isLoading) {
    return (
      <div className="text-muted-foreground text-center text-sm">
        {t("common.loading")}
      </div>
    );
  }

  const invitation = preview.data;
  if (!invitation || invitation.status !== "pending") {
    return (
      <AuthCard
        title={t("auth.register.invited_unavailable_title")}
        description={t("auth.register.invited_unavailable_description")}
      >
        <Button asChild className="w-full">
          <Link href={here}>{t("auth.register.invited_back")}</Link>
        </Button>
      </AuthCard>
    );
  }
  const email = invitation.email;

  const errorBlock = formError ? (
    <Field data-invalid={true}>
      <FieldError>{formError}</FieldError>
    </Field>
  ) : null;

  const onSubmit = async (values: RegisterFormValues) => {
    setFormError(null);
    setPending(true);
    try {
      await authService.register({
        email,
        password: values.password,
        name: values.name,
        surname: values.surname,
        invitation_token: token,
      });
      const result = await signIn("credentials", {
        email,
        password: values.password,
        redirect: false,
      });
      if (result?.error) {
        router.replace(loginHref);
        return;
      }
      await finish();
    } catch (error) {
      if (isApiError(error) && error.code === "CONFLICT") {
        setFormError(t("auth.register.invited_exists"));
      } else {
        setFormError(
          isApiError(error) && error.message
            ? error.message
            : t("auth.register.error"),
        );
      }
      setPending(false);
    }
  };

  const onRequestCode = async () => {
    setFormError(null);
    setPending(true);
    try {
      await authService.requestEmailCode(email, token);
      setCodeSent(true);
    } catch (error) {
      setFormError(
        isApiError(error) && error.code === "RATE_LIMITED"
          ? t("auth.login.rate_limited")
          : t("auth.email_code.request_error"),
      );
    } finally {
      setPending(false);
    }
  };

  const onVerifyCode = async (values: EmailCodeFormValues) => {
    setFormError(null);
    setPending(true);
    try {
      const result = (await signIn("email-code", {
        email,
        code: values.code.trim(),
        invitation_token: token,
        totp_code: "",
        redirect: false,
      })) as CredentialSignInResult | undefined;
      const errorCode = result ? resolveCredentialErrorCode(result) : null;
      if (errorCode || !result?.ok) {
        // An account with 2FA belongs on the login page; the rest is a bad code.
        setFormError(
          errorCode === "MFA_REQUIRED"
            ? t("auth.register.invited_exists")
            : t("auth.email_code.invalid"),
        );
        setPending(false);
        return;
      }
      await finish();
    } catch {
      setFormError(t("auth.email_code.invalid"));
      setPending(false);
    }
  };

  const title = t("auth.register.invited_title", {
    name: invitation.organization.name,
  });

  const loginLink = (
    <p className="text-muted-foreground text-center text-sm">
      {t("auth.register.have_account")}{" "}
      <Link href={loginHref} className="underline-offset-4 hover:underline">
        {t("auth.register.login_link")}
      </Link>
    </p>
  );

  if (codeSent) {
    return (
      <AuthCard
        title={title}
        description={t("auth.register.invited_code_sent", { email })}
      >
        <div className="space-y-4">
          <AppForm
            key="invitation-code"
            schema={codeSchema}
            defaultValues={{ code: "" }}
            onSubmit={onVerifyCode}
          >
            <FieldGroup>
              {errorBlock}
              <AppInput
                name="code"
                label={t("auth.email_code.code")}
                autoComplete="one-time-code"
                inputMode="numeric"
                maxLength={6}
                placeholder={t("auth.email_code.code_placeholder")}
              />
              <Field>
                <Button type="submit" disabled={pending}>
                  {pending
                    ? t("auth.login.submitting")
                    : t("auth.register.submit")}
                </Button>
              </Field>
            </FieldGroup>
          </AppForm>
          {loginLink}
        </div>
      </AuthCard>
    );
  }

  // The account must be able to sign in with what it signs up with.
  const showPassword = appConfig?.auth_methods?.password.login !== false;

  return (
    <AuthCard
      title={title}
      description={t("auth.register.invited_description", { email })}
    >
      <div className="space-y-4">
        {showPassword ? (
          <AppForm
            key="invitation-password"
            schema={schema}
            defaultValues={{ email, name: "", surname: "", password: "" }}
            onSubmit={onSubmit}
          >
            <FieldGroup>
              {errorBlock}
              <AppInput
                name="name"
                label={t("auth.register.name")}
                autoComplete="given-name"
                placeholder={t("auth.placeholders.name")}
              />
              <AppInput
                name="surname"
                label={t("auth.register.surname")}
                autoComplete="family-name"
                placeholder={t("auth.placeholders.surname")}
              />
              <AppInput
                name="email"
                label={t("auth.register.email")}
                type="email"
                autoComplete="email"
                readOnly
              />
              <AppPassword
                name="password"
                label={t("auth.register.password")}
                autoComplete="new-password"
                placeholder={t("auth.placeholders.new_password")}
              />
              <Field>
                <Button type="submit" disabled={pending}>
                  {pending
                    ? t("auth.register.submitting")
                    : t("auth.register.submit")}
                </Button>
              </Field>
            </FieldGroup>
          </AppForm>
        ) : (
          errorBlock
        )}
        <Button
          type="button"
          variant="outline"
          className="w-full"
          disabled={pending}
          onClick={() => void onRequestCode()}
        >
          <Mail aria-hidden />
          {t("auth.register.invited_email_code")}
        </Button>
        {loginLink}
      </div>
    </AuthCard>
  );
}
