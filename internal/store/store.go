package store

import (
	"context"
	_ "embed"
	"database/sql"
	"fmt"
	"time"

	"github.com/aperture/aperture/internal/types"
	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

type Store struct {
	db   *sql.DB
	path string
}

func Open(path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	if _, err := db.Exec(schemaSQL); err != nil {
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &Store{db: db, path: path}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Path returns the on-disk path of the SQLite file. Used by the API to
// stat the database for size reporting in /api/system/info.
func (s *Store) Path() string { return s.path }

// UpsertHost inserts or updates a host record and bumps last_seen.
func (s *Store) UpsertHost(ctx context.Context, h types.Host) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO hosts (id, name, os, platform, kernel, arch, cpu_model, cpu_count, mem_total, source, created_at, last_seen)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			os = excluded.os,
			platform = excluded.platform,
			kernel = excluded.kernel,
			arch = excluded.arch,
			cpu_model = excluded.cpu_model,
			cpu_count = excluded.cpu_count,
			mem_total = excluded.mem_total,
			source = excluded.source,
			last_seen = excluded.last_seen
	`, h.ID, h.Name, h.OS, h.Platform, h.Kernel, h.Arch, h.CPUModel, h.CPUCount, h.MemTotal, h.Source, h.CreatedAt, h.LastSeen)
	return err
}

func (s *Store) TouchHost(ctx context.Context, hostID string, t time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE hosts SET last_seen = ? WHERE id = ?`, t, hostID)
	return err
}

func (s *Store) ListHosts(ctx context.Context) ([]types.Host, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, os, platform, kernel, arch, cpu_model, cpu_count, mem_total, source, created_at, last_seen
		FROM hosts ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []types.Host
	for rows.Next() {
		var h types.Host
		if err := rows.Scan(&h.ID, &h.Name, &h.OS, &h.Platform, &h.Kernel, &h.Arch,
			&h.CPUModel, &h.CPUCount, &h.MemTotal, &h.Source, &h.CreatedAt, &h.LastSeen); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (s *Store) GetHost(ctx context.Context, id string) (*types.Host, error) {
	var h types.Host
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, os, platform, kernel, arch, cpu_model, cpu_count, mem_total, source, created_at, last_seen
		FROM hosts WHERE id = ?`, id).Scan(
		&h.ID, &h.Name, &h.OS, &h.Platform, &h.Kernel, &h.Arch,
		&h.CPUModel, &h.CPUCount, &h.MemTotal, &h.Source, &h.CreatedAt, &h.LastSeen)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &h, nil
}

func (s *Store) InsertMetric(ctx context.Context, m types.MetricSample) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO metrics (host_id, ts, cpu_pct, mem_used, mem_total, mem_pct, swap_used, swap_total,
			disk_used, disk_total, disk_pct, net_rx, net_tx, load1, load5, load15, uptime_secs)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.HostID, m.Timestamp, m.CPUPercent, m.MemUsed, m.MemTotal, m.MemPercent,
		m.SwapUsed, m.SwapTotal, m.DiskUsed, m.DiskTotal, m.DiskPercent,
		m.NetRxBytes, m.NetTxBytes, m.LoadAvg1, m.LoadAvg5, m.LoadAvg15, m.UptimeSecs)
	return err
}

// LatestMetric returns the most recent sample for a host, or nil if none.
func (s *Store) LatestMetric(ctx context.Context, hostID string) (*types.MetricSample, error) {
	var m types.MetricSample
	m.HostID = hostID
	err := s.db.QueryRowContext(ctx, `
		SELECT ts, cpu_pct, mem_used, mem_total, mem_pct, swap_used, swap_total,
			disk_used, disk_total, disk_pct, net_rx, net_tx, load1, load5, load15, uptime_secs
		FROM metrics WHERE host_id = ? ORDER BY ts DESC LIMIT 1`, hostID).Scan(
		&m.Timestamp, &m.CPUPercent, &m.MemUsed, &m.MemTotal, &m.MemPercent,
		&m.SwapUsed, &m.SwapTotal, &m.DiskUsed, &m.DiskTotal, &m.DiskPercent,
		&m.NetRxBytes, &m.NetTxBytes, &m.LoadAvg1, &m.LoadAvg5, &m.LoadAvg15, &m.UptimeSecs)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// MetricsRange returns samples for a host between since and until (inclusive),
// optionally downsampled to at most maxPoints rows by uniform stride.
func (s *Store) MetricsRange(ctx context.Context, hostID string, since, until time.Time, maxPoints int) ([]types.MetricSample, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT ts, cpu_pct, mem_used, mem_total, mem_pct, swap_used, swap_total,
			disk_used, disk_total, disk_pct, net_rx, net_tx, load1, load5, load15, uptime_secs
		FROM metrics
		WHERE host_id = ? AND ts >= ? AND ts <= ?
		ORDER BY ts ASC`, hostID, since, until)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var all []types.MetricSample
	for rows.Next() {
		var m types.MetricSample
		m.HostID = hostID
		if err := rows.Scan(&m.Timestamp, &m.CPUPercent, &m.MemUsed, &m.MemTotal, &m.MemPercent,
			&m.SwapUsed, &m.SwapTotal, &m.DiskUsed, &m.DiskTotal, &m.DiskPercent,
			&m.NetRxBytes, &m.NetTxBytes, &m.LoadAvg1, &m.LoadAvg5, &m.LoadAvg15, &m.UptimeSecs); err != nil {
			return nil, err
		}
		all = append(all, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if maxPoints <= 0 || len(all) <= maxPoints {
		return all, nil
	}
	stride := len(all) / maxPoints
	if stride < 1 {
		stride = 1
	}
	out := make([]types.MetricSample, 0, maxPoints+1)
	for i := 0; i < len(all); i += stride {
		out = append(out, all[i])
	}
	if out[len(out)-1].Timestamp != all[len(all)-1].Timestamp {
		out = append(out, all[len(all)-1])
	}
	return out, nil
}

// PruneMetrics deletes samples older than cutoff.
func (s *Store) PruneMetrics(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM metrics WHERE ts < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// --- alert rules ---

// scanAlertRule reads one alert_rules row. hostID may be NULL (applies to all
// hosts), so we scan into a sql.NullString and convert.
func scanAlertRule(rs interface {
	Scan(dest ...any) error
}) (types.AlertRule, error) {
	var r types.AlertRule
	var hostID sql.NullString
	var enabled int
	if err := rs.Scan(&r.ID, &hostID, &r.Metric, &r.Op, &r.Threshold, &r.DurationS, &enabled, &r.CreatedAt); err != nil {
		return r, err
	}
	if hostID.Valid {
		s := hostID.String
		r.HostID = &s
	}
	r.Enabled = enabled != 0
	return r, nil
}

// ListAlertRules returns all rules. If hostID is non-nil, only rules that
// apply to that host (matching host_id, or host_id IS NULL).
func (s *Store) ListAlertRules(ctx context.Context, hostID *string) ([]types.AlertRule, error) {
	var rows *sql.Rows
	var err error
	if hostID != nil {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, host_id, metric, op, threshold, duration_s, enabled, created_at
			FROM alert_rules
			WHERE host_id IS NULL OR host_id = ?
			ORDER BY id`, *hostID)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, host_id, metric, op, threshold, duration_s, enabled, created_at
			FROM alert_rules ORDER BY id`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []types.AlertRule
	for rows.Next() {
		r, err := scanAlertRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListEnabledRulesFor returns enabled rules that apply to a specific host.
// Used on the evaluator hot path.
func (s *Store) ListEnabledRulesFor(ctx context.Context, hostID string) ([]types.AlertRule, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, host_id, metric, op, threshold, duration_s, enabled, created_at
		FROM alert_rules
		WHERE enabled = 1 AND (host_id IS NULL OR host_id = ?)`, hostID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []types.AlertRule
	for rows.Next() {
		r, err := scanAlertRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) GetAlertRule(ctx context.Context, id int64) (*types.AlertRule, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, host_id, metric, op, threshold, duration_s, enabled, created_at
		FROM alert_rules WHERE id = ?`, id)
	r, err := scanAlertRule(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) CreateAlertRule(ctx context.Context, r types.AlertRule) (int64, error) {
	var hostID any
	if r.HostID != nil {
		hostID = *r.HostID
	}
	enabled := 0
	if r.Enabled {
		enabled = 1
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO alert_rules (host_id, metric, op, threshold, duration_s, enabled)
		VALUES (?, ?, ?, ?, ?, ?)`,
		hostID, r.Metric, r.Op, r.Threshold, r.DurationS, enabled)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateAlertRule(ctx context.Context, r types.AlertRule) error {
	var hostID any
	if r.HostID != nil {
		hostID = *r.HostID
	}
	enabled := 0
	if r.Enabled {
		enabled = 1
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE alert_rules
		SET host_id = ?, metric = ?, op = ?, threshold = ?, duration_s = ?, enabled = ?
		WHERE id = ?`,
		hostID, r.Metric, r.Op, r.Threshold, r.DurationS, enabled, r.ID)
	return err
}

func (s *Store) DeleteAlertRule(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM alert_rules WHERE id = ?`, id)
	return err
}

// --- alert events ---

func (s *Store) InsertAlertEvent(ctx context.Context, e types.AlertEvent) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO alert_events (rule_id, host_id, fired_at, value)
		VALUES (?, ?, ?, ?)`, e.RuleID, e.HostID, e.FiredAt, e.Value)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) ResolveAlertEvent(ctx context.Context, id int64, resolvedAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE alert_events SET resolved_at = ? WHERE id = ? AND resolved_at IS NULL`,
		resolvedAt, id)
	return err
}

// AlertEventFilter narrows ListAlertEvents.
type AlertEventFilter struct {
	HostID   string // empty = any host
	OpenOnly bool
	Limit    int // 0 = default 200
}

func (s *Store) ListAlertEvents(ctx context.Context, f AlertEventFilter) ([]types.AlertEvent, error) {
	q := `SELECT id, rule_id, host_id, fired_at, resolved_at, value FROM alert_events WHERE 1=1`
	args := []any{}
	if f.HostID != "" {
		q += ` AND host_id = ?`
		args = append(args, f.HostID)
	}
	if f.OpenOnly {
		q += ` AND resolved_at IS NULL`
	}
	q += ` ORDER BY fired_at DESC LIMIT ?`
	limit := f.Limit
	if limit <= 0 {
		limit = 200
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []types.AlertEvent
	for rows.Next() {
		var e types.AlertEvent
		var resolved sql.NullTime
		if err := rows.Scan(&e.ID, &e.RuleID, &e.HostID, &e.FiredAt, &resolved, &e.Value); err != nil {
			return nil, err
		}
		if resolved.Valid {
			t := resolved.Time
			e.ResolvedAt = &t
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
