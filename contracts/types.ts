/** Reference wire types for contract tests and API e2e. OpenAPI is canonical.
 * T01 verifies these against OpenAPI and generates production Go/TS bindings.
 * UUID and UTCDateTime are strings on the wire; validators enforce formats.
 */
export type UUID = string;
export type UTCDateTime = string;
export type TaskStatus = 'queued' | 'running' | 'done' | 'failed';
export type ErrorCode =
  | 'unauthorized'
  | 'invalid_credentials'
  | 'login_taken'
  | 'validation_error'
  | 'syntax_error'
  | 'not_found'
  | 'not_queued'
  | 'task_running'
  | 'internal_error'
  | 'unavailable';

export interface Credentials {
  login: string;
  /** 8–72 UTF-8 bytes; validate with TextEncoder, not string.length. */
  password: string;
}
export interface User {
  id: UUID;
  login: string;
  created_at: UTCDateTime;
}
export interface AuthSession {
  token: string;
  expires_at: UTCDateTime;
  user: User;
}
export interface ExpressionInput {
  expression: string;
}
export type ComputationError =
  | { code: 'division_by_zero'; message: 'деление на ноль' }
  | { code: 'overflow'; message: 'переполнение' };
export interface Task {
  id: UUID;
  expression: string;
  status: TaskStatus;
  result: number | null;
  error: ComputationError | null;
  created_at: UTCDateTime;
  started_at: UTCDateTime | null;
  finished_at: UTCDateTime | null;
}
export interface TaskList {
  items: Task[];
}
export interface ApiError {
  code: ErrorCode;
  message: string;
}
export interface ErrorEnvelope {
  error: ApiError;
}
export interface Health {
  status: 'ok';
}
/** The two 204 operations have no JSON response, including no null body. */
export type NoContent = undefined;
