-- Runs once when a new volume is initialized. Existing passwords are never rotated on restart.
DO $$
BEGIN
    EXECUTE format('CREATE ROLE carsharing_app LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION PASSWORD %L',
        rtrim(pg_read_file('/run/secrets/db_app_password'), E'\r\n'));
END
$$;

-- The mail stub's role, provisioned the same way and from a secret file of its own. The guard keeps a
-- repeated run from failing and keeps a password the volume already holds from being rotated: the
-- migration that grants this role its schema creates it too, so that a volume created before mail
-- existed ends up with the same role rather than with grants naming nobody.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'mailstub_app') THEN
        EXECUTE format('CREATE ROLE mailstub_app LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION PASSWORD %L',
            rtrim(pg_read_file('/run/secrets/mailstub_app_password'), E'\r\n'));
    END IF;
END
$$;

REVOKE ALL ON DATABASE carsharing FROM PUBLIC;
GRANT CONNECT ON DATABASE carsharing TO carsharing_app;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
