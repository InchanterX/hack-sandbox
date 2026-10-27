import { QueryClient } from '@tanstack/react-query';
import { QueryClientProvider } from '@tanstack/react-query';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { client, setAuthToken, setUnauthorizedHandler } from '../api/client';
import { apiError, apiResponse, alice, bob, renderApp, sessionOf } from '../hooks/testHelpers';
import { AuthProvider } from './AuthProvider';
import { loadToken, saveToken } from './session';
import { useAuth } from './useAuth';

vi.mock('../api/client', () => ({
  client: { GET: vi.fn(), POST: vi.fn(), PATCH: vi.fn(), DELETE: vi.fn() },
  setAuthToken: vi.fn(),
  setUnauthorizedHandler: vi.fn(),
}));

const get = vi.mocked(client.GET) as unknown as ReturnType<typeof vi.fn>;
const post = vi.mocked(client.POST) as unknown as ReturnType<typeof vi.fn>;

function handlerRegisteredLast(): () => void {
  const calls = vi.mocked(setUnauthorizedHandler).mock.calls.filter(([h]) => h !== null);
  return calls[calls.length - 1]?.[0] as () => void;
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('session bootstrap', () => {
  it('shows loading, validates the stored token with /api/me and opens the tasks', async () => {
    saveToken('stored-token');
    let resolve: (value: unknown) => void = () => undefined;
    get.mockImplementation((path: string) =>
      path === '/api/me' ? new Promise((r) => (resolve = r)) : Promise.resolve(apiResponse(200, { items: [] })),
    );
    renderApp('/');
    expect(screen.getByRole('status')).toHaveTextContent('Загрузка');
    expect(setAuthToken).toHaveBeenCalledWith('stored-token');
    resolve(apiResponse(200, alice));
    expect(await screen.findByRole('heading', { name: 'Calc Queue' })).toBeInTheDocument();
    expect(screen.getByText('alice')).toBeInTheDocument();
  });

  it('drops a token rejected by /api/me and sends the guest to /login', async () => {
    saveToken('expired');
    get.mockResolvedValue(apiError(401, 'unauthorized', 'нужен вход'));
    renderApp('/');
    expect(await screen.findByRole('heading', { name: 'Вход' })).toBeInTheDocument();
    expect(loadToken()).toBeNull();
  });

  it('does not call /api/me without a stored token', async () => {
    renderApp('/');
    expect(await screen.findByRole('heading', { name: 'Вход' })).toBeInTheDocument();
    expect(get).not.toHaveBeenCalled();
  });
});

describe('session lifecycle', () => {
  function Probe() {
    const { user, signIn } = useAuth();
    return (
      <>
        <output data-testid="who">{user?.login ?? 'guest'}</output>
        <button onClick={() => signIn(sessionOf(bob, 'bob-token'))}>switch</button>
      </>
    );
  }

  it('clears the old user cache and the token on a protected 401', async () => {
    saveToken('tok');
    get.mockImplementation((path: string) =>
      Promise.resolve(path === '/api/me' ? apiResponse(200, alice) : apiResponse(200, { items: [] })),
    );
    const queryClient = new QueryClient();
    renderApp('/', queryClient);
    await screen.findByRole('heading', { name: 'Calc Queue' });
    queryClient.setQueryData(['tasks', alice.id], [{ id: 'x' }]);
    act(() => handlerRegisteredLast()());
    expect(await screen.findByRole('heading', { name: 'Вход' })).toBeInTheDocument();
    expect(queryClient.getQueryData(['tasks', alice.id])).toBeUndefined();
    expect(loadToken()).toBeNull();
    expect(setAuthToken).toHaveBeenLastCalledWith(null);
  });

  it('logout calls the API and clears the session even on a network error', async () => {
    saveToken('tok');
    get.mockImplementation((path: string) =>
      Promise.resolve(path === '/api/me' ? apiResponse(200, alice) : apiResponse(200, { items: [] })),
    );
    post.mockRejectedValue(new TypeError('offline'));
    renderApp('/');
    await userEvent.click(await screen.findByRole('button', { name: 'Выйти' }));
    expect(post).toHaveBeenCalledWith('/api/auth/logout');
    expect(await screen.findByRole('heading', { name: 'Вход' })).toBeInTheDocument();
    expect(loadToken()).toBeNull();
  });

  it('removes the previous account data and persists the new token when the account changes', async () => {
    saveToken('tok');
    get.mockResolvedValue(apiResponse(200, alice));
    const queryClient = new QueryClient();
    render(
      <QueryClientProvider client={queryClient}>
        <AuthProvider>
          <Probe />
        </AuthProvider>
      </QueryClientProvider>,
    );
    await waitFor(() => expect(screen.getByTestId('who')).toHaveTextContent('alice'));
    queryClient.setQueryData(['tasks', alice.id], [{ id: 'alice-task' }]);
    await userEvent.click(screen.getByText('switch'));
    expect(screen.getByTestId('who')).toHaveTextContent('bob');
    expect(queryClient.getQueryData(['tasks', alice.id])).toBeUndefined();
    expect(loadToken()).toBe('bob-token');
  });
});
