import clsx from 'clsx';

import type { components } from '../../api/schema';
import styles from './StatusBadge.module.css';

type TaskStatus = components['schemas']['TaskStatus'];

/** The single status → label/colour map of the UI. */
const STATUS_LABELS: Record<TaskStatus, string> = {
  queued: 'в очереди',
  running: 'выполняется',
  done: 'готово',
  failed: 'ошибка',
};

export function StatusBadge({ status }: { status: TaskStatus }) {
  return (
    <span className={clsx(styles.badge, styles[status])} data-status={status} role="status" aria-atomic="true">
      <span className={styles.dot} aria-hidden="true" />
      {STATUS_LABELS[status]}
    </span>
  );
}
