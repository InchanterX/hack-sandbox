import { afterEach, describe, expect, it, vi } from 'vitest';

import { client, setAuthToken, setUnauthorizedHandler } from '../api/client';

afterEach(() => {
  setAuthToken(null);
  setUnauthorizedHandler(null);
  vi.unstubAllGlobals();
});

describe('client', () => {
  it('sends the bearer token', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response('{"items":[]}', { status: 200 }));
    setAuthToken('abc');
    await client.GET('/api/tasks', { fetch: fetchMock });
    const request = fetchMock.mock.calls[0]?.[0] as Request;
    expect(request.headers.get('Authorization')).toBe('Bearer abc');
  });

  it('reports 401 of protected requests but not of login', async () => {
    const handler = vi.fn();
    setUnauthorizedHandler(handler);
    const unauthorized = () => new Response('{"error":{"code":"unauthorized","message":"x"}}', { status: 401 });
    await client.POST('/api/auth/login', {
      body: { login: 'alice', password: 'secret-pass' },
      fetch: () => Promise.resolve(unauthorized()),
    });
    expect(handler).not.toHaveBeenCalled();
    await client.GET('/api/me', { fetch: () => Promise.resolve(unauthorized()) });
    expect(handler).toHaveBeenCalledOnce();
  });

  it('drops a delayed response of a previous session', async () => {
    let finish: (response: Response) => void = () => undefined;
    let started: () => void = () => undefined;
    const requestSent = new Promise<void>((resolve) => (started = resolve));
    setAuthToken('old-account');
    const pending = client.GET('/api/tasks', {
      fetch: () => {
        started();
        return new Promise<Response>((resolve) => (finish = resolve));
      },
    });
    await requestSent;
    setAuthToken('new-account');
    finish(new Response('{"items":[{"id":"old"}]}', { status: 200 }));
    await expect(pending).rejects.toMatchObject({ name: 'AbortError' });
  });
});
