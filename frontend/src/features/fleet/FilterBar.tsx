import {
  FILTERABLE_POWERTRAIN_TYPES,
  FILTERABLE_STATUSES,
  showsOnlyAvailable,
  withOnlyAvailable,
  withPowertrainToggled,
  withStatusToggled,
  type FleetFilters,
} from '../../shared/vehicle/filters.ts';
import { POWERTRAIN_LABELS, STATUS_LABELS } from '../../shared/vehicle/spell.ts';

type FilterBarProps = { filters: FleetFilters; onChange: (filters: FleetFilters) => void };

/**
 * FilterBar is the one place the fleet is narrowed from. The map and the list are drawn from the
 * result it produces, so neither can be filtered differently from the other.
 */
export function FilterBar({ filters, onChange }: FilterBarProps) {
  const onlyAvailable = showsOnlyAvailable(filters);

  return (
    <section className="filters" aria-label="Фильтры парка">
      <label className="filter-switch">
        <input
          type="checkbox"
          checked={onlyAvailable}
          onChange={(event) => onChange(withOnlyAvailable(filters, event.target.checked))}
        />
        Только свободные
      </label>

      <FilterGroup label="Тип">
        {FILTERABLE_POWERTRAIN_TYPES.map((powertrain) => (
          <FilterChip
            key={powertrain}
            label={POWERTRAIN_LABELS[powertrain]}
            chosen={filters.powertrainTypes.has(powertrain)}
            onToggle={() => onChange(withPowertrainToggled(filters, powertrain))}
          />
        ))}
      </FilterGroup>

      <FilterGroup label="Состояние">
        {FILTERABLE_STATUSES.map((status) => (
          <FilterChip
            key={status}
            label={STATUS_LABELS[status]}
            chosen={filters.statuses.has(status)}
            onToggle={() => onChange(withStatusToggled(filters, status))}
          />
        ))}
      </FilterGroup>
    </section>
  );
}

function FilterGroup({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <fieldset className="filter-group">
      <legend className="filter-legend">{label}</legend>
      <div className="filter-chips">{children}</div>
    </fieldset>
  );
}

function FilterChip({ label, chosen, onToggle }: { label: string; chosen: boolean; onToggle: () => void }) {
  return (
    <button className="filter-chip" type="button" aria-pressed={chosen} onClick={onToggle}>
      {label}
    </button>
  );
}
