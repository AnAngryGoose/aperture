// Package alerts evaluates threshold-based rules against incoming metric
// samples and emits alert events when a rule's breach condition is sustained
// for at least its configured duration.
//
// Lifecycle: the hub calls Evaluator.Evaluate after every successful metric
// insert. Evaluation is host-scoped (only rules that apply to the sample's
// host are considered) so the work scales with rules-per-host, not total
// rules.
//
// State model:
//   - Persistent: alert_rules (configuration) and alert_events (history,
//     including currently-open events with NULL resolved_at) live in SQLite.
//   - Transient: a per-(rule, host) "first observed breach" timestamp lives
//     only in memory. On hub restart, transient state is rebuilt by loading
//     all open events back into the firing map; a sustained breach that had
//     not yet fired before the restart will start its duration timer fresh.
//     This is acceptable for v0.1: pending breach state is short-lived
//     (seconds-to-minutes), losing it across restarts only delays a fire by
//     up to one duration window, and persisting every-tick state would be a
//     hot SQLite write per rule per sample.
package alerts

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/aperture/aperture/internal/store"
	"github.com/aperture/aperture/internal/types"
)

// Evaluator runs all enabled rules against incoming samples and persists
// alert_events transitions (fire / resolve).
type Evaluator struct {
	store    *store.Store
	log      *slog.Logger
	notifier *Notifier // nil-safe; set via SetNotifier before Run
	mu       sync.Mutex
	// pending tracks the first time a rule's breach condition was observed,
	// per host. Cleared when the breach ends (resolves) or when the event
	// fires (moves to "open"). Used to enforce sustained-duration semantics.
	pending map[ruleHostKey]time.Time
	// open is the live map of currently-firing (rule_id, host_id) -> stored
	// AlertEvent. Storing the full event (not just the ID) means resolve
	// notifications carry the original FiredAt without an extra DB round-trip.
	// Loaded from the DB on construction so a hub restart can resolve events
	// that were open at shutdown.
	open map[ruleHostKey]types.AlertEvent
}

// SetNotifier wires in a Notifier so fired/resolved events are dispatched to
// configured channels. Must be called before the first Evaluate call.
func (e *Evaluator) SetNotifier(n *Notifier) { e.notifier = n }

type ruleHostKey struct {
	ruleID int64
	hostID string
}

// New constructs an Evaluator and rehydrates its open-events cache.
func New(ctx context.Context, st *store.Store, log *slog.Logger) (*Evaluator, error) {
	if log == nil {
		log = slog.Default()
	}
	e := &Evaluator{
		store:   st,
		log:     log,
		pending: make(map[ruleHostKey]time.Time),
		open:    make(map[ruleHostKey]types.AlertEvent),
	}
	openEvents, err := st.ListAlertEvents(ctx, store.AlertEventFilter{OpenOnly: true, Limit: 10_000})
	if err != nil {
		return nil, fmt.Errorf("load open alert events: %w", err)
	}
	for _, ev := range openEvents {
		e.open[ruleHostKey{ev.RuleID, ev.HostID}] = ev
	}
	if len(openEvents) > 0 {
		log.Info("alert evaluator restored open events", "count", len(openEvents))
	}
	return e, nil
}

// Evaluate runs every enabled rule that applies to sample.HostID and persists
// any state transitions. It is safe to call concurrently with itself for
// different hosts; the internal mutex keeps the open/pending maps consistent.
func (e *Evaluator) Evaluate(ctx context.Context, sample types.MetricSample) {
	rules, err := e.store.ListEnabledRulesFor(ctx, sample.HostID)
	if err != nil {
		e.log.Error("alerts: list rules", "host_id", sample.HostID, "err", err)
		return
	}
	for _, r := range rules {
		e.evalOne(ctx, r, sample)
	}
}

