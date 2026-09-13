import { useState } from 'react';
import type { Tariff, Vehicle } from '../../shared/api/catalog';
import type { RateText } from '../reservation/reservationCopy';
import {
  BOOK_ACTION,
  CONFIRM_ACTION,
  CONFIRM_HEADING,
  FREE_PERIOD,
  FREE_RESERVATION_WARNING,
  KEEP_ACTION,
  rateTextOf,
} from '../reservation/reservationCopy';
import {
  POWERTRAIN_LABELS,
  sourceText,
  statusText,
  telemetryText,
  UNAVAILABLE_REASON_LABELS,
  unavailableReasons,
} from './fleetCopy';
import { ResourceNotice, type AbsenceCopy } from './ResourceNotice';
import type { Found } from './useCatalog';

/** What the card says when the vehicle has dropped out of the current result. */
const OUTSIDE_FILTERS = 'Автомобиль больше не соответствует фильтрам';

/** What the card says when the vehicle cannot be booked at all, because it is not free. */
const NOT_FREE = 'Забронировать можно только свободный автомобиль';

/**
 * What the card says instead of a price, and why there is none to show. A tariff that is still on
 * its way is not the same as one the operator published none of, and neither is a price of zero.
 */
const TARIFF_ABSENCE: AbsenceCopy = {
  loading: 'Загружаем тариф…',
  none: 'Тариф временно недоступен',
  unreachable: 'Тариф временно недоступен',
};

/**
 * What the card needs to offer a booking: whether the person may book at all, what the day's
 * allowance says, and where the last command stands. The card decides nothing of this itself; it
 * shows what the account and the server said.
 */
export type VehicleBooking = {
  signedIn: boolean;
  limit: string;
  limitAllows: boolean;
  awaitingRepeat: boolean;
  notice?: string;
  book: (vehicleId: string, shownRates: RateText) => void;
};

type VehicleCardProps = {
  vehicle: Vehicle;
  tariff: Found<Tariff>;
  onRetryTariff: () => void;
  withinFilters: boolean;
  onClose: () => void;
  booking: VehicleBooking;
};

/**
 * VehicleCard is what a person opened a vehicle to read. It stays open when a reload or a changed
 * filter drops the vehicle out of the result: the vehicle leaves the list and the map, and the card
 * says why it did, until the person closes it.
 *
 * Booking is a step of its own: the conditions are shown and agreed to before anything is sent, and
 * once sent the control waits for the server rather than for a second press.
 */
export function VehicleCard({ vehicle, tariff, onRetryTariff, withinFilters, onClose, booking }: VehicleCardProps) {
  const [confirming, setConfirming] = useState(false);
  const rates = tariff.state === 'found' ? rateTextOf(tariff.value) : undefined;

  return (
    <aside className="vehicle-card" aria-label={`Автомобиль ${vehicle.model}`}>
      <header className="vehicle-card-header">
        <div>
          <h2 className="vehicle-card-model">{vehicle.model}</h2>
          <p className="vehicle-card-type">{POWERTRAIN_LABELS[vehicle.powertrain_type]}</p>
        </div>
        <button className="vehicle-card-close" type="button" onClick={onClose} aria-label="Закрыть карточку">
          ✕
        </button>
      </header>

      {!withinFilters && <p className="vehicle-card-notice">{OUTSIDE_FILTERS}</p>}

      <p className="vehicle-card-status" data-status={vehicle.status}>
        {statusText(vehicle)}
      </p>
      <p className="vehicle-card-telemetry">{telemetryText(vehicle)}</p>

      <UnavailableReasons vehicle={vehicle} />

      <section className="vehicle-card-section">
        <h3 className="vehicle-card-heading">Источники энергии</h3>
        <ul className="vehicle-card-sources">
          {vehicle.energy_sources.map((source) => (
            <li className="vehicle-card-source" key={source.kind}>
              {sourceText(source)}
            </li>
          ))}
        </ul>
      </section>

      <section className="vehicle-card-section">
        <h3 className="vehicle-card-heading">Тариф</h3>
        <ResourceNotice found={tariff} copy={TARIFF_ABSENCE} onRetry={onRetryTariff} />
        {rates !== undefined && <TariffRates rates={rates} />}
      </section>

      <Booking
        vehicle={vehicle}
        rates={rates}
        booking={booking}
        confirming={confirming}
        onConfirm={() => setConfirming(true)}
        onKeep={() => setConfirming(false)}
      />
    </aside>
  );
}

