// Binding generators shared by `npm run generate` and the drift check.
// Both read contracts/openapi.yaml and write to the given output file.
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { parse, stringify } from 'yaml';
import { contractDir, mustRun, readText, repoPath } from '../test/helpers.ts';

export const tsTarget = 'web/src/api/schema.d.ts';
export const goTarget = 'server/internal/openapi/openapi.gen.go';

/** openapi-typescript 7, pinned in this package's lockfile. */
export function generateTs(outFile: string): void {
  const cli = path.join(contractDir, 'node_modules/openapi-typescript/bin/cli.js');
  mustRun(process.execPath, [cli, repoPath('contracts/openapi.yaml'), '-o', outFile], repoPath('web'));
}

/** oapi-codegen v2.8.0, pinned by the tool directive of server/go.mod. */
// The committed config's `output` wins over -o, so a copy points it at outFile.
export function generateGo(outFile: string): void {
  const config = parse(readText('server/internal/openapi/oapi-codegen.yaml')) as Record<string, unknown>;
  const dir = mkdtempSync(path.join(tmpdir(), 'oapi-codegen-'));
  try {
    const configFile = path.join(dir, 'oapi-codegen.yaml');
    writeFileSync(configFile, stringify({ ...config, output: path.resolve(outFile) }));
    mustRun('go', ['tool', 'oapi-codegen', '-config', configFile, '../contracts/openapi.yaml'], repoPath('server'));
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}
