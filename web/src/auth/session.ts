const TOKEN_KEY = 'calc-queue.token';

/** Reads the persisted session token, or null for a guest. */
export function loadToken(): string | null {
  try {
    return localStorage.getItem(TOKEN_KEY);
  } catch {
    return null;
  }
}

/** Persists the session token; null removes it. */
export function saveToken(token: string | null): void {
  try {
    if (token === null) {
      localStorage.removeItem(TOKEN_KEY);
    } else {
      localStorage.setItem(TOKEN_KEY, token);
    }
  } catch {
    // storage may be unavailable (private mode); the session then lives in memory only
  }
}
