/**
 * The one place this application talks to the gateway.
 *
 * Every call goes through `request`. That is the point of the file: credentials,
 * the tenant selector, the CSRF token, the timeout, and the classification of
 * failure are attached in exactly one place, so there is no second call site
 * that forgets one of them. The previous client had four different failure
 * conventions in one module -- a result object for two endpoints, `null` for
 * two more, and a thrown Error for the rest -- and the `null` ones were the
 * reason a backend outage rendered as a healthy empty screen.
 *
 * There is no mock fallback, silent or otherwise. A failure is a state the
 * caller must branch on, and the type system makes not branching on it a
 * compile error.
 */

import type { Page } from './types';

export const API_BASE_URL: string =
  (import.meta as { env?: Record<string, string> }).env?.VITE_API_BASE_URL ??
  (typeof window !== 'undefined' && window.location.hostname !== 'localhost' && window.location.hostname !== '127.0.0.1'
    ? '/api/v1'
    : 'http://localhost:8080/api/v1');

/**
 * Every way a request can end.
 *
 * These are transport outcomes, not screen states: `loading` and `empty` belong
 * to the component, because only it knows whether an empty list is a normal
 * result or a filter with no matches. The rest are distinguished because they
 * call for different words on the screen and different actions from the
 * operator.
 */
export type ApiResult<T> =
  | { state: 'ok'; data: T }
  /** 401. The session is gone or expired; re-authentication is the remedy. */
  | { state: 'unauthenticated'; error: string }
  /** 403. Authenticated and not permitted. Re-authenticating changes nothing. */
  | { state: 'forbidden'; error: string }
  | { state: 'notFound'; error: string }
  /**
   * 409. Somebody else changed the thing between reading it and acting on it.
   * The remedy is to re-read and decide again, never to retry the same write.
   */
  | { state: 'conflict'; error: string; detail?: string }
  /** 400/422. This client sent something the server will never accept. */
  | { state: 'invalid'; error: string; detail?: string }
  /** Network failure, timeout, or 5xx. Retrying may work. */
  | { state: 'unavailable'; error: string };

export function isOk<T>(r: ApiResult<T>): r is { state: 'ok'; data: T } {
  return r.state === 'ok';
}

/**
 * How long a request may take before it is treated as unavailable.
 *
 * Without this a request against a server that accepts the connection and then
 * never answers hangs forever, and the screen sits in its loading state
 * indefinitely -- which reads to an operator as "still working" rather than
 * "broken", the same confusion as an outage rendering as empty.
 */
const REQUEST_TIMEOUT_MS = 15_000;

let selectedTenant: string | null = null;

/**
 * Names which tenant subsequent requests are for.
 *
 * Only meaningful for a principal with more than one membership; the server
 * refuses a tenant the caller does not belong to, so this is a selector and
 * never an assertion of access.
 */
export function setTenant(tenantId: string | null): void {
  selectedTenant = tenantId;
}

export function currentTenant(): string | null {
  return selectedTenant;
}

/**
 * Reads the double-submit CSRF token.
 *
 * The session cookie is HttpOnly and unreadable here, which is correct. The
 * CSRF cookie is deliberately readable so this function can echo it in the
 * header; it is not a credential on its own and is meaningless without the
 * session cookie the browser attaches for us.
 */
function csrfToken(): string | null {
  if (typeof document === 'undefined') return null;
  for (const part of document.cookie.split(';')) {
    const [name, ...rest] = part.trim().split('=');
    if (name === 'sentinel_csrf') return decodeURIComponent(rest.join('='));
  }
  return null;
}

interface RequestOptions {
  method?: string;
  body?: unknown;
  /** Absolute URL override; otherwise `path` is appended to API_BASE_URL. */
  signal?: AbortSignal;
}

