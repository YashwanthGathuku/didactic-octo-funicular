/**
 * The subscriber's reconnect behaviour.
 *
 * The guide's requirement is "SSE disconnect/replay without duplicates". The
 * server half is tested in Go against the real outbox; this is the client half:
 * that the cursor advances from the delivered ids, that a reconnect sends it
 * back, and that a refusal stops the loop instead of retrying forever.
 */

import { afterEach, describe, expect, it, vi } from 'vitest';
import { setTenant } from '../client';
import { subscribe } from '../stream';
import type { StreamEvent } from '../types';
import type { StreamState } from '../stream';

const realFetch = globalThis.fetch;

afterEach(() => {
  globalThis.fetch = realFetch;
  setTenant(null);
  vi.restoreAllMocks();
});

/** Builds a readable stream that emits the given chunks and then ends. */
function sseBody(chunks: string[]): ReadableStream<Uint8Array> {
  const encoder = new TextEncoder();
  return new ReadableStream({
    start(controller) {
      for (const c of chunks) controller.enqueue(encoder.encode(c));
      controller.close();
    },
  });
}

function frame(id: number | null, event: string, data: unknown): string {
  const idLine = id === null ? '' : `id: ${id}\n`;
  return `${idLine}event: ${event}\ndata: ${JSON.stringify(data)}\n\n`;
}

function sseResponse(chunks: string[]): Response {
  return new Response(sseBody(chunks), {
    status: 200,
    headers: { 'Content-Type': 'text/event-stream' },
  });
}

/** Waits until `check` is true or the attempts run out. */
async function until(check: () => boolean, attempts = 200): Promise<void> {
  for (let i = 0; i < attempts; i++) {
    if (check()) return;
    await new Promise((r) => setTimeout(r, 5));
  }
}

describe('reconnect with a cursor', () => {
  it('resumes from the last delivered id and delivers nothing twice', async () => {
    const requestedCursors: Array<string | null> = [];

    // A cursor-honouring double. This is the property under test on the server
    // side -- "strictly after the cursor" -- so a double that replayed the same
    // body regardless would prove nothing about the client except that it
    // received whatever it was handed.
    const log = [1, 2, 3, 4, 5].map((id) => ({
      id,
      eventType: 'ARTIFACT_VALIDATED',
      subjectType: 'artifact',
      subjectId: 10 + id,
      payload: {},
      createdAt: '',
    }));
    const head = 5;

    // The first connection ends after three events, standing in for the
    // server's connection-lifetime cap, a proxy, or a restart. From the
    // client's side those are the same event.
    let firstConnection = true;

    globalThis.fetch = vi.fn((_input: RequestInfo | URL, init?: RequestInit) => {
      const headers = (init?.headers ?? {}) as Record<string, string>;
      const raw = headers['Last-Event-ID'] ?? null;
      requestedCursors.push(raw);
      const after = raw === null ? 0 : Number(raw);

      let due = log.filter((e) => e.id > after);
      if (firstConnection) {
        due = due.slice(0, 3);
        firstConnection = false;
      }
      return Promise.resolve(
        sseResponse([
          frame(null, 'hello', {
            cursor: after, head, tenantId: 'T', replay: raw !== null, gap: false, serverNow: '',
          }),
          ...due.map((e) => frame(e.id, e.eventType, e)),
        ]),
      );
    }) as unknown as typeof fetch;

    const received: StreamEvent[] = [];
    const sub = subscribe({ onEvent: (e) => received.push(e), onState: () => {} });

    await until(() => received.length >= 5);
    // Let another reconnect cycle run. A client that re-delivered what it had
    // already seen would fail here rather than in the assertion above.
    await new Promise((r) => setTimeout(r, 1200));
    sub.close();

    expect(received.map((e) => e.id)).toEqual([1, 2, 3, 4, 5]);

    // The first connection asks for nothing; the second resumes from 3, the id
    // of the last event the first delivered.
    expect(requestedCursors[0]).toBeNull();
    expect(requestedCursors[1]).toBe('3');

    // No id twice. Nothing on either side keeps a set of what it has seen --
    // it falls out of the log's ordering and a strictly-greater-than cursor.
    expect(new Set(received.map((e) => e.id)).size).toBe(received.length);
  });

  it('reassembles a frame split across chunk boundaries', async () => {
    // A chunk boundary lands mid-frame often enough that treating a partial
    // frame as complete corrupts one event in every long run.
    const event = {
      id: 9, eventType: 'ARTIFACT_VALIDATED', subjectType: 'artifact',
      subjectId: 99, payload: { note: 'split' }, createdAt: '',
    };
    const whole = frame(9, 'ARTIFACT_VALIDATED', event);
    const cut = Math.floor(whole.length / 2);

    globalThis.fetch = vi.fn((_input: RequestInfo | URL, init?: RequestInit) => {
      const headers = (init?.headers ?? {}) as Record<string, string>;
      const after = Number(headers['Last-Event-ID'] ?? 0);
      const chunks = [
        frame(null, 'hello', { cursor: after, head: 9, tenantId: 'T', replay: after > 0, gap: false, serverNow: '' }),
      ];
      if (after < 9) chunks.push(whole.slice(0, cut), whole.slice(cut));
      return Promise.resolve(sseResponse(chunks));
    }) as unknown as typeof fetch;

    const received: StreamEvent[] = [];
    const sub = subscribe({ onEvent: (e) => received.push(e), onState: () => {} });
    await until(() => received.length >= 1);
    await new Promise((r) => setTimeout(r, 1200));
    sub.close();

    expect(received).toHaveLength(1);
    expect(received[0].id).toBe(9);
    expect((received[0].payload as { note: string }).note).toBe('split');
  });
});

