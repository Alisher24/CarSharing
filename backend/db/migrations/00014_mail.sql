-- +goose Up
-- Mail is a schema of its own under a role of its own: the stub that receives one letter per delivery
-- key shares the database with the application and nothing else. Neither role can read the other's
-- schema, so a defect in the stub cannot reach a ride, an invoice or a signal, and the application
-- cannot reach a letter it did not send.
--
-- The role is normally provisioned when the volume is created, beside the application's role. A
-- database whose volume predates this migration has no such role yet, so the migration creates it
-- from the same secret file rather than leaving the grants below naming a role that does not exist.
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'mailstub_app') THEN
        EXECUTE format(
            'CREATE ROLE mailstub_app LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION PASSWORD %L',
            rtrim(pg_read_file('/run/secrets/mailstub_app_password'), E'\r\n'));
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
DO $$
BEGIN
    EXECUTE format('GRANT CONNECT ON DATABASE %I TO mailstub_app', current_database());
END
$$;
-- +goose StatementEnd

-- The schema is the schema owner's, and the mail role reaches it as a user of one schema rather than
-- as a second owner: no right on the application's schema, and no right to create anything here.
CREATE SCHEMA mailstub;

-- +goose StatementBegin
DO $$
BEGIN
    EXECUTE format('REVOKE ALL ON SCHEMA public FROM mailstub_app');
END
$$;
-- +goose StatementEnd

GRANT USAGE ON SCHEMA mailstub TO mailstub_app;

-- One letter per delivery key. The key is what the whole delivery guarantee rests on — a repeat of a
-- task that was already stored must find its letter rather than write a second one — so the
-- uniqueness is a constraint of the table and the writer resolves a repeat by inserting and reading
-- what the insert left, never by reading before it.
--
-- The content is stored exactly as it arrived: a letter is the evidence that one message was sent
-- once, and a normalized copy would be evidence of something nobody received.
CREATE TABLE mailstub.messages (
    id uuid PRIMARY KEY,
    delivery_key text NOT NULL UNIQUE CHECK (length(btrim(delivery_key)) > 0),
    recipient text NOT NULL CHECK (length(btrim(recipient)) > 0 AND length(recipient) <= 254),
    subject text NOT NULL CHECK (length(subject) > 0),
    body text NOT NULL,
    accepted_at timestamptz NOT NULL
);

-- The order the collection publishes: newest first, with the identifier deciding two letters accepted
-- at the same moment. It is declared here because the keyset page reads it in exactly this order.
CREATE INDEX messages_accepted_order ON mailstub.messages (accepted_at DESC, id DESC);

-- A stored letter is never changed and never removed by the application: what is written is the
-- answer to every later delivery of the same key, and losing it would lose the proof that one letter
-- was sent once.
GRANT SELECT, INSERT ON mailstub.messages TO mailstub_app;

-- One demand for the loss of the next response after the stub accepted a letter. A demonstration asks
-- for it so that the retry a lost answer causes can be shown at all.
--
-- It is spent by the delivery it decided, in the transaction that stores the letter, so a rolled back
-- delivery leaves it armed and a repeat of the same key cannot lose a second answer. The mail role
-- reads it and deletes it and cannot make one: arming the loss is a deliberate act rather than
-- something the process that serves mail can decide for itself.
CREATE TABLE mailstub.demo_delivery_faults (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    armed_at timestamptz NOT NULL
);

GRANT SELECT, DELETE ON mailstub.demo_delivery_faults TO mailstub_app;

-- The answer of one demonstration action, remembered by the identifier the caller chose: a repeat of
-- that identifier reproduces the answer and arms nothing a second time. The row is written by the
-- process that serves the action and is never changed afterwards.
CREATE TABLE mailstub.demo_actions (
    action_id uuid PRIMARY KEY,
    fingerprint text NOT NULL CHECK (length(btrim(fingerprint)) > 0),
    server_time timestamptz NOT NULL
);

GRANT SELECT, INSERT ON mailstub.demo_actions TO mailstub_app;

-- Arming the loss runs with the rights of the schema owner, which is the only role that may write the
-- demand: the mail role is granted the call rather than the insert, so the demonstration surface
-- serves the action while the process that serves mail keeps no right to create the demand itself.
-- +goose StatementBegin
CREATE FUNCTION mailstub.arm_delivery_fault() RETURNS timestamptz
LANGUAGE sql
SECURITY DEFINER
SET search_path = mailstub, pg_temp
AS $$
    INSERT INTO mailstub.demo_delivery_faults (singleton, armed_at)
    VALUES (true, clock_timestamp())
    ON CONFLICT (singleton) DO UPDATE SET armed_at = excluded.armed_at
    RETURNING armed_at
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION mailstub.arm_delivery_fault() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION mailstub.arm_delivery_fault() TO mailstub_app;

-- +goose Down
DROP FUNCTION mailstub.arm_delivery_fault();
DROP TABLE mailstub.demo_actions;
DROP TABLE mailstub.demo_delivery_faults;
DROP TABLE mailstub.messages;
DROP SCHEMA mailstub;
-- The role is provisioned the way the application's role is, so it stays: a schema this build no
-- longer declares is not a reason to remove a credential the installation was set up with.
