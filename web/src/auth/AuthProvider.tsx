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
  const userIdRef = useRef<string | null>(null);

  const dropUserData = useCallback(() => {
    void queryClient.cancelQueries();
    queryClient.removeQueries();
  }, [queryClient]);

  const clearSession = useCallback(() => {
    setAuthToken(null);
    saveToken(null);
    userIdRef.current = null;
    dropUserData();
    setUser(null);
  }, [dropUserData]);

  const signIn = useCallback(
    (session: AuthSession) => {
      if (userIdRef.current !== session.user.id) {
        dropUserData();
      }
      userIdRef.current = session.user.id;
      setAuthToken(session.token);
      saveToken(session.token);
      setUser(session.user);
      setIsLoading(false);
    },
    [dropUserData],
  );

  const signOut = useCallback(async () => {
    try {
      await client.POST('/api/auth/logout');
    } catch {
      // network failure: the local session is cleared anyway
    } finally {
      clearSession();
    }
  }, [clearSession]);

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
    setAuthToken(token);
    client
      .GET('/api/me')
      .then(({ data, response }) => {
        if (!active) {
          return;
        }
        if (data) {
          userIdRef.current = data.id;
          setUser(data);
        } else if (response.status === 401) {
          clearSession();
        }
      })
      .catch(() => undefined)
      .finally(() => {
        if (active) {
          setIsLoading(false);
        }
      });
    return () => {
      active = false;
    };
  }, [clearSession]);

  const value = useMemo<AuthState>(
    () => ({ user, isLoading, signIn, signOut }),
    [user, isLoading, signIn, signOut],
  );

  const provider = <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
  return fallbackClient ? (
    <QueryClientProvider client={fallbackClient}>{provider}</QueryClientProvider>
  ) : (
    provider
  );
}
