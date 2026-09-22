import type { Vehicle } from '../../shared/api/catalog.ts';
import { POWERTRAIN_LABELS, statusText } from '../../shared/vehicle/spell.ts';

type VehicleListProps = {
  vehicles: readonly Vehicle[];
  selectedId: string | undefined;
  onSelect: (vehicleId: string) => void;
};

/**
 * VehicleList shows the same filtered result the map draws, in the order the service published it.
 * That order does not change when the fleet is reloaded, so a row does not move under a pointer
 * that is already on it.
 */
export function VehicleList({ vehicles, selectedId, onSelect }: VehicleListProps) {
  if (vehicles.length === 0) {
    return <p className="fleet-empty">Ни один автомобиль не подходит под выбранные фильтры.</p>;
  }

  return (
    <ul className="fleet-list">
      {vehicles.map((vehicle) => (
        <li key={vehicle.id}>
          <button
            className="fleet-row"
            type="button"
            aria-current={vehicle.id === selectedId}
            onClick={() => onSelect(vehicle.id)}
          >
            <span className="fleet-row-model">{vehicle.model}</span>
            <span className="fleet-row-type">{POWERTRAIN_LABELS[vehicle.powertrain_type]}</span>
            <span className="fleet-row-status" data-status={vehicle.status}>
              {statusText(vehicle)}
            </span>
          </button>
        </li>
      ))}
    </ul>
  );
}
