import type { Session } from '@supabase/supabase-js';
import { useQueryClient } from '@tanstack/react-query';
import { createContext, use, useEffect, useState, type ReactNode } from 'react';

import { startAuthAutoRefresh, supabase } from '@/lib/supabase';
import { useUIStore } from '@/stores/ui-store';

export interface SessionState {
  session: Session | null;
  /** False once the stored session has been restored (or found absent). */
  isRestoring: boolean;
  isSignedIn: boolean;
}

const SessionContext = createContext<SessionState>({
  session: null,
  isRestoring: true,
  isSignedIn: false,
});

/**
 * Restores the persisted Supabase session at launch and tracks it thereafter.
 *
 * Signing out clears the query cache and client state. Durable offline data is
 * drained and removed before Supabase receives the sign-out request; see
 * prepareForSignOut (docs/09-security-privacy.md).
 */
export function SessionProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<Session | null>(null);
  const [isRestoring, setIsRestoring] = useState(true);
  const queryClient = useQueryClient();

  useEffect(() => {
    let active = true;

    void supabase.auth.getSession().then(({ data }) => {
      if (!active) return;
      setSession(data.session);
      setIsRestoring(false);
    });

    const {
      data: { subscription },
    } = supabase.auth.onAuthStateChange((event, nextSession) => {
      setSession(nextSession);
      setIsRestoring(false);

      if (event === 'SIGNED_OUT') {
        queryClient.clear();
        useUIStore.getState().reset();
      }
    });

    const stopAutoRefresh = startAuthAutoRefresh();

    return () => {
      active = false;
      subscription.unsubscribe();
      stopAutoRefresh();
    };
  }, [queryClient]);

  return (
    <SessionContext value={{ session, isRestoring, isSignedIn: session !== null }}>
      {children}
    </SessionContext>
  );
}

/** Returns the current session state. */
export function useSession(): SessionState {
  return use(SessionContext);
}