function UnavailableReasons({ vehicle }: { vehicle: Vehicle }) {
  const reasons = unavailableReasons(vehicle);
  if (reasons.length === 0) return null;

  return (
    <section className="vehicle-card-section">
      <h3 className="vehicle-card-heading">Причины недоступности</h3>
      <ul className="vehicle-card-reasons">
        {reasons.map((reason) => (
          <li className="vehicle-card-reason" key={reason}>
            {UNAVAILABLE_REASON_LABELS[reason]}
          </li>
        ))}
      </ul>
    </section>
  );
}

/**
 * Booking is the step that turns an open card into a reservation. A free vehicle is offered one
 * control; pressing it shows what a person is agreeing to — the vehicle, both rates, the free period
 * and the warning that the day is not given back — and only the confirming control sends anything.
 */
function Booking({
  vehicle,
  rates,
  booking,
  confirming,
  onConfirm,
  onKeep,
}: {
  vehicle: Vehicle;
  rates: RateText | undefined;
  booking: VehicleBooking;
  confirming: boolean;
  onConfirm: () => void;
  onKeep: () => void;
}) {
  if (vehicle.status !== 'available') {
    return <p className="vehicle-card-later">{NOT_FREE}</p>;
  }
  if (rates === undefined) {
    return <p className="vehicle-card-later">{TARIFF_ABSENCE.none}</p>;
  }
  if (!booking.signedIn) {
    return <p className="vehicle-card-later">{booking.limit}</p>;
  }

  if (confirming) {
    return (
      <section className="vehicle-card-booking" aria-label={CONFIRM_HEADING}>
        <h3 className="vehicle-card-heading">{CONFIRM_HEADING}</h3>
        <p className="vehicle-card-booking-vehicle">{vehicle.model}</p>
        <TariffRates rates={rates} />
        <p className="vehicle-card-booking-period">{FREE_PERIOD}</p>
        <p className="vehicle-card-warning">{FREE_RESERVATION_WARNING}</p>
        <div className="vehicle-card-booking-actions">
          <button
            className="action-button"
            type="button"
            disabled={!booking.limitAllows || booking.awaitingRepeat}
            onClick={() => booking.book(vehicle.id, rates)}
          >
            {CONFIRM_ACTION}
          </button>
          <button className="action-button" type="button" onClick={onKeep}>
            {KEEP_ACTION}
          </button>
        </div>
        {booking.notice !== undefined && <p className="vehicle-card-notice">{booking.notice}</p>}
      </section>
    );
  }

  return (
    <div className="vehicle-card-booking">
      <button
        className="action-button"
        type="button"
        disabled={!booking.limitAllows || booking.awaitingRepeat}
        onClick={onConfirm}
      >
        {BOOK_ACTION}
      </button>
      <p className="vehicle-card-limit">{booking.limit}</p>
      {booking.notice !== undefined && <p className="vehicle-card-notice">{booking.notice}</p>}
    </div>
  );
}

/**
 * The rates the service published as whole tyiyn. A value the interface cannot read as a price is
 * left out rather than repaired, because a made-up price is worse than a missing one.
 */
function TariffRates({ rates }: { rates: RateText }) {
  if (rates.driving === undefined || rates.paused === undefined) {
    return <p className="vehicle-card-tariff-missing">{TARIFF_ABSENCE.none}</p>;
  }

  return (
    <dl className="vehicle-card-tariff">
      <dt>Движение</dt>
      <dd>{rates.driving} за начатую минуту</dd>
      <dt>Пауза</dt>
      <dd>{rates.paused} за начатую минуту</dd>
    </dl>
  );
}
