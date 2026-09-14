import assert from 'node:assert/strict';
import { describe, test } from 'node:test';
import { createReadCoordinator, PUBLIC_DOCUMENT_KINDS, type ReadCoordinator } from './coordinator.ts';
import type { AnswerHandlers, DocumentKind } from './readCycle.ts';
import type { ObservedVersions } from './changes.ts';

/** An answer handler that accepts everything and covers whatever it is given. */
function plainHandlers(covered: ObservedVersions = new Map()): AnswerHandlers {
  return {
    accepts: () => true,
    observe: (value) => ({ value, covered }),
  };
}

function coordinator(session = 'session-1'): ReadCoordinator {
  const handlers = {
    vehicles: plainHandlers(),
    zones: plainHandlers(),
    tariffs: plainHandlers(),
    current: plainHandlers(),
    notifications: plainHandlers(),
  } satisfies Record<DocumentKind, AnswerHandlers>;

  return createReadCoordinator(session, handlers);
}

/** One complete read of a document: opened, answered, stored and closed the way a hook closes it. */
function firstRead(read: ReadCoordinator, document: DocumentKind, covered: ObservedVersions = new Map()): void {
  const ticket = read.begin(document);
  assert.notEqual(ticket, undefined);

  read.accepts(ticket!);
  read.cover(document, covered);
  read.stored(ticket!);
  read.settle(document);
}

describe('deciding whether there is anything to read', () => {
  // The first read of a resource is opened by the hook that holds it: reconciliation repairs what
  // is on screen, and a resource that has answered nothing has nothing to repair.
  test('a document that has never been read is read by its own hook, not by reconciliation', () => {
    const read = coordinator();

    assert.equal(read.due('vehicles'), false);
    assert.notEqual(read.begin('vehicles'), undefined);
  });

  test('a document that is waiting for an answer is not read again', () => {
    const read = coordinator();
    read.begin('vehicles');

    assert.equal(read.due('vehicles'), false);
  });

  test('a document that answered once is not read again for nothing', () => {
    const read = coordinator();
    firstRead(read, 'vehicles');

    assert.equal(read.due('vehicles'), false);
  });

  // A read that failed answers nothing, and the next reconciliation is what tries again.
  test('a document that never answered is read again', () => {
    const read = coordinator();
    read.begin('vehicles');
    read.settle('vehicles');

    assert.equal(read.due('vehicles'), true);
  });

  test('a request is read even though nothing changed', () => {
    const read = coordinator();
    firstRead(read, 'vehicles');
    read.request(['vehicles']);

    assert.equal(read.due('vehicles'), true);
    firstRead(read, 'vehicles');
    assert.equal(read.due('vehicles'), false, 'a request is answered once');
  });

  test('a ready frame asks every resource to be read again', () => {
    const read = coordinator();
    read.request(PUBLIC_DOCUMENT_KINDS);

    for (const document of PUBLIC_DOCUMENT_KINDS) {
      assert.equal(read.due(document), true);
      firstRead(read, document);
    }

    assert.equal(read.due('zones'), false);
  });
});

describe('remembering a change', () => {
  test('a change asks for a read of its resource', () => {
    const read = coordinator();
    firstRead(read, 'vehicles');
    read.observe('vehicles', 'v1', '42');

    assert.equal(read.due('vehicles'), true);
  });

  test('a repeated or older change asks for nothing more', () => {
    const read = coordinator();
    read.observe('vehicles', 'v1', '42');
    firstRead(read, 'vehicles', new Map([['v1', '42']]));

    read.observe('vehicles', 'v1', '42');
    read.observe('vehicles', 'v1', '41');

    assert.equal(read.due('vehicles'), false);
  });

  // A signal that arrives while a request is in flight is kept: the request may have been computed
  // before the change it is asking about, so it is not assumed to answer for it.
  test('a change that arrives during a read is read after it', () => {
    const read = coordinator();
    firstRead(read, 'vehicles');

    read.begin('vehicles');
    read.observe('vehicles', 'v1', '42');

    assert.equal(read.due('vehicles'), false, 'the read in flight is not restarted');

    read.cover('vehicles', new Map([['v1', '41']]));
    read.settle('vehicles');

    assert.equal(read.due('vehicles'), true, 'the answer did not publish the change');
  });

  test('a change the answer covers is not read for again', () => {
    const read = coordinator();
    read.observe('vehicles', 'v1', '42');
    firstRead(read, 'vehicles', new Map([['v1', '42']]));

    assert.equal(read.due('vehicles'), false);
  });

  // A version is remembered for the object it names, so a change to one vehicle is not answered by
  // a snapshot that does not contain that vehicle.
  test('a change to an object the answer does not have stays uncovered', () => {
    const read = coordinator();
    read.observe('zones', 'z9', '5');
    firstRead(read, 'zones', new Map([['z1', '5']]));

    assert.equal(read.due('zones'), true);
  });
});

