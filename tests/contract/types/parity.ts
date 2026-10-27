// Compile-time parity between contracts/types.ts (reference) and the generated
// web/src/api/schema.d.ts (production). Each pair must be mutually assignable
// and structurally identical, including nullable and required properties.
import type * as Ref from '../../../contracts/types.ts';
import type { components, paths } from '../../../web/src/api/schema.d.ts';

type S = components['schemas'];

type Equal<A, B> = (<T>() => T extends A ? 1 : 2) extends <T>() => T extends B ? 1 : 2 ? true : false;
type Assert<T extends true> = T;

export type Parity = [
  Assert<Equal<Ref.Credentials, S['Credentials']>>,
  Assert<Equal<Ref.User, S['User']>>,
  Assert<Equal<Ref.AuthSession, S['AuthSession']>>,
  Assert<Equal<Ref.ExpressionInput, S['ExpressionInput']>>,
  Assert<Equal<Ref.TaskStatus, S['TaskStatus']>>,
  Assert<Equal<Ref.ComputationError, S['ComputationError']>>,
  Assert<Equal<Ref.Task, S['Task']>>,
  Assert<Equal<Ref.TaskList, S['TaskList']>>,
  Assert<Equal<Ref.ErrorCode, S['ErrorCode']>>,
  Assert<Equal<Ref.ApiError, S['ApiError']>>,
  Assert<Equal<Ref.ErrorEnvelope, S['ErrorEnvelope']>>,
  Assert<Equal<Ref.Health, S['Health']>>,
];

// The twelve reference schemas are exactly the generated component schemas.
type ReferenceNames =
  | 'Credentials'
  | 'User'
  | 'AuthSession'
  | 'ExpressionInput'
  | 'TaskStatus'
  | 'ComputationError'
  | 'Task'
  | 'TaskList'
  | 'ErrorCode'
  | 'ApiError'
  | 'ErrorEnvelope'
  | 'Health';
export type SchemaNames = Assert<Equal<keyof S, ReferenceNames>>;

// The two 204 operations declare no response content.
type NoContent<P extends keyof paths, M extends keyof paths[P]> = paths[P][M] extends {
  responses: { 204: { content?: never } };
}
  ? true
  : false;
export type Logout = Assert<NoContent<'/api/auth/logout', 'post'>>;
export type Delete = Assert<NoContent<'/api/tasks/{id}', 'delete'>>;
export type RefNoContent = Assert<Equal<Ref.NoContent, undefined>>;
