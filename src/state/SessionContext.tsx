/**
 * Who is signed in, what they may do, and whether this is a demo build.
 *
 * Loaded once from GET /session and shared, so no screen re-derives a
 * permission from a role name. Roles map to permissions on the server and that
 * mapping is the server's; a component that read `roles.includes('reviewer')`
 * would be a second copy of it, and the two would eventually disagree about
 * something that decides whether money moves.
 */

import React, { createContext, useCallback, useContext, useEffect, useState } from 'react';
import { getSession } from '../api/endpoints';
import type { ApiResult } from '../api/client';
import type { Permission, Session } from '../api/types';

interface SessionContextValue {
  result: ApiResult<Session> | null;
  session: Session | null;
  /** True only when the server said so. Unknown is not permitted. */
  can(permission: Permission): boolean;
  reload(): void;
}

const Ctx = createContext<SessionContextValue | null>(null);

export const SessionProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [result, setResult] = useState<ApiResult<Session> | null>(null);

  const load = useCallback(() => {
    setResult(null);
    void getSession().then(setResult);
  }, []);

  useEffect(load, [load]);

  const session = result?.state === 'ok' ? result.data : null;

  const can = useCallback(
    (permission: Permission): boolean => {
      // No session means no permission. Defaulting to "allowed while we find
      // out" would render every control enabled during the load, and an
      // operator who clicked one would get a refusal from the server -- which
      // reads as a broken product rather than as a control working.
      if (!session) return false;
      return session.permissions.includes(permission);
    },
    [session],
  );

  return <Ctx.Provider value={{ result, session, can, reload: load }}>{children}</Ctx.Provider>;
};

export function useSession(): SessionContextValue {
  const v = useContext(Ctx);
  if (!v) throw new Error('useSession must be used inside a SessionProvider');
  return v;
}