describe('states the subscriber must surface', () => {
  it('stops on a permission denial instead of retrying forever', async () => {
    let calls = 0;
    globalThis.fetch = vi.fn(() => {
      calls += 1;
      return Promise.resolve(new Response(JSON.stringify({ error: 'forbidden' }), { status: 403 }));
    }) as unknown as typeof fetch;

    const states: StreamState[] = [];
    const sub = subscribe({ onEvent: () => {}, onState: (s) => states.push(s) });

    await until(() => states.some((s) => s.state === 'denied'));
    // Give a retry loop, if there were one, room to fire.
    await new Promise((r) => setTimeout(r, 60));
    sub.close();

    expect(states.some((s) => s.state === 'denied')).toBe(true);
    // A UI that retries a 403 forever looks like it is trying rather than
    // saying what is wrong.
    expect(calls).toBe(1);
  });

  it('reports a gap rather than pretending the view is continuous', async () => {
    globalThis.fetch = vi.fn(() =>
      Promise.resolve(
        sseResponse([
          frame(null, 'hello', { cursor: 500, head: 900, tenantId: 'T', replay: true, gap: true, serverNow: '' }),
        ]),
      ),
    ) as unknown as typeof fetch;

    const states: StreamState[] = [];
    const sub = subscribe({ onEvent: () => {}, onState: (s) => states.push(s) }, 5);
    await until(() => states.some((s) => s.state === 'gap'));
    sub.close();

    // The caller has to reload rather than patch forward: events between the
    // stale cursor and the head were never delivered, and a view built on the
    // ones that were would be missing transitions it can never learn about.
    expect(states.some((s) => s.state === 'gap')).toBe(true);
    expect(states.some((s) => s.state === 'open')).toBe(false);
  });

  it('retries a network failure and says it is reconnecting', async () => {
    let calls = 0;
    globalThis.fetch = vi.fn(() => {
      calls += 1;
      if (calls === 1) return Promise.reject(new TypeError('Failed to fetch'));
      return Promise.resolve(
        sseResponse([
          frame(null, 'hello', { cursor: 0, head: 0, tenantId: 'T', replay: false, gap: false, serverNow: '' }),
        ]),
      );
    }) as unknown as typeof fetch;

    const states: StreamState[] = [];
    const sub = subscribe({ onEvent: () => {}, onState: (s) => states.push(s) });

    await until(() => states.some((s) => s.state === 'open'), 600);
    sub.close();

    expect(states.some((s) => s.state === 'reconnecting')).toBe(true);
    expect(states.some((s) => s.state === 'open')).toBe(true);
  });
});
