package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
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
	// Idempotent migrations for columns added after the initial schema.
	migrations := []string{
		`ALTER TABLE alert_rules ADD COLUMN severity TEXT NOT NULL DEFAULT 'warning'`,
		`ALTER TABLE hosts ADD COLUMN agent_version TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE hosts ADD COLUMN tags TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE hosts ADD COLUMN kind TEXT NOT NULL DEFAULT 'linux'`,
	}
	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			return nil, fmt.Errorf("migration: %w", err)
		}
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
		INSERT INTO hosts (id, name, os, platform, kernel, arch, cpu_model, cpu_count, mem_total, source, agent_version, created_at, last_seen)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
			agent_version = excluded.agent_version,
			last_seen = excluded.last_seen
	`, h.ID, h.Name, h.OS, h.Platform, h.Kernel, h.Arch, h.CPUModel, h.CPUCount, h.MemTotal, h.Source, h.AgentVersion, h.CreatedAt, h.LastSeen)
	return err
}

func (s *Store) TouchHost(ctx context.Context, hostID string, t time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE hosts SET last_seen = ? WHERE id = ?`, t, hostID)
	return err
}

func scanHost(scan func(...any) error) (types.Host, error) {
	var h types.Host
	var tagsJSON string
	err := scan(&h.ID, &h.Name, &h.OS, &h.Platform, &h.Kernel, &h.Arch,
		&h.CPUModel, &h.CPUCount, &h.MemTotal, &h.Source, &h.AgentVersion,
		&h.Kind, &tagsJSON, &h.CreatedAt, &h.LastSeen)
	if err != nil {
		return h, err
	}
	if tagsJSON != "" && tagsJSON != "[]" {
		_ = json.Unmarshal([]byte(tagsJSON), &h.Tags)
	}
	if h.Tags == nil {
		h.Tags = []string{}
	}
	return h, nil
}