describe('deciding whether an answer may still be stored', () => {
  test('the answer of the read that is running is stored', () => {
    const read = coordinator();
    const ticket = read.begin('vehicles');

    assert.equal(read.accepts(ticket!), true);
  });

  test('an answer of an ended session is not stored', () => {
    const read = coordinator('session-1');
    const ticket = read.begin('vehicles');

    assert.equal(read.accepts({ ...ticket!, session: 'session-2' }), false);
  });

  // The request that started earlier may answer later, and its answer must not replace one that is
  // already on screen.
  test('an answer of an earlier read is not stored once a later read has answered', () => {
    const read = coordinator();
    const first = read.begin('vehicles')!;
    read.settle('vehicles');

    const second = read.begin('vehicles')!;
    read.accepts(second);
    read.stored(second);

    assert.equal(read.accepts(first), false);
  });

  test('a read that answers twice is stored once', () => {
    const read = coordinator();
    const ticket = read.begin('vehicles')!;

    assert.equal(read.accepts(ticket), true);
    read.stored(ticket);

    assert.equal(read.accepts(ticket), false);
  });

  test('an answer of a read that has been superseded is not stored', () => {
    const read = coordinator();
    const first = read.begin('vehicles')!;
    read.settle('vehicles');
    const second = read.begin('vehicles')!;

    assert.equal(read.accepts(first), false, 'a later read is the one being waited for');
    assert.equal(read.accepts(second), true);
  });

  test('a ticket from another document is judged by its own document', () => {
    const read = coordinator();
    const vehicles = read.begin('vehicles')!;
    const zones = read.begin('zones')!;

    read.accepts(vehicles);
    read.stored(vehicles);

    assert.equal(read.accepts(zones), true);
  });
});

describe('waking the reader of a document', () => {
  // A request is what starts a read: a reader watches how many have been asked for, so a signal
  // that arrives while an answer is still on its way is read rather than waiting for the next
  // reconciliation.
  test('a request wakes the watcher and counts as one read', () => {
    const read = coordinator();
    let woken = 0;
    read.watch('vehicles', () => {
      woken += 1;
    });

    read.force('vehicles');
    assert.equal(woken, 1);
    assert.equal(read.asked('vehicles'), 1);

    read.request(PUBLIC_DOCUMENT_KINDS);
    assert.equal(woken, 2, 'a request of every document wakes each of their readers');
    assert.equal(read.asked('vehicles'), 2);
    assert.equal(read.asked('zones'), 1);
  });

  test('a document nobody watches is still asked for', () => {
    const read = coordinator();
    assert.equal(read.asked('tariffs'), 0);
    read.request(['tariffs']);
    assert.equal(read.asked('tariffs'), 1);
    assert.equal(read.due('tariffs'), true);
  });

  // Reading a document is not a request, so a reader that watches requests does not wake itself in
  // a loop: the count only moves when somebody asks.
  test('beginning and settling a read ask for nothing', () => {
    const read = coordinator();
    let woken = 0;
    read.watch('vehicles', () => {
      woken += 1;
    });

    firstRead(read, 'vehicles');
    assert.equal(woken, 0);
    assert.equal(read.asked('vehicles'), 0);
  });

  test('a watcher that has stopped is not woken', () => {
    const read = coordinator();
    let woken = 0;
    const stop = read.watch('vehicles', () => {
      woken += 1;
    });
    stop();

    read.force('vehicles');
    assert.equal(woken, 0);
    assert.equal(read.asked('vehicles'), 1);
  });
});
