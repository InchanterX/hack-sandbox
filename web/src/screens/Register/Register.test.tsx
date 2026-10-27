import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { client } from '../../api/client';
import { apiError, apiResponse, alice, renderApp, sessionOf } from '../../hooks/testHelpers';

vi.mock('../../api/client', () => ({
  client: { GET: vi.fn(), POST: vi.fn(), PATCH: vi.fn(), DELETE: vi.fn() },
  setAuthToken: vi.fn(),
  setUnauthorizedHandler: vi.fn(),
}));

const post = vi.mocked(client.POST) as unknown as ReturnType<typeof vi.fn>;

async function fill(login: string, password: string) {
  const user = userEvent.setup();
  await user.type(screen.getByLabelText('Логин'), login);
  await user.type(screen.getByLabelText('Пароль'), password);
  await user.click(screen.getByRole('button', { name: 'Зарегистрироваться' }));
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(client.GET).mockResolvedValue(apiResponse(200, { items: [] }) as never);
});

describe('Register', () => {
  it('registers and navigates to /', async () => {
    post.mockResolvedValue(apiResponse(201, sessionOf(alice)));
    renderApp('/register');
    await fill('alice', 'secret-pass');
    expect(await screen.findByRole('heading', { name: 'Calc Queue' })).toBeInTheDocument();
    expect(screen.getByTestId('pathname')).toHaveTextContent('/');
  });

  it('counts password bytes, not characters', async () => {
    renderApp('/register');
    // 4 Cyrillic letters = 8 bytes: accepted by the byte rule; 3 letters = 6 bytes: rejected
    await fill('alice', 'абв');
    expect(await screen.findByText(/от 8 до 72 байт/)).toBeInTheDocument();
    expect(post).not.toHaveBeenCalled();
  });

  it('rejects passwords above 72 bytes even when short in characters', async () => {
    renderApp('/register');
    await fill('alice', 'ж'.repeat(37));
    expect(await screen.findByText(/от 8 до 72 байт/)).toBeInTheDocument();
    expect(post).not.toHaveBeenCalled();
  });

  it('accepts 8 bytes written as 4 Cyrillic characters', async () => {
    post.mockResolvedValue(apiResponse(201, sessionOf(alice)));
    renderApp('/register');
    await fill('alice', 'абвг');
    expect(post).toHaveBeenCalledOnce();
  });

  it('shows 409 at the login field', async () => {
    post.mockResolvedValue(apiError(409, 'login_taken', 'логин занят'));
    renderApp('/register');
    await fill('alice', 'secret-pass');
    expect(await screen.findByText('логин занят')).toBeInTheDocument();
    expect(screen.getByLabelText('Логин')).toHaveAccessibleDescription('логин занят');
  });

  it('shows 422 for an invalid login under the login field', async () => {
    post.mockResolvedValue(apiError(422, 'validation_error', 'проверьте поля формы'));
    renderApp('/register');
    await fill('Alice', 'secret-pass');
    expect(await screen.findByLabelText('Логин')).toHaveAccessibleDescription('проверьте поля формы');
    expect(screen.getByLabelText('Пароль')).toHaveAccessibleDescription('');
    expect(screen.getByLabelText('Логин')).toHaveAttribute('aria-invalid', 'true');
  });

  it('shows 422 for a valid login under the password field', async () => {
    post.mockResolvedValue(apiError(422, 'validation_error', 'проверьте поля формы'));
    renderApp('/register');
    await fill('alice', 'secret-pass');
    expect(await screen.findByLabelText('Пароль')).toHaveAccessibleDescription('проверьте поля формы');
    expect(screen.getByLabelText('Логин')).toHaveAccessibleDescription('');
    expect(screen.getByLabelText('Пароль')).toHaveAttribute('aria-invalid', 'true');
  });
});
