import { API_VERSION_PREFIX } from '@genxcare/contracts';

import { ApiError } from './api-error';
import { env } from './env';
import { supabase } from './supabase';

/**
 * Typed client for the Go API.
 *
 * Every call carries the Supabase access token. The API derives the user from
 * that token, so the client never sends a user ID, role, or permission
 * (docs/09-security-privacy.md).
 */

export interface RequestOptions {
  method?: 'GET' | 'POST' | 'PATCH' | 'DELETE';
  body?: unknown;
  signal?: AbortSignal;
  /**
   * Idempotency key for mutations that may be retried, e.g. replaying a queued
   * offline completion (docs/05-api-and-backend-spec.md).
   */
  idempotencyKey?: string;
  /**
   * Set false for the few endpoints that work without a session — reading an
   * invitation from its token, which someone must be able to do before they
   * have an account. Defaults to true.
   */
  authenticated?: boolean;
}

/** Resolves the current access token, refreshing it if Supabase deems it stale. */
async function accessToken(): Promise<string> {
  const { data, error } = await supabase.auth.getSession();
  if (error) throw ApiError.network(error);

  const token = data.session?.access_token;
  if (!token) {
    throw new ApiError(401, 'UNAUTHENTICATED', 'Sign in to continue.');
  }
  return token;
}

/**
 * Performs one authenticated request against `/v1`.
 *
 * @param path Path below the version prefix, e.g. `/me`.
 */
export async function apiRequest<TResponse>(
  path: string,
  options: RequestOptions = {},
): Promise<TResponse> {
  const { method = 'GET', body, signal, idempotencyKey, authenticated = true } = options;

  const headers: Record<string, string> = { Accept: 'application/json' };
  if (authenticated) {
    headers.Authorization = `Bearer ${await accessToken()}`;
  }
  if (body !== undefined) headers['Content-Type'] = 'application/json';
  if (idempotencyKey) headers['Idempotency-Key'] = idempotencyKey;

  let response: Response;
  try {
    response = await fetch(`${env.apiUrl}${API_VERSION_PREFIX}${path}`, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
      signal: signal ?? null,
    });
  } catch (cause) {
    // Rethrow deliberate cancellations rather than reporting them as offline.
    if (cause instanceof Error && cause.name === 'AbortError') throw cause;
    throw ApiError.network(cause);
  }

  if (response.status === 204) {
    return undefined as TResponse;
  }

  const payload = await readJson(response);

  if (!response.ok) {
    throw ApiError.fromResponse(response.status, payload);
  }
  return payload as TResponse;
}

/** Reads a JSON body, tolerating an empty or non-JSON response. */
async function readJson(response: Response): Promise<unknown> {
  const text = await response.text();
  if (text.length === 0) return null;

  try {
    return JSON.parse(text) as unknown;
  } catch {
    return null;
  }
}

/** Unauthenticated liveness check, used by the connection banner. */
export async function apiHealth(signal?: AbortSignal): Promise<boolean> {
  try {
    const response = await fetch(`${env.apiUrl}/healthz`, { signal: signal ?? null });
    return response.ok;
  } catch {
    return false;
  }
}
