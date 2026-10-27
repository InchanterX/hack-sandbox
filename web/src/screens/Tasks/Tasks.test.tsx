import { act, fireEvent, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { client } from '../../api/client';
import { saveToken } from '../../auth/session';
import { alice, apiError, apiResponse, makeTask, renderApp } from '../../hooks/testHelpers';
import type { Task } from '../../hooks/useTasks';

vi.mock('../../api/client', () => ({
  client: { GET: vi.fn(), POST: vi.fn(), PATCH: vi.fn(), DELETE: vi.fn() },
  setAuthToken: vi.fn(),
  setUnauthorizedHandler: vi.fn(),
}));

type Mock = ReturnType<typeof vi.fn>;
const get = client.GET as unknown as Mock;
const post = client.POST as unknown as Mock;
const patch = client.PATCH as unknown as Mock;
const del = client.DELETE as unknown as Mock;

let tasks: Task[] = [];
let listError: unknown = null;

beforeEach(() => {
  vi.clearAllMocks();
  saveToken('tok');
  tasks = [];
  listError = null;
  get.mockImplementation((path: string) => {
    if (path === '/api/me') {
      return Promise.resolve(apiResponse(200, alice));
    }
    return listError ? Promise.reject(listError) : Promise.resolve(apiResponse(200, { items: tasks }));
  });
});

afterEach(() => {
  vi.useRealTimers();
});

const buttonsOf = (row: HTMLElement) => within(row).queryAllByRole('button').map((b) => b.textContent);

describe('Tasks states', () => {
  it('shows loading, then the exact empty hint', async () => {
    let resolve: (value: unknown) => void = () => undefined;
    get.mockImplementation((path: string) =>
      path === '/api/me' ? Promise.resolve(apiResponse(200, alice)) : new Promise((r) => (resolve = r)),
    );
    renderApp('/');
    await waitFor(() => expect(get).toHaveBeenCalledWith('/api/tasks', expect.anything()));
    expect(screen.getByText('Загрузка…')).toBeInTheDocument();
    resolve(apiResponse(200, { items: [] }));
    expect(await screen.findByText('Задач пока нет — отправьте первое выражение')).toBeInTheDocument();
    expect(screen.getByText('alice')).toBeInTheDocument();
  });

  it('shows a network error with Повторить that refetches', async () => {
    listError = new TypeError('offline');
    renderApp('/');
    const retry = await screen.findByRole('button', { name: 'Повторить' });
    expect(screen.getByText('Не удалось связаться с сервером')).toBeInTheDocument();
    listError = null;
    await userEvent.click(retry);
    expect(await screen.findByText('Задач пока нет — отправьте первое выражение')).toBeInTheDocument();
  });

  it('shows numeric result 0 and the failed error message', async () => {
    tasks = [
      makeTask({ id: 'a', status: 'done', expression: '1 - 1', result: 0 }),
      makeTask({
        id: 'b',
        status: 'failed',
        expression: '1 / 0',
        error: { code: 'division_by_zero', message: 'деление на ноль' },
      }),
    ];
    renderApp('/');
    expect(await screen.findByText('деление на ноль')).toBeInTheDocument();
    const doneRow = screen.getByText('1 - 1').closest('tr') as HTMLElement;
    expect(within(doneRow).getByText('0')).toBeInTheDocument();
    expect(within(doneRow).getByText('готово')).toBeInTheDocument();
    expect(screen.getByText('ошибка')).toBeInTheDocument();
  });
});

describe('Task actions by status', () => {
  it('offers Изменить/Удалить for queued, nothing for running, only Удалить for terminal', async () => {
    tasks = [
      makeTask({ id: 'q', status: 'queued', expression: '1 + 1' }),
      makeTask({ id: 'r', status: 'running', expression: '2 + 2' }),
      makeTask({ id: 'd', status: 'done', expression: '3 + 3', result: 6 }),
      makeTask({ id: 'f', status: 'failed', expression: '4 / 0', error: { code: 'division_by_zero', message: 'деление на ноль' } }),
    ];
    renderApp('/');
    await screen.findByText('1 + 1');
    const row = (text: string) => screen.getByText(text).closest('tr') as HTMLElement;
    expect(buttonsOf(row('1 + 1'))).toEqual(['Изменить', 'Удалить']);
    expect(buttonsOf(row('2 + 2'))).toEqual([]);
    expect(buttonsOf(row('3 + 3'))).toEqual(['Удалить']);
    expect(buttonsOf(row('4 / 0'))).toEqual(['Удалить']);
  });

  it('edits inline: keeps input on 422, saves, and cancels', async () => {
    tasks = [makeTask({ expression: '1 + 1' })];
    renderApp('/');
    const user = userEvent.setup();
    await user.click(await screen.findByRole('button', { name: 'Изменить' }));
    const input = screen.getByRole('textbox', { name: 'Выражение задачи' });
    expect(input).toHaveValue('1 + 1');
    await user.clear(input);
    await user.type(input, '1 +* 2');
    patch.mockResolvedValueOnce(apiError(422, 'syntax_error', 'синтаксическая ошибка: неожиданный \'*\' в позиции 4'));
    await user.click(screen.getByRole('button', { name: 'Сохранить' }));
    expect(await screen.findByText(/синтаксическая ошибка/)).toBeInTheDocument();
    expect(input).toHaveValue('1 +* 2');

    await user.clear(input);
    await user.type(input, '5 * 5');
    patch.mockImplementationOnce(() => {
      tasks = [makeTask({ expression: '5 * 5' })];
      return Promise.resolve(apiResponse(200, tasks[0]));
    });
    await user.click(screen.getByRole('button', { name: 'Сохранить' }));
    expect(await screen.findByText('5 * 5')).toBeInTheDocument();
    expect(patch).toHaveBeenLastCalledWith('/api/tasks/{id}', {
      params: { path: { id: tasks[0]?.id } },
      body: { expression: '5 * 5' },
    });
    expect(screen.queryByRole('textbox', { name: 'Выражение задачи' })).toBeNull();

    await user.click(screen.getByRole('button', { name: 'Изменить' }));
    await user.click(screen.getByRole('button', { name: 'Отмена' }));
    expect(screen.getByRole('button', { name: 'Изменить' })).toBeInTheDocument();
  });

  it('closes the edit action on 409 and keeps a read-only draft even if the refreshed snapshot is queued', async () => {
    tasks = [makeTask({ expression: '1 + 1' })];
    renderApp('/');
    const user = userEvent.setup();
    await user.click(await screen.findByRole('button', { name: 'Изменить' }));
    await user.type(screen.getByRole('textbox', { name: 'Выражение задачи' }), '0');
    patch.mockResolvedValueOnce(apiError(409, 'not_queued', 'задача уже выполняется'));
    await user.click(screen.getByRole('button', { name: 'Сохранить' }));
    expect(await screen.findByText('задача уже выполняется')).toBeInTheDocument();
    expect(screen.getByRole('textbox', { name: 'Выражение задачи' })).toHaveValue('1 + 10');
    expect(screen.getByRole('textbox', { name: 'Выражение задачи' })).toHaveAttribute('readonly');
    expect(screen.queryByRole('button', { name: 'Сохранить' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Изменить' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Отмена' })).toBeNull();
    expect(get.mock.calls.filter(([path]) => path === '/api/tasks')).toHaveLength(2);
    // Enter in the retained copyable draft cannot resubmit the unavailable edit.
    await user.click(screen.getByRole('textbox', { name: 'Выражение задачи' }));
    await user.keyboard('{Enter}');
    expect(patch).toHaveBeenCalledOnce();
  });

  it('refreshes a 409 edit conflict to running, hides actions and preserves the draft for copying', async () => {
    tasks = [makeTask({ expression: '1 + 1' })];
    renderApp('/');
    const user = userEvent.setup();
    await user.click(await screen.findByRole('button', { name: 'Изменить' }));
    const input = screen.getByRole('textbox', { name: 'Выражение задачи' });
    await user.clear(input);
    await user.type(input, '99 + 1');
    patch.mockImplementationOnce(() => {
      tasks = [makeTask({ expression: '1 + 1', status: 'running' })];
      return Promise.resolve(apiError(409, 'not_queued', 'изменить можно только задачу в очереди'));
    });
    await user.click(screen.getByRole('button', { name: 'Сохранить' }));
    expect(await screen.findByText('изменить можно только задачу в очереди')).toHaveAttribute('role', 'alert');
    expect(screen.getByText('выполняется')).toBeInTheDocument();
    expect(screen.getByRole('textbox', { name: 'Выражение задачи' })).toBe(input);
    expect(input).toHaveValue('99 + 1');
    expect(input).toHaveAttribute('readonly');
    expect(buttonsOf(input.closest('tr') as HTMLElement)).toEqual([]);
    await user.click(input);
    await user.keyboard('{Control>}a{/Control}');
    expect(input).toHaveFocus();
    expect((input as HTMLInputElement).selectionEnd).toBe('99 + 1'.length);
  });

  it('deletes a task and updates the list without reload', async () => {
    tasks = [makeTask({ id: 'a', status: 'done', result: 3 })];
    renderApp('/');
    del.mockImplementationOnce(() => {
      tasks = [];
      return Promise.resolve(apiResponse(204));
    });
    await userEvent.click(await screen.findByRole('button', { name: 'Удалить' }));
    expect(await screen.findByText('Задач пока нет — отправьте первое выражение')).toBeInTheDocument();
    expect(del).toHaveBeenCalledWith('/api/tasks/{id}', { params: { path: { id: 'a' } } });
  });

  it('shows a delete conflict message and refreshes to running without delete controls', async () => {
    tasks = [makeTask({ status: 'queued' })];
    renderApp('/');
    del.mockImplementationOnce(() => {
      tasks = [makeTask({ status: 'running' })];
      return Promise.resolve(apiError(409, 'task_running', 'задача выполняется'));
    });
    await userEvent.click(await screen.findByRole('button', { name: 'Удалить' }));
    expect(await screen.findByText('задача выполняется')).toBeInTheDocument();
    expect(screen.getByText('выполняется')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Удалить' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Изменить' })).toBeNull();
    expect(get.mock.calls.filter(([path]) => path === '/api/tasks')).toHaveLength(2);
  });
});

