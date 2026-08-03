-- Migration: 000 схема и расширения
--
-- Отдельной миграцией, потому что это единственное место, где нужны права
-- выше прикладных: CREATE SCHEMA и CREATE EXTENSION администратор базы
-- выполняет один раз, дальше миграции катятся обычной ролью.

BEGIN;

CREATE SCHEMA IF NOT EXISTS inv;

-- Расширение ставится явно в public. Под `search_path = inv` оно уехало бы
-- внутрь inv: операторы стали бы не видны сессиям без inv в пути, а
-- DROP SCHEMA inv унёс бы расширение с собой.
--
-- CREATE EXTENSION требует прав, которых на управляемом Postgres у прикладной
-- роли обычно нет. Молчаливое падение выглядит как поломка миграции, поэтому
-- причина проговаривается явно.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_trgm') THEN
        BEGIN
            CREATE EXTENSION pg_trgm WITH SCHEMA public;
        EXCEPTION WHEN insufficient_privilege THEN
            RAISE EXCEPTION 'нужно расширение pg_trgm: попросите администратора базы выполнить CREATE EXTENSION pg_trgm WITH SCHEMA public';
        END;
    END IF;
END $$;

COMMIT;
