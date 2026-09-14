import assert from 'node:assert/strict';
import { describe, test } from 'node:test';
import {
  clearUnfinished,
  REPEAT_WINDOW_MILLISECONDS,
  repeatableCommand,
  storeUnfinished,
  withinRepeatWindow,
  type CommandStorage,
  type UnfinishedCommand,
} from './unfinishedCommand.ts';

/** A storage of one test, so the checks below never touch a real browser. */
function memoryStorage(initial?: string): CommandStorage & { held: string | null } {
  const storage = {
    held: initial ?? null,
    getItem: () => storage.held,
    setItem: (_key: string, value: string) => {
      storage.held = value;
    },
    removeItem: () => {
      storage.held = null;
    },
  };
  return storage;
}

const sentAt = Date.parse('2026-09-13T07:15:30.000Z');

function command(overrides: Partial<UnfinishedCommand> = {}): UnfinishedCommand {
  return {
    owner: 'owner-1',
    action: 'reserve',
    parameters: { vehicleId: 'vehicle-1' },
    key: '11111111-1111-4111-8111-111111111111',
    sentAt,
    ...overrides,
  };
}

describe('the command a browser remembers', () => {
  test('keeps the parameters and the original key of one attempt', () => {
    const storage = memoryStorage();
    storeUnfinished(command(), storage);

    const held = repeatableCommand('owner-1', sentAt + 60_000, storage);
    assert.equal(held?.key, '11111111-1111-4111-8111-111111111111');
    assert.equal(held?.action, 'reserve');
    assert.deepEqual(held?.parameters, { vehicleId: 'vehicle-1' });
  });

  test('is repeatable until its window has passed, and not a moment longer', () => {
    const storage = memoryStorage();
    storeUnfinished(command(), storage);

    assert.ok(withinRepeatWindow(command(), sentAt + REPEAT_WINDOW_MILLISECONDS - 1));
    assert.equal(withinRepeatWindow(command(), sentAt + REPEAT_WINDOW_MILLISECONDS), false);
    assert.equal(repeatableCommand('owner-1', sentAt + REPEAT_WINDOW_MILLISECONDS, storage), undefined);
  });

  test('is not carried to another account', () => {
    const storage = memoryStorage();
    storeUnfinished(command(), storage);

    assert.equal(repeatableCommand('owner-2', sentAt + 1_000, storage), undefined);
  });

  test('names each ride command with the rental it moves', () => {
    for (const action of ['start', 'pause', 'resume'] as const) {
      const storage = memoryStorage();
      storeUnfinished(command({ action, parameters: { rentalId: 'rental-1' } }), storage);

      const held = repeatableCommand('owner-1', sentAt + 1_000, storage);
      assert.equal(held?.action, action);
      assert.deepEqual(held?.parameters, { rentalId: 'rental-1' });
    }
  });

  test('is forgotten once its outcome is known', () => {
    const storage = memoryStorage();
    storeUnfinished(command(), storage);
    clearUnfinished(storage);

    assert.equal(repeatableCommand('owner-1', sentAt + 1_000, storage), undefined);
  });

  test('is refused rather than repaired when the record cannot be read', () => {
    assert.equal(repeatableCommand('owner-1', sentAt, memoryStorage('{')), undefined);
    assert.equal(repeatableCommand('owner-1', sentAt, memoryStorage('{"owner":"owner-1"}')), undefined);
    assert.equal(
      repeatableCommand('owner-1', sentAt, memoryStorage('{"owner":"owner-1","key":"k","sentAt":"now"}')),
      undefined,
    );
  });

  test('is refused when the record names an action this browser does not send', () => {
    const stored = '{"owner":"owner-1","action":"finish","key":"k","sentAt":1}';

    assert.equal(repeatableCommand('owner-1', sentAt, memoryStorage(stored)), undefined);
  });

  test('costs the repeat, not the command, when storage refuses the write', () => {
    const refusing: CommandStorage = {
      getItem: () => null,
      setItem: () => {
        throw new Error('storage is disabled');
      },
      removeItem: () => undefined,
    };

    storeUnfinished(command(), refusing);
    assert.equal(repeatableCommand('owner-1', sentAt + 1_000, refusing), undefined);
  });
});
