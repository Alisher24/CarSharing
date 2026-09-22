import { INTERFACE_LOCALE } from '../locale.ts';
import { clockMoment } from '../time.ts';
import type { EnergySource, PowertrainType, SourceKind, UnavailableReason, Vehicle } from '../api/catalog.ts';
import { RIDE_MODE_TEXT } from '../ride/pace.ts';

/**
 * The Russian wording of one vehicle, and the rules that produce it: how each value the contract
 * publishes is named, and what a reading of the vehicle states. It is declared here rather than in
 * one view because a map marker, a list row, a card and a summary strip all name the same vehicle.
 */

/** How each powertrain is named in the interface. */
export const POWERTRAIN_LABELS: Record<PowertrainType, string> = {
  electric: 'Электро',
  gasoline: 'Бензин',
  diesel: 'Дизель',
  hybrid: 'Гибрид',
  gas: 'Газ',
};

/**
 * Every public state of a vehicle, in the order the interface offers them. The list is the one
 * declaration: the type is read from it, so the states a person may filter by and the states the
 * legend names cannot drift from the contract's own values.
 */
export const STATUSES = ['available', 'reserved', 'in_trip', 'unavailable'] as const;

/** The public state of a vehicle, which is every state the contract publishes for one. */
export type VehicleStatus = (typeof STATUSES)[number];

/** The powertrains a person can choose between, in the order they are offered. */
export const POWERTRAIN_TYPES = ['electric', 'gasoline', 'diesel', 'hybrid', 'gas'] as const;

/** How each public state is named in the interface. */
export const STATUS_LABELS: Record<VehicleStatus, string> = {
  available: 'Свободен',
  reserved: 'Забронирован',
  in_trip: 'В поездке',
  unavailable: 'Недоступен',
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

/** How each energy source is named, and the unit its inventory is measured in. */
const SOURCE_LABELS: Record<SourceKind, string> = {
  battery: 'Батарея',
  gasoline: 'Бензин',
  diesel: 'Дизель',
  lpg: 'Сжиженный газ',
  cng: 'Сжатый газ',
};

/** What a source of a kind this build does not know is called. */
const SOURCE_UNKNOWN = 'неизвестный источник';

/** How the freshness of the last confirmed position is described. */
export const TELEMETRY_LABELS: Record<Vehicle['telemetry_status'], string> = {
  fresh: 'Телеметрия свежая',
  stale: 'Телеметрия устарела',
  offline: 'Нет связи с автомобилем',
};

const UNIT_LABELS: Record<EnergySource['unit'], string> = {
  wh: 'Вт·ч',
  ml: 'мл',
  g: 'г',
};

const BASIS_POINTS_IN_A_PERCENT = 100;

/**
 * What one energy source is called inside a sentence, rather than as the label of a value, which is
 * how the sources of a vehicle that ran out are listed. A kind this build does not know is answered
 * as an unknown source, because a name that is not in the table cannot be written in lower case.
 */
export function sourceName(kind: SourceKind): string {
  return (SOURCE_LABELS[kind] ?? SOURCE_UNKNOWN).toLocaleLowerCase(INTERFACE_LOCALE);
}

/** The state of a vehicle as a person reads it, with the ride mode when there is one. */
export function statusText(vehicle: Vehicle): string {
  if (vehicle.status !== 'in_trip') return STATUS_LABELS[vehicle.status];

  return `${STATUS_LABELS.in_trip} · ${RIDE_MODE_TEXT[vehicle.ride_mode]}`;
}

/** One energy source written out: what it is, how full it is and how much is left. */
export function sourceText(source: EnergySource): string {
  return `${SOURCE_LABELS[source.kind]} · ${percentText(source)} · ${source.remaining} ${UNIT_LABELS[source.unit]}`;
}

/**
 * How much the published position can be trusted, and when the vehicle last confirmed it. The
 * moment is shown in the reader's own zone, because it answers "how long ago", not "at what time
 * in Bishkek".
 */
export function telemetryText(vehicle: Vehicle): string {
  const label = TELEMETRY_LABELS[vehicle.telemetry_status];
  const confirmedAt = new Date(vehicle.telemetry_at);
  if (Number.isNaN(confirmedAt.valueOf())) return label;

  return `${label} · подтверждено в ${clockMoment(confirmedAt)}`;
}

/** The reasons a vehicle states for being untakeable, and none for any other state of it. */
export function unavailableReasons(vehicle: Vehicle): readonly UnavailableReason[] {
  return vehicle.status === 'unavailable' ? vehicle.unavailable_reasons : [];
}

function percentText(source: EnergySource): string {
  return `${(source.remaining_basis_points / BASIS_POINTS_IN_A_PERCENT).toFixed(1)} %`;
}
