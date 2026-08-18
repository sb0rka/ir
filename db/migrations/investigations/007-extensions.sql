DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_trgm') THEN
        BEGIN
            CREATE EXTENSION pg_trgm WITH SCHEMA public;
        EXCEPTION WHEN insufficient_privilege THEN
            RAISE EXCEPTION 'insufficient privilege to create extension pg_trgm';
        END;
    END IF;
END $$;
