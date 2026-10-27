// Shared paths and loaders for the contract checks. Every check is read-only:
// generation drift writes to temporary files only.
import { spawnSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { parse, parseAllDocuments } from 'yaml';

export const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../../..');
export const contractDir = path.join(root, 'tests/contract');

export function repoPath(...parts: string[]): string {
  return path.join(root, ...parts);
}

export function readText(rel: string): string {
  return readFileSync(repoPath(rel), 'utf8');
}

export function readYaml<T = unknown>(rel: string): T {
  return parse(readText(rel)) as T;
}

export function parseYamlStream(text: string): Record<string, unknown>[] {
  return parseAllDocuments(text)
    .map((doc) => doc.toJS() as Record<string, unknown> | null)
    .filter((doc): doc is Record<string, unknown> => doc !== null);
}

export interface RunResult {
  status: number | null;
  stdout: string;
  stderr: string;
}

export function run(cmd: string, args: string[], cwd: string): RunResult {
  const res = spawnSync(cmd, args, { cwd, encoding: 'utf8', maxBuffer: 64 * 1024 * 1024 });
  if (res.error) {
    throw new Error(`${cmd} ${args.join(' ')}: ${res.error.message}`);
  }
  return { status: res.status, stdout: res.stdout, stderr: res.stderr };
}

export function mustRun(cmd: string, args: string[], cwd: string): string {
  const res = run(cmd, args, cwd);
  if (res.status !== 0) {
    throw new Error(`${cmd} ${args.join(' ')} exited ${res.status}:\n${res.stdout}\n${res.stderr}`);
  }
  return res.stdout;
}

export type OpenAPIDoc = {
  openapi: string;
  paths: Record<string, Record<string, unknown>>;
  components: { schemas: Record<string, unknown> } & Record<string, unknown>;
  security?: unknown;
};

export function loadOpenAPI(): OpenAPIDoc {
  return readYaml<OpenAPIDoc>('contracts/openapi.yaml');
}

/** The ten operations of the API: method, path, operationId, public. */
export const operations = [
  { method: 'post', path: '/api/auth/register', id: 'register', public: true },
  { method: 'post', path: '/api/auth/login', id: 'login', public: true },
  { method: 'post', path: '/api/auth/logout', id: 'logout', public: false },
  { method: 'get', path: '/api/me', id: 'getMe', public: false },
  { method: 'post', path: '/api/tasks', id: 'createTask', public: false },
  { method: 'get', path: '/api/tasks', id: 'listTasks', public: false },
  { method: 'get', path: '/api/tasks/{id}', id: 'getTask', public: false },
  { method: 'patch', path: '/api/tasks/{id}', id: 'updateTask', public: false },
  { method: 'delete', path: '/api/tasks/{id}', id: 'deleteTask', public: false },
  { method: 'get', path: '/api/healthz', id: 'healthz', public: true },
] as const;