describe('Create form', () => {
  it('creates a task, clears the field and lists it', async () => {
    renderApp('/');
    const user = userEvent.setup();
    await screen.findByText('Задач пока нет — отправьте первое выражение');
    const input = screen.getByLabelText('Выражение');
    expect(input).toHaveAttribute('placeholder', '2 + 3 * (4 - 1)');
    await user.type(input, '2 + 3');
    post.mockImplementationOnce(() => {
      tasks = [makeTask({ expression: '2 + 3' })];
      return Promise.resolve(apiResponse(201, tasks[0]));
    });
    await user.click(screen.getByRole('button', { name: 'Вычислить' }));
    expect(await screen.findByRole('table')).toHaveTextContent('2 + 3');
    expect(input).toHaveValue('');
    expect(post).toHaveBeenCalledWith('/api/tasks', { body: { expression: '2 + 3' } });
  });

  it('keeps a rejected expression and shows 422 under the field', async () => {
    renderApp('/');
    const user = userEvent.setup();
    await user.type(await screen.findByLabelText('Выражение'), '2 ^ 3');
    post.mockResolvedValueOnce(apiError(422, 'syntax_error', 'синтаксическая ошибка'));
    await user.click(screen.getByRole('button', { name: 'Вычислить' }));
    expect(await screen.findByText('синтаксическая ошибка')).toBeInTheDocument();
    expect(screen.getByLabelText('Выражение')).toHaveValue('2 ^ 3');
  });

  it('counts code points: 200 emoji are accepted locally, 201 are not', async () => {
    renderApp('/');
    const user = userEvent.setup();
    const input = await screen.findByLabelText('Выражение');
    await user.click(input);
    await user.paste('😀'.repeat(201));
    await user.click(screen.getByRole('button', { name: 'Вычислить' }));
    expect(await screen.findByText(/не длиннее 200/)).toBeInTheDocument();
    expect(post).not.toHaveBeenCalled();
  });

  it('disables submit while the request is pending', async () => {
    renderApp('/');
    const user = userEvent.setup();
    await user.type(await screen.findByLabelText('Выражение'), '1+1');
    let resolve: (value: unknown) => void = () => undefined;
    post.mockReturnValueOnce(new Promise((r) => (resolve = r)));
    await user.click(screen.getByRole('button', { name: 'Вычислить' }));
    expect(screen.getByRole('button', { name: 'Вычислить' })).toBeDisabled();
    await act(async () => resolve(apiResponse(201, makeTask())));
    await waitFor(() => expect(screen.getByRole('button', { name: 'Вычислить' })).toBeEnabled());
  });
});

