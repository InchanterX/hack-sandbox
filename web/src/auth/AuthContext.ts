import { createContext } from 'react';

import type { components } from '../api/schema';

export type User = components['schemas']['User'];
export type AuthSession = components['schemas']['AuthSession'];

export interface AuthState {
  user: User | null;
  isLoading: boolean;
  signIn: (session: AuthSession) => void;
  signOut: () => void;
}

export const AuthContext = createContext<AuthState | null>(null);