func (e *Evaluator) evalOne(ctx context.Context, r types.AlertRule, sample types.MetricSample) {
	val, ok := MetricValue(sample, r.Metric)
	if !ok {
		// Unknown metric in a rule. Log once and bail; misconfigured rules
		// shouldn't burn CPU but they also shouldn't crash anything.
		e.log.Warn("alerts: unsupported metric", "rule_id", r.ID, "metric", r.Metric)
		return
	}
	breach := compare(val, r.Op, r.Threshold)
	key := ruleHostKey{ruleID: r.ID, hostID: sample.HostID}

	e.mu.Lock()
	defer e.mu.Unlock()

	if breach {
		// Already firing: nothing to do.
		if _, firing := e.open[key]; firing {
			return
		}
		first, hasPending := e.pending[key]
		if !hasPending {
			e.pending[key] = sample.Timestamp
			if r.DurationS == 0 {
				e.fire(ctx, r, sample, val, key)
			}
			return
		}
		if sample.Timestamp.Sub(first) >= time.Duration(r.DurationS)*time.Second {
			e.fire(ctx, r, sample, val, key)
		}
		// Otherwise still in the "wait for sustained breach" window.
	} else {
		// Not breaching anymore. Cancel any pending tracking…
		delete(e.pending, key)
		// …and resolve any currently-open event.
		if openEv, firing := e.open[key]; firing {
			if err := e.store.ResolveAlertEvent(ctx, openEv.ID, sample.Timestamp); err != nil {
				e.log.Error("alerts: resolve event", "id", openEv.ID, "err", err)
				return
			}
			delete(e.open, key)
			e.log.Info("alert resolved", "rule_id", r.ID, "host_id", sample.HostID, "event_id", openEv.ID, "value", val)
			if e.notifier != nil {
				// Use the stored FiredAt so notifications show when the alert
				// originally fired, not when it resolved.
				resolvedEv := openEv
				resolvedEv.Value = val
				go e.notifier.Dispatch(ctx, resolvedEv, r, true)
			}
		}
	}
}

// fire records a new alert_event and clears the pending entry. Caller holds
// e.mu.
func (e *Evaluator) fire(ctx context.Context, r types.AlertRule, sample types.MetricSample, val float64, key ruleHostKey) {
	ev := types.AlertEvent{
		RuleID:  r.ID,
		HostID:  sample.HostID,
		FiredAt: sample.Timestamp,
		Value:   val,
	}
	id, err := e.store.InsertAlertEvent(ctx, ev)
	if err != nil {
		e.log.Error("alerts: insert event", "rule_id", r.ID, "err", err)
		return
	}
	ev.ID = id
	e.open[key] = ev
	delete(e.pending, key)
	e.log.Warn("alert fired",
		"rule_id", r.ID, "host_id", sample.HostID, "event_id", id,
		"metric", r.Metric, "op", r.Op, "threshold", r.Threshold, "value", val)
	if e.notifier != nil {
		go e.notifier.Dispatch(ctx, ev, r, false)
	}
}

// HandleRuleDelete drops any in-memory state for a rule that's been deleted.
// Call from the API layer after a successful DELETE so we don't keep stale
// open/pending entries that can never resolve.
func (e *Evaluator) HandleRuleDelete(ruleID int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for k := range e.pending {
		if k.ruleID == ruleID {
			delete(e.pending, k)
		}
	}
	for k := range e.open {
		if k.ruleID == ruleID {
			delete(e.open, k)
		}
	}
}

// SupportedMetrics is the canonical list of metric names rules can target.
// Kept here (rather than in types) because it's evaluator-specific; the API
// uses it for validation.
var SupportedMetrics = []string{
	"cpu_pct", "mem_pct", "disk_pct", "swap_pct",
	"load_1", "load_5", "load_15",
}

// MetricValue extracts the named metric from a sample. Returns false on an
// unknown name so the evaluator can warn instead of fire spuriously.
func MetricValue(s types.MetricSample, metric string) (float64, bool) {
	switch metric {
	case "cpu_pct":
		return s.CPUPercent, true
	case "mem_pct":
		return s.MemPercent, true
	case "disk_pct":
		return s.DiskPercent, true
	case "swap_pct":
		if s.SwapTotal == 0 {
			return 0, true
		}
		return float64(s.SwapUsed) / float64(s.SwapTotal) * 100.0, true
	case "load_1":
		return s.LoadAvg1, true
	case "load_5":
		return s.LoadAvg5, true
	case "load_15":
		return s.LoadAvg15, true
	}
	return 0, false
}

// SupportedOps is the canonical comparison-operator list. Used for API
// validation so rule configs can't slip through with invalid operators.
var SupportedOps = []string{">", ">=", "<", "<="}

func compare(v float64, op string, threshold float64) bool {
	switch op {
	case ">":
		return v > threshold
	case ">=":
		return v >= threshold
	case "<":
		return v < threshold
	case "<=":
		return v <= threshold
	}
	return false
}

// ValidateRule checks a rule's metric/op fields against the canonical lists.
// Centralized so create and update share validation logic.
func ValidateRule(r types.AlertRule) error {
	if r.Metric == "" {
		return errors.New("metric is required")
	}
	if !contains(SupportedMetrics, r.Metric) {
		return fmt.Errorf("unsupported metric %q (allowed: %v)", r.Metric, SupportedMetrics)
	}
	if !contains(SupportedOps, r.Op) {
		return fmt.Errorf("unsupported op %q (allowed: %v)", r.Op, SupportedOps)
	}
	if r.DurationS < 0 {
		return errors.New("duration_s must be >= 0")
	}
	return nil
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
