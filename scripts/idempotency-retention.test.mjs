import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { describe, test } from 'node:test';

const CONTRACT = await readFile(new URL('../openapi/public.yaml', import.meta.url), 'utf8');
const SERVICE_STORE = await readFile(new URL('../backend/internal/idempotency/store.go', import.meta.url), 'utf8');
const BROWSER_COMMAND = await readFile(
  new URL('../frontend/src/shared/command/unfinishedCommand.ts', import.meta.url),
  'utf8',
);

function declaredRetentionSeconds() {
  const operationBlocks = [...CONTRACT.matchAll(/^    (?:post|put|patch|delete):\n([\s\S]*?)(?=^    \w|\Z)/gm)].map(
    ([, operation]) => operation,
  );
  const idempotentOperations = operationBlocks.filter((operation) => operation.includes('name: Idempotency-Key'));
  assert.ok(idempotentOperations.length > 0, 'the contract declares no idempotent operation');

  const retentionSeconds = idempotentOperations.map((operation) => {
    const declared = operation.match(/x-idempotency-retention-seconds:\s*(\d+)/);
    assert.ok(declared, 'an operation with Idempotency-Key declares no retention');
    return Number(declared[1]);
  });
  assert.equal(new Set(retentionSeconds).size, 1, 'idempotent operations declare different retention windows');

  return retentionSeconds[0];
}

describe('the idempotency retention', () => {
  test('is the same in the contract, service and browser', () => {
    const service = SERVICE_STORE.match(/Retention\s*=\s*([\d_]+)\s*\*\s*time\.Second/);
    assert.ok(service, 'the Go idempotency retention is not stated in seconds');
    const serviceSeconds = Number(service[1].replaceAll('_', ''));

    const browser = BROWSER_COMMAND.match(/REPEAT_WINDOW_MILLISECONDS\s*=\s*([\d_]+)\s*\*\s*([\d_]+)/);
    assert.ok(browser, 'the browser repeat window is not stated in seconds and milliseconds');
    const browserSeconds = (Number(browser[1].replaceAll('_', '')) * Number(browser[2].replaceAll('_', ''))) / 1_000;

    const contractSeconds = declaredRetentionSeconds();
    assert.equal(serviceSeconds, contractSeconds, 'the service stores a result for another window');
    assert.equal(browserSeconds, contractSeconds, 'the browser repeats a command for another window');
  });
});
