// Placeholder owned by stage S6, which replaces it with the full API demo
// scenario (SPEC.md#Verification). It fails on purpose: the scaffold must not
// report the demo as verified before the real scenario exists.
import assert from 'node:assert/strict';
import { test } from 'node:test';

test('API demo scenario (written by S6)', () => {
  assert.fail('not implemented: S6 writes the API demo e2e in tests/e2e/*.test.ts');
});
