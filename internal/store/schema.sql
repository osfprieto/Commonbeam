CREATE TABLE IF NOT EXISTS metadata (key text PRIMARY KEY, value text NOT NULL);
CREATE TABLE IF NOT EXISTS pools (
    name text PRIMARY KEY CHECK (name IN ('public','contributor','paid')),
    balance bigint NOT NULL DEFAULT 0 CHECK (balance >= 0)
);
INSERT INTO pools(name) VALUES ('public'),('contributor'),('paid') ON CONFLICT DO NOTHING;
CREATE TABLE IF NOT EXISTS providers (
    id text PRIMARY KEY,
    token_hash text UNIQUE NOT NULL,
    manifest jsonb NOT NULL,
    available bigint NOT NULL CHECK (available >= 0),
    slots integer NOT NULL CHECK (slots BETWEEN 1 AND 16),
    in_flight integer NOT NULL DEFAULT 0 CHECK (in_flight >= 0 AND in_flight <= slots),
    active boolean NOT NULL DEFAULT true,
    heartbeat timestamptz NOT NULL DEFAULT now(),
    expires bigint NOT NULL
);
CREATE TABLE IF NOT EXISTS accounts (
    id text PRIMARY KEY,
    pool text NOT NULL REFERENCES pools(name),
    balance bigint NOT NULL CHECK (balance >= 0),
    created timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS jobs (
    id text PRIMARY KEY,
    account_id text NOT NULL REFERENCES accounts(id),
    idem text NOT NULL,
    request_hash text NOT NULL,
    provider_id text NOT NULL REFERENCES providers(id),
    request bytea NOT NULL,
    output bytea NOT NULL,
    reserved bigint NOT NULL CHECK (reserved > 0),
    cost bigint NOT NULL DEFAULT 0 CHECK (cost >= 0 AND cost <= reserved),
    state text NOT NULL CHECK (state IN ('reserved','accepted','streaming','cancelling','complete','failed','cancelled','expired')),
    deadline bigint NOT NULL,
    receipt jsonb,
    audit boolean NOT NULL DEFAULT false,
    evidence_retained boolean NOT NULL DEFAULT true,
    created timestamptz NOT NULL DEFAULT now(),
    UNIQUE(account_id,idem)
);
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS evidence_retained boolean NOT NULL DEFAULT true;
CREATE INDEX IF NOT EXISTS jobs_work ON jobs(provider_id,state,created);
CREATE TABLE IF NOT EXISTS events (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind text NOT NULL,
    subject text NOT NULL,
    units bigint NOT NULL,
    created timestamptz NOT NULL DEFAULT now()
);
CREATE OR REPLACE FUNCTION prevent_event_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'accounting events are append-only'; END $$;
DROP TRIGGER IF EXISTS immutable_events ON events;
CREATE TRIGGER immutable_events BEFORE UPDATE OR DELETE ON events
FOR EACH ROW EXECUTE FUNCTION prevent_event_mutation();
