-- Migration: 001 журнал применённых миграций
--
-- Своей таблицей, а не внешним инструментом: миграции катятся psql-ом из
-- docker-compose и из CI, и единственное, чего им не хватает, — памяти о том,
-- что уже применено. Раннер читает эту таблицу и пропускает применённое.

BEGIN;

SET LOCAL search_path = inv, public, pg_temp;

CREATE TABLE IF NOT EXISTS schema_versions (
    version VARCHAR(64) NOT NULL,

    -- Контрольная сумма файла на момент применения: расхождение означает,
    -- что применённую миграцию отредактировали, а это молча расходящиеся базы.
    checksum VARCHAR(64),
    applied_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_schema_versions PRIMARY KEY (version)
);

COMMIT;
