/**
 * The event stream subscriber.
 *
 * The cursor is the whole design. The server's SSE ids are outbox row ids, so
 * "where I got to" is one number, and a reconnect that sends it back receives
 * exactly what it missed and nothing it already had. There is no de-duplication
 * set here and there must not be one: a seen-ids cache is a second source of
 * truth about what arrived, and the two drift.
 *
 * EventSource would handle the reconnect and the Last-Event-ID header by
 * itself, and it is not used, for one reason: it cannot send credentials or the
 * tenant selector on a cross-origin request, and it cannot read a non-2xx
 * response body, so an authorization failure would present as an endless
 * reconnect loop rather than as "you are not permitted to watch this". Fetch
 * plus a manual reader gives up the built-in reconnect and gains the ability to
 * tell the operator what is wrong.
 */

import { API_BASE_URL, currentTenant } from './client';
import type { StreamEvent, StreamHello } from './types';

export type StreamState =
  | { state: 'connecting' }
  | { state: 'open'; cursor: number; head: number }
  /** The stream is up and nothing has happened. Distinct from broken. */
  | { state: 'idle'; cursor: number }
  /** Retrying. `attempt` lets the UI say how long it has been trying. */
  | { state: 'reconnecting'; attempt: number; error: string }
  /** Terminal: retrying will not help. */
  | { state: 'denied'; error: string }
  /**
   * The requested cursor predates what the server retains, so events were
   * missed. The view must be reloaded rather than patched forward -- a stream
   * with an undetected hole is worse than one that admits it.
   */
  | { state: 'gap'; cursor: number };

export interface StreamHandlers {
  onEvent(event: StreamEvent): void;
  onState(state: StreamState): void;
}

/** Backoff bounds. Capped so a long outage does not become a long silence. */
const RECONNECT_MIN_MS = 1_000;
const RECONNECT_MAX_MS = 30_000;

export interface Subscription {
  close(): void;
  /** The last event id delivered, so a caller can persist and resume. */
  cursor(): number;
}

export function subscribe(handlers: StreamHandlers, from?: number): Subscription {
  let cursor = from ?? 0;
  let closed = false;
  let attempt = 0;
  let controller: AbortController | null = null;
  let timer: ReturnType<typeof setTimeout> | null = null;

  const backoff = (): number => {
    // Exponential with jitter. Without jitter every browser that lost the
    // connection to a restarting gateway returns at the same instant.
    const base = Math.min(RECONNECT_MIN_MS * 2 ** attempt, RECONNECT_MAX_MS);
    return base / 2 + Math.random() * (base / 2);
  };

  const run = async (): Promise<void> => {
    if (closed) return;
    controller = new AbortController();
    handlers.onState(attempt === 0 ? { state: 'connecting' } : { state: 'reconnecting', attempt, error: '' });

    const headers: Record<string, string> = { Accept: 'text/event-stream' };
    const tenant = currentTenant();
    if (tenant) headers['X-Sentinel-Tenant'] = tenant;
    // Sent as a header when we have one, exactly as a browser's own
    // EventSource would on reconnect.
    if (cursor > 0) headers['Last-Event-ID'] = String(cursor);

    let res: Response;
    try {
      res = await fetch(`${API_BASE_URL}/stream`, {
        headers,
        credentials: 'include',
        signal: controller.signal,
      });
    } catch (e) {
      if (closed) return;
      scheduleRetry(e instanceof Error ? e.message : 'The gateway is unreachable.');
      return;
    }

    // A refusal is terminal. Retrying a 403 forever produces a UI that looks
    // like it is trying rather than one that says what is wrong.
    if (res.status === 401 || res.status === 403) {
      handlers.onState({
        state: 'denied',
        error:
          res.status === 401
            ? 'Your session is no longer valid, so live updates have stopped.'
            : 'You do not hold the permission required to watch live updates.',
      });
      return;
    }
    if (!res.ok || !res.body) {
      scheduleRetry(`The gateway returned HTTP ${res.status}.`);
      return;
    }

    attempt = 0;
    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';

    try {
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });

        // Frames are separated by a blank line. Everything before the last
        // separator is complete; the remainder stays buffered, because a chunk
        // boundary lands in the middle of a frame often enough that treating
        // partial frames as complete corrupts one event in every long run.
        let sep: number;
        while ((sep = buffer.indexOf('\n\n')) !== -1) {
          const frame = buffer.slice(0, sep);
          buffer = buffer.slice(sep + 2);
          handleFrame(frame);
        }
      }
    } catch (e) {
      if (closed) return;
      scheduleRetry(e instanceof Error ? e.message : 'The event stream was interrupted.');
      return;
    }

    // A clean end of stream is the server's connection-lifetime cap, not an
    // error. Reconnect immediately with the cursor; nothing is missed.
    if (!closed) scheduleRetry('', 0);
  };

  const handleFrame = (frame: string): void => {
    let event = 'message';
    let id: number | null = null;
    const dataLines: string[] = [];

    for (const line of frame.split('\n')) {
      if (line.startsWith(':')) continue; // keepalive comment
      if (line.startsWith('event: ')) event = line.slice(7);
      else if (line.startsWith('id: ')) id = Number(line.slice(4));
      else if (line.startsWith('data: ')) dataLines.push(line.slice(6));
    }
    if (dataLines.length === 0) return;

    let payload: unknown;
    try {
      payload = JSON.parse(dataLines.join('\n'));
    } catch {
      return;
    }

    if (event === 'hello') {
      const hello = payload as StreamHello;
      cursor = hello.cursor;
      if (hello.gap) {
        handlers.onState({ state: 'gap', cursor: hello.cursor });
        return;
      }
      handlers.onState({ state: 'open', cursor: hello.cursor, head: hello.head });
      return;
    }
    if (event === 'reconnect' || event === 'degraded') {
      handlers.onState({ state: 'idle', cursor });
      return;
    }

    const ev = payload as StreamEvent;
    // The cursor advances from the SSE id, not from the payload, because the
    // id is what the server will compare against on the next reconnect.
    if (id !== null && Number.isFinite(id)) cursor = id;
    handlers.onEvent(ev);
    handlers.onState({ state: 'open', cursor, head: cursor });
  };

  const scheduleRetry = (error: string, delay?: number): void => {
    if (closed) return;
    const wait = delay ?? backoff();
    attempt += 1;
    if (error) handlers.onState({ state: 'reconnecting', attempt, error });
    timer = setTimeout(() => {
      void run();
    }, wait);
  };

  void run();

  return {
    close(): void {
      closed = true;
      if (timer) clearTimeout(timer);
      controller?.abort();
    },
    cursor: () => cursor,
  };
}
