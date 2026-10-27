// Strict TypeScript check of the contract tooling and the bidirectional parity
// of contracts/types.ts with the generated OpenAPI types (types/parity.ts).
import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import { contractDir, readText, run } from './helpers.ts';

describe('types.ts', () => {
  it('matches the generated OpenAPI types in both directions', () => {
    const res = run(process.execPath, ['node_modules/typescript/bin/tsc', '-p', 'tsconfig.json', '--noEmit'], contractDir);
    assert.equal(res.status, 0, `tsc failed:\n${res.stdout}${res.stderr}`);
  });

  it('exports exactly the reference types', () => {
    const exported = [...readText('contracts/types.ts').matchAll(/^export (?:type|interface) (\w+)/gm)].map((m) => m[1]);
    assert.deepEqual(exported.sort(), [
      'ApiError',
      'AuthSession',
      'ComputationError',
      'Credentials',
      'ErrorCode',
      'ErrorEnvelope',
      'ExpressionInput',
      'Health',
      'NoContent',
      'Task',
      'TaskList',
      'TaskStatus',
      'UTCDateTime',
      'UUID',
      'User',
    ]);
  });
});
