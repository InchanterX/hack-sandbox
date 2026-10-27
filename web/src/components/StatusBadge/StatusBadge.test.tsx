/// <reference types="node" />
import { readFileSync } from 'node:fs';
import { join } from 'node:path';

import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { StatusBadge } from './StatusBadge';

const read = (relative: string): string => readFileSync(join(import.meta.dirname, relative), 'utf8');
const sheet = (name: string): string => read(`../${name}/${name}.module.css`);
const badgeCss = sheet('StatusBadge');
const listCss = sheet('TaskList');
const itemCss = sheet('TaskItem');
const formCss = sheet('ExpressionForm');
const pageCss = read('../../screens/Tasks/Tasks.module.css');

describe('StatusBadge', () => {
  it.each([
    ['queued', 'в очереди'],
    ['running', 'выполняется'],
    ['done', 'готово'],
    ['failed', 'ошибка'],
  ] as const)('labels %s as %s', (status, label) => {
    render(<StatusBadge status={status} />);
    expect(screen.getByText(label)).toBeInTheDocument();
  });
});

describe('styles', () => {
  it('animates the running dot and respects reduced motion', () => {
    expect(badgeCss).toMatch(/\.running \.dot\s*{[^}]*animation:/);
    expect(badgeCss).toMatch(/prefers-reduced-motion: reduce[\s\S]*animation: none/);
  });

  it('uses the status tokens', () => {
    for (const token of ['queued', 'running', 'done', 'failed']) {
      expect(badgeCss).toContain(`var(--color-${token})`);
    }
  });

  it('switches table to cards at 640 px and keeps a 720 px column', () => {
    expect(listCss).toContain('@media (max-width: 640px)');
    expect(itemCss).toContain('@media (max-width: 640px)');
    expect(pageCss).toContain('max-width: var(--content-width)');
  });

  it('uses mono for expressions and results and no raw colours', () => {
    expect(itemCss).toContain('var(--font-mono)');
    expect(formCss).toContain('var(--font-mono)');
    for (const text of [badgeCss, listCss, itemCss, formCss, pageCss]) {
      expect(text).not.toMatch(/#[0-9a-f]{3,8}\b/i);
    }
  });
});
