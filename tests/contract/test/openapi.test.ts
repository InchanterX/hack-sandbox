// OpenAPI 3.1 document: structure, the ten operations, error envelopes and examples.
import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import { Validator } from '@seriousme/openapi-schema-validator';
import { loadOpenAPI, operations, readText, repoPath } from './helpers.ts';
import { compileValidator } from './schema.ts';

const doc = loadOpenAPI();
const check = compileValidator(doc);
const methods = ['get', 'put', 'post', 'delete', 'options', 'head', 'patch', 'trace'];

type Operation = {
  operationId?: string;
  security?: unknown[];
  requestBody?: unknown;
  responses: Record<string, unknown>;
};

function resolve<T>(node: unknown): T {
  let cur = node as Record<string, unknown>;
  while (cur && typeof cur.$ref === 'string') {
    const parts = cur.$ref.replace(/^#\//, '').split('/');
    cur = parts.reduce<unknown>((acc, p) => (acc as Record<string, unknown>)[p], doc) as Record<string, unknown>;
  }
  return cur as T;
}

function op(method: string, p: string): Operation {
  const item = doc.paths[p];
  assert.ok(item, `path ${p} missing`);
  const o = item[method] as Operation | undefined;
  assert.ok(o, `${method.toUpperCase()} ${p} missing`);
  return o;
}

type Media = { schema?: unknown; example?: unknown; examples?: Record<string, { value: unknown }> };

function jsonMedia(response: unknown): Media | undefined {
  const r = resolve<{ content?: Record<string, Media> }>(response);
  return r.content?.['application/json'];
}

function schemaRef(schema: unknown): string {
  const ref = (schema as { $ref?: string }).$ref;
  assert.ok(ref, 'response schemas must reference components');
  return ref.replace(/^#/, '');
}

describe('openapi.yaml', () => {
  it('is a valid OpenAPI 3.1 document', async () => {
    assert.match(doc.openapi, /^3\.1\./);
    const validator = new Validator();
    const res = await validator.validate(repoPath('contracts/openapi.yaml'));
    assert.ok(res.valid, JSON.stringify(res.errors, null, 2));
  });

  it('declares exactly the ten operations', () => {
    const actual = Object.entries(doc.paths)
      .flatMap(([p, item]) =>
        Object.entries(item)
          .filter(([m]) => methods.includes(m))
          .map(([m, o]) => `${m.toUpperCase()} ${p} ${(o as Operation).operationId}`),
      )
      .sort();
    const expected = operations.map((o) => `${o.method.toUpperCase()} ${o.path} ${o.id}`).sort();
    assert.deepEqual(actual, expected);
  });

  it('requires bearer auth everywhere except register, login and healthz', () => {
    assert.deepEqual(doc.security, [{ bearerAuth: [] }]);
    for (const o of operations) {
      const security = op(o.method, o.path).security;
      if (o.public) {
        assert.deepEqual(security, [], `${o.id} must be public`);
      } else {
        assert.ok(security === undefined || JSON.stringify(security) === '[{"bearerAuth":[]}]', `${o.id} must require auth`);
        assert.ok(op(o.method, o.path).responses['401'], `${o.id} must document 401`);
      }
    }
  });

  it('uses the ErrorEnvelope for every error response and validates every example', () => {
    for (const o of operations) {
      for (const [status, response] of Object.entries(op(o.method, o.path).responses)) {
        const media = jsonMedia(response);
        if (status === '204') {
          assert.equal(media, undefined, `${o.id} 204 must have no JSON body`);
          continue;
        }
        assert.ok(media?.schema, `${o.id} ${status} must have an application/json schema`);
        const ref = schemaRef(media.schema);
        if (Number(status) >= 400) {
          assert.equal(ref, '/components/schemas/ErrorEnvelope', `${o.id} ${status}`);
        }
        const examples = [
          ...(media.example === undefined ? [] : [media.example]),
          ...Object.values(media.examples ?? {}).map((e) => e.value),
        ];
        for (const example of examples) {
          assert.equal(check(ref, example), null, `${o.id} ${status} example`);
        }
      }
    }
  });

  it('documents the status codes of every operation', () => {
    const statuses = (method: string, p: string) => Object.keys(op(method, p).responses).sort();
    assert.deepEqual(statuses('post', '/api/auth/register'), ['201', '409', '422', '500'].sort());
    assert.deepEqual(statuses('post', '/api/auth/login'), ['200', '401', '422', '500']);
    assert.deepEqual(statuses('post', '/api/auth/logout'), ['204', '401', '500']);
    assert.deepEqual(statuses('get', '/api/me'), ['200', '401', '500']);
    assert.deepEqual(statuses('post', '/api/tasks'), ['201', '401', '422', '500']);
    assert.deepEqual(statuses('get', '/api/tasks'), ['200', '401', '500']);
    assert.deepEqual(statuses('get', '/api/tasks/{id}'), ['200', '401', '404', '500']);
    assert.deepEqual(statuses('patch', '/api/tasks/{id}'), ['200', '401', '404', '409', '422', '500']);
    assert.deepEqual(statuses('delete', '/api/tasks/{id}'), ['204', '401', '404', '409', '500']);
    assert.deepEqual(statuses('get', '/api/healthz'), ['200', '503']);
  });

  it('keeps the wire schemas strict', () => {
    for (const [name, schema] of Object.entries(doc.components.schemas)) {
      const s = schema as { type?: unknown; additionalProperties?: unknown };
      if (s.type === 'object') {
        assert.equal(s.additionalProperties, false, `${name} must forbid unknown properties`);
      }
    }
    const task = {
      id: '00000000-0000-4000-8000-000000000002',
      expression: '1 + 1',
      status: 'queued',
      result: null,
      error: null,
      created_at: '2026-01-01T00:00:00Z',
      started_at: null,
      finished_at: null,
    };
    assert.equal(check('/components/schemas/Task', task), null);
    const { result: _omitted, ...withoutResult } = task;
    assert.notEqual(check('/components/schemas/Task', withoutResult), null, 'nullable fields are required');
    assert.notEqual(check('/components/schemas/Task', { ...task, user_id: task.id }), null, 'user_id is private');
    assert.equal(check('/components/schemas/TaskList', { items: [] }), null);
    assert.notEqual(check('/components/schemas/Health', { status: 'degraded' }), null);
  });

  it('documents every error message in Russian', () => {
    const text = readText('contracts/openapi.yaml');
    const messages = [...text.matchAll(/code: \w+, message: ([^}\n]+)}/g)].map((m) => m[1] ?? '');
    assert.ok(messages.length >= 10, 'error examples missing');
    for (const m of messages) {
      assert.match(m, /^'?[а-яё]/i, `error message ${m} must be Russian`);
    }
  });
});
