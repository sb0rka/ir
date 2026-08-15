CREATE TABLE IF NOT EXISTS :"DB_INV_SCHEMA_NAME".version_investigations (
    version_num VARCHAR(32) NOT NULL,
    CONSTRAINT version_investigations_pkc PRIMARY KEY (version_num)
);
