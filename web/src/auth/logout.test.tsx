import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { Middleware } from 'openapi-fetch';
import { afterEach, describe, expect, it } from 'vitest';

import { client, setAuthToken } from '../api/client';
import { alice, sessionOf } from '../hooks/testHelpers';
import { AuthProvider } from './AuthProvider';
import { loadToken } from './session';
import { useAuth } from './useAuth';

const sent: { path: string; authorization: string | null }[] = [];

// Runs after the auth middleware, records the outgoing request and answers it without network.
const recorder: Middleware = {
  onRequest({ request }) {
    sent.push({ path: new URL(request.url).pathname, authorization: request.headers.get('Authorization') });
    return new Response(null, { status: 204 });
  },
};

afterEach(() => {
  client.eject(recorder);
  sent.length = 0;
  setAuthToken(null);
});

function Probe() {
  const { user, signIn, signOut } = useAuth();
  return (
    <>
      <output data-testid="who">{user?.login ?? 'guest'}</output>
      <button onClick={() => void signIn(sessionOf(alice, 'alice-token'))}>login</button>
      <button onClick={() => void signOut()}>logout</button>
    </>
  );
}

describe('signOut', () => {
  it('sends logout with the current bearer token and clears the session afterwards', async () => {
    client.use(recorder);
    render(
      <QueryClientProvider client={new QueryClient()}>
        <AuthProvider><Probe /></AuthProvider>
      </QueryClientProvider>,
    );
    await userEvent.click(screen.getByText('login'));
    await waitFor(() => expect(screen.getByTestId('who')).toHaveTextContent('alice'));
    expect(loadToken()).toBe('alice-token');

    await act(async () => userEvent.click(screen.getByText('logout')));

    expect(sent).toEqual([{ path: '/api/auth/logout', authorization: 'Bearer alice-token' }]);
    await waitFor(() => expect(screen.getByTestId('who')).toHaveTextContent('guest'));
    expect(loadToken()).toBeNull();
  });
});
