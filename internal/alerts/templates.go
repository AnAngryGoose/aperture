// Package alerts — predefined alert-rule templates.
//
// A template is a named set of AlertRule rows that can be applied to a host
// (or globally) with one POST. The catalog returned by
// /api/monitoring/catalog (see api/monitoring.go) lists the same templates
// the editor uses, so the frontend doesn't need a parallel list.
//
// Why server-side: the rules belong to the same threshold space the
// status-derivation code uses (host_config). Keeping the canonical
// definitions next to the evaluator means a future tweak (e.g. lifting a
// default threshold) is a one-line change with no UI deploy.
package alerts

import (
	"context"
	"fmt"

	"github.com/aperture/aperture/internal/store"
	"github.com/aperture/aperture/internal/types"
)

// Template is one entry in the global template registry. Rules are
// host-agnostic — they'll be cloned per-host (or globally) at apply time.
type Template struct {
	Name        string
	Description string
	Rules       []TemplateRule
}

// TemplateRule is the wire-shape entry the frontend renders, mirroring the
// alertTemplateRule struct in api/monitoring.go.
type TemplateRule struct {
	Metric    string
	Op        string
	Threshold float64
	DurationS int
	Severity  string
}

// Templates returns the built-in template set. Defined as a function rather
// than a package-level var so future additions (e.g. user-defined templates
// from user_settings) can layer in without a struct-field break.
func Templates() []Template {
	return []Template{
		{
			Name:        "Beszel defaults",
			Description: "Common thresholds matching the Beszel out-of-the-box rule set.",
			Rules: []TemplateRule{
				{Metric: "cpu_pct", Op: ">", Threshold: 90, DurationS: 60, Severity: "warning"},
				{Metric: "mem_pct", Op: ">", Threshold: 90, DurationS: 60, Severity: "warning"},
				{Metric: "disk_pct", Op: ">", Threshold: 85, DurationS: 300, Severity: "warning"},
				{Metric: "temp.max", Op: ">", Threshold: 80, DurationS: 60, Severity: "critical"},
				// host.status encoding: ok=0, warn=1, crit=2, offline=3.
				// "offline" alert: status == 3 sustained for 2 minutes.
				{Metric: "host.status", Op: "==", Threshold: 3, DurationS: 120, Severity: "critical"},
			},
		},
		{
			Name:        "Aggressive",
			Description: "Lower thresholds and shorter durations for environments that should never spike.",
			Rules: []TemplateRule{
				{Metric: "cpu_pct", Op: ">", Threshold: 75, DurationS: 30, Severity: "warning"},
				{Metric: "mem_pct", Op: ">", Threshold: 80, DurationS: 30, Severity: "warning"},
				{Metric: "disk_pct", Op: ">", Threshold: 75, DurationS: 60, Severity: "warning"},
				{Metric: "host.status", Op: "==", Threshold: 3, DurationS: 60, Severity: "critical"},
			},
		},
		{
			Name:        "Quiet",
			Description: "Higher thresholds for noisy workloads where short spikes are normal.",
			Rules: []TemplateRule{
				{Metric: "cpu_pct", Op: ">", Threshold: 95, DurationS: 300, Severity: "warning"},
				{Metric: "mem_pct", Op: ">", Threshold: 95, DurationS: 300, Severity: "warning"},
				{Metric: "disk_pct", Op: ">", Threshold: 92, DurationS: 600, Severity: "warning"},
			},
		},
	}
}

// TemplateByName returns the named template or nil if it doesn't exist.
func TemplateByName(name string) *Template {
	for _, t := range Templates() {
		if t.Name == name {
			return &t
		}
	}
	return nil
}

// ApplyTemplate clones a template's rules into the alert_rules table,
// scoped to the given host (or globally when hostID is nil). Returns the
// created rule IDs. Existing rules with the same (metric, op, threshold)
// triple are left alone — apply is additive, not destructive.
func ApplyTemplate(ctx context.Context, st *store.Store, template Template, hostID *string) ([]int64, error) {
	// Snapshot existing rules so we can skip duplicates without a per-rule
	// SELECT round-trip.
	existing, err := st.ListAlertRules(ctx, hostID)
	if err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	seen := make(map[string]bool, len(existing))
	for _, r := range existing {
		seen[ruleKey(r.Metric, r.Op, r.Threshold)] = true
	}

	created := make([]int64, 0, len(template.Rules))
	for _, tr := range template.Rules {
		if seen[ruleKey(tr.Metric, tr.Op, tr.Threshold)] {
			continue
		}
		rule := types.AlertRule{
			HostID:    hostID,
			Metric:    tr.Metric,
			Op:        tr.Op,
			Threshold: tr.Threshold,
			DurationS: tr.DurationS,
			Severity:  tr.Severity,
			Enabled:   true,
		}
		// Defensive: bail on a template that fails the canonical validator
		// rather than persist an unusable rule. Surfaces bad templates loudly.
		if err := ValidateRule(rule); err != nil {
			return created, fmt.Errorf("template %q rule %s %s %g: %w",
				template.Name, tr.Metric, tr.Op, tr.Threshold, err)
		}
		id, err := st.CreateAlertRule(ctx, rule)
		if err != nil {
			return created, fmt.Errorf("create rule: %w", err)
		}
		created = append(created, id)
	}
	return created, nil
}

func ruleKey(metric, op string, threshold float64) string {
	return fmt.Sprintf("%s|%s|%g", metric, op, threshold)
}
