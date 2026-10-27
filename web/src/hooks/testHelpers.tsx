import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render } from '@testing-library/react';
import { MemoryRouter, useLocation, useRoutes } from 'react-router';

import { AuthProvider } from '../auth/AuthProvider';
import { routes } from '../router';
import type { Task } from './useTasks';

export const alice = { id: '00000000-0000-4000-8000-000000000001', login: 'alice', created_at: '2026-10-27T12:00:00Z' };
export const bob = { id: '00000000-0000-4000-8000-000000000002', login: 'bob', created_at: '2026-10-27T12:00:00Z' };

export const sessionOf = (user: typeof alice, token = 't'.repeat(43)) => ({
  token,
  expires_at: '2026-10-28T12:00:00Z',
  user,
});

export function makeTask(overrides: Partial<Task> = {}): Task {
  return {
    id: '10000000-0000-4000-8000-000000000001',
    expression: '1 + 2',
    status: 'queued',
    result: null,
    error: null,
    created_at: '2026-10-27T12:00:00Z',
    started_at: null,
    finished_at: null,
    ...overrides,
  };
}

/** An openapi-fetch style response. */
export const apiResponse = (status: number, body?: unknown) => {
  const response = new Response(null, { status });
  return status >= 400
    ? { data: undefined, error: body, response }
    : { data: body, error: undefined, response };
};
export const apiError = (status: number, code: string, message: string) =>
  apiResponse(status, { error: { code, message } });

function Pathname() {
  return <div data-testid="pathname">{useLocation().pathname}</div>;
}
function Routes() {
  return (
    <>
      {useRoutes(routes)}
      <Pathname />
    </>
  );
}

/** Renders the whole app at a path with a fresh query client. */
export function renderApp(path: string, queryClient = new QueryClient()) {
  const view = render(
    <QueryClientProvider client={queryClient}>
      <AuthProvider>
        <MemoryRouter initialEntries={[path]}>
          <Routes />
        </MemoryRouter>
      </AuthProvider>
    </QueryClientProvider>,
  );
  return { queryClient, ...view };
}
