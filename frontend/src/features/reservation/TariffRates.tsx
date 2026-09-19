import type { RateText } from './reservationCopy.ts';
import { pricedRates, RATE_UNIT, TARIFF_MISSING } from './reservationCopy.ts';

/** What each mode of a rental is called, and which rate of the stored snapshot it names. */
const RATE_ROWS: { mode: keyof RateText; title: string }[] = [
  { mode: 'driving', title: 'Движение' },
  { mode: 'paused', title: 'Пауза' },
];

/**
 * TariffRates is what a rental costs, as the snapshot the rental was made under states it. A rate the
 * interface cannot read is left out and said to be missing rather than shown as zero, and a reservation,
 * a ride in force and a vehicle card all read their rates from here, so one rental cannot be priced two
 * ways on one screen.
 */
export function TariffRates({ rates }: { rates: RateText }) {
  const priced = pricedRates(rates);
  if (priced === undefined) {
    return <p className="tariff-rates-missing">{TARIFF_MISSING}</p>;
  }

  return (
    <dl className="tariff-rates">
      {RATE_ROWS.map((row) => (
        <Row key={row.mode} title={row.title} rate={priced[row.mode]} />
      ))}
    </dl>
  );
}

function Row({ title, rate }: { title: string; rate: string }) {
  return (
    <>
      <dt>{title}</dt>
      <dd>{`${rate} ${RATE_UNIT}`}</dd>
    </>
  );
}
