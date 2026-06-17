-- Dashboard users.
CREATE TABLE IF NOT EXISTS users (
    id            BIGSERIAL PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'viewer',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Session store for alexedwards/scs (postgresstore schema).
CREATE TABLE IF NOT EXISTS sessions (
    token  TEXT PRIMARY KEY,
    data   BYTEA NOT NULL,
    expiry TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS sessions_expiry_idx ON sessions (expiry);

-- Zerops integration tokens (token stored encrypted).
CREATE TABLE IF NOT EXISTS zerops_tokens (
    id                BIGSERIAL PRIMARY KEY,
    name              TEXT NOT NULL,
    token_ciphertext  TEXT NOT NULL,
    client_id         TEXT NOT NULL DEFAULT '',
    last_validated_at TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- S3-compatible export destinations (secret key stored encrypted).
CREATE TABLE IF NOT EXISTS export_targets (
    id                    BIGSERIAL PRIMARY KEY,
    name                  TEXT NOT NULL,
    endpoint              TEXT NOT NULL,
    region                TEXT NOT NULL DEFAULT 'us-east-1',
    bucket                TEXT NOT NULL,
    prefix                TEXT NOT NULL DEFAULT 'zerops-backups',
    access_key_ciphertext TEXT NOT NULL,
    secret_key_ciphertext TEXT NOT NULL,
    use_path_style        BOOLEAN NOT NULL DEFAULT true,
    use_ssl               BOOLEAN NOT NULL DEFAULT true,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Configured, schedulable export rules.
CREATE TABLE IF NOT EXISTS export_jobs (
    id               BIGSERIAL PRIMARY KEY,
    name             TEXT NOT NULL,
    zerops_token_id  BIGINT NOT NULL REFERENCES zerops_tokens(id) ON DELETE CASCADE,
    project_id       TEXT NOT NULL DEFAULT '',
    project_name     TEXT NOT NULL DEFAULT '',
    service_stack_id TEXT NOT NULL,
    service_name     TEXT NOT NULL DEFAULT '',
    target_id        BIGINT NOT NULL REFERENCES export_targets(id) ON DELETE CASCADE,
    tag_filter       TEXT[] NOT NULL DEFAULT '{}',
    schedule_cron    TEXT NOT NULL DEFAULT '',
    enabled          BOOLEAN NOT NULL DEFAULT true,
    last_run_at      TIMESTAMPTZ,
    next_run_at      TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Individual export executions; drives history and live progress.
CREATE TABLE IF NOT EXISTS export_runs (
    id                BIGSERIAL PRIMARY KEY,
    job_id            BIGINT REFERENCES export_jobs(id) ON DELETE SET NULL,
    status            TEXT NOT NULL DEFAULT 'pending',
    backup_name       TEXT NOT NULL DEFAULT '',
    backup_size       BIGINT NOT NULL DEFAULT 0,
    bytes_transferred BIGINT NOT NULL DEFAULT 0,
    s3_key            TEXT NOT NULL DEFAULT '',
    error             TEXT NOT NULL DEFAULT '',
    started_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at       TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS export_runs_job_idx ON export_runs (job_id);
CREATE INDEX IF NOT EXISTS export_runs_status_idx ON export_runs (status);
CREATE INDEX IF NOT EXISTS export_runs_started_idx ON export_runs (started_at DESC);
