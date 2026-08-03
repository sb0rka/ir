-- Migration: 001 журнал применённых миграций
--
-- Своей таблицей, а не внешним инструментом: миграции катятся psql-ом из
-- docker-compose и из CI, и единственное, чего им не хватает, — памяти о том,
-- что уже применено. Раннер читает эту таблицу и пропускает применённое.

BEGIN;

SET LOCAL search_path = inv, public, pg_temp;

CREATE TABLE IF NOT EXISTS schema_versions (
    version VARCHAR(64) NOT NULL,
    applied_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,

    CONSTRAINT pk_schema_versions PRIMARY KEY (version)
);

CREATE TABLE IF NOT EXISTS version_investigations (
    version_num VARCHAR(32) NOT NULL,

    CONSTRAINT pk_version_investigations PRIMARY KEY (version_num)
);

COMMIT;
