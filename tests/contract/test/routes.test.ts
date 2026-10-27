// The shared Go route table and the web client cover exactly the ten operations.
import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import { operations, readText } from './helpers.ts';

describe('route wiring', () => {
  it('server/internal/server/routes.go registers the ten operations', () => {
    const routes = readText('server/internal/server/routes.go');
    const registered = [...routes.matchAll(/mux\.Handle(?:Func)?\("([A-Z]+) (\/api\/[^"]*)"/g)]
      .map((m) => `${m[1]} ${m[2]}`)
      .sort();
    const expected = operations.map((o) => `${o.method.toUpperCase()} ${o.path}`).sort();
    assert.deepEqual(registered, expected);
  });

  it('the generated TypeScript paths cover the same operations', () => {
    const schema = readText('web/src/api/schema.d.ts');
    for (const o of operations) {
      assert.ok(schema.includes(`"${o.path}": {`), `${o.path} missing from schema.d.ts`);
      assert.match(schema, new RegExp(`\\b${o.id}: \\{`), `operation ${o.id} missing from schema.d.ts`);
    }
  });
});