describe('Polling', () => {
  it('refetches every 1000 ms without replacing a typed edit or moving focus', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: false });
    tasks = [makeTask({ expression: '1 + 1' })];
    renderApp('/');
    for (let i = 0; i < 10; i += 1) {
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1);
      });
    }
    expect(screen.getByRole('button', { name: 'Изменить' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Изменить' }));
    const input = screen.getByRole('textbox', { name: 'Выражение задачи' });
    input.focus();
    fireEvent.change(input, { target: { value: '1 + 19' } });
    expect(input).toHaveFocus();
    const before = get.mock.calls.filter(([p]) => p === '/api/tasks').length;
    tasks = [makeTask({ expression: '1 + 1', status: 'running' })];
    await act(async () => {
      await vi.advanceTimersByTimeAsync(980);
    });
    expect(get.mock.calls.filter(([p]) => p === '/api/tasks').length).toBe(before);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(40);
    });
    expect(get.mock.calls.filter(([p]) => p === '/api/tasks').length).toBe(before + 1);
    expect(screen.getByText('выполняется')).toBeInTheDocument();
    expect(screen.getByRole('textbox', { name: 'Выражение задачи' })).toBe(input);
    expect(input).toHaveValue('1 + 19');
    expect(input).toHaveFocus();
    expect(screen.queryByRole('button', { name: 'Сохранить' })).toBeNull();
    expect(screen.getByRole('button', { name: 'Отмена' })).toBeInTheDocument();
  });
});

