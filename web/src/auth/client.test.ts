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

  it('aborts in-flight HTTP requests when the account changes', async () => {
    let signal: AbortSignal | undefined;
    setAuthToken('old-account');
    const pending = client.GET('/api/tasks', {
      fetch: (request) => new Promise<Response>((_resolve, reject) => {
        signal = (request as Request).signal;
        signal.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')));
      }),
    });
    // Attach rejection handling before causing the abort.
    const rejected = expect(pending).rejects.toMatchObject({ name: 'AbortError' });
    await vi.waitFor(() => expect(signal).toBeDefined());
    setAuthToken('new-account');
    expect(signal?.aborted).toBe(true);
    await rejected;
  });

  it('forwards query cancellation to the HTTP request', async () => {
    const controller = new AbortController();
    let signal: AbortSignal | undefined;
    const pending = client.GET('/api/tasks', {
      signal: controller.signal,
      fetch: (request) => new Promise<Response>((_resolve, reject) => {
        signal = (request as Request).signal;
        signal.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')));
      }),
    });
    const rejected = expect(pending).rejects.toMatchObject({ name: 'AbortError' });
    await vi.waitFor(() => expect(signal).toBeDefined());
    controller.abort();
    expect(signal?.aborted).toBe(true);
    await rejected;
  });

  it('does not log out a new account on a delayed old protected 401', async () => {
    const handler = vi.fn();
    setUnauthorizedHandler(handler);
    setAuthToken('old-account');
    let finish: ((response: Response) => void) | undefined;
    const pending = client.GET('/api/tasks', {
      fetch: () => new Promise<Response>((resolve) => { finish = resolve; }),
    });
    await vi.waitFor(() => expect(finish).toBeTypeOf('function'));
    setAuthToken('new-account');
    finish?.(new Response('{"error":{"code":"unauthorized","message":"нужен вход"}}', { status: 401 }));
    await expect(pending).rejects.toMatchObject({ name: 'AbortError' });
    expect(handler).not.toHaveBeenCalled();
  });
});
