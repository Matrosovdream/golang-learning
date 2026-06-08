CREATE TABLE IF NOT EXISTS links (
    id         BIGSERIAL   PRIMARY KEY,
    code       TEXT        NOT NULL UNIQUE,
    long_url   TEXT        NOT NULL,
    clicks     BIGINT      NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