export async function request<T>(
  path: string,
  opts: RequestOptions = {},
): Promise<ApiResult<T>> {
  const method = opts.method ?? 'GET';
  const headers: Record<string, string> = { Accept: 'application/json' };

  if (opts.body !== undefined) headers['Content-Type'] = 'application/json';
  if (selectedTenant) headers['X-Sentinel-Tenant'] = selectedTenant;

  // Only on mutations. A safe method is not CSRF-checked, and sending the
  // token on reads would put it in more logs for no benefit.
  if (method !== 'GET' && method !== 'HEAD') {
    const token = csrfToken();
    if (token) headers['X-CSRF-Token'] = token;
  }

  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), REQUEST_TIMEOUT_MS);
  // A caller-supplied signal still aborts: the component that unmounts should
  // cancel its own request.
  opts.signal?.addEventListener('abort', () => controller.abort());

  let res: Response;
  try {
    res = await fetch(`${API_BASE_URL}${path}`, {
      method,
      headers,
      // The session cookie has to be attached, and the gateway's CORS policy
      // names one origin rather than echoing the request's.
      credentials: 'include',
      body: opts.body === undefined ? undefined : JSON.stringify(opts.body),
      signal: controller.signal,
    });
  } catch (e) {
    clearTimeout(timer);
    if (controller.signal.aborted) {
      return { state: 'unavailable', error: 'The gateway did not answer in time.' };
    }
    return {
      state: 'unavailable',
      error: e instanceof Error ? e.message : 'The gateway is unreachable.',
    };
  }
  clearTimeout(timer);

  if (res.ok) {
    // 204 and an empty body are legitimate answers to a mutation. Parsing them
    // as JSON would turn a success into an unavailable.
    if (res.status === 204) return { state: 'ok', data: undefined as T };
    const text = await res.text();
    if (text.trim() === '') return { state: 'ok', data: undefined as T };
    try {
      return { state: 'ok', data: JSON.parse(text) as T };
    } catch {
      return {
        state: 'unavailable',
        error: 'The gateway returned a body that is not JSON.',
      };
    }
  }

  // The server's stable error code, when it sent one. Its wording is the
  // server's to choose; inventing a friendlier message here would hide which
  // control refused the request.
  let code = '';
  let detail = '';
  try {
    const body = (await res.json()) as { error?: string; detail?: string };
    code = body.error ?? '';
    detail = body.detail ?? '';
  } catch {
    /* an error response with no JSON body is still an error */
  }

  switch (res.status) {
    case 401:
      return {
        state: 'unauthenticated',
        error: code || 'Your session is no longer valid.',
      };
    case 403:
      return {
        state: 'forbidden',
        error: code || 'You do not hold the permission this action requires.',
      };
    case 404:
      return { state: 'notFound', error: code || 'Not found.' };
    case 409:
      return {
        state: 'conflict',
        error: code || 'This changed while you were looking at it.',
        detail,
      };
    case 400:
    case 422:
      return { state: 'invalid', error: code || 'The gateway refused this request.', detail };
    default:
      return {
        state: 'unavailable',
        error: code
          ? `${code} (HTTP ${res.status})`
          : `The gateway returned HTTP ${res.status}.`,
      };
  }
}

/** Builds a query string, omitting empty values so `?status=` never appears. */
export function query(params: Record<string, string | number | undefined | null>): string {
  const parts: string[] = [];
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined || v === null || v === '') continue;
    parts.push(`${encodeURIComponent(k)}=${encodeURIComponent(String(v))}`);
  }
  return parts.length ? `?${parts.join('&')}` : '';
}

/**
 * Fetches one page. Paging is the server's; this only carries the cursor.
 *
 * There is deliberately no `fetchAll` helper. One would exist to be called, and
 * the first caller in a hurry would use it on the evidence timeline.
 */
export async function getPage<T>(
  path: string,
  params: Record<string, string | number | undefined | null> = {},
  signal?: AbortSignal,
): Promise<ApiResult<Page<T>>> {
  return request<Page<T>>(`${path}${query(params)}`, { signal });
}
