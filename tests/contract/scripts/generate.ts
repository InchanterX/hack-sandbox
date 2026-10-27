// Regenerates the committed Go and TypeScript bindings from contracts/openapi.yaml.
import { goTarget, generateGo, generateTs, tsTarget } from './gen.ts';
import { repoPath } from '../test/helpers.ts';

generateTs(repoPath(tsTarget));
generateGo(repoPath(goTarget));
console.log(`generated ${tsTarget} and ${goTarget}`);
