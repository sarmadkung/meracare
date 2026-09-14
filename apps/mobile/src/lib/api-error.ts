import { isApiErrorBody, type ApiErrorBody } from '@genxcare/contracts';

/**
 * A failed API call.
 *
 * `message` is always safe to show: it is either the server's user-facing
 * message or a generic fallback, never a raw transport error.
 */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly details: Record<string, string> | undefined;

  constructor(status: number, code: string, message: string, details?: Record<string, string>) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
    this.details = details;
  }

  /** True when the session is missing or expired and the user must sign in. */
  get isUnauthenticated(): boolean {
    return this.status === 401;
  }

  /** True when the caller is signed in but lacks permission for this senior. */
  get isForbidden(): boolean {
    return this.status === 403;
  }

  /** True for 5xx and network failures — the class of error worth retrying. */
  get isRetryable(): boolean {
    return this.status === 0 || this.status >= 500;
  }

  /**
   * True when the request never reached the server.
   *
   * Distinct from `isRetryable`: a 500 is worth retrying but the user is
   * online, whereas this is what sends a mutation to the offline queue.
   */
  get isOffline(): boolean {
    return this.status === 0;
  }

  /** Builds an ApiError from a non-2xx response body. */
  static fromResponse(status: number, body: unknown): ApiError {
    if (isApiErrorBody(body)) {
      const { error } = body as ApiErrorBody;
      return new ApiError(status, error.code, error.message, error.details);
    }
    return new ApiError(status, 'INTERNAL', 'Something went wrong. Please try again.');
  }

  /** Builds an ApiError for a request that never reached the server. */
  static network(cause: unknown): ApiError {
    const error = new ApiError(
      0,
      'NETWORK_UNAVAILABLE',
      'You appear to be offline. We will try again when you are back online.',
    );
    error.cause = cause;
    return error;
  }
}
