import createClient, { type Middleware } from 'openapi-fetch';

import type { paths } from './schema';

/** Scaffold client (T01); T06 owns this file and completes session handling. */
type UnauthorizedHandler = () => void;

let authToken: string | null = null;
let onUnauthorized: UnauthorizedHandler | null = null;

/** Sets the Bearer token sent with every request; null removes the header. */
export function setAuthToken(token: string | null): void {
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
    return request;
  },
  onResponse({ request, response }) {
    if (response.status === 401 && !publicPaths.has(new URL(request.url).pathname)) {
      onUnauthorized?.();
    }
    return response;
  },
};

/** The singleton typed API client; every network request goes through it. */
export const client = createClient<paths>({ baseUrl: window.location.origin });
client.use(authMiddleware);
