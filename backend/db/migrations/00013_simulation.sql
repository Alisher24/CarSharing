-- +goose Up
-- The model's own state: where a simulated vehicle stands, what its sources hold and the moment
-- everything above describes. It is the state the simulator restores after a restart, so a journey
-- interrupted by a stopped process is reproduced from the time that passed rather than from a guess.
--
-- The fractional part of a charge is kept here rather than a reserve rounded to whole millionths:
-- what a window costs follows from the time it covers alone, and a rounded reserve would make the
-- same journey cost a little less when it is played in many short ticks than in one call.
CREATE TABLE simulation_states (
    vehicle_id uuid PRIMARY KEY REFERENCES vehicles (id) ON DELETE CASCADE,
    route_id text NOT NULL CHECK (route_id <> ''),
    path bigint NOT NULL,
    join_path bigint NOT NULL,
    is_off_route boolean NOT NULL,
    longitude double precision NOT NULL,
    latitude double precision NOT NULL,
    -- A vehicle that has run out stands where it stopped and spends nothing more, however long the
    -- demonstration goes on: only an explicit refill or servicing moves it again.
    depleted boolean NOT NULL,
    processed_at timestamptz NOT NULL
);

-- One source of one simulated vehicle: what it holds when full and the sum its windows have charged
-- since the ride began, in millionths of the unit multiplied by the period of the source's rate. The
-- sum is an arbitrary-precision number rather than a 64-bit one, because a period of an hour in
-- nanoseconds multiplied by a capacity in millionths does not fit the signed 64-bit range.
CREATE TABLE simulation_sources (
    vehicle_id uuid NOT NULL REFERENCES simulation_states (vehicle_id) ON DELETE CASCADE,
    source_kind text NOT NULL
        CHECK (source_kind IN ('battery', 'gasoline', 'diesel', 'lpg', 'cng')),
    capacity bigint NOT NULL CHECK (capacity > 0),
    charged numeric NOT NULL CHECK (charged >= 0),
    PRIMARY KEY (vehicle_id, source_kind)
);

GRANT SELECT, INSERT, UPDATE, DELETE ON simulation_states, simulation_sources TO carsharing_app;

-- The route a vehicle's model travels. It belongs to the vehicle rather than to the model state,
-- because it is what the vehicle was placed on rather than what it has done since: a demonstration
-- command that moves a vehicle away from its route does not change the route it drives back to.
ALTER TABLE vehicles
    ADD COLUMN route_id text CHECK (route_id IS NULL OR route_id <> '');

-- A vehicle no source can move is taken out of service until somebody has looked at it. Refilling it
-- does not clear this: the ride ended because the reserves ran out, and only servicing answers that.
ALTER TABLE vehicles
    ADD COLUMN service_required boolean NOT NULL DEFAULT false;

-- A run-out ride states which sources were empty when it ran out. The list is written with the
-- ending and never recomputed from what the vehicle holds now, because a refill after the ending
-- must not rewrite why the ride ended. Every record that publishes the ending carries its own copy,
-- written by the one transaction that ended the ride, so a reader needs no second query to explain
-- the reason it was given.
--
-- The report of an ending also states the moment the ride ended, which is not the moment the report
-- was written: a ride that ran out at noon and was noticed at ten past is reported at ten past about
-- noon, and a reader is told both.
ALTER TABLE rentals ADD COLUMN exhausted_sources text[];
ALTER TABLE invoices ADD COLUMN exhausted_sources text[];
ALTER TABLE notifications ADD COLUMN exhausted_sources text[];
ALTER TABLE notifications ADD COLUMN ended_at timestamptz;

ALTER TABLE notifications
    ADD CONSTRAINT notifications_ended_at_with_completion
        CHECK ((kind = 'rental_completed') = (ended_at IS NOT NULL));

ALTER TABLE rentals
    ADD CONSTRAINT rentals_exhausted_sources_with_reason
        CHECK ((completion_reason = 'energy_depleted') = (exhausted_sources IS NOT NULL)),
    ADD CONSTRAINT rentals_exhausted_sources_not_empty
        CHECK (exhausted_sources IS NULL OR cardinality(exhausted_sources) >= 1);

