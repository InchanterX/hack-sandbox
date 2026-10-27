import { useContext } from 'react';

import { AuthContext, type AuthState } from './AuthContext';

/** Returns the session state provided by AuthProvider. */
export function useAuth(): AuthState {
  const state = useContext(AuthContext);
  if (!state) {
    throw new Error('useAuth must be used inside AuthProvider');
  }
  return state;
}
