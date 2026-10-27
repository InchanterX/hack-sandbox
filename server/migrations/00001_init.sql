-- +goose Up
-- DDL copied verbatim from contracts/schema.sql; tests/contract checks parity.
CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    login text NOT NULL UNIQUE,
    password_hash text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT users_login_valid CHECK (login ~ '^[a-z0-9_-]{3,32}$')
);

CREATE TABLE sessions (
    token_sha256 bytea PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at timestamptz NOT NULL,
    CONSTRAINT sessions_hash_length CHECK (octet_length(token_sha256) = 32)
);
CREATE INDEX sessions_user_id_idx ON sessions (user_id);

CREATE TABLE tasks (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id),
    expression text NOT NULL,
    status text NOT NULL DEFAULT 'queued',
    result double precision,
    error_code text,
    error_message text,
    created_at timestamptz NOT NULL DEFAULT now(),
    started_at timestamptz,
    finished_at timestamptz,
    CONSTRAINT tasks_expression_length CHECK (char_length(expression) BETWEEN 1 AND 200),
    CONSTRAINT tasks_status_valid CHECK (status IN ('queued', 'running', 'done', 'failed')),
    CONSTRAINT tasks_finite_result CHECK (
        result IS NULL OR result NOT IN (
            'Infinity'::double precision, '-Infinity'::double precision, 'NaN'::double precision
        )
    ),
    CONSTRAINT tasks_state_fields CHECK (
        (status = 'queued' AND result IS NULL AND error_code IS NULL AND error_message IS NULL
            AND started_at IS NULL AND finished_at IS NULL)
        OR (status = 'running' AND result IS NULL AND error_code IS NULL AND error_message IS NULL
            AND started_at IS NOT NULL AND finished_at IS NULL)
        OR (status = 'done' AND result IS NOT NULL AND error_code IS NULL AND error_message IS NULL
            AND started_at IS NOT NULL AND finished_at IS NOT NULL)
        OR (status = 'failed' AND result IS NULL AND error_code IS NOT NULL AND error_message IS NOT NULL
            AND started_at IS NOT NULL AND finished_at IS NOT NULL
            AND ((error_code = 'division_by_zero' AND error_message = 'деление на ноль')
                OR (error_code = 'overflow' AND error_message = 'переполнение')))
    )
);
CREATE INDEX tasks_queue_idx ON tasks (status, created_at);
CREATE INDEX tasks_user_created_idx ON tasks (user_id, created_at DESC);

-- +goose Down
DROP TABLE tasks;
DROP TABLE sessions;
DROP TABLE users;