ALTER TABLE invoices
    ADD CONSTRAINT invoices_exhausted_sources_with_reason
        CHECK ((completion_reason = 'energy_depleted') = (exhausted_sources IS NOT NULL)),
    ADD CONSTRAINT invoices_exhausted_sources_not_empty
        CHECK (exhausted_sources IS NULL OR cardinality(exhausted_sources) >= 1);

ALTER TABLE notifications
    ADD CONSTRAINT notifications_exhausted_sources_with_reason
        CHECK ((completion_reason = 'energy_depleted' AND kind = 'rental_completed')
            = (exhausted_sources IS NOT NULL)),
    ADD CONSTRAINT notifications_exhausted_sources_not_empty
        CHECK (exhausted_sources IS NULL OR cardinality(exhausted_sources) >= 1);

-- The telemetry source publishes what the model holds and where it stands, which is the only way the
-- catalog learns that a simulated vehicle has moved or spent something. It changes the reserve and
-- nothing else about the vehicle's energy: a capacity is a property of the fleet, not of a ride.
GRANT UPDATE (remaining) ON vehicle_energy_sources TO carsharing_app;

-- A demonstration command sets the outcome of the next payment attempt, so the application role is
-- given the statements that write such a demand: setting one that already exists is an update rather
-- than a second row, and a ride keeps its identity while what it asks for changes. The public surface
-- has no operation that reaches this table; the write exists for the separate internal demonstration
-- surface, which is served under a token of its own and is not published through the external proxy.
GRANT INSERT, UPDATE (outcome, set_at) ON demo_payment_outcomes TO carsharing_app;

-- A remembered command belonged to an account, and the commands of the internal surface belong to no
-- account at all. The owner becomes a name of its own rather than the sender, so the simulation tick
-- and the demonstration commands are remembered by the same table and answered the same way, and one
-- reaper still cleans all of them.
--
-- The key is replaced before the sender is made nullable, because a column of a primary key cannot
-- hold nothing: the order is part of the change rather than an accident of how it was written.
ALTER TABLE idempotency_requests DROP CONSTRAINT idempotency_requests_pkey;

ALTER TABLE idempotency_requests
    ALTER COLUMN user_id DROP NOT NULL,
    ADD COLUMN owner text;

UPDATE idempotency_requests SET owner = 'account:' || user_id::text;

ALTER TABLE idempotency_requests
    ALTER COLUMN owner SET NOT NULL,
    ADD CONSTRAINT idempotency_requests_owner_names_sender
        CHECK ((user_id IS NOT NULL AND owner = 'account:' || user_id::text)
            OR (user_id IS NULL AND owner = 'internal'));

ALTER TABLE idempotency_requests ADD PRIMARY KEY (owner, command_key);

-- +goose Down
DELETE FROM idempotency_requests WHERE user_id IS NULL;

ALTER TABLE idempotency_requests DROP CONSTRAINT idempotency_requests_pkey;

ALTER TABLE idempotency_requests
    ALTER COLUMN user_id SET NOT NULL,
    DROP CONSTRAINT idempotency_requests_owner_names_sender,
    DROP COLUMN owner,
    ADD PRIMARY KEY (user_id, command_key);

REVOKE INSERT ON demo_payment_outcomes FROM carsharing_app;
REVOKE UPDATE (outcome, set_at) ON demo_payment_outcomes FROM carsharing_app;
REVOKE UPDATE (remaining) ON vehicle_energy_sources FROM carsharing_app;

ALTER TABLE notifications
    DROP CONSTRAINT notifications_exhausted_sources_with_reason,
    DROP CONSTRAINT notifications_exhausted_sources_not_empty,
    DROP CONSTRAINT notifications_ended_at_with_completion,
    DROP COLUMN exhausted_sources,
    DROP COLUMN ended_at;

ALTER TABLE invoices
    DROP CONSTRAINT invoices_exhausted_sources_with_reason,
    DROP CONSTRAINT invoices_exhausted_sources_not_empty,
    DROP COLUMN exhausted_sources;

ALTER TABLE rentals
    DROP CONSTRAINT rentals_exhausted_sources_with_reason,
    DROP CONSTRAINT rentals_exhausted_sources_not_empty,
    DROP COLUMN exhausted_sources;

ALTER TABLE vehicles DROP COLUMN service_required, DROP COLUMN route_id;

DROP TABLE simulation_sources;
DROP TABLE simulation_states;
