// examples.json: deterministic calculator cases consistent with the wire schemas.
import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import { loadOpenAPI, readText } from './helpers.ts';
import { compileValidator } from './schema.ts';

type Examples = {
  success: { expression: string; result: number }[];
  syntax_error: string[];
  validation_error: string[];
  failed: { expression: string; code: string; message: string }[];
  generated_cases: string[];
  calculator_only_overflow: { expression: string; code: string; message: string; reason: string }[];
};

const examples = JSON.parse(readText('contracts/examples.json')) as Examples;
const check = compileValidator(loadOpenAPI());
const input = '/components/schemas/ExpressionInput';
const allowed = /^[0-9.+\-*/() ]*$/;

function task(expression: string, extra: Record<string, unknown>) {
  return {
    id: '00000000-0000-4000-8000-000000000002',
    expression,
    created_at: '2026-01-01T00:00:00Z',
    started_at: '2026-01-01T00:00:01Z',
    finished_at: '2026-01-01T00:00:02Z',
    ...extra,
  };
}

describe('examples.json', () => {
  it('has exactly the documented groups', () => {
    assert.deepEqual(Object.keys(examples).sort(), [
      'calculator_only_overflow',
      'failed',
      'generated_cases',
      'success',
      'syntax_error',
      'validation_error',
    ]);
  });

  it('contains the demo results 11, 2 and 93', () => {
    const results = new Map(examples.success.map((e) => [e.expression, e.result]));
    assert.equal(results.get('2 + 3 * (4 - 1)'), 11);
    assert.equal(results.get('(10 - 4) / 3'), 2);
    assert.equal(results.get('100 - 7'), 93);
  });

  it('success cases are valid inputs with finite results rounded to 10 digits', () => {
    for (const e of examples.success) {
      assert.equal(check(input, { expression: e.expression }), null, e.expression);
      assert.match(e.expression, allowed);
      assert.ok(Number.isFinite(e.result), e.expression);
      assert.equal(Math.round(e.result * 1e10) / 1e10, e.result, `${e.expression} is not rounded`);
      assert.ok(!Object.is(e.result, -0), `${e.expression} must normalise -0`);
      const done = task(e.expression, { status: 'done', result: e.result, error: null });
      assert.equal(check('/components/schemas/Task', done), null, e.expression);
    }
  });

  it('syntax errors have a valid length, so they fail only in the grammar', () => {
    assert.ok(examples.syntax_error.length > 0);
    for (const s of examples.syntax_error) {
      assert.equal(check(input, { expression: s }), null, JSON.stringify(s));
    }
  });

  it('validation errors violate the expression length', () => {
    for (const s of examples.validation_error) {
      assert.notEqual(check(input, { expression: s }), null, JSON.stringify(s));
    }
    assert.notEqual(check(input, { expression: '1'.repeat(201) }), null, '201 digits');
    assert.equal(check(input, { expression: '1'.repeat(200) }), null, '200 digits');
  });

  it('failed cases carry the exact Russian computation error', () => {
    const pairs: Record<string, string> = { division_by_zero: 'деление на ноль', overflow: 'переполнение' };
    for (const e of [...examples.failed, ...examples.calculator_only_overflow]) {
      assert.equal(pairs[e.code], e.message, e.code);
      assert.match(e.expression, allowed);
      const error = { code: e.code, message: e.message };
      assert.equal(check('/components/schemas/ComputationError', error), null);
    }
    for (const e of examples.failed) {
      assert.equal(check(input, { expression: e.expression }), null);
      const failed = task(e.expression, { status: 'failed', result: null, error: { code: e.code, message: e.message } });
      assert.equal(check('/components/schemas/Task', failed), null);
    }
    for (const e of examples.calculator_only_overflow) {
      assert.ok(e.reason.length > 0);
    }
  });

  it('password limits are UTF-8 bytes', () => {
    const bytes = (s: string) => new TextEncoder().encode(s).length;
    assert.equal(bytes('пароль!!'.slice(0, 4)), 8, '4 Cyrillic letters are 8 bytes');
    assert.equal(bytes('я'.repeat(37)), 74, '37 Cyrillic letters exceed 72 bytes');
  });
});
