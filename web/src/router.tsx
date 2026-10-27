import { Navigate, type RouteObject } from 'react-router';

import { ProtectedRoute } from './auth/ProtectedRoute';
import { Login } from './screens/Login/Login';
import { Register } from './screens/Register/Register';
import { Tasks } from './screens/Tasks/Tasks';

/** All routes of the SPA; unknown paths go through / (and guests on to /login). */
export const routes: RouteObject[] = [
  { path: '/login', element: <Login /> },
  { path: '/register', element: <Register /> },
  {
    path: '/',
    element: (
      <ProtectedRoute>
        <Tasks />
      </ProtectedRoute>
    ),
  },
  { path: '*', element: <Navigate to="/" replace /> },
];
