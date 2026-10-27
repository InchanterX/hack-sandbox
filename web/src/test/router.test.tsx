import { render, screen } from '@testing-library/react';
import { useEffect } from 'react';
import { MemoryRouter, useLocation, useRoutes } from 'react-router';
import { describe, expect, it } from 'vitest';

import { AuthProvider } from '../auth/AuthProvider';
import { useAuth } from '../auth/useAuth';
import { routes } from '../router';

function SignedIn({ children }: { children: React.ReactNode }) {
  const { user, signIn } = useAuth();
  useEffect(() => {
    signIn({
      token: 'a'.repeat(43),
      expires_at: '2026-10-28T12:00:00Z',
      user: { id: '00000000-0000-4000-8000-000000000001', login: 'alice', created_at: '2026-10-27T12:00:00Z' },
    });
  }, [signIn]);
  return user ? children : null;
}

// The declarative MemoryRouter renders the same route table as the browser
// data router; jsdom's AbortSignal is incompatible with the data router's Request.
function App() {
  const { pathname } = useLocation();
  return (
    <>
      {useRoutes(routes)}
      <output data-testid="pathname">{pathname}</output>
    </>
  );
}

function renderAt(path: string, signedIn = false) {
  const app = (
    <MemoryRouter initialEntries={[path]}>
      <App />
    </MemoryRouter>
  );
  render(<AuthProvider>{signedIn ? <SignedIn>{app}</SignedIn> : app}</AuthProvider>);
}

describe('router', () => {
  it('resolves /login', async () => {
    renderAt('/login');
    expect(await screen.findByRole('heading', { name: 'Вход' })).toBeInTheDocument();
  });

  it('resolves /register', async () => {
    renderAt('/register');
    expect(await screen.findByRole('heading', { name: 'Регистрация' })).toBeInTheDocument();
  });

  it('sends a guest from protected / to /login', async () => {
    renderAt('/');
    expect(await screen.findByRole('heading', { name: 'Вход' })).toBeInTheDocument();
    expect(screen.getByTestId('pathname')).toHaveTextContent('/login');
  });

  it('sends a guest from an unknown path through / to /login', async () => {
    renderAt('/no/such/page');
    expect(await screen.findByRole('heading', { name: 'Вход' })).toBeInTheDocument();
    expect(screen.getByTestId('pathname')).toHaveTextContent('/login');
  });

  it('shows the tasks screen on / for a signed-in user', async () => {
    renderAt('/', true);
    expect(await screen.findByRole('heading', { name: 'Calc Queue' })).toBeInTheDocument();
    expect(screen.getByTestId('pathname')).toHaveTextContent('/');
  });
});
