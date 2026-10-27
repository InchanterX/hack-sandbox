import type { components } from '../api/schema';

export type ApiErrorBody = components['schemas']['ErrorEnvelope'];

export const NETWORK_ERROR_TEXT = 'Не удалось связаться с сервером';

/** Error carrying the HTTP status and the Russian message of the API envelope. */
export class ApiRequestError extends Error {
  readonly status: number;
  readonly code: string | null;

  constructor(status: number, message: string, code: string | null) {
    super(message);
    this.name = 'ApiRequestError';
    this.status = status;
    this.code = code;
  }
}

/** Converts an openapi-fetch result into data, throwing ApiRequestError for API errors. */
export function unwrap<T>(result: { data?: T; error?: unknown; response: Response }): T {
  if (result.error !== undefined || result.data === undefined) {
    const envelope = result.error as Partial<ApiErrorBody> | undefined;
    throw new ApiRequestError(
      result.response.status,
      envelope?.error?.message ?? NETWORK_ERROR_TEXT,
      envelope?.error?.code ?? null,
    );
  }
  return result.data;
}

/** Same as unwrap for 204 operations that have no body. */
export function ensureOk(result: { error?: unknown; response: Response }): void {
  if (result.error !== undefined || !result.response.ok) {
    const envelope = result.error as Partial<ApiErrorBody> | undefined;
    throw new ApiRequestError(
      result.response.status,
      envelope?.error?.message ?? NETWORK_ERROR_TEXT,
      envelope?.error?.code ?? null,
    );
  }
}

/** User-facing text of any thrown value; network failures get a fixed Russian message. */
export function errorText(error: unknown): string {
  return error instanceof ApiRequestError ? error.message : NETWORK_ERROR_TEXT;
}