func (s *Store) ListHosts(ctx context.Context) ([]types.Host, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, os, platform, kernel, arch, cpu_model, cpu_count, mem_total, source,
		       agent_version, kind, tags, created_at, last_seen
		FROM hosts ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []types.Host
	for rows.Next() {
		h, err := scanHost(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (s *Store) GetHost(ctx context.Context, id string) (*types.Host, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, os, platform, kernel, arch, cpu_model, cpu_count, mem_total, source,
		       agent_version, kind, tags, created_at, last_seen
		FROM hosts WHERE id = ?`, id)
	h, err := scanHost(row.Scan)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &h, nil
}

// UpdateHostTags persists a JSON-encoded tag array for a host.
func (s *Store) UpdateHostTags(ctx context.Context, hostID string, tags []string) error {
	b, err := json.Marshal(tags)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE hosts SET tags = ? WHERE id = ?`, string(b), hostID)
	return err
}

// UpdateHostKind sets the kind (docker|linux|edge) for a host.
func (s *Store) UpdateHostKind(ctx context.Context, hostID, kind string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE hosts SET kind = ? WHERE id = ?`, kind, hostID)
	return err
}

// UserSetting returns the stored value for a settings key, or "" if unset.
func (s *Store) UserSetting(ctx context.Context, key string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM user_settings WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

// SetUserSetting upserts a key-value user preference.
func (s *Store) SetUserSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO user_settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value)
	return err
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

// PruneMetrics deletes samples older than cutoff from all metric tables.
func (s *Store) PruneMetrics(ctx context.Context, cutoff time.Time) (int64, error) {
	var total int64
	for _, table := range []string{"metrics", "net_iface_metrics", "disk_mount_metrics", "disk_io_metrics"} {
		res, err := s.db.ExecContext(ctx, `DELETE FROM `+table+` WHERE ts < ?`, cutoff)
		if err != nil {
			return total, err
		}
		n, _ := res.RowsAffected()
		total += n
	}
	return total, nil
}

// InsertNetIfaces bulk-inserts per-interface byte counters from a sample.
// Called best-effort from ingestLoop; errors are logged and ignored.
func (s *Store) InsertNetIfaces(ctx context.Context, m types.MetricSample) error {
	if len(m.NetIfaces) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR IGNORE INTO net_iface_metrics (host_id, ts, iface, rx_bytes, tx_bytes)
		VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, iface := range m.NetIfaces {
		if _, err := stmt.ExecContext(ctx, m.HostID, m.Timestamp, iface.Name, iface.RxBytes, iface.TxBytes); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// InsertDiskMounts bulk-inserts per-mount usage from a sample.
func (s *Store) InsertDiskMounts(ctx context.Context, m types.MetricSample) error {
	if len(m.DiskMounts) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR IGNORE INTO disk_mount_metrics (host_id, ts, mount, device, fstype, used, total)
		VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, dm := range m.DiskMounts {
		if _, err := stmt.ExecContext(ctx, m.HostID, m.Timestamp, dm.Mount, dm.Device, dm.FSType, dm.Used, dm.Total); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// InsertDiskIO bulk-inserts per-device I/O counters from a sample.
func (s *Store) InsertDiskIO(ctx context.Context, m types.MetricSample) error {
	if len(m.DiskIO) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR IGNORE INTO disk_io_metrics (host_id, ts, device, read_bytes, write_bytes)
		VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, d := range m.DiskIO {
		if _, err := stmt.ExecContext(ctx, m.HostID, m.Timestamp, d.Device, d.ReadBytes, d.WriteBytes); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// NetIfaceRange returns per-interface byte counters for historical charting,
// downsampled to at most maxPoints timestamp groups.
func (s *Store) NetIfaceRange(ctx context.Context, hostID string, since, until time.Time, maxPoints int) (*types.NetIfaceHistory, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT ts, iface, rx_bytes, tx_bytes
		FROM net_iface_metrics
		WHERE host_id = ? AND ts >= ? AND ts <= ?
		ORDER BY ts ASC, iface ASC`, hostID, since, until)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type ifaceRow struct {
		ts         time.Time
		iface      string
		rx, tx     uint64
	}
	var all []ifaceRow
	for rows.Next() {
		var r ifaceRow
		if err := rows.Scan(&r.ts, &r.iface, &r.rx, &r.tx); err != nil {
			return nil, err
		}
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	tsSeen := make(map[time.Time]bool)
	for _, r := range all {
		tsSeen[r.ts] = true
	}
	tsList := make([]time.Time, 0, len(tsSeen))
	for t := range tsSeen {
		tsList = append(tsList, t)
	}
	sort.Slice(tsList, func(i, j int) bool { return tsList[i].Before(tsList[j]) })

	stride := 1
	if maxPoints > 0 && len(tsList) > maxPoints {
		stride = len(tsList) / maxPoints
	}
	kept := make(map[time.Time]bool)
	for i := 0; i < len(tsList); i += stride {
		kept[tsList[i]] = true
	}
	if len(tsList) > 0 {
		kept[tsList[len(tsList)-1]] = true
	}

	tsKept := make([]time.Time, 0, len(kept))
	for _, t := range tsList {
		if kept[t] {
			tsKept = append(tsKept, t)
		}
	}
	tsIndex := make(map[time.Time]int, len(tsKept))
	for i, t := range tsKept {
		tsIndex[t] = i
	}

	out := &types.NetIfaceHistory{
		Timestamps: []int64{},
		Ifaces:     map[string]*types.NetIfaceSeries{},
	}
	for _, t := range tsKept {
		out.Timestamps = append(out.Timestamps, t.Unix())
	}
	for _, r := range all {
		if !kept[r.ts] {
			continue
		}
		is, ok := out.Ifaces[r.iface]
		if !ok {
			is = &types.NetIfaceSeries{
				RxBytes: make([]uint64, len(tsKept)),
				TxBytes: make([]uint64, len(tsKept)),
			}
			out.Ifaces[r.iface] = is
		}
		idx := tsIndex[r.ts]
		is.RxBytes[idx] = r.rx
		is.TxBytes[idx] = r.tx
	}
	return out, nil
}

// DiskMountRange returns per-mount used/total counters for historical charting.
func (s *Store) DiskMountRange(ctx context.Context, hostID string, since, until time.Time, maxPoints int) (*types.DiskMountHistory, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT ts, mount, used, total
		FROM disk_mount_metrics
		WHERE host_id = ? AND ts >= ? AND ts <= ?
		ORDER BY ts ASC, mount ASC`, hostID, since, until)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type mountRow struct {
		ts            time.Time
		mount         string
		used, total   uint64
	}
	var all []mountRow
	for rows.Next() {
		var r mountRow
		if err := rows.Scan(&r.ts, &r.mount, &r.used, &r.total); err != nil {
			return nil, err
		}
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Collect unique timestamps.
	tsSeen := make(map[time.Time]bool)
	for _, r := range all {
		tsSeen[r.ts] = true
	}
	tsList := make([]time.Time, 0, len(tsSeen))
	for t := range tsSeen {
		tsList = append(tsList, t)
	}
	sort.Slice(tsList, func(i, j int) bool { return tsList[i].Before(tsList[j]) })

	stride := 1
	if maxPoints > 0 && len(tsList) > maxPoints {
		stride = len(tsList) / maxPoints
	}
	kept := make(map[time.Time]bool)
	for i := 0; i < len(tsList); i += stride {
		kept[tsList[i]] = true
	}
	if len(tsList) > 0 {
		kept[tsList[len(tsList)-1]] = true
	}

	out := &types.DiskMountHistory{
		Timestamps: []int64{},
		Mounts:     map[string]*types.DiskMountSeries{},
	}
	// First pass: collect kept timestamps in order.
	tsKept := make([]time.Time, 0, len(kept))
	for _, t := range tsList {
		if kept[t] {
			tsKept = append(tsKept, t)
		}
	}
	for _, t := range tsKept {
		out.Timestamps = append(out.Timestamps, t.Unix())
	}
	// Build index: ts -> position.
	tsIndex := make(map[time.Time]int, len(tsKept))
	for i, t := range tsKept {
		tsIndex[t] = i
	}
	// Second pass: fill series.
	for _, r := range all {
		if !kept[r.ts] {
			continue
		}
		ms, ok := out.Mounts[r.mount]
		if !ok {
			ms = &types.DiskMountSeries{
				Used:  make([]uint64, len(tsKept)),
				Total: make([]uint64, len(tsKept)),
			}
			out.Mounts[r.mount] = ms
		}
		idx := tsIndex[r.ts]
		ms.Used[idx] = r.used
		ms.Total[idx] = r.total
	}
	return out, nil
}

// DiskIORange returns per-device cumulative I/O counters for historical charting.
func (s *Store) DiskIORange(ctx context.Context, hostID string, since, until time.Time, maxPoints int) (*types.DiskIOHistory, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT ts, device, read_bytes, write_bytes
		FROM disk_io_metrics
		WHERE host_id = ? AND ts >= ? AND ts <= ?
		ORDER BY ts ASC, device ASC`, hostID, since, until)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type ioRow struct {
		ts             time.Time
		device         string
		read, write    uint64
	}
	var all []ioRow
	for rows.Next() {
		var r ioRow
		if err := rows.Scan(&r.ts, &r.device, &r.read, &r.write); err != nil {
			return nil, err
		}
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	tsSeen := make(map[time.Time]bool)
	for _, r := range all {
		tsSeen[r.ts] = true
	}
	tsList := make([]time.Time, 0, len(tsSeen))
	for t := range tsSeen {
		tsList = append(tsList, t)
	}
	sort.Slice(tsList, func(i, j int) bool { return tsList[i].Before(tsList[j]) })

	stride := 1
	if maxPoints > 0 && len(tsList) > maxPoints {
		stride = len(tsList) / maxPoints
	}
	kept := make(map[time.Time]bool)
	for i := 0; i < len(tsList); i += stride {
		kept[tsList[i]] = true
	}
	if len(tsList) > 0 {
		kept[tsList[len(tsList)-1]] = true
	}

	tsKept := make([]time.Time, 0, len(kept))
	for _, t := range tsList {
		if kept[t] {
			tsKept = append(tsKept, t)
		}
	}
	tsIndex := make(map[time.Time]int, len(tsKept))
	for i, t := range tsKept {
		tsIndex[t] = i
	}

	out := &types.DiskIOHistory{
		Timestamps: []int64{},
		Devices:    map[string]*types.DiskIOSeries{},
	}
	for _, t := range tsKept {
		out.Timestamps = append(out.Timestamps, t.Unix())
	}
	for _, r := range all {
		if !kept[r.ts] {
			continue
		}
		ds, ok := out.Devices[r.device]
		if !ok {
			ds = &types.DiskIOSeries{
				ReadBytes:  make([]uint64, len(tsKept)),
				WriteBytes: make([]uint64, len(tsKept)),
			}
			out.Devices[r.device] = ds
		}
		idx := tsIndex[r.ts]
		ds.ReadBytes[idx] = r.read
		ds.WriteBytes[idx] = r.write
	}
	return out, nil
}

// --- rich monitoring history: inserts ---

// InsertTemps bulk-inserts per-sensor temperature readings from a sample.
// Best-effort: errors are logged by the caller and don't abort ingest.
func (s *Store) InsertTemps(ctx context.Context, m types.MetricSample) error {
	if len(m.Temps) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR IGNORE INTO temp_metrics (host_id, ts, sensor, temp_c)
		VALUES (?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, t := range m.Temps {
		if _, err := stmt.ExecContext(ctx, m.HostID, m.Timestamp, t.Name, t.Temp); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// InsertCPUCores bulk-inserts per-core CPU percentages from a sample.
func (s *Store) InsertCPUCores(ctx context.Context, m types.MetricSample) error {
	if len(m.CPUPerCore) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR IGNORE INTO cpu_core_metrics (host_id, ts, core, pct)
		VALUES (?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for i, pct := range m.CPUPerCore {
		if _, err := stmt.ExecContext(ctx, m.HostID, m.Timestamp, i, pct); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// InsertProcessSnapshot bulk-inserts the top-N process list from a sample.
func (s *Store) InsertProcessSnapshot(ctx context.Context, m types.MetricSample) error {
	if len(m.Processes) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR IGNORE INTO process_metrics (host_id, ts, pid, name, cpu_pct, mem_rss)
		VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, p := range m.Processes {
		if _, err := stmt.ExecContext(ctx, m.HostID, m.Timestamp, p.PID, p.Name, p.CPUPct, p.MemRSS); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// InsertContainerMetrics bulk-inserts per-container stats from a single tick.
// Separate from the metric-sample path because container stats can be
// collected on a different cadence than the host sample (the agent fetches
// docker stats on its own cycle).
func (s *Store) InsertContainerMetrics(ctx context.Context, samples []types.ContainerMetricSample) error {
	if len(samples) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR IGNORE INTO container_metrics
			(host_id, ts, container_id, name, state, cpu_pct, mem_used, mem_limit, net_rx, net_tx)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, c := range samples {
		if _, err := stmt.ExecContext(ctx,
			c.HostID, c.Timestamp, c.ContainerID, c.Name, c.State,
			c.CPUPct, c.MemUsed, c.MemLimit, c.NetRx, c.NetTx); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// --- rich monitoring history: ranges ---

// stridePicks returns a sorted list of timestamps from `all` downsampled to
// at most maxPoints entries (uniform stride; the last timestamp is always
// kept so charts align to the right edge of the range).
func stridePicks(all []time.Time, maxPoints int) ([]time.Time, map[time.Time]bool) {
	sort.Slice(all, func(i, j int) bool { return all[i].Before(all[j]) })
	stride := 1
	if maxPoints > 0 && len(all) > maxPoints {
		stride = len(all) / maxPoints
	}
	kept := make(map[time.Time]bool, len(all))
	for i := 0; i < len(all); i += stride {
		kept[all[i]] = true
	}
	if len(all) > 0 {
		kept[all[len(all)-1]] = true
	}
	out := make([]time.Time, 0, len(kept))
	for _, t := range all {
		if kept[t] {
			out = append(out, t)
		}
	}
	return out, kept
}

// TempRange returns per-sensor temperature readings for charting.
func (s *Store) TempRange(ctx context.Context, hostID string, since, until time.Time, maxPoints int) (*types.TempHistory, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT ts, sensor, temp_c
		FROM temp_metrics
		WHERE host_id = ? AND ts >= ? AND ts <= ?
		ORDER BY ts ASC, sensor ASC`, hostID, since, until)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type tempRow struct {
		ts     time.Time
		sensor string
		temp   float64
	}
	var all []tempRow
	tsSeen := make(map[time.Time]bool)
	for rows.Next() {
		var r tempRow
		if err := rows.Scan(&r.ts, &r.sensor, &r.temp); err != nil {
			return nil, err
		}
		all = append(all, r)
		tsSeen[r.ts] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	tsList := make([]time.Time, 0, len(tsSeen))
	for t := range tsSeen {
		tsList = append(tsList, t)
	}
	tsKept, kept := stridePicks(tsList, maxPoints)
	tsIndex := make(map[time.Time]int, len(tsKept))
	for i, t := range tsKept {
		tsIndex[t] = i
	}
	out := &types.TempHistory{
		Timestamps: make([]int64, 0, len(tsKept)),
		Sensors:    map[string][]float64{},
	}
	for _, t := range tsKept {
		out.Timestamps = append(out.Timestamps, t.Unix())
	}
	for _, r := range all {
		if !kept[r.ts] {
			continue
		}
		series, ok := out.Sensors[r.sensor]
		if !ok {
			series = make([]float64, len(tsKept))
			out.Sensors[r.sensor] = series
		}
		series[tsIndex[r.ts]] = r.temp
	}
	return out, nil
}

// CPUCoreRange returns per-core CPU% plus the aggregate from the metrics table.
// Aggregate comes from a parallel MetricsRange query (single source of truth for
// the overall CPU% line).
func (s *Store) CPUCoreRange(ctx context.Context, hostID string, since, until time.Time, maxPoints int) (*types.CPUCoreHistory, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT ts, core, pct
		FROM cpu_core_metrics
		WHERE host_id = ? AND ts >= ? AND ts <= ?
		ORDER BY ts ASC, core ASC`, hostID, since, until)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type coreRow struct {
		ts   time.Time
		core int
		pct  float64
	}
	var all []coreRow
	tsSeen := make(map[time.Time]bool)
	for rows.Next() {
		var r coreRow
		if err := rows.Scan(&r.ts, &r.core, &r.pct); err != nil {
			return nil, err
		}
		all = append(all, r)
		tsSeen[r.ts] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	tsList := make([]time.Time, 0, len(tsSeen))
	for t := range tsSeen {
		tsList = append(tsList, t)
	}
	tsKept, kept := stridePicks(tsList, maxPoints)
	tsIndex := make(map[time.Time]int, len(tsKept))
	for i, t := range tsKept {
		tsIndex[t] = i
	}
	out := &types.CPUCoreHistory{
		Timestamps: make([]int64, 0, len(tsKept)),
		Cores:      map[int][]float64{},
		Aggregate:  make([]float64, len(tsKept)),
	}
	for _, t := range tsKept {
		out.Timestamps = append(out.Timestamps, t.Unix())
	}
	for _, r := range all {
		if !kept[r.ts] {
			continue
		}
		series, ok := out.Cores[r.core]
		if !ok {
			series = make([]float64, len(tsKept))
			out.Cores[r.core] = series
		}
		series[tsIndex[r.ts]] = r.pct
	}
	// Fill aggregate from the metrics table over the same kept timestamps.
	aggRows, err := s.db.QueryContext(ctx, `
		SELECT ts, cpu_pct FROM metrics
		WHERE host_id = ? AND ts >= ? AND ts <= ?
		ORDER BY ts ASC`, hostID, since, until)
	if err == nil {
		defer aggRows.Close()
		for aggRows.Next() {
			var ts time.Time
			var pct float64
			if err := aggRows.Scan(&ts, &pct); err == nil {
				if i, ok := tsIndex[ts]; ok {
					out.Aggregate[i] = pct
				}
			}
		}
	}
	return out, nil
}

// ProcessRange returns one process's CPU + RSS series, keyed by name (PIDs
// churn so name is the stable axis for historical queries).
func (s *Store) ProcessRange(ctx context.Context, hostID, name string, since, until time.Time, maxPoints int) (*types.ProcessHistory, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT ts, cpu_pct, mem_rss
		FROM process_metrics
		WHERE host_id = ? AND name = ? AND ts >= ? AND ts <= ?
		ORDER BY ts ASC`, hostID, name, since, until)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type pRow struct {
		ts  time.Time
		cpu float64
		rss uint64
	}
	var all []pRow
	for rows.Next() {
		var r pRow
		if err := rows.Scan(&r.ts, &r.cpu, &r.rss); err != nil {
			return nil, err
		}
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Same process can appear with different PIDs across reboots — group by
	// timestamp and take the max (or first; first is fine since same-tick
	// duplicates are PK-rejected).
	tsList := make([]time.Time, 0, len(all))
	for _, r := range all {
		tsList = append(tsList, r.ts)
	}
	tsKept, kept := stridePicks(tsList, maxPoints)
	tsIndex := make(map[time.Time]int, len(tsKept))
	for i, t := range tsKept {
		tsIndex[t] = i
	}
	out := &types.ProcessHistory{
		Timestamps: make([]int64, 0, len(tsKept)),
		Name:       name,
		CPUPct:     make([]float64, len(tsKept)),
		MemRSS:     make([]uint64, len(tsKept)),
	}
	for _, t := range tsKept {
		out.Timestamps = append(out.Timestamps, t.Unix())
	}
	for _, r := range all {
		if !kept[r.ts] {
			continue
		}
		idx := tsIndex[r.ts]
		if r.cpu > out.CPUPct[idx] {
			out.CPUPct[idx] = r.cpu
		}
		if r.rss > out.MemRSS[idx] {
			out.MemRSS[idx] = r.rss
		}
	}
	return out, nil
}

// ContainerMetricRange returns one container's CPU/mem/net history.
func (s *Store) ContainerMetricRange(ctx context.Context, hostID, containerID string, since, until time.Time, maxPoints int) (*types.ContainerHistory, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT ts, name, cpu_pct, mem_used, net_rx, net_tx
		FROM container_metrics
		WHERE host_id = ? AND container_id = ? AND ts >= ? AND ts <= ?
		ORDER BY ts ASC`, hostID, containerID, since, until)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type cRow struct {
		ts             time.Time
		name           string
		cpu            float64
		mem            uint64
		netRx, netTx   uint64
	}
	var all []cRow
	for rows.Next() {
		var r cRow
		if err := rows.Scan(&r.ts, &r.name, &r.cpu, &r.mem, &r.netRx, &r.netTx); err != nil {
			return nil, err
		}
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	tsList := make([]time.Time, 0, len(all))
	for _, r := range all {
		tsList = append(tsList, r.ts)
	}
	tsKept, kept := stridePicks(tsList, maxPoints)
	tsIndex := make(map[time.Time]int, len(tsKept))
	for i, t := range tsKept {
		tsIndex[t] = i
	}
	out := &types.ContainerHistory{
		Timestamps:  make([]int64, 0, len(tsKept)),
		ContainerID: containerID,
		CPUPct:      make([]float64, len(tsKept)),
		MemUsed:     make([]uint64, len(tsKept)),
		NetRx:       make([]uint64, len(tsKept)),
		NetTx:       make([]uint64, len(tsKept)),
	}
	for _, t := range tsKept {
		out.Timestamps = append(out.Timestamps, t.Unix())
	}
	for _, r := range all {
		if !kept[r.ts] {
			continue
		}
		idx := tsIndex[r.ts]
		out.Name = r.name
		out.CPUPct[idx] = r.cpu
		out.MemUsed[idx] = r.mem
		out.NetRx[idx] = r.netRx
		out.NetTx[idx] = r.netTx
	}
	return out, nil
}

// --- per-host monitoring config ---

// DefaultHostConfig returns the built-in defaults used when a host has no
// host_config row. Mirrors the column defaults in schema.sql so behavior
// matches whether the row exists or not.
func DefaultHostConfig(hostID string) types.HostConfig {
	return types.HostConfig{
		HostID:             hostID,
		SampleIntervalS:    5,
		EnabledFamilies:    []string{"cpu", "mem", "disk", "net", "load", "temps", "processes", "cpu_per_core", "disk_io", "mounts", "containers"},
		FamilyIntervals:    map[string]int{},
		Filters:            types.HostConfigFilters{},
		MemCalc:            "used",
		RetentionDays:      30,
		RetentionOverrides: map[string]int{},
		WarnCPU: 70, CritCPU: 90,
		WarnMem: 80, CritMem: 90,
		WarnDisk: 80, CritDisk: 90,
		WarnTemp: 70, CritTemp: 85,
	}
}

// GetHostConfig returns the per-host monitoring policy, or the built-in
// defaults if no row exists for hostID. Never returns (nil, nil) — callers
// always get a usable config.
func (s *Store) GetHostConfig(ctx context.Context, hostID string) (types.HostConfig, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT sample_interval_s, enabled_families, family_intervals, filters, mem_calc,
		       retention_days, retention_overrides, primary_sensor, primary_mount,
		       warn_cpu, crit_cpu, warn_mem, crit_mem, warn_disk, crit_disk, warn_temp, crit_temp, updated_at
		FROM host_config WHERE host_id = ?`, hostID)
	var (
		cfg                = DefaultHostConfig(hostID)
		familiesJSON       string
		intervalsJSON      string
		filtersJSON        string
		retentionOverrides string
	)
	err := row.Scan(
		&cfg.SampleIntervalS, &familiesJSON, &intervalsJSON, &filtersJSON, &cfg.MemCalc,
		&cfg.RetentionDays, &retentionOverrides, &cfg.PrimarySensor, &cfg.PrimaryMount,
		&cfg.WarnCPU, &cfg.CritCPU, &cfg.WarnMem, &cfg.CritMem, &cfg.WarnDisk, &cfg.CritDisk, &cfg.WarnTemp, &cfg.CritTemp, &cfg.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if familiesJSON != "" {
		_ = json.Unmarshal([]byte(familiesJSON), &cfg.EnabledFamilies)
	}
	if intervalsJSON != "" {
		_ = json.Unmarshal([]byte(intervalsJSON), &cfg.FamilyIntervals)
	}
	if filtersJSON != "" {
		_ = json.Unmarshal([]byte(filtersJSON), &cfg.Filters)
	}
	if retentionOverrides != "" {
		_ = json.Unmarshal([]byte(retentionOverrides), &cfg.RetentionOverrides)
	}
	return cfg, nil
}

// UpsertHostConfig writes the per-host monitoring policy. JSON-typed fields
// are serialized; numeric and string fields go in directly.
func (s *Store) UpsertHostConfig(ctx context.Context, cfg types.HostConfig) error {
	familiesJSON, _ := json.Marshal(cfg.EnabledFamilies)
	intervalsJSON, _ := json.Marshal(cfg.FamilyIntervals)
	filtersJSON, _ := json.Marshal(cfg.Filters)
	retentionJSON, _ := json.Marshal(cfg.RetentionOverrides)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO host_config (host_id, sample_interval_s, enabled_families, family_intervals, filters, mem_calc,
		                        retention_days, retention_overrides, primary_sensor, primary_mount,
		                        warn_cpu, crit_cpu, warn_mem, crit_mem, warn_disk, crit_disk, warn_temp, crit_temp, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(host_id) DO UPDATE SET
			sample_interval_s   = excluded.sample_interval_s,
			enabled_families    = excluded.enabled_families,
			family_intervals    = excluded.family_intervals,
			filters             = excluded.filters,
			mem_calc            = excluded.mem_calc,
			retention_days      = excluded.retention_days,
			retention_overrides = excluded.retention_overrides,
			primary_sensor      = excluded.primary_sensor,
			primary_mount       = excluded.primary_mount,
			warn_cpu = excluded.warn_cpu, crit_cpu = excluded.crit_cpu,
			warn_mem = excluded.warn_mem, crit_mem = excluded.crit_mem,
			warn_disk = excluded.warn_disk, crit_disk = excluded.crit_disk,
			warn_temp = excluded.warn_temp, crit_temp = excluded.crit_temp,
			updated_at = CURRENT_TIMESTAMP`,
		cfg.HostID, cfg.SampleIntervalS, string(familiesJSON), string(intervalsJSON), string(filtersJSON), cfg.MemCalc,
		cfg.RetentionDays, string(retentionJSON), cfg.PrimarySensor, cfg.PrimaryMount,
		cfg.WarnCPU, cfg.CritCPU, cfg.WarnMem, cfg.CritMem, cfg.WarnDisk, cfg.CritDisk, cfg.WarnTemp, cfg.CritTemp,
	)
	return err
}

// PruneMetricsPerTable deletes rows older than the per-table cutoff. The
// caller computes cutoffs from each host's retention policy and passes a
// table→cutoff map. Tables not in the map are skipped (not pruned).
// Returns the total number of rows deleted across all tables.
func (s *Store) PruneMetricsPerTable(ctx context.Context, cutoffs map[string]time.Time) (int64, error) {
	var total int64
	for table, cutoff := range cutoffs {
		if !isMetricTable(table) {
			continue
		}
		res, err := s.db.ExecContext(ctx, `DELETE FROM `+table+` WHERE ts < ?`, cutoff)
		if err != nil {
			return total, fmt.Errorf("prune %s: %w", table, err)
		}
		n, _ := res.RowsAffected()
		total += n
	}
	return total, nil
}

// PruneHostMetrics deletes rows older than the per-table cutoff scoped to one
// host. Used by the per-host retention loop so host A's 7-day temp retention
// doesn't have to wait for host B's 30-day retention to expire.
func (s *Store) PruneHostMetrics(ctx context.Context, hostID string, cutoffs map[string]time.Time) (int64, error) {
	var total int64
	for table, cutoff := range cutoffs {
		if !isMetricTable(table) {
			continue
		}
		res, err := s.db.ExecContext(ctx, `DELETE FROM `+table+` WHERE host_id = ? AND ts < ?`, hostID, cutoff)
		if err != nil {
			return total, fmt.Errorf("prune host %s table %s: %w", hostID, table, err)
		}
		n, _ := res.RowsAffected()
		total += n
	}
	return total, nil
}

// isMetricTable allow-lists the metric tables that prune may touch. Prevents
// a future bug from injecting an arbitrary table name into a DELETE.
func isMetricTable(table string) bool {
	switch table {
	case "metrics", "net_iface_metrics", "disk_mount_metrics", "disk_io_metrics",
		"temp_metrics", "cpu_core_metrics", "process_metrics", "container_metrics":
		return true
	}
	return false
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
	if err := rs.Scan(&r.ID, &hostID, &r.Metric, &r.Op, &r.Threshold, &r.DurationS, &enabled, &r.Severity, &r.CreatedAt); err != nil {
		return r, err
	}
	if hostID.Valid {
		s := hostID.String
		r.HostID = &s
	}
	r.Enabled = enabled != 0
	if r.Severity == "" {
		r.Severity = "warning"
	}
	return r, nil
}

// ListAlertRules returns all rules. If hostID is non-nil, only rules that
// apply to that host (matching host_id, or host_id IS NULL).
func (s *Store) ListAlertRules(ctx context.Context, hostID *string) ([]types.AlertRule, error) {
	var rows *sql.Rows
	var err error
	if hostID != nil {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, host_id, metric, op, threshold, duration_s, enabled, severity, created_at
			FROM alert_rules
			WHERE host_id IS NULL OR host_id = ?
			ORDER BY id`, *hostID)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, host_id, metric, op, threshold, duration_s, enabled, severity, created_at
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
		SELECT id, host_id, metric, op, threshold, duration_s, enabled, severity, created_at
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
		SELECT id, host_id, metric, op, threshold, duration_s, enabled, severity, created_at
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
	if r.Severity == "" {
		r.Severity = "warning"
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO alert_rules (host_id, metric, op, threshold, duration_s, enabled, severity)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		hostID, r.Metric, r.Op, r.Threshold, r.DurationS, enabled, r.Severity)
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
	if r.Severity == "" {
		r.Severity = "warning"
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE alert_rules
		SET host_id = ?, metric = ?, op = ?, threshold = ?, duration_s = ?, enabled = ?, severity = ?
		WHERE id = ?`,
		hostID, r.Metric, r.Op, r.Threshold, r.DurationS, enabled, r.Severity, r.ID)
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

// --- alert channels ---

func scanAlertChannel(rs interface {
	Scan(dest ...any) error
}) (types.AlertChannel, error) {
	var ch types.AlertChannel
	var enabled, notifyResolve int
	var cfg string
	if err := rs.Scan(&ch.ID, &ch.Name, &ch.Type, &cfg, &enabled, &ch.MinSeverity, &notifyResolve, &ch.CreatedAt); err != nil {
		return ch, err
	}
	ch.Config = []byte(cfg)
	ch.Enabled = enabled != 0
	ch.NotifyResolve = notifyResolve != 0
	if ch.MinSeverity == "" {
		ch.MinSeverity = "info"
	}
	return ch, nil
}

func (s *Store) ListAlertChannels(ctx context.Context) ([]types.AlertChannel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, type, config, enabled, min_severity, notify_resolve, created_at
		FROM alert_channels ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []types.AlertChannel
	for rows.Next() {
		ch, err := scanAlertChannel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ch)
	}
	return out, rows.Err()
}

func (s *Store) ListEnabledChannels(ctx context.Context) ([]types.AlertChannel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, type, config, enabled, min_severity, notify_resolve, created_at
		FROM alert_channels WHERE enabled = 1 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []types.AlertChannel
	for rows.Next() {
		ch, err := scanAlertChannel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ch)
	}
	return out, rows.Err()
}

func (s *Store) GetAlertChannel(ctx context.Context, id int64) (*types.AlertChannel, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, type, config, enabled, min_severity, notify_resolve, created_at
		FROM alert_channels WHERE id = ?`, id)
	ch, err := scanAlertChannel(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ch, nil
}

func (s *Store) CreateAlertChannel(ctx context.Context, ch types.AlertChannel) (int64, error) {
	enabled := 0
	if ch.Enabled {
		enabled = 1
	}
	notifyResolve := 0
	if ch.NotifyResolve {
		notifyResolve = 1
	}
	if ch.MinSeverity == "" {
		ch.MinSeverity = "info"
	}
	cfg := ch.Config
	if len(cfg) == 0 {
		cfg = []byte("{}")
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO alert_channels (name, type, config, enabled, min_severity, notify_resolve)
		VALUES (?, ?, ?, ?, ?, ?)`,
		ch.Name, ch.Type, string(cfg), enabled, ch.MinSeverity, notifyResolve)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateAlertChannel(ctx context.Context, ch types.AlertChannel) error {
	enabled := 0
	if ch.Enabled {
		enabled = 1
	}
	notifyResolve := 0
	if ch.NotifyResolve {
		notifyResolve = 1
	}
	if ch.MinSeverity == "" {
		ch.MinSeverity = "info"
	}
	cfg := ch.Config
	if len(cfg) == 0 {
		cfg = []byte("{}")
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE alert_channels
		SET name = ?, type = ?, config = ?, enabled = ?, min_severity = ?, notify_resolve = ?
		WHERE id = ?`,
		ch.Name, ch.Type, string(cfg), enabled, ch.MinSeverity, notifyResolve, ch.ID)
	return err
}

func (s *Store) DeleteAlertChannel(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM alert_channels WHERE id = ?`, id)
	return err
}

// --- agent tokens ---

// hashToken returns the hex-encoded SHA-256 digest of the plaintext token.
func hashToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// CreateAgentToken generates a new 32-byte cryptographically random token,
// stores its SHA-256 hash, and returns the AgentToken with the plaintext
// Token field set. This is the only time the plaintext is available.
func (s *Store) CreateAgentToken(ctx context.Context, name string) (types.AgentToken, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return types.AgentToken{}, fmt.Errorf("generate token: %w", err)
	}
	plaintext := hex.EncodeToString(raw) // 64 hex chars
	hash := hashToken(plaintext)

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO agent_tokens (name, token_hash) VALUES (?, ?)`, name, hash)
	if err != nil {
		return types.AgentToken{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return types.AgentToken{}, err
	}
	return types.AgentToken{
		ID:        id,
		Name:      name,
		CreatedAt: time.Now().UTC(),
		Token:     plaintext,
	}, nil
}

// ListAgentTokens returns all non-revoked agent tokens (without plaintext).
func (s *Store) ListAgentTokens(ctx context.Context) ([]types.AgentToken, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, created_at, last_used, revoked
		FROM agent_tokens
		WHERE revoked = 0
		ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []types.AgentToken
	for rows.Next() {
		var t types.AgentToken
		var revoked int
		var lastUsed sql.NullTime
		if err := rows.Scan(&t.ID, &t.Name, &t.CreatedAt, &lastUsed, &revoked); err != nil {
			return nil, err
		}
		t.Revoked = revoked != 0
		if lastUsed.Valid {
			lu := lastUsed.Time
			t.LastUsed = &lu
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// RevokeAgentToken sets revoked=1 for the given token id.
func (s *Store) RevokeAgentToken(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE agent_tokens SET revoked = 1 WHERE id = ?`, id)
	return err
}

// VerifyAgentToken hashes the plaintext, looks it up, checks it is not
// revoked, updates last_used, and returns the token metadata.
func (s *Store) VerifyAgentToken(ctx context.Context, plaintext string) (types.AgentToken, error) {
	hash := hashToken(plaintext)
	var t types.AgentToken
	var revoked int
	var lastUsed sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, created_at, last_used, revoked
		FROM agent_tokens WHERE token_hash = ?`, hash).Scan(
		&t.ID, &t.Name, &t.CreatedAt, &lastUsed, &revoked)
	if err == sql.ErrNoRows {
		return types.AgentToken{}, fmt.Errorf("invalid token")
	}
	if err != nil {
		return types.AgentToken{}, err
	}
	if revoked != 0 {
		return types.AgentToken{}, fmt.Errorf("token revoked")
	}
	t.Revoked = false
	if lastUsed.Valid {
		lu := lastUsed.Time
		t.LastUsed = &lu
	}
	// Best-effort update of last_used; don't fail auth if this write fails.
	now := time.Now().UTC()
	_, _ = s.db.ExecContext(ctx, `UPDATE agent_tokens SET last_used = ? WHERE id = ?`, now, t.ID)
	return t, nil
}

// --- compose versions ---

func (s *Store) SaveComposeVersion(ctx context.Context, hostID, project, content string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Insert the new version
	_, err = tx.ExecContext(ctx, `
		INSERT INTO compose_versions (host_id, project, yaml_content)
		VALUES (?, ?, ?)`, hostID, project, content)
	if err != nil {
		return err
	}

	// Prune older versions, keeping only the 10 most recent for this project
	_, err = tx.ExecContext(ctx, `
		DELETE FROM compose_versions 
		WHERE id NOT IN (
			SELECT id FROM compose_versions 
			WHERE host_id = ? AND project = ? 
			ORDER BY created_at DESC 
			LIMIT 10
		) AND host_id = ? AND project = ?`, hostID, project, hostID, project)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Store) ListComposeVersions(ctx context.Context, hostID, project string) ([]types.ComposeVersion, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, host_id, project, created_at 
		FROM compose_versions 
		WHERE host_id = ? AND project = ? 
		ORDER BY created_at DESC`, hostID, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []types.ComposeVersion
	for rows.Next() {
		var v types.ComposeVersion
		var t time.Time
		if err := rows.Scan(&v.ID, &v.HostID, &v.Project, &t); err != nil {
			return nil, err
		}
		v.CreatedAt = t.Format(time.RFC3339)
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

func (s *Store) GetComposeVersion(ctx context.Context, id int64) (*types.ComposeVersion, error) {
	var v types.ComposeVersion
	var t time.Time
	err := s.db.QueryRowContext(ctx, `
		SELECT id, host_id, project, created_at, yaml_content
		FROM compose_versions
		WHERE id = ?`, id).Scan(&v.ID, &v.HostID, &v.Project, &t, &v.Content)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	v.CreatedAt = t.Format(time.RFC3339)
	return &v, nil
}

// --- auth ---

// IsPasswordSet reports whether an admin password has been configured.
func (s *Store) IsPasswordSet(ctx context.Context) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM auth_config`).Scan(&n)
	return n > 0, err
}

// GetPasswordHash retrieves the stored bcrypt hash. Returns ("", nil) if no
// password has been set yet.
func (s *Store) GetPasswordHash(ctx context.Context) (string, error) {
	var h string
	err := s.db.QueryRowContext(ctx, `SELECT password_hash FROM auth_config WHERE id = 1`).Scan(&h)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return h, err
}

// SetPasswordHash upserts the bcrypt hash for the single admin account.
func (s *Store) SetPasswordHash(ctx context.Context, hash string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO auth_config (id, password_hash) VALUES (1, ?)
		ON CONFLICT(id) DO UPDATE SET password_hash = excluded.password_hash, created_at = CURRENT_TIMESTAMP`,
		hash)
	return err
}

// CreateSession stores a new session token with an expiry time.
func (s *Store) CreateSession(ctx context.Context, token string, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (token, expires_at) VALUES (?, ?)`, token, expiresAt)
	return err
}

// ValidateSession returns true if the token exists and has not expired.
func (s *Store) ValidateSession(ctx context.Context, token string) (bool, error) {
	var exp time.Time
	err := s.db.QueryRowContext(ctx,
		`SELECT expires_at FROM sessions WHERE token = ?`, token).Scan(&exp)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return time.Now().Before(exp), nil
}

// DeleteSession removes a session (logout).
func (s *Store) DeleteSession(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token = ?`, token)
	return err
}

// PruneExpiredSessions removes sessions whose expiry has passed.
func (s *Store) PruneExpiredSessions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= CURRENT_TIMESTAMP`)
	return err
}
