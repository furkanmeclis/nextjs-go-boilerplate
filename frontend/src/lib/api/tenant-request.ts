import { apiConfig } from "@/config/api";
import { unwrap } from "@/lib/api";
import { withStepUpRetry } from "@/features/step-up-engine/lib/step-up-interceptor";

/** Header the Go API reads on `/v1/tenant/*` to pick the organization. */
export const ORGANIZATION_HEADER = "X-Organization";

type TenantRequestOptions = {
  query?: Record<string, string | number | undefined>;
  body?: unknown;
  /** Skip the global error toast (the caller renders the error itself). */
  silent?: boolean;
};

/**
 * BFF fetch for `/v1/tenant/*` endpoints, scoped to one organization.
 *
 * The organization travels per request (`X-Organization: <slug>`), never in
 * the session, so two tabs on two organizations stay independent. The BFF
 * forwards the header only on tenant paths.
 */
export async function tenantRequest<T>(
  slug: string,
  method: string,
  path: string,
  options?: TenantRequestOptions,
): Promise<T> {
  return withStepUpRetry(async () => {
    const base = apiConfig.baseUrl.replace(/\/$/, "");
    const pathname = path.startsWith("/") ? path : `/${path}`;
    let url = `${base}${pathname}`;

    if (options?.query) {
      const params = new URLSearchParams();
      for (const [key, value] of Object.entries(options.query)) {
        if (value !== undefined && value !== "") {
          params.set(key, String(value));
        }
      }
      const qs = params.toString();
      if (qs) url += `?${qs}`;
    }

    const response = await fetch(url, {
      method,
      credentials: "include",
      headers: {
        Accept: "application/json",
        [ORGANIZATION_HEADER]: slug,
        ...(options?.body ? { "Content-Type": "application/json" } : {}),
      },
      body: options?.body ? JSON.stringify(options.body) : undefined,
    });

    const payload = await response.json().catch(() => undefined);
    return unwrap<T>({ data: payload, response }, { silent: options?.silent });
  });
}
