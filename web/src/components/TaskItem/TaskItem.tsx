import { useEffect, useId, useRef, useState, type FormEvent } from 'react';

import { ApiRequestError, errorText } from '../../hooks/apiError';
import { useDeleteTask, useUpdateTask, type Task } from '../../hooks/useTasks';
import { validateExpression } from '../ExpressionForm/ExpressionForm';
import { StatusBadge } from '../StatusBadge/StatusBadge';
import styles from './TaskItem.module.css';

const timeFormat = new Intl.DateTimeFormat('ru-RU', { dateStyle: 'short', timeStyle: 'medium' });

function outcome(task: Task): string {
  if (task.error) {
    return task.error.message;
  }
  return task.result !== null ? String(task.result) : '—';
}

/** One task as a table row (cards on narrow screens) with an inline expression editor. */
export function TaskItem({ task }: { task: Task }) {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState('');
  const [fieldError, setFieldError] = useState<string | null>(null);
  const [conflict, setConflict] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const update = useUpdateTask();
  const remove = useDeleteTask();
  const formId = useId();
  const inputId = useId();
  const errorId = useId();
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (editing) inputRef.current?.focus();
  }, [editing]);

  const canEdit = task.status === 'queued' && conflict === null;
  const canDelete = task.status !== 'running';

  const startEditing = () => {
    setDraft(task.expression);
    setFieldError(null);
    setConflict(null);
    setEditing(true);
  };
  const stopEditing = () => {
    setEditing(false);
    setFieldError(null);
    setConflict(null);
  };

  const save = (event: FormEvent) => {
    event.preventDefault();
    const invalid = validateExpression(draft);
    setFieldError(invalid);
    if (invalid || update.isPending || !canEdit) {
      return;
    }
    update.mutate(
      { id: task.id, expression: draft },
      {
        onSuccess: stopEditing,
        onError: (cause) => {
          if (cause instanceof ApiRequestError && cause.status === 409) {
            setConflict(cause.message);
            setEditing(false);
          } else {
            setFieldError(errorText(cause));
          }
        },
      },
    );
  };

  const confirmDelete = () => {
    setActionError(null);
    remove.mutate(task.id, { onError: (cause) => setActionError(errorText(cause)) });
  };

  const showSave = editing && canEdit && conflict === null;

  return (
    <tr className={styles.row} data-status={task.status}>
      <td className={styles.expression} data-label="Выражение">
        {editing || conflict !== null ? (
          <>
            {editing && <form id={formId} onSubmit={save} noValidate />}
            <label htmlFor={inputId} className={styles.hidden}>
              Выражение задачи
            </label>
            <input
              ref={inputRef}
              id={inputId}
              form={editing ? formId : undefined}
              className={styles.input}
              value={draft}
              readOnly={!editing}
              autoComplete="off"
              spellCheck={false}
              aria-invalid={fieldError !== null}
              aria-describedby={errorId}
              onChange={(event) => setDraft(event.target.value)}
            />
            <p id={errorId} className={styles.error} role="alert">
              {fieldError}
            </p>
          </>
        ) : (
          <code className={styles.mono}>{task.expression}</code>
        )}
      </td>
      <td data-label="Статус">
        <StatusBadge status={task.status} />
      </td>
      <td data-label="Результат">
        <span className={task.error ? styles.failed : styles.mono}>{outcome(task)}</span>
      </td>
      <td className={styles.time} data-label="Создана">
        <time dateTime={task.created_at}>{timeFormat.format(new Date(task.created_at))}</time>
      </td>
      <td>
        <div className={styles.actions}>
          {editing ? (
            <>
              {showSave && (
                <button type="submit" form={formId} disabled={update.isPending}>
                  Сохранить
                </button>
              )}
              <button type="button" onClick={stopEditing}>
                Отмена
              </button>
            </>
          ) : (
            <>
              {canEdit && (
                <button type="button" onClick={startEditing}>
                  Изменить
                </button>
              )}
              {canDelete && (
                <button type="button" onClick={confirmDelete} disabled={remove.isPending}>
                  Удалить
                </button>
              )}
            </>
          )}
          <p className={styles.error} role="alert">
            {conflict ?? actionError}
          </p>
        </div>
      </td>
    </tr>
  );
}
