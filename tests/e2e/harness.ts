// Harness of the API demo e2e: builds the server, gives it an isolated
// Postgres schema on the compose db, starts/kills/restarts it and offers a
// small typed HTTP client plus bounded polling. Server output is kept and
// printed when a step fails.
import { execFile, spawn, type ChildProcess } from 'node:child_process';
import { access, constants, mkdtemp, rm } from 'node:fs/promises';
import { createServer } from 'node:net';
import { homedir, tmpdir } from 'node:os';
import { delimiter, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';

const run = promisify(execFile);

const serverDir = fileURLToPath(new URL('../../server/', import.meta.url));

/** Delay per binary operation of the e2e server (SPEC.md#Verification). */
export const OP_DELAY_MS = 50;

export interface ApiResponse<T> {
  status: number;
  body: T;
  text: string;
}

export class App {
  private binDir = '';
  private schema = '';
  private databaseUrl = '';
  private proc: ChildProcess | undefined;
  private exited: Promise<void> = Promise.resolve();
  private output: string[] = [];
  baseUrl = '';

  /** Builds the server and the schema helper, creates the schema. */
  async setup(): Promise<void> {
    const go = await findGo();
    this.binDir = await mkdtemp(join(tmpdir(), 'calc-e2e-'));
    await run(go, ['build', '-o', join(this.binDir, 'server'), './cmd/server'], { cwd: serverDir });
    await run(go, ['build', '-o', join(this.binDir, 'e2eschema'), './internal/testdb/e2eschema'], {
      cwd: serverDir,
    });
    const { stdout } = await run(join(this.binDir, 'e2eschema'), ['create']).catch((err: unknown) => {
      throw new Error(
        'cannot create the e2e schema: the compose db must be reachable through DATABASE_URL or ' +
          'DB_HOST/DB_NAME/DB_USER/POSTGRES_PASSWORD (run the e2e in compose service dev)',
        { cause: err },
      );
    });
    const created = JSON.parse(stdout) as { schema: string; database_url: string };
    this.schema = created.schema;
    this.databaseUrl = created.database_url;
  }

  /** Starts the server on a free port and waits until healthz answers 200. */
  async start(): Promise<void> {
    const port = await freePort();
    this.baseUrl = `http://127.0.0.1:${port}`;
    const proc = spawn(join(this.binDir, 'server'), [], {
      env: {
        ...process.env,
        DATABASE_URL: this.databaseUrl,
        PORT: String(port),
        WORKERS: '2',
        OP_DELAY_MS: String(OP_DELAY_MS),
        POLL_INTERVAL_MS: '50',
        SESSION_TTL_HOURS: '24',
      },
      stdio: ['ignore', 'pipe', 'pipe'],
    });
    this.proc = proc;
    const label = `[server ${proc.pid}]`;
    proc.stdout.on('data', (d: Buffer) => this.output.push(`${label} ${d.toString()}`));
    proc.stderr.on('data', (d: Buffer) => this.output.push(`${label} ${d.toString()}`));
    this.exited = new Promise((resolve) => proc.once('exit', () => resolve()));
    await waitFor('server healthz 200', 60_000, async () => {
      if (proc.exitCode !== null) throw new Error(`server exited with ${proc.exitCode}`);
      try {
        const res = await fetch(`${this.baseUrl}/api/healthz`);
        return res.status === 200;
      } catch {
        return false;
      }
    });
  }

  /** Stops the running server with sig and waits for the process to exit. */
  async stop(sig: NodeJS.Signals = 'SIGTERM'): Promise<void> {
    const proc = this.proc;
    if (!proc || proc.exitCode !== null || proc.signalCode !== null) return;
    proc.kill(sig);
    const timeout = setTimeout(() => proc.kill('SIGKILL'), 30_000);
    await this.exited;
    clearTimeout(timeout);
  }

  /** Stops the server and removes the schema and the binaries. */
  async teardown(): Promise<void> {
    await this.stop();
    if (this.schema) await run(join(this.binDir, 'e2eschema'), ['drop', this.schema]);
    if (this.binDir) await rm(this.binDir, { recursive: true, force: true });
  }

  /** Server output so far, for failure reports. */
  logs(): string {
    return this.output.join('');
  }

  async request<T>(method: string, path: string, opts: { token?: string; body?: unknown } = {}): Promise<ApiResponse<T>> {
    const headers: Record<string, string> = {};
    if (opts.token) headers.Authorization = `Bearer ${opts.token}`;
    if (opts.body !== undefined) headers['Content-Type'] = 'application/json';
    const res = await fetch(`${this.baseUrl}${path}`, {
      method,
      headers,
      body: opts.body === undefined ? undefined : JSON.stringify(opts.body),
    });
    const text = await res.text();
    return { status: res.status, body: (text ? JSON.parse(text) : undefined) as T, text };
  }
}

/** Polls cond every 50 ms until it returns true, failing after timeoutMs. */
export async function waitFor(what: string, timeoutMs: number, cond: () => Promise<boolean>): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    if (await cond()) return;
    if (Date.now() > deadline) throw new Error(`timed out after ${timeoutMs} ms waiting for ${what}`);
    await new Promise((r) => setTimeout(r, 50));
  }
}

/**
 * Finds the go tool. A login shell (sh -l) resets PATH and drops
 * /usr/local/go/bin of the golang image, so known install places are tried
 * after GO and PATH.
 */
async function findGo(): Promise<string> {
  const candidates = [
    process.env.GO,
    ...(process.env.PATH ?? '').split(delimiter).filter(Boolean).map((dir) => join(dir, 'go')),
    process.env.GOROOT && join(process.env.GOROOT, 'bin', 'go'),
    '/usr/local/go/bin/go',
    join(homedir(), 'sdk', 'go', 'bin', 'go'),
    join(homedir(), 'go', 'bin', 'go'),
  ];
  for (const c of candidates) {
    if (!c) continue;
    try {
      await access(c, constants.X_OK);
      return c;
    } catch {
      // try the next one
    }
  }
  throw new Error('go not found: set GO or PATH (the e2e builds the server; compose service dev has Go)');
}

function freePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const srv = createServer();
    srv.once('error', reject);
    srv.listen(0, '127.0.0.1', () => {
      const addr = srv.address();
      srv.close(() => (addr && typeof addr === 'object' ? resolve(addr.port) : reject(new Error('no port'))));
    });
  });
}
