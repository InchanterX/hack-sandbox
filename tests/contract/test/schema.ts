// Ajv (JSON Schema 2020-12, the OpenAPI 3.1 dialect) over the contract components.
import { Ajv2020 } from 'ajv/dist/2020.js';
import addFormatsModule from 'ajv-formats';
import type { OpenAPIDoc } from './helpers.ts';

const addFormats = addFormatsModule as unknown as (ajv: Ajv2020) => Ajv2020;

export const DOC_ID = 'https://contract.local/openapi.json';

export function compileValidator(doc: OpenAPIDoc): (ref: string, value: unknown) => string | null {
  const ajv = new Ajv2020({ strict: false, allErrors: true });
  addFormats(ajv);
  ajv.addFormat('password', true);
  ajv.addFormat('double', true);
  ajv.addSchema({ $id: DOC_ID, components: doc.components });
  return (ref, value) => {
    const validate = ajv.getSchema(`${DOC_ID}#${ref}`);
    if (!validate) {
      throw new Error(`unknown schema ${ref}`);
    }
    return validate(value) ? null : ajv.errorsText(validate.errors);
  };
}
