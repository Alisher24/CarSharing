-- Runs once when a new volume is initialized. Existing passwords are never rotated on restart.
DO $$
BEGIN
    EXECUTE format('CREATE ROLE carsharing_app LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION PASSWORD %L',
        rtrim(pg_read_file('/run/secrets/db_app_password'), E'\r\n'));
END
$$;

REVOKE ALL ON DATABASE carsharing FROM PUBLIC;
GRANT CONNECT ON DATABASE carsharing TO carsharing_app;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
