// The embedded goose migration carries exactly the DDL of contracts/schema.sql.
import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import { readText } from './helpers.ts';

/** Drops comments and blank lines and collapses whitespace, statement by statement. */
function normalise(sql: string): string[] {
  return sql
    .split('\n')
    .map((line) => line.replace(/--.*$/, '').trim())
    .filter((line) => line !== '')
    .join(' ')
    .replace(/\s+/g, ' ')
    .replace(/\( /g, '(')
    .replace(/ \)/g, ')')
    .split(';')
    .map((s) => s.trim())
    .filter((s) => s !== '');
}

const migration = readText('server/migrations/00001_init.sql');
const [, afterUp = ''] = migration.split('-- +goose Up');
const [up = '', down = ''] = afterUp.split('-- +goose Down');

describe('server/migrations/00001_init.sql', () => {
  it('has goose Up and Down sections', () => {
    assert.ok(migration.trimStart().startsWith('-- +goose Up'));
    assert.ok(down.length > 0, 'Down section missing');
  });

  it('Up equals contracts/schema.sql statement by statement', () => {
    assert.deepEqual(normalise(up), normalise(readText('contracts/schema.sql')));
  });

  it('creates the required tables and indexes', () => {
    const ddl = normalise(up).join(';\n');
    for (const table of ['users', 'sessions', 'tasks']) {
      assert.match(ddl, new RegExp(`CREATE TABLE ${table} \\(`));
    }
    assert.match(ddl, /CREATE INDEX tasks_queue_idx ON tasks \(status, created_at\)/);
    assert.match(ddl, /CREATE INDEX tasks_user_created_idx ON tasks \(user_id, created_at DESC\)/);
    assert.match(ddl, /CONSTRAINT tasks_state_fields CHECK/);
  });

  it('Down drops every table', () => {
    for (const table of ['users', 'sessions', 'tasks']) {
      assert.match(down, new RegExp(`DROP TABLE[^;]*\\b${table}\\b`));
    }
  });
});
