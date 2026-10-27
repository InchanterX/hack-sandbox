import styles from './RequestState.module.css';

interface RequestStateProps {
  kind: 'loading' | 'empty' | 'error';
  message: string;
  onRetry?: () => void;
}

/** Loading, empty and error placeholder of a request; announced politely, never steals focus. */
export function RequestState({ kind, message, onRetry }: RequestStateProps) {
  const isError = kind === 'error';
  return (
    <div className={isError ? styles.error : styles.state} role={isError ? 'alert' : 'status'}>
      <p className={styles.message}>{message}</p>
      {isError && onRetry && (
        <button type="button" className={styles.retry} onClick={onRetry}>
          Повторить
        </button>
      )}
    </div>
  );
}
