import type { ReactNode } from 'react';
import { Navigate } from 'react-router';

import { useAuth } from './useAuth';

/** Renders children for a signed-in user and sends guests to /login. */
export function ProtectedRoute({ children }: { children: ReactNode }) {
  const { user, isLoading } = useAuth();
  if (isLoading) {
    return <p role="status">Загрузка…</p>;
  }
  if (!user) {
    return <Navigate to="/login" replace />;
  }
  return children;
}
