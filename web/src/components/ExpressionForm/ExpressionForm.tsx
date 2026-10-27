import { useId, useState, type FormEvent } from 'react';

import { useCreateTask } from '../../hooks/useTasks';
import { errorText } from '../../hooks/apiError';
import styles from './ExpressionForm.module.css';

const MAX_EXPRESSION_LENGTH = 200;
const EMPTY_TEXT = 'Введите выражение';
const TOO_LONG_TEXT = 'Выражение не длиннее 200 символов';

/** Returns a Russian validation message, or null when the expression may be sent. */
export function validateExpression(value: string): string | null {
  const length = Array.from(value).length;
  if (length === 0) {
    return EMPTY_TEXT;
  }
  return length > MAX_EXPRESSION_LENGTH ? TOO_LONG_TEXT : null;
}

export function ExpressionForm() {
  const [value, setValue] = useState('');
  const [error, setError] = useState<string | null>(null);
  const create = useCreateTask();
  const inputId = useId();
  const errorId = useId();

  const submit = (event: FormEvent) => {
    event.preventDefault();
    const invalid = validateExpression(value);
    setError(invalid);
    if (invalid || create.isPending) {
      return;
    }
    create.mutate(value, {
      onSuccess: () => setValue(''),
      onError: (cause) => setError(errorText(cause)),
    });
  };

  return (
    <form className={styles.form} onSubmit={submit} noValidate>
      <label htmlFor={inputId} className={styles.label}>
        Выражение
      </label>
      <div className={styles.row}>
        <input
          id={inputId}
          className={styles.input}
          value={value}
          placeholder="2 + 3 * (4 - 1)"
          autoComplete="off"
          spellCheck={false}
          aria-invalid={error !== null}
          aria-describedby={error ? errorId : undefined}
          onChange={(event) => setValue(event.target.value)}
        />
        <button type="submit" className={styles.submit} disabled={create.isPending}>
          Вычислить
        </button>
      </div>
      <p id={errorId} className={styles.error} role="alert">
        {error}
      </p>
    </form>
  );
}
