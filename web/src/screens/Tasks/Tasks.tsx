import { useAuth } from '../../auth/useAuth';
import { ExpressionForm } from '../../components/ExpressionForm/ExpressionForm';
import { RequestState } from '../../components/RequestState/RequestState';
import { TaskList } from '../../components/TaskList/TaskList';
import { errorText } from '../../hooks/apiError';
import { useTasks } from '../../hooks/useTasks';
import styles from './Tasks.module.css';

const LOADING_TEXT = 'Загрузка…';
const EMPTY_TEXT = 'Задач пока нет — отправьте первое выражение';

function TaskListSection({ userId }: { userId: string }) {
  const tasks = useTasks(userId);

  if (tasks.data === undefined) {
    return tasks.isError ? (
      <RequestState kind="error" message={errorText(tasks.error)} onRetry={() => void tasks.refetch()} />
    ) : (
      <RequestState kind="loading" message={LOADING_TEXT} />
    );
  }
  return (
    <>
      {tasks.isError && (
        <RequestState kind="error" message={errorText(tasks.error)} onRetry={() => void tasks.refetch()} />
      )}
      {tasks.data.length === 0 ? (
        <RequestState kind="empty" message={EMPTY_TEXT} />
      ) : (
        <TaskList tasks={tasks.data} />
      )}
    </>
  );
}

export function Tasks() {
  const { user, signOut } = useAuth();
  if (!user) {
    return null;
  }
  return (
    <main className={styles.page}>
      <header className={styles.header}>
        <h1 className={styles.title}>Calc Queue</h1>
        <span className={styles.login}>{user.login}</span>
        <button type="button" className={styles.logout} onClick={() => void signOut()}>
          Выйти
        </button>
      </header>
      <ExpressionForm />
      <TaskListSection key={user.id} userId={user.id} />
    </main>
  );
}
