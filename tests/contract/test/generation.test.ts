// Regeneration from contracts/openapi.yaml reproduces the committed bindings.
// Output goes to a temporary directory; the working tree is never modified.
import assert from 'node:assert/strict';
import { mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { after, describe, it } from 'node:test';
import { generateGo, generateTs, goTarget, tsTarget } from '../scripts/gen.ts';
import { readText } from './helpers.ts';

const tmp = mkdtempSync(path.join(tmpdir(), 'contract-gen-'));
after(() => rmSync(tmp, { recursive: true, force: true }));

describe('generated bindings', () => {
  it(`${tsTarget} is up to date`, () => {
    const out = path.join(tmp, 'schema.d.ts');
    generateTs(out);
    assert.ok(readFileSync(out, 'utf8') === readText(tsTarget), `${tsTarget} drifted: run npm --prefix tests/contract run generate`);
  });

  it(`${goTarget} is up to date`, () => {
    const out = path.join(tmp, 'openapi.gen.go');
    generateGo(out);
    assert.ok(readFileSync(out, 'utf8') === readText(goTarget), `${goTarget} drifted: run npm --prefix tests/contract run generate`);
  });

  it('Go bindings keep nullable task fields as pointers', () => {
    const go = readText(goTarget);
    assert.match(go, /Result\s+\*float64\s+`json:"result"`/);
    assert.match(go, /StartedAt\s+\*time\.Time\s+`json:"started_at"`/);
    assert.match(go, /FinishedAt\s+\*time\.Time\s+`json:"finished_at"`/);
  });
});
