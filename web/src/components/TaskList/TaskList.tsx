import type { Task } from '../../hooks/useTasks';
import { TaskItem } from '../TaskItem/TaskItem';
import styles from './TaskList.module.css';

/** Tasks as a table; at <= 640 px the same markup is laid out as cards. */
export function TaskList({ tasks }: { tasks: Task[] }) {
  return (
    <table className={styles.table}>
      <caption className={styles.caption}>Ваши задачи</caption>
      <thead className={styles.head}>
        <tr>
          <th scope="col">Выражение</th>
          <th scope="col">Статус</th>
          <th scope="col">Результат</th>
          <th scope="col">Создана</th>
          <th scope="col">Действия</th>
        </tr>
      </thead>
      <tbody>
        {tasks.map((task) => (
          <TaskItem key={task.id} task={task} />
        ))}
      </tbody>
    </table>
  );
}
