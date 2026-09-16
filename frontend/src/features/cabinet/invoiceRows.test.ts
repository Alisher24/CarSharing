import assert from 'node:assert/strict';
import { describe, test } from 'node:test';
import type { InvoiceView, Payment } from '../../shared/api/current.ts';
import { invoiceLineRows, invoiceRow } from './invoiceRows.ts';

// The interface locale groups thousands with a non-breaking space, which is what these checks state
// rather than the ordinary space it looks like.
const GROUP_SEPARATOR = '\u00a0';

function view(overrides: { payment?: Payment; total?: string } = {}): InvoiceView {
  return {
    version: '1',
    invoice: {
      id: 'invoice-1',
      rental_id: 'ride-1',
      issued_at: '2026-09-12T07:45:30.123456Z',
      currency: 'KGS',
      billing_policy: 'per_mode_started_minute_v1',
      completion: { reason: 'user_finished' },
      lines: [
        {
          mode: 'driving',
          duration_microseconds: '1500000000',
          billed_started_minutes: '1250',
          rate_tyiyn_per_started_minute: '1500',
          amount_tyiyn: '1875000',
        },
        {
          mode: 'paused',
          duration_microseconds: '0',
          billed_started_minutes: '0',
          rate_tyiyn_per_started_minute: '500',
          amount_tyiyn: '0',
        },
      ],
      total_amount_tyiyn: overrides.total ?? '1875000',
    },
    payment: overrides.payment ?? { status: 'pending', updated_at: '2026-09-12T07:45:30.123456Z' },
  };
}

describe('one row of the invoice feed', () => {
  test('states when it was issued, what it came to and where paying it stands', () => {
    const row = invoiceRow(view());

    assert.equal(row.id, 'invoice-1');
    assert.equal(row.issuedAt, '12 сентября 2026 г. в 13:45');
    assert.equal(row.total, `18${GROUP_SEPARATOR}750,00 сома`);
    assert.equal(row.payment, 'Ожидает оплаты');
  });

  test('a settled invoice states that it is settled', () => {
    const row = invoiceRow(view({ payment: { status: 'paid', paid_at: '2026-09-12T08:00:00.000000Z' } }));

    assert.equal(row.payment, 'Оплачено');
  });

  // The total the service published is the amount a person is charged, so an amount this build
  // cannot read is written as missing rather than added up from the lines beside it.
  test('a total that cannot be read is written as missing', () => {
    assert.equal(invoiceRow(view({ total: 'not an amount' })).total, '—');
  });
});

describe('the lines of one invoice', () => {
  test('are the two modes, each with its minutes, its rate and what it cost', () => {
    const [driving, paused] = invoiceLineRows(view());

    assert.deepEqual(driving, {
      mode: 'Движение',
      minutes: `1${GROUP_SEPARATOR}250`,
      rate: '15,00 сома за начатую минуту',
      amount: `18${GROUP_SEPARATOR}750,00 сома`,
    });
    assert.deepEqual(paused, {
      mode: 'Пауза',
      minutes: '0',
      rate: '5,00 сома за начатую минуту',
      amount: '0,00 сома',
    });
  });
});
