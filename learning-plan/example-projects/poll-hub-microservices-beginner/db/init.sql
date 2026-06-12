-- Runs once, on the FIRST start of the postgres container (any *.sql in
-- /docker-entrypoint-initdb.d does). To re-run it from scratch:
--   docker compose down -v && docker compose up --build

CREATE TABLE polls (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    question   TEXT        NOT NULL,
    status     TEXT        NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'closed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE options (
    id      BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    poll_id BIGINT NOT NULL REFERENCES polls (id) ON DELETE CASCADE,
    label   TEXT   NOT NULL,
    -- lets votes reference (option_id, poll_id) as a pair — see below
    UNIQUE (id, poll_id)
);

CREATE TABLE votes (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    poll_id    BIGINT      NOT NULL,
    option_id  BIGINT      NOT NULL,
    voter      TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- one vote per voter per poll, enforced even under concurrent requests
    UNIQUE (poll_id, voter),
    -- the voted option must belong to that same poll — the database
    -- guarantees consistency no service code could under concurrency
    FOREIGN KEY (option_id, poll_id) REFERENCES options (id, poll_id) ON DELETE CASCADE
);

CREATE INDEX options_poll_id_idx ON options (poll_id);
CREATE INDEX votes_option_id_idx ON votes (option_id);
CREATE INDEX votes_poll_id_idx   ON votes (poll_id);

-- A starter poll so the API has something to show immediately.
INSERT INTO polls (question) VALUES ('Which topic should the next lesson cover?');
INSERT INTO options (poll_id, label) VALUES
    (1, 'Goroutines & channels'),
    (1, 'Generics'),
    (1, 'Testing'),
    (1, 'gRPC');