describe('Task keyboard controls', () => {
  it('reaches logout, create, edit, save and cancel using Tab and Enter', async () => {
    tasks = [makeTask()];
    renderApp('/');
    const user = userEvent.setup();
    await screen.findByRole('button', { name: 'Изменить' });
    await user.tab();
    expect(screen.getByRole('button', { name: 'Выйти' })).toHaveFocus();
    await user.tab();
    expect(screen.getByLabelText('Выражение')).toHaveFocus();
    await user.tab();
    expect(screen.getByRole('button', { name: 'Вычислить' })).toHaveFocus();
    await user.tab();
    expect(screen.getByRole('button', { name: 'Изменить' })).toHaveFocus();
    await user.keyboard('{Enter}');
    expect(screen.getByRole('textbox', { name: 'Выражение задачи' })).toHaveFocus();
    await user.tab();
    expect(screen.getByRole('button', { name: 'Сохранить' })).toHaveFocus();
    await user.tab();
    expect(screen.getByRole('button', { name: 'Отмена' })).toHaveFocus();
    await user.keyboard('{Enter}');
    expect(screen.queryByRole('textbox', { name: 'Выражение задачи' })).toBeNull();
  });

  it('retries a failed list using the keyboard and announces the error', async () => {
    listError = new TypeError('offline');
    renderApp('/');
    const user = userEvent.setup();
    const retry = await screen.findByRole('button', { name: 'Повторить' });
    expect(retry.closest('[role="alert"]')).toHaveTextContent('Не удалось связаться с сервером');
    await user.tab();
    await user.tab();
    await user.tab();
    await user.tab();
    expect(retry).toHaveFocus();
    listError = null;
    await user.keyboard('{Enter}');
    expect((await screen.findByText('Задач пока нет — отправьте первое выражение')).closest('[role="status"]')).toBeInTheDocument();
  });
});
