// API e2e of the demo scenario in brief/01_product.md (SPEC.md#Verification):
// the nine demo steps in order, plus restart recovery, against a real server
// with WORKERS=2 and OP_DELAY_MS=50 in an isolated schema.
//
// A separate user "load" keeps both workers busy with long tasks when a step
// needs the queue to be visible, so alice's list stays exactly as in the demo.
import assert from 'node:assert/strict';
import { randomBytes } from 'node:crypto';
import { after, before, describe, it } from 'node:test';

import type { AuthSession, ErrorEnvelope, Task, TaskList, User } from '../../contracts/types.ts';
import { App, waitFor } from './harness.ts';

const app = new App();
const suffix = randomBytes(4).toString('hex');
const alice = { login: `alice_${suffix}`, password: 'alice-pass-1' };
const bob = { login: `bob_${suffix}`, password: 'bob-pass-12' };
const load = { login: `load_${suffix}`, password: 'load-pass-1' };

/** 100 ones joined by "+": 199 characters, 99 operations, about 5 s per task. */
const LONG = Array.from({ length: 100 }, () => '1').join('+');
/** Generous bound for a task to reach its next state. */
const TASK_TIMEOUT = 30_000;

let aliceToken = '';
let loadToken = '';
const ids = { first: '', second: '', divZero: '', edited: '' };

/** Runs a demo step; on failure prints the server output. */
function step(name: string, fn: () => Promise<void>): void {
  it(name, async () => {
    try {
      await fn();
    } catch (err) {
      console.error(`--- server output ---\n${app.logs()}--- end of server output ---`);
      throw err;
    }
  });
}

function expectError(res: { status: number; body: unknown }, status: number, code: string): void {
  assert.equal(res.status, status, `want ${status} ${code}, got ${res.status} ${JSON.stringify(res.body)}`);
  const env = res.body as ErrorEnvelope;
  assert.equal(env.error.code, code);
  assert.ok(env.error.message.length > 0, 'error message must not be empty');
}

async function register(creds: { login: string; password: string }): Promise<string> {
  const res = await app.request<AuthSession>('POST', '/api/auth/register', { body: creds });
  assert.equal(res.status, 201, res.text);
  assert.equal(res.body.user.login, creds.login);
  assert.ok(res.body.token);
  return res.body.token;
}

async function list(token: string): Promise<Task[]> {
  const res = await app.request<TaskList>('GET', '/api/tasks', { token });
  assert.equal(res.status, 200, res.text);
  return res.body.items;
}

async function getTask(token: string, id: string): Promise<Task> {
  const res = await app.request<Task>('GET', `/api/tasks/${id}`, { token });
  assert.equal(res.status, 200, res.text);
  return res.body;
}

async function create(token: string, expression: string): Promise<Task> {
  const res = await app.request<Task>('POST', '/api/tasks', { token, body: { expression } });
  assert.equal(res.status, 201, res.text);
  assert.equal(res.body.expression, expression);
  return res.body;
}

async function waitStatus(token: string, id: string, statuses: Task['status'][]): Promise<Task> {
  let task: Task | undefined;
  await waitFor(`task ${id} in ${statuses.join('/')}`, TASK_TIMEOUT, async () => {
    task = await getTask(token, id);
    return statuses.includes(task.status);
  });
  return task as Task;
}

/** Occupies both workers with long tasks of the load user and waits until both run. */
async function occupyWorkers(): Promise<string[]> {
  const blockers = [await create(loadToken, LONG), await create(loadToken, LONG)];
  for (const b of blockers) await waitStatus(loadToken, b.id, ['running']);
  return blockers.map((b) => b.id);
}

