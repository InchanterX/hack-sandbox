import { QueryClient, QueryClientContext, QueryClientProvider } from '@tanstack/react-query';
import { useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';

import { client, setAuthToken, setUnauthorizedHandler } from '../api/client';
import { AuthContext, type AuthSession, type AuthState, type User } from './AuthContext';
import { loadToken, saveToken } from './session';

/** Owns the session: persisted token, /api/me bootstrap, cache and request cleanup. */
export function AuthProvider({ children }: { children: ReactNode }) {
  const outerClient = useContext(QueryClientContext);
  const [fallbackClient] = useState(() => (outerClient ? null : new QueryClient()));
  const queryClient = outerClient ?? fallbackClient;
  if (!queryClient) {
    throw new Error('missing query client');
  }

  const [user, setUser] = useState<User | null>(null);
  const [isLoading, setIsLoading] = useState(() => loadToken() !== null);
  const generation = useRef(0);
  const cleanup = useRef<Promise<void>>(Promise.resolve());

  const replaceSession = useCallback(async (session: AuthSession | null) => {
    const current = ++generation.current;
    // Hide private UI and reject old HTTP responses before starting asynchronous cleanup.
    setAuthToken(null);
    saveToken(null);
    setUser(null);
    setIsLoading(true);
    const canceled = queryClient.cancelQueries();
    // An overlapping transition must also wait for earlier cancellation to settle.
    cleanup.current = Promise.all([cleanup.current, canceled]).then(() => undefined);
    await cleanup.current;
    if (current !== generation.current) {
      return;
    }
    queryClient.removeQueries();
    queryClient.getMutationCache().clear();
    if (session) {
      setAuthToken(session.token);
      saveToken(session.token);
      setUser(session.user);
    }
    setIsLoading(false);
  }, [queryClient]);

  const clearSession = useCallback(() => {
    void replaceSession(null);
  }, [replaceSession]);

  const signOut = useCallback(async () => {
    const current = generation.current;
    try {
      await client.POST('/api/auth/logout');
    } catch {
      // network failure: the local session is cleared anyway
    } finally {
      // A delayed logout response must not sign out a newer account.
      if (current === generation.current) {
        await replaceSession(null);
      }
    }
  }, [replaceSession]);

  useEffect(() => {
    setUnauthorizedHandler(clearSession);
    return () => setUnauthorizedHandler(null);
  }, [clearSession]);

  useEffect(() => {
    const token = loadToken();
    if (!token) {
      return;
    }
    let active = true;
    const current = generation.current;
    const controller = new AbortController();
    setAuthToken(token);
    client
      .GET('/api/me', { signal: controller.signal })
      .then(({ data, response }) => {
        if (!active || current !== generation.current) {
          return;
        }
        if (data) {
          setUser(data);
        } else if (response.status === 401) {
          clearSession();
        }
      })
      .catch(() => undefined)
      .finally(() => {
        if (active && current === generation.current) {
          setIsLoading(false);
        }
      });
    return () => {
      active = false;
      controller.abort();
    };
  }, [clearSession]);

  const value = useMemo<AuthState>(
    () => ({ user, isLoading, signIn: replaceSession, signOut }),
    [user, isLoading, replaceSession, signOut],
  );

  const provider = <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
  return fallbackClient ? (
    <QueryClientProvider client={fallbackClient}>{provider}</QueryClientProvider>
  ) : (
    provider
  );
}
