-- Deploy provenance: one row per deploy, written by cmd/release-record right
-- after `migrate up` (APP_ROLE=migrate). Append-only event log: redeploying the
-- same version is a new row, so there is no unique key, no upsert and no
-- updated_at. built_at is NULL for dev/local images with no baked build time.
CREATE TABLE app_releases (
    id          BIGSERIAL PRIMARY KEY,
    version     TEXT        NOT NULL,
    git_sha     TEXT        NOT NULL DEFAULT '',
    built_at    TIMESTAMPTZ NULL,
    notes       TEXT        NOT NULL DEFAULT '',
    deployed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_app_releases_deployed_at ON app_releases (deployed_at DESC);
