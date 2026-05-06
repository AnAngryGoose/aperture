-- Aperture schema. Designed multi-host from day 1 even though v0.1 ships
-- with a single locally-collected host.

CREATE TABLE IF NOT EXISTS hosts (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    os          TEXT NOT NULL DEFAULT '',
    platform    TEXT NOT NULL DEFAULT '',
    kernel      TEXT NOT NULL DEFAULT '',
    arch        TEXT NOT NULL DEFAULT '',
    cpu_model   TEXT NOT NULL DEFAULT '',
    cpu_count   INTEGER NOT NULL DEFAULT 0,
    mem_total   INTEGER NOT NULL DEFAULT 0,
    source      TEXT NOT NULL DEFAULT 'local',
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS metrics (
    host_id      TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    ts           TIMESTAMP NOT NULL,
    cpu_pct      REAL NOT NULL DEFAULT 0,
    mem_used     INTEGER NOT NULL DEFAULT 0,
    mem_total    INTEGER NOT NULL DEFAULT 0,
    mem_pct      REAL NOT NULL DEFAULT 0,
    swap_used    INTEGER NOT NULL DEFAULT 0,
    swap_total   INTEGER NOT NULL DEFAULT 0,
    disk_used    INTEGER NOT NULL DEFAULT 0,
    disk_total   INTEGER NOT NULL DEFAULT 0,
    disk_pct     REAL NOT NULL DEFAULT 0,
    net_rx       INTEGER NOT NULL DEFAULT 0,
    net_tx       INTEGER NOT NULL DEFAULT 0,
    load1        REAL NOT NULL DEFAULT 0,
    load5        REAL NOT NULL DEFAULT 0,
    load15       REAL NOT NULL DEFAULT 0,
    uptime_secs  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (host_id, ts)
);

CREATE INDEX IF NOT EXISTS idx_metrics_host_ts ON metrics(host_id, ts DESC);

-- Alert rules: thresholds the user configures, evaluated by the hub each
-- ingest. Storage is here from day 1 so v0.1 can grow into alerts without
-- a schema migration.
CREATE TABLE IF NOT EXISTS alert_rules (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    host_id      TEXT REFERENCES hosts(id) ON DELETE CASCADE,
    metric       TEXT NOT NULL,            -- e.g. cpu_pct, mem_pct, disk_pct
    op           TEXT NOT NULL DEFAULT '>', -- comparison operator
    threshold    REAL NOT NULL,
    duration_s   INTEGER NOT NULL DEFAULT 0, -- sustained for N seconds before firing
    enabled      INTEGER NOT NULL DEFAULT 1,
    created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS alert_events (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    rule_id      INTEGER NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
    host_id      TEXT NOT NULL,
    fired_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    resolved_at  TIMESTAMP,
    value        REAL NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_alert_events_host ON alert_events(host_id, fired_at DESC);
