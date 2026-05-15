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
    severity     TEXT NOT NULL DEFAULT 'warning', -- 'info'|'warning'|'critical'
    created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Hot-path index for the evaluator's per-sample rule lookup:
-- WHERE enabled = 1 AND (host_id IS NULL OR host_id = ?).
CREATE INDEX IF NOT EXISTS idx_alert_rules_eval ON alert_rules(enabled, host_id);

CREATE TABLE IF NOT EXISTS alert_channels (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    name           TEXT NOT NULL,
    type           TEXT NOT NULL,                   -- 'discord'|'slack'|'ntfy'|'gotify'|'webhook'
    config         TEXT NOT NULL DEFAULT '{}',      -- JSON, type-specific fields
    enabled        INTEGER NOT NULL DEFAULT 1,
    min_severity   TEXT NOT NULL DEFAULT 'info',    -- minimum severity to notify
    notify_resolve INTEGER NOT NULL DEFAULT 1,      -- send a message when alert resolves
    created_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
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

-- Per-interface network counters. Stored alongside the aggregate net_rx/net_tx
-- so the UI can chart per-interface rates over time. Rates are derived client-side
-- from consecutive cumulative byte deltas (same pattern as the aggregate chart).
CREATE TABLE IF NOT EXISTS net_iface_metrics (
    host_id   TEXT    NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    ts        TIMESTAMP NOT NULL,
    iface     TEXT    NOT NULL,
    rx_bytes  INTEGER NOT NULL DEFAULT 0,
    tx_bytes  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (host_id, ts, iface)
);
CREATE INDEX IF NOT EXISTS idx_net_iface_metrics_host_ts ON net_iface_metrics(host_id, ts DESC);

-- Per-mount disk usage. Used + total stored so the UI can show both a usage
-- percentage chart and absolute GB over time per mount.
CREATE TABLE IF NOT EXISTS disk_mount_metrics (
    host_id  TEXT    NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    ts       TIMESTAMP NOT NULL,
    mount    TEXT    NOT NULL,
    device   TEXT    NOT NULL DEFAULT '',
    fstype   TEXT    NOT NULL DEFAULT '',
    used     INTEGER NOT NULL DEFAULT 0,
    total    INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (host_id, ts, mount)
);
CREATE INDEX IF NOT EXISTS idx_disk_mount_metrics_host_ts ON disk_mount_metrics(host_id, ts DESC);

-- Per-device disk I/O counters. Cumulative bytes, rates derived client-side.
CREATE TABLE IF NOT EXISTS disk_io_metrics (
    host_id     TEXT    NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    ts          TIMESTAMP NOT NULL,
    device      TEXT    NOT NULL,
    read_bytes  INTEGER NOT NULL DEFAULT 0,
    write_bytes INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (host_id, ts, device)
);
CREATE INDEX IF NOT EXISTS idx_disk_io_metrics_host_ts ON disk_io_metrics(host_id, ts DESC);

-- Agent tokens: pre-shared secrets that remote agents use to authenticate
-- the WebSocket upgrade. The plaintext token is never stored; only the
-- SHA-256 hex digest is kept.
CREATE TABLE IF NOT EXISTS agent_tokens (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,  -- SHA-256 hex of plaintext token
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used  TIMESTAMP,
    revoked    INTEGER NOT NULL DEFAULT 0
);

-- TODO: Rework this database storage system in the near future to a more efficient and sensible system.
CREATE TABLE IF NOT EXISTS compose_versions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    host_id TEXT NOT NULL,
    project TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    yaml_content TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_compose_versions_lookup ON compose_versions(host_id, project);

-- Auth: single-row table holding the bcrypt-hashed admin password.
-- The CHECK constraint enforces at most one row; REPLACE INTO resets the password.
CREATE TABLE IF NOT EXISTS auth_config (
    id            INTEGER PRIMARY KEY CHECK (id = 1),
    password_hash TEXT NOT NULL,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Sessions: short-lived bearer tokens issued on successful login.
-- Expired rows are pruned lazily on store open and periodically at runtime.
CREATE TABLE IF NOT EXISTS sessions (
    token      TEXT PRIMARY KEY,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL
);

-- User settings: arbitrary key-value pairs for user preferences (dashboard
-- layout, appearance, etc.). A single-user app so no user_id column needed.
CREATE TABLE IF NOT EXISTS user_settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT ''
);

-- ---------------------------------------------------------------------------
-- Rich monitoring history. The legacy `metrics` table holds aggregate scalar
-- fields per host per tick. The following tables persist the *high-cardinality*
-- live-only fields (temps, per-core CPU, process top-N, container stats) so
-- the UI can chart history for each, not just the latest snapshot.
-- Each is keyed by (host_id, ts, <element>) so duplicate-timestamp ingests
-- from a misbehaving source are silently rejected at the PK layer.
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS temp_metrics (
    host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    ts      TIMESTAMP NOT NULL,
    sensor  TEXT NOT NULL,
    temp_c  REAL NOT NULL,
    PRIMARY KEY (host_id, ts, sensor)
);
CREATE INDEX IF NOT EXISTS idx_temp_metrics_host_ts ON temp_metrics(host_id, ts DESC);

CREATE TABLE IF NOT EXISTS cpu_core_metrics (
    host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    ts      TIMESTAMP NOT NULL,
    core    INTEGER NOT NULL,
    pct     REAL NOT NULL,
    PRIMARY KEY (host_id, ts, core)
);
CREATE INDEX IF NOT EXISTS idx_cpu_core_metrics_host_ts ON cpu_core_metrics(host_id, ts DESC);

-- Process history is queried by name (PIDs churn so the index leads with name).
CREATE TABLE IF NOT EXISTS process_metrics (
    host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    ts      TIMESTAMP NOT NULL,
    pid     INTEGER NOT NULL,
    name    TEXT NOT NULL,
    cpu_pct REAL NOT NULL,
    mem_rss INTEGER NOT NULL,
    PRIMARY KEY (host_id, ts, pid)
);
CREATE INDEX IF NOT EXISTS idx_process_metrics_host_name_ts ON process_metrics(host_id, name, ts DESC);

CREATE TABLE IF NOT EXISTS container_metrics (
    host_id      TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    ts           TIMESTAMP NOT NULL,
    container_id TEXT NOT NULL,
    name         TEXT NOT NULL,
    state        TEXT NOT NULL,
    cpu_pct      REAL NOT NULL,
    mem_used     INTEGER NOT NULL,
    mem_limit    INTEGER NOT NULL,
    net_rx       INTEGER NOT NULL,
    net_tx       INTEGER NOT NULL,
    PRIMARY KEY (host_id, ts, container_id)
);
CREATE INDEX IF NOT EXISTS idx_container_metrics_host_ts ON container_metrics(host_id, ts DESC);
CREATE INDEX IF NOT EXISTS idx_container_metrics_lookup ON container_metrics(host_id, container_id, ts DESC);

-- Per-host monitoring configuration. Absent row = use the global defaults
-- stored in user_settings under 'monitoring.defaults'. Stored as JSON for the
-- list-typed and map-typed fields (enabled_families, family_intervals,
-- filters, retention_overrides) so adding a new family or filter doesn't
-- require a schema migration.
CREATE TABLE IF NOT EXISTS host_config (
    host_id              TEXT PRIMARY KEY REFERENCES hosts(id) ON DELETE CASCADE,
    sample_interval_s    INTEGER NOT NULL DEFAULT 5,
    enabled_families     TEXT NOT NULL DEFAULT '["cpu","mem","disk","net","load","temps","processes","cpu_per_core","disk_io","mounts","containers"]',
    family_intervals     TEXT NOT NULL DEFAULT '{}',
    filters              TEXT NOT NULL DEFAULT '{}',
    mem_calc             TEXT NOT NULL DEFAULT 'used',
    retention_days       INTEGER NOT NULL DEFAULT 30,
    retention_overrides  TEXT NOT NULL DEFAULT '{}',
    primary_sensor       TEXT NOT NULL DEFAULT '',
    primary_mount        TEXT NOT NULL DEFAULT '',
    warn_cpu             REAL NOT NULL DEFAULT 70,
    crit_cpu             REAL NOT NULL DEFAULT 90,
    warn_mem             REAL NOT NULL DEFAULT 80,
    crit_mem             REAL NOT NULL DEFAULT 90,
    warn_disk            REAL NOT NULL DEFAULT 80,
    crit_disk            REAL NOT NULL DEFAULT 90,
    warn_temp            REAL NOT NULL DEFAULT 70,
    crit_temp            REAL NOT NULL DEFAULT 85,
    updated_at           TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

