import { createContext } from 'react';

import type { components } from '../api/schema';

export type User = components['schemas']['User'];
export type AuthSession = components['schemas']['AuthSession'];

export interface AuthState {
  user: User | null;
  isLoading: boolean;
  signIn: (session: AuthSession) => Promise<void>;
  /** Calls the logout API; the local session is cleared even if the request fails. */
  signOut: () => Promise<void>;
}

export const AuthContext = createContext<AuthState | null>(null);
