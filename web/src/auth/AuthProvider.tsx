import { useCallback, useMemo, useState, type ReactNode } from 'react';

import { setAuthToken } from '../api/client';
import { AuthContext, type AuthSession, type AuthState, type User } from './AuthContext';

/**
 * Scaffold provider (T01): keeps the session in memory only. T06 persists the
 * token in localStorage, validates it with GET /api/me and clears user caches.
 */
export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);

  const signIn = useCallback((session: AuthSession) => {
    setAuthToken(session.token);
    setUser(session.user);
  }, []);
  const signOut = useCallback(() => {
    setAuthToken(null);
    setUser(null);
  }, []);

  const value = useMemo<AuthState>(
    () => ({ user, isLoading: false, signIn, signOut }),
    [user, signIn, signOut],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}
