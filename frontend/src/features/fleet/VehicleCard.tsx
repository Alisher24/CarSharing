import type { Tariff, Vehicle } from '../../shared/api/catalog';
import {
  POWERTRAIN_LABELS,
  sourceText,
  statusText,
  telemetryText,
  UNAVAILABLE_REASON_LABELS,
  unavailableReasons,
} from './fleetCopy';
import { somText } from './money';
import { ResourceNotice, type AbsenceCopy } from './ResourceNotice';
import type { Found } from './useCatalog';

/** What the card says while the command that will use it does not exist yet. */
const RESERVATION_LATER = 'Бронирование появится позже';

/** What the card says when the vehicle has dropped out of the current result. */
const OUTSIDE_FILTERS = 'Автомобиль больше не соответствует фильтрам';

/**
 * What the card says instead of a price, and why there is none to show. A tariff that is still on
 * its way is not the same as one the operator published none of, and neither is a price of zero.
 */
const TARIFF_ABSENCE: AbsenceCopy = {
  loading: 'Загружаем тариф…',
  none: 'Тариф временно недоступен',
  unreachable: 'Тариф временно недоступен',
};

type VehicleCardProps = {
  vehicle: Vehicle;
  tariff: Found<Tariff>;
  onRetryTariff: () => void;
  withinFilters: boolean;
  onClose: () => void;
};

/**
 * VehicleCard is what a person opened a vehicle to read. It stays open when a reload or a changed
 * filter drops the vehicle out of the result: the vehicle leaves the list and the map, and the card
 * says why it did, until the person closes it.
 */
export function VehicleCard({ vehicle, tariff, onRetryTariff, withinFilters, onClose }: VehicleCardProps) {
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
        {tariff.state === 'found' && <TariffRates tariff={tariff.value} />}
      </section>

      <p className="vehicle-card-later">{RESERVATION_LATER}</p>
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
 * The rates the service published as whole tyiyn. A value the interface cannot read as a price is
 * left out rather than repaired, because a made-up price is worse than a missing one.
 */
function TariffRates({ tariff }: { tariff: Tariff }) {
  const driving = somText(tariff.driving_rate_tyiyn_per_started_minute);
  const paused = somText(tariff.paused_rate_tyiyn_per_started_minute);
  if (driving === undefined || paused === undefined) {
    return <p className="vehicle-card-tariff-missing">{TARIFF_ABSENCE.none}</p>;
  }

  return (
    <dl className="vehicle-card-tariff">
      <dt>Движение</dt>
      <dd>{driving} за начатую минуту</dd>
      <dt>Пауза</dt>
      <dd>{paused} за начатую минуту</dd>
    </dl>
  );
}
