export type ApiErrorDetail = {
  field?: string;
  message?: string;
  code?: string;
};

export type ApiErrorBody = {
  success: false;
  error: {
    code: string;
    message: string;
    details?: ApiErrorDetail[];
  };
  meta?: {
    request_id?: string;
  };
};

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly details: ApiErrorDetail[];
  readonly requestId?: string;
  readonly body?: ApiErrorBody;

  constructor(opts: {
    status: number;
    code: string;
    message: string;
    details?: ApiErrorDetail[];
    requestId?: string;
    body?: ApiErrorBody;
  }) {
    super(opts.message);
    this.name = "ApiError";
    this.status = opts.status;
    this.code = opts.code;
    this.details = opts.details ?? [];
    this.requestId = opts.requestId;
    this.body = opts.body;
  }

  get isUnauthorized() {
    return this.status === 401;
  }

  get isForbidden() {
    return this.status === 403;
  }

  get isNotFound() {
    return this.status === 404;
  }

  get isValidation() {
    return this.status === 422 || this.status === 400;
  }

  get isServer() {
    return this.status >= 500;
  }

  fieldErrors(): Record<string, string> {
    const map: Record<string, string> = {};
    for (const detail of this.details) {
      if (detail.field && detail.message) {
        map[detail.field] = detail.message;
      }
    }
    return map;
  }
}

export function parseApiError(status: number, data: unknown): ApiError {
  const body = data as Partial<ApiErrorBody> | null;
  const code = body?.error?.code ?? `HTTP_${status}`;
  const message = body?.error?.message ?? defaultMessage(status);
  return new ApiError({
    status,
    code,
    message,
    details: body?.error?.details ?? [],
    requestId: body?.meta?.request_id,
    body: body as ApiErrorBody | undefined,
  });
}

function defaultMessage(status: number) {
  switch (status) {
    case 401:
      return "Oturum gerekli";
    case 403:
      return "Yetkisiz";
    case 404:
      return "Bulunamadı";
    case 422:
      return "Doğrulama hatası";
    case 500:
      return "Sunucu hatası";
    default:
      return "İstek başarısız";
  }
}

export type ErrorHandler = (error: ApiError) => void;

let globalErrorHandler: ErrorHandler | null = null;

export function setGlobalApiErrorHandler(handler: ErrorHandler | null) {
  globalErrorHandler = handler;
}

export function emitApiError(error: ApiError) {
  globalErrorHandler?.(error);
}
