CREATE TABLE config_versions (
    id         SERIAL PRIMARY KEY,
    namespace  TEXT NOT NULL,
    key        TEXT NOT NULL,
    value      TEXT NOT NULL,
    version    INT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);