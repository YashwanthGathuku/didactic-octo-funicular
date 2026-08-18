/**
 * One paged, filtered, server-backed list.
 *
 * Shared because every screen needs the same five things and the one each would
 * get wrong on its own is the same one: a request whose response arrives after
 * the filter changed must not be applied. That is why the effect carries a
 * generation number as well as an AbortSignal -- aborting is best-effort and a
 * response already in flight can still resolve.
 *
 * Pages accumulate rather than replace, so "load more" appends. Changing a
 * filter resets, because a filtered list that kept the previous filter's rows
 * would be showing an answer to a question nobody asked.
 */

import { useCallback, useEffect, useRef, useState } from 'react';
import type { ApiResult } from '../api/client';
import type { Page } from '../api/types';

export interface PagedList<T> {
  items: T[];
  /** The most recent result, for rendering loading and failure states. */
  result: ApiResult<Page<T>> | null;
  loading: boolean;
  loadingMore: boolean;
  hasMore: boolean;
  partial: string | null;
  loadMore(): void;
  reload(): void;
}

export function usePagedList<T>(
  fetchPage: (cursor: string | undefined, signal: AbortSignal) => Promise<ApiResult<Page<T>>>,
  /**
   * Values that invalidate the list when they change -- the filters. Passed as
   * a string the caller builds, so the dependency is stable and a caller cannot
   * accidentally pass a new object every render and loop forever.
   */
  filterKey: string,
): PagedList<T> {
  const [items, setItems] = useState<T[]>([]);
  const [result, setResult] = useState<ApiResult<Page<T>> | null>(null);
  const [cursor, setCursor] = useState<string | undefined>(undefined);
  const [hasMore, setHasMore] = useState(false);
  const [partial, setPartial] = useState<string | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);
  const [nonce, setNonce] = useState(0);

  const generation = useRef(0);

  // Reset when the filter changes. The nonce lets reload() force this without
  // the caller having to perturb its filter.
  useEffect(() => {
    setItems([]);
    setResult(null);
    setCursor(undefined);
    setHasMore(false);
    setPartial(null);
  }, [filterKey, nonce]);

  useEffect(() => {
    const mine = ++generation.current;
    const controller = new AbortController();

    void fetchPage(cursor, controller.signal).then((r) => {
      // A response from a superseded request is discarded rather than
      // rendered. Without this a slow first page can overwrite a fast second
      // one and the list shows rows the current filter excludes.
      if (mine !== generation.current) return;

      setResult(r);
      setLoadingMore(false);
      if (r.state !== 'ok') return;

      // Defensive, for the same reason ContractsScreen normalises its list: a
      // server that regresses to `null` for an empty page must produce an
      // empty list, never a crash that unmounts the console.
      const page = Array.isArray(r.data.items) ? r.data.items : [];
      setItems((prev) => (cursor === undefined ? page : [...prev, ...page]));
      setHasMore(r.data.hasMore);
      setPartial(r.data.partial ? (r.data.partialReason ?? 'some rows could not be read') : null);
    });

    return () => controller.abort();
    // fetchPage is intentionally not a dependency: callers build it inline, so
    // depending on it would refetch every render. filterKey is the declared
    // input, which is what makes that safe.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filterKey, cursor, nonce]);

  const loadMore = useCallback(() => {
    if (!hasMore || loadingMore) return;
    if (result?.state !== 'ok' || !result.data.nextCursor) return;
    setLoadingMore(true);
    setCursor(result.data.nextCursor);
  }, [hasMore, loadingMore, result]);

  const reload = useCallback(() => {
    setNonce((n) => n + 1);
    setCursor(undefined);
  }, []);

  return {
    items,
    result,
    loading: result === null,
    loadingMore,
    hasMore,
    partial,
    loadMore,
    reload,
  };
}