describe('demo scenario (brief/01_product.md)', () => {
  before(async () => {
    await app.setup();
    await app.start();
    loadToken = await register(load);
  });

  after(async () => {
    await app.teardown();
  });

  step('1. without a token the API answers 401 unauthorized', async () => {
    expectError(await app.request('GET', '/api/me'), 401, 'unauthorized');
    expectError(await app.request('GET', '/api/tasks'), 401, 'unauthorized');
    expectError(await app.request('POST', '/api/tasks', { body: { expression: '1 + 1' } }), 401, 'unauthorized');
    expectError(await app.request('GET', '/api/tasks', { token: 'not-a-real-token' }), 401, 'unauthorized');
  });

  step('2. alice registers and lands on an empty task list; a taken login is 409', async () => {
    aliceToken = await register(alice);
    const me = await app.request<User>('GET', '/api/me', { token: aliceToken });
    assert.equal(me.status, 200, me.text);
    assert.equal(me.body.login, alice.login);
    const res = await app.request<TaskList>('GET', '/api/tasks', { token: aliceToken });
    assert.equal(res.status, 200);
    assert.deepEqual(res.body, { items: [] });

    expectError(await app.request('POST', '/api/auth/register', { body: alice }), 409, 'login_taken');
  });

  step('3. "2 + 3 * (4 - 1)" appears in the list as queued', async () => {
    await occupyWorkers();
    const task = await create(aliceToken, '2 + 3 * (4 - 1)');
    assert.equal(task.status, 'queued');
    assert.equal(task.result, null);
    assert.equal(task.error, null);
    assert.equal(task.started_at, null);
    assert.equal(task.finished_at, null);
    ids.first = task.id;
    const items = await list(aliceToken);
    assert.deepEqual(
      items.map((t) => [t.id, t.status]),
      [[task.id, 'queued']],
    );
  });

  step('4. two more tasks wait in the queue; at most two run at a time, in FIFO order', async () => {
    ids.second = (await create(aliceToken, '(10 - 4) / 3')).id;
    ids.divZero = (await create(aliceToken, '7 / (2 - 2)')).id;
    const items = await list(aliceToken);
    assert.deepEqual(
      items.map((t) => [t.id, t.status]),
      [
        [ids.divZero, 'queued'],
        [ids.second, 'queued'],
        [ids.first, 'queued'],
      ],
      'newest first, all waiting while both workers are busy',
    );

    const seen = new Map<string, Set<Task['status']>>();
    await waitFor('the three tasks to finish', 3 * TASK_TIMEOUT, async () => {
      const all = [...(await list(aliceToken)), ...(await list(loadToken))];
      const running = all.filter((t) => t.status === 'running').length;
      assert.ok(running <= 2, `${running} tasks running with WORKERS=2`);
      const mine = all.filter((t) => [ids.first, ids.second, ids.divZero].includes(t.id));
      for (const t of mine) {
        const s = seen.get(t.id) ?? new Set();
        s.add(t.status);
        seen.set(t.id, s);
      }
      return mine.every((t) => t.status === 'done' || t.status === 'failed');
    });
    for (const id of [ids.first, ids.second, ids.divZero]) {
      assert.ok(seen.get(id)?.has('queued'), `task ${id} was observed queued`);
    }

    const [first, second, divZero] = await Promise.all(
      [ids.first, ids.second, ids.divZero].map((id) => getTask(aliceToken, id)),
    );
    assert.ok(first && second && divZero);
    const started = (t: Task) => Date.parse(t.started_at ?? '');
    assert.ok(started(first) <= started(second), 'first started before second');
    assert.ok(started(second) <= started(divZero), 'second started before third');
  });

  step('5. results without reload: 11, 2 and a division-by-zero failure', async () => {
    const first = await getTask(aliceToken, ids.first);
    assert.equal(first.status, 'done');
    assert.equal(first.result, 11);
    assert.equal(first.error, null);
    assert.ok(first.started_at && first.finished_at);

    const second = await getTask(aliceToken, ids.second);
    assert.equal(second.status, 'done');
    assert.equal(second.result, 2);

    const divZero = await getTask(aliceToken, ids.divZero);
    assert.equal(divZero.status, 'failed');
    assert.equal(divZero.result, null);
    assert.deepEqual(divZero.error, { code: 'division_by_zero', message: 'деление на ноль' });
    assert.ok(divZero.finished_at);

    const health = await app.request('GET', '/api/healthz');
    assert.equal(health.status, 200, 'the service keeps working after a failed task');
  });

  step('6. a queued "100 / 7 + 1" is edited to "100 - 7" and gives 93', async () => {
    const blockers = await occupyWorkers();

    // Only queued tasks change; running ones can be neither edited nor deleted.
    const running = await getTask(loadToken, blockers[0] ?? '');
    expectError(
      await app.request('PATCH', `/api/tasks/${running.id}`, { token: loadToken, body: { expression: '1 + 1' } }),
      409,
      'not_queued',
    );
    expectError(await app.request('DELETE', `/api/tasks/${running.id}`, { token: loadToken }), 409, 'task_running');
    assert.equal((await getTask(loadToken, running.id)).expression, LONG);

    const task = await create(aliceToken, '100 / 7 + 1');
    assert.equal(task.status, 'queued');
    ids.edited = task.id;

    // Rejected edits leave the task unchanged.
    expectError(
      await app.request('PATCH', `/api/tasks/${task.id}`, { token: aliceToken, body: { expression: '2 + * 3' } }),
      422,
      'syntax_error',
    );
    expectError(
      await app.request('PATCH', `/api/tasks/${task.id}`, {
        token: aliceToken,
        body: { expression: '1'.repeat(201) },
      }),
      422,
      'validation_error',
    );
    assert.equal((await getTask(aliceToken, task.id)).expression, '100 / 7 + 1');

    const edited = await app.request<Task>('PATCH', `/api/tasks/${task.id}`, {
      token: aliceToken,
      body: { expression: '100 - 7' },
    });
    assert.equal(edited.status, 200, edited.text);
    assert.equal(edited.body.id, task.id);
    assert.equal(edited.body.expression, '100 - 7');
    assert.equal(edited.body.status, 'queued');
    assert.equal(edited.body.created_at, task.created_at);

    const done = await waitStatus(aliceToken, task.id, ['done', 'failed']);
    assert.equal(done.status, 'done');
    assert.equal(done.expression, '100 - 7');
    assert.equal(done.result, 93);

    // A finished task can no longer be edited.
    expectError(
      await app.request('PATCH', `/api/tasks/${task.id}`, { token: aliceToken, body: { expression: '1 + 1' } }),
      409,
      'not_queued',
    );
    assert.deepEqual(await getTask(aliceToken, task.id), done);
  });

  step('7. "2 + * 3" is rejected with syntax_error and no task is created', async () => {
    const before = await list(aliceToken);
    expectError(
      await app.request('POST', '/api/tasks', { token: aliceToken, body: { expression: '2 + * 3' } }),
      422,
      'syntax_error',
    );
    expectError(
      await app.request('POST', '/api/tasks', { token: aliceToken, body: { expression: '' } }),
      422,
      'validation_error',
    );
    expectError(
      await app.request('POST', '/api/tasks', { token: aliceToken, body: { expression: '1'.repeat(201) } }),
      422,
      'validation_error',
    );
    assert.deepEqual(await list(aliceToken), before);
  });

  step('8. deleting the failed task removes it from the list', async () => {
    const res = await app.request('DELETE', `/api/tasks/${ids.divZero}`, { token: aliceToken });
    assert.equal(res.status, 204);
    assert.equal(res.text, '');
    expectError(await app.request('GET', `/api/tasks/${ids.divZero}`, { token: aliceToken }), 404, 'not_found');
    const items = await list(aliceToken);
    assert.ok(!items.some((t) => t.id === ids.divZero));
    assert.deepEqual(
      items.map((t) => t.id),
      [ids.edited, ids.second, ids.first],
    );
  });

  step("9. after logout bob sees an empty list and cannot reach alice's tasks; alice's tasks remain", async () => {
    const out = await app.request('POST', '/api/auth/logout', { token: aliceToken });
    assert.equal(out.status, 204);
    assert.equal(out.text, '');
    expectError(await app.request('GET', '/api/tasks', { token: aliceToken }), 401, 'unauthorized');

    const bobToken = await register(bob);
    assert.deepEqual(await list(bobToken), []);
    for (const id of [ids.first, '00000000-0000-0000-0000-000000000000', 'not-a-uuid']) {
      expectError(await app.request('GET', `/api/tasks/${id}`, { token: bobToken }), 404, 'not_found');
      expectError(
        await app.request('PATCH', `/api/tasks/${id}`, { token: bobToken, body: { expression: '1 + 1' } }),
        404,
        'not_found',
      );
      expectError(await app.request('DELETE', `/api/tasks/${id}`, { token: bobToken }), 404, 'not_found');
    }

    expectError(
      await app.request('POST', '/api/auth/login', { body: { login: alice.login, password: 'wrong-pass-1' } }),
      401,
      'invalid_credentials',
    );
    const login = await app.request<AuthSession>('POST', '/api/auth/login', { body: alice });
    assert.equal(login.status, 200, login.text);
    aliceToken = login.body.token;
    const items = await list(aliceToken);
    assert.deepEqual(
      items.map((t) => [t.id, t.status, t.result]),
      [
        [ids.edited, 'done', 93],
        [ids.second, 'done', 2],
        [ids.first, 'done', 11],
      ],
    );
  });

  step('restart: a task running when the server is killed is queued again and finishes', async () => {
    const task = await create(loadToken, LONG);
    await waitStatus(loadToken, task.id, ['running']);
    await app.stop('SIGKILL');
    const killedAt = Date.now();

    await app.start();
    const done = await waitStatus(loadToken, task.id, ['done', 'failed']);
    assert.equal(done.status, 'done');
    assert.equal(done.result, 100);
    assert.ok(Date.parse(done.started_at ?? '') >= killedAt - 1000, 'the task was claimed again after restart');

    // Data survives the restart: alice still sees her tasks with the old token.
    assert.equal((await list(aliceToken)).length, 3);
  });
});
