import { useId, useState, type FormEvent, type ReactNode } from 'react';

import { ApiRequestError, errorText } from '../../hooks/apiError';
import styles from './AuthForm.module.css';

export const PASSWORD_MIN_BYTES = 8;
export const PASSWORD_MAX_BYTES = 72;
const PASSWORD_BYTES_TEXT = 'Пароль должен занимать от 8 до 72 байт (кириллица — 2 байта на символ)';
const LOGIN_REQUIRED_TEXT = 'Введите логин';
/** Login format of the API Credentials schema. */
const LOGIN_PATTERN = /^[a-z0-9_-]{3,32}$/;

/** UTF-8 byte length of a password (bcrypt limit is in bytes, not characters). */
export function passwordBytes(password: string): number {
  return new TextEncoder().encode(password).length;
}

interface AuthFormProps {
  heading: string;
  submitLabel: string;
  /** Register checks the password byte length before sending. */
  checkPasswordBytes: boolean;
  newPassword: boolean;
  footer: ReactNode;
  onSubmit: (login: string, password: string) => Promise<void>;
}

interface Errors {
  login: string | null;
  password: string | null;
  form: string | null;
}
const NO_ERRORS: Errors = { login: null, password: null, form: null };

/**
 * The 422 envelope does not name the field, so it is placed at the field that breaks
 * the Credentials schema: an invalid login first, otherwise the password.
 */
function serverErrors(cause: unknown, login: string): Errors {
  if (cause instanceof ApiRequestError) {
    if (cause.status === 409) {
      return { ...NO_ERRORS, login: cause.message };
    }
    if (cause.status === 422) {
      return LOGIN_PATTERN.test(login)
        ? { ...NO_ERRORS, password: cause.message }
        : { ...NO_ERRORS, login: cause.message };
    }
  }
  return { ...NO_ERRORS, form: errorText(cause) };
}

/** Shared login/register form: labelled fields, inline errors, disabled submit while pending. */
export function AuthForm({ heading, submitLabel, checkPasswordBytes, newPassword, footer, onSubmit }: AuthFormProps) {
  const [login, setLogin] = useState('');
  const [password, setPassword] = useState('');
  const [errors, setErrors] = useState<Errors>(NO_ERRORS);
  const [pending, setPending] = useState(false);
  const loginId = useId();
  const passwordId = useId();
  const loginErrorId = useId();
  const passwordErrorId = useId();

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (pending) {
      return;
    }
    const next: Errors = { ...NO_ERRORS };
    if (login === '') {
      next.login = LOGIN_REQUIRED_TEXT;
    }
    const bytes = passwordBytes(password);
    if (checkPasswordBytes && (bytes < PASSWORD_MIN_BYTES || bytes > PASSWORD_MAX_BYTES)) {
      next.password = PASSWORD_BYTES_TEXT;
    }
    if (next.login || next.password) {
      setErrors(next);
      return;
    }
    setErrors(NO_ERRORS);
    setPending(true);
    try {
      await onSubmit(login, password);
    } catch (cause) {
      setErrors(serverErrors(cause, login));
      setPending(false);
    }
  };

  return (
    <main className={styles.page}>
      <form className={styles.form} onSubmit={submit} noValidate>
        <h1 className={styles.heading}>{heading}</h1>
        <div className={styles.field}>
          <label htmlFor={loginId}>Логин</label>
          <input
            id={loginId}
            value={login}
            autoComplete="username"
            autoCapitalize="none"
            spellCheck={false}
            aria-invalid={errors.login !== null}
            aria-describedby={loginErrorId}
            onChange={(event) => setLogin(event.target.value)}
          />
          <p id={loginErrorId} className={styles.error} role="alert">
            {errors.login}
          </p>
        </div>
        <div className={styles.field}>
          <label htmlFor={passwordId}>Пароль</label>
          <input
            id={passwordId}
            type="password"
            value={password}
            autoComplete={newPassword ? 'new-password' : 'current-password'}
            aria-invalid={errors.password !== null}
            aria-describedby={passwordErrorId}
            onChange={(event) => setPassword(event.target.value)}
          />
          <p id={passwordErrorId} className={styles.error} role="alert">
            {errors.password}
          </p>
        </div>
        <p className={styles.error} role="alert">
          {errors.form}
        </p>
        <button type="submit" className={styles.submit} disabled={pending}>
          {submitLabel}
        </button>
        <p className={styles.footer}>{footer}</p>
      </form>
    </main>
  );
}
