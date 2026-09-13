import type { EnergySource, PowertrainType, SourceKind, UnavailableReason, Vehicle } from '../../shared/api/catalog';
import { INTERFACE_LOCALE } from '../../shared/locale';
import type { VehicleStatus } from './filters';

/** How each powertrain is named in the interface. */
export const POWERTRAIN_LABELS: Record<PowertrainType, string> = {
  electric: 'Электро',
  gasoline: 'Бензин',
  diesel: 'Дизель',
  hybrid: 'Гибрид',
  gas: 'Газ',
};

/** How each public state is named in the interface. */
export const STATUS_LABELS: Record<VehicleStatus, string> = {
  available: 'Свободен',
  reserved: 'Забронирован',
  in_trip: 'В поездке',
  unavailable: 'Недоступен',
};

const RIDE_MODE_LABELS: Record<'driving' | 'paused', string> = {
  driving: 'движение',
  paused: 'пауза',
};

/** How each energy source is named, and the unit its inventory is measured in. */
const SOURCE_LABELS: Record<SourceKind, string> = {
  battery: 'Батарея',
  gasoline: 'Бензин',
  diesel: 'Дизель',
  lpg: 'Сжиженный газ',
  cng: 'Сжатый газ',
};

const UNIT_LABELS: Record<EnergySource['unit'], string> = {
  wh: 'Вт·ч',
  ml: 'мл',
  g: 'г',
};

/** How the freshness of the last confirmed position is described. */
export const TELEMETRY_LABELS: Record<Vehicle['telemetry_status'], string> = {
  fresh: 'Телеметрия свежая',
  stale: 'Телеметрия устарела',
  offline: 'Нет связи с автомобилем',
};

/**
 * Why a vehicle cannot be taken. Every reason the service reports is shown, so the wording of each
 * one describes only itself and never implies it is the only one.
 */
export const UNAVAILABLE_REASON_LABELS: Record<UnavailableReason, string> = {
  insufficient_energy: 'Недостаточно запаса энергии',
  telemetry_stale: 'Устаревшая телеметрия',
  outside_service_zone: 'Вне зоны обслуживания',
  service_required: 'Требуется обслуживание',
  technical_unavailable: 'Нет связи с транспортом',
};

/** The state of a vehicle as a person reads it, with the ride mode when there is one. */
export function statusText(vehicle: Vehicle): string {
  if (vehicle.status !== 'in_trip') return STATUS_LABELS[vehicle.status];
  return `${STATUS_LABELS.in_trip} · ${RIDE_MODE_LABELS[vehicle.ride_mode]}`;
}

/** One energy source written out: what it is, how full it is and how much is left. */
export function sourceText(source: EnergySource): string {
  return `${SOURCE_LABELS[source.kind]} · ${percentText(source)} · ${source.remaining} ${UNIT_LABELS[source.unit]}`;
}

const BASIS_POINTS_IN_A_PERCENT = 100;

function percentText(source: EnergySource): string {
  return `${(source.remaining_basis_points / BASIS_POINTS_IN_A_PERCENT).toFixed(1)} %`;
}

const confirmedAtFormat = new Intl.DateTimeFormat(INTERFACE_LOCALE, {
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
});

/**
 * How much the published position can be trusted, and when the vehicle last confirmed it. The
 * moment is shown in the reader's own zone, because it answers "how long ago", not "at what time
 * in Bishkek".
 */
export function telemetryText(vehicle: Vehicle): string {
  const label = TELEMETRY_LABELS[vehicle.telemetry_status];
  const confirmedAt = new Date(vehicle.telemetry_at);
  if (Number.isNaN(confirmedAt.valueOf())) return label;
  return `${label} · подтверждено в ${confirmedAtFormat.format(confirmedAt)}`;
}

export function unavailableReasons(vehicle: Vehicle): readonly UnavailableReason[] {
  return vehicle.status === 'unavailable' ? vehicle.unavailable_reasons : [];
}
