/**
 * The client's failure behaviour.
 *
 * These are the guide's Prompt 12 tests, and they are about one thing: a
 * dependency failure must never be indistinguishable from a healthy result. The
 * previous client caught every error, logged "using local mock state" and
 * returned [], so an outage rendered as "no incidents" -- a screen that told an
 * operator everything was fine while nothing was reachable.
 */

import { afterEach, describe, expect, it, vi } from 'vitest';
import { request, setTenant, isOk } from '../client';
import type { ApiResult } from '../client';

const realFetch = globalThis.fetch;

afterEach(() => {
  globalThis.fetch = realFetch;
  setTenant(null);
  vi.restoreAllMocks();
});

function mockFetch(impl: (url: string, init: RequestInit) => Promise<Response> | Response): void {
  globalThis.fetch = vi.fn(
    (input: RequestInfo | URL, init?: RequestInit) =>
      Promise.resolve(impl(String(input), init ?? {})),
  ) as unknown as typeof fetch;
}

function json(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

describe('a backend outage', () => {
  it('is unavailable, never an empty success', async () => {
    mockFetch(() => {
      throw new TypeError('Failed to fetch');
    });

    const r = await request<unknown[]>('/incidents');
    expect(r.state).toBe('unavailable');
    // The decisive assertion: there is no branch on which a failed request
    // yields data at all, so no screen can render one as content.
    expect(isOk(r)).toBe(false);
    expect('data' in r).toBe(false);
  });

  it('treats a 500 as unavailable rather than as an empty list', async () => {
    mockFetch(() => json(500, { error: 'internal_error' }));
    const r = await request<unknown[]>('/incidents');
    expect(r.state).toBe('unavailable');
  });

  it('treats a server that never answers as unavailable, not as still loading', async () => {
    // A request against a server that accepts the connection and then goes
    // silent must end. A screen stuck in its loading state reads as "still
    // working" rather than "broken", which is the same confusion as an outage
    // rendering as empty.
    mockFetch(
      (_url, init) =>
        new Promise((_resolve, reject) => {
          init.signal?.addEventListener('abort', () =>
            reject(Object.assign(new Error('aborted'), { name: 'AbortError' })),
          );
        }) as unknown as Response,
    );

    vi.useFakeTimers();
    const pending = request<unknown>('/health');
    await vi.advanceTimersByTimeAsync(20_000);
    const r = await pending;
    vi.useRealTimers();

    expect(r.state).toBe('unavailable');
    if (r.state === 'unavailable') {
      expect(r.error).toMatch(/in time/i);
    }
  });
});

describe('session and permission failures', () => {
  it('distinguishes an expired session from a permission denial', async () => {
    // They need different words and different actions. Re-authenticating fixes
    // one and changes nothing about the other, so collapsing them into a single
    // "error" sends an operator to the login screen for a permission they will
    // never hold.
    mockFetch(() => json(401, { error: 'unauthorized' }));
    const expired = await request<unknown>('/review-queue');
    expect(expired.state).toBe('unauthenticated');

    mockFetch(() => json(403, { error: 'forbidden' }));
    const denied = await request<unknown>('/review-queue');
    expect(denied.state).toBe('forbidden');
  });

  it('reports a stale decision as a conflict, not as a generic failure', async () => {
    mockFetch(() =>
      json(409, { error: 'decision_stale', detail: 'the validation findings changed since it was approved' }),
    );
    const r = await request<unknown>('/decisions/1/release', { method: 'POST', body: {} });
    expect(r.state).toBe('conflict');
    if (r.state === 'conflict') {
      // The server names what changed. "The approval expired" is not
      // actionable; "the findings changed since it was approved" is.
      expect(r.detail).toContain('findings changed');
    }
  });
});

describe('what every request carries', () => {
  it('sends credentials and the tenant selector, and a CSRF token only on mutations', async () => {
    // The double-submit token. The session cookie is HttpOnly and unreadable
    // here by design; this is the readable companion.
    Object.defineProperty(globalThis, 'document', {
      value: { cookie: 'sentinel_csrf=token-abc; other=x' },
      configurable: true,
    });

    const calls: Array<{ url: string; init: RequestInit }> = [];
    mockFetch((url, init) => {
      calls.push({ url, init });
      return json(200, {});
    });

    setTenant('TENANT-A');
    await request<unknown>('/artifacts');
    await request<unknown>('/decisions/1/approve', { method: 'POST', body: { reason: 'x' } });

    for (const c of calls) {
      expect(c.init.credentials).toBe('include');
      expect((c.init.headers as Record<string, string>)['X-Sentinel-Tenant']).toBe('TENANT-A');
    }
    const read = calls[0].init.headers as Record<string, string>;
    const write = calls[1].init.headers as Record<string, string>;
    expect(read['X-CSRF-Token']).toBeUndefined();
    expect(write['X-CSRF-Token']).toBe('token-abc');
  });

  it('never sends an actor; identity is the server\'s to determine', async () => {
    let sentBody = '';
    mockFetch((_url, init) => {
      sentBody = String(init.body ?? '');
      return json(200, {});
    });
    const { voteOnDecision } = await import('../endpoints');
    await voteOnDecision(7, true, 'checked the control totals');
    expect(sentBody).not.toContain('actor');
    expect(sentBody).toContain('checked the control totals');
  });
});

describe('successful but empty responses', () => {
  it('parses a 204 as success rather than as a parse failure', async () => {
    mockFetch(() => new Response(null, { status: 204 }));
    const r: ApiResult<unknown> = await request<unknown>('/connections/1', { method: 'DELETE' });
    expect(r.state).toBe('ok');
  });

  it('reports a non-JSON 200 as unavailable rather than crashing', async () => {
    // A proxy error page served with a 200 is a real deployment condition, and
    // it must not surface as a component-level exception.
    mockFetch(() => new Response('<html>gateway timeout</html>', { status: 200 }));
    const r = await request<unknown>('/artifacts');
    expect(r.state).toBe('unavailable');
  });
});
