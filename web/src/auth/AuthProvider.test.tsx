import { QueryClient } from '@tanstack/react-query';
import { QueryClientProvider } from '@tanstack/react-query';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { client, setAuthToken, setUnauthorizedHandler } from '../api/client';
import { apiError, apiResponse, alice, bob, makeTask, renderApp, sessionOf } from '../hooks/testHelpers';
import { tasksKey, useTasks } from '../hooks/useTasks';
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

function AccountTasks({ userId }: { userId: string }) {
  const tasks = useTasks(userId);
  return <div>{tasks.data?.map((task) => <p key={task.id}>{task.expression}</p>)}</div>;
}

function SessionProbe() {
  const { user, isLoading, signIn, signOut } = useAuth();
  return (
    <>
      <output data-testid="who">{isLoading ? 'loading' : user?.login ?? 'guest'}</output>
      <button onClick={() => void signIn(sessionOf(bob, 'bob-token'))}>switch</button>
      <button onClick={() => void signOut()}>logout</button>
      {user && <AccountTasks key={user.id} userId={user.id} />}
    </>
  );
}

function renderSession(queryClient = new QueryClient()) {
  render(
    <QueryClientProvider client={queryClient}>
      <AuthProvider><SessionProbe /></AuthProvider>
    </QueryClientProvider>,
  );
  return queryClient;
}

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

  it.each(['401', 'logout', 'account change'])(
    'awaits cancellation before removing cache on %s, ignoring a delayed old query', async (reason) => {
      saveToken('alice-token');
      let oldSignal: AbortSignal | undefined;
      let finishOld: (value: unknown) => void = () => undefined;
      let listCalls = 0;
      get.mockImplementation((path: string, options?: { signal: AbortSignal }) => {
        if (path === '/api/me') return Promise.resolve(apiResponse(200, alice));
        listCalls += 1;
        if (listCalls === 1) {
          oldSignal = options?.signal;
          // Deliberately ignore abort at the transport: QueryClient must discard this result.
          return new Promise((resolve) => { finishOld = resolve; });
        }
        return Promise.resolve(apiResponse(200, { items: [makeTask({ expression: 'bob task' })] }));
      });
      post.mockResolvedValue(apiResponse(204));
      const queryClient = new QueryClient();
      queryClient.setQueryData(tasksKey(alice.id), [makeTask({ expression: 'cached alice task' })]);
      const cancelQueries = queryClient.cancelQueries.bind(queryClient);
      let releaseCancellation: () => void = () => undefined;
      const cancellationSettled = new Promise<void>((resolve) => { releaseCancellation = resolve; });
      vi.spyOn(queryClient, 'cancelQueries').mockImplementation(async (...args) => {
        await cancelQueries(...args);
        await cancellationSettled;
      });
      const remove = vi.spyOn(queryClient, 'removeQueries');
      renderSession(queryClient);
      await screen.findByText('cached alice task');
      await waitFor(() => expect(oldSignal).toBeDefined());

      if (reason === '401') {
        act(() => handlerRegisteredLast()());
      } else {
        await userEvent.click(screen.getByText(reason === 'logout' ? 'logout' : 'switch'));
      }
      await waitFor(() => expect(oldSignal?.aborted).toBe(true));
      expect(remove).not.toHaveBeenCalled();
      expect(screen.getByTestId('who')).toHaveTextContent('loading');
      expect(screen.queryByText('cached alice task')).toBeNull();
      expect(listCalls).toBe(1);

      await act(async () => releaseCancellation());
      await waitFor(() => expect(screen.getByTestId('who')).toHaveTextContent(
        reason === 'account change' ? 'bob' : 'guest',
      ));
      expect(queryClient.getQueryData(tasksKey(alice.id))).toBeUndefined();
      await act(async () => finishOld(apiResponse(200, { items: [makeTask({ expression: 'late alice task' })] })));
      expect(screen.queryByText('late alice task')).toBeNull();
      expect(queryClient.getQueryData(tasksKey(alice.id))).toBeUndefined();
      if (reason === 'account change') {
        expect(await screen.findByText('bob task')).toBeInTheDocument();
        expect(queryClient.getQueryData(tasksKey(bob.id))).toEqual([makeTask({ expression: 'bob task' })]);
      }
    },
  );

  it('does not restore a delayed /api/me after signing in to a different account', async () => {
    saveToken('alice-token');
    let finishBootstrap: (value: unknown) => void = () => undefined;
    get.mockImplementation((path: string) => path === '/api/me'
      ? new Promise((resolve) => { finishBootstrap = resolve; })
      : Promise.resolve(apiResponse(200, { items: [] })));
    renderSession();
    await userEvent.click(screen.getByText('switch'));
    await waitFor(() => expect(screen.getByTestId('who')).toHaveTextContent('bob'));
    await act(async () => finishBootstrap(apiResponse(200, alice)));
    expect(screen.getByTestId('who')).toHaveTextContent('bob');
    expect(loadToken()).toBe('bob-token');
  });

  it('does not clear a new account when an old logout finishes late', async () => {
    saveToken('alice-token');
    get.mockImplementation((path: string) => Promise.resolve(path === '/api/me'
      ? apiResponse(200, alice) : apiResponse(200, { items: [] })));
    let finishLogout: (value: unknown) => void = () => undefined;
    post.mockImplementationOnce(() => new Promise((resolve) => { finishLogout = resolve; }));
    renderSession();
    await waitFor(() => expect(screen.getByTestId('who')).toHaveTextContent('alice'));
    await userEvent.click(screen.getByText('logout'));
    await userEvent.click(screen.getByText('switch'));
    await waitFor(() => expect(screen.getByTestId('who')).toHaveTextContent('bob'));
    await act(async () => finishLogout(apiResponse(204)));
    expect(screen.getByTestId('who')).toHaveTextContent('bob');
    expect(loadToken()).toBe('bob-token');
  });
});
