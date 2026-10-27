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
const get = vi.mocked(client.GET) as unknown as ReturnType<typeof vi.fn>;

async function fill(login: string, password: string) {
  const user = userEvent.setup();
  await user.type(screen.getByLabelText('Логин'), login);
  await user.type(screen.getByLabelText('Пароль'), password);
  return user;
}

beforeEach(() => {
  vi.clearAllMocks();
  get.mockResolvedValue(apiResponse(200, { items: [] }));
});

describe('Login', () => {
  it('sends credentials and navigates to / on success', async () => {
    post.mockResolvedValue(apiResponse(200, sessionOf(alice)));
    renderApp('/login');
    const user = await fill('alice', 'secret-pass');
    await user.click(screen.getByRole('button', { name: 'Войти' }));
    expect(post).toHaveBeenCalledWith('/api/auth/login', { body: { login: 'alice', password: 'secret-pass' } });
    expect(await screen.findByRole('heading', { name: 'Calc Queue' })).toBeInTheDocument();
    expect(screen.getByTestId('pathname')).toHaveTextContent('/');
    expect(localStorage.length).toBe(1);
  });

  it('shows the 401 message in the form and allows retry', async () => {
    post.mockResolvedValueOnce(apiError(401, 'invalid_credentials', 'неверный логин или пароль'));
    renderApp('/login');
    const user = await fill('alice', 'wrong-pass');
    await user.click(screen.getByRole('button', { name: 'Войти' }));
    expect(await screen.findByText('неверный логин или пароль')).toBeInTheDocument();
    expect(screen.getByLabelText('Логин')).toHaveValue('alice');
    post.mockResolvedValueOnce(apiResponse(200, sessionOf(alice)));
    await user.click(screen.getByRole('button', { name: 'Войти' }));
    expect(await screen.findByRole('heading', { name: 'Calc Queue' })).toBeInTheDocument();
  });

  it('shows 422 under the field and disables submit while pending', async () => {
    let resolve: (value: unknown) => void = () => undefined;
    post.mockReturnValueOnce(new Promise((r) => (resolve = r)));
    renderApp('/login');
    const user = await fill('al', 'secret-pass');
    await user.click(screen.getByRole('button', { name: 'Войти' }));
    expect(screen.getByRole('button', { name: 'Войти' })).toBeDisabled();
    resolve(apiError(422, 'validation_error', 'проверьте поля формы'));
    expect(await screen.findByText('проверьте поля формы')).toBeInTheDocument();
    expect(screen.getByLabelText('Логин')).toHaveAccessibleDescription('проверьте поля формы');
    expect(screen.getByRole('button', { name: 'Войти' })).toBeEnabled();
  });

  it('shows a network error without losing input', async () => {
    post.mockRejectedValueOnce(new TypeError('Failed to fetch'));
    renderApp('/login');
    const user = await fill('alice', 'secret-pass');
    await user.click(screen.getByRole('button', { name: 'Войти' }));
    expect(await screen.findByText('Не удалось связаться с сервером')).toBeInTheDocument();
    expect(screen.getByLabelText('Пароль')).toHaveValue('secret-pass');
  });

  it('links to registration', () => {
    renderApp('/login');
    expect(screen.getByRole('link', { name: 'Регистрация' })).toHaveAttribute('href', '/register');
  });

  it('reaches every control with the keyboard', async () => {
    renderApp('/login');
    const user = userEvent.setup();
    await user.tab();
    expect(screen.getByLabelText('Логин')).toHaveFocus();
    await user.tab();
    expect(screen.getByLabelText('Пароль')).toHaveFocus();
    await user.tab();
    expect(screen.getByRole('button', { name: 'Войти' })).toHaveFocus();
    await user.tab();
    expect(screen.getByRole('link', { name: 'Регистрация' })).toHaveFocus();
  });
});
