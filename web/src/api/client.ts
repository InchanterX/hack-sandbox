import createClient, { type Middleware } from 'openapi-fetch';

import type { paths } from './schema';

type UnauthorizedHandler = () => void;

let authToken: string | null = null;
/** Bumped on every token change; responses to requests of an older session are dropped. */
let sessionGeneration = 0;
const requestGeneration = new WeakMap<Request, number>();
const pendingRequests = new Map<Request, AbortController>();
let onUnauthorized: UnauthorizedHandler | null = null;

/** Sets the Bearer token sent with every request; null removes the header. */
export function setAuthToken(token: string | null): void {
  if (token !== authToken) {
    sessionGeneration += 1;
    for (const controller of pendingRequests.values()) {
      controller.abort();
    }
    pendingRequests.clear();
  }
  authToken = token;
}

/** Registers the shared handler of 401 responses to protected requests. */
export function setUnauthorizedHandler(handler: UnauthorizedHandler | null): void {
  onUnauthorized = handler;
}

const publicPaths = new Set(['/api/auth/login', '/api/auth/register']);

const authMiddleware: Middleware = {
  onRequest({ request }) {
    if (authToken) {
      request.headers.set('Authorization', `Bearer ${authToken}`);
    }
    const controller = new AbortController();
    const scopedRequest = new Request(request, {
      signal: AbortSignal.any([request.signal, controller.signal]),
    });
    pendingRequests.set(scopedRequest, controller);
    requestGeneration.set(scopedRequest, sessionGeneration);
    return scopedRequest;
  },
  onResponse({ request, response }) {
    pendingRequests.delete(request);
    if (requestGeneration.get(request) !== sessionGeneration) {
      throw new DOMException('the session changed while the request was in flight', 'AbortError');
    }
    if (response.status === 401 && !publicPaths.has(new URL(request.url).pathname)) {
      onUnauthorized?.();
    }
    return response;
  },
  onError({ request }) {
    pendingRequests.delete(request);
  },
};

/** The singleton typed API client; every network request goes through it. */
export const client = createClient<paths>({ baseUrl: window.location.origin });
client.use(authMiddleware);
