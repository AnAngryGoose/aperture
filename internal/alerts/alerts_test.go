package alerts

import (
	"testing"
	"time"

	"github.com/aperture/aperture/internal/types"
)

func TestMetricValueFlat(t *testing.T) {
	s := types.MetricSample{
		CPUPercent: 47.3,
		MemPercent: 62.1,
		SwapUsed:   50, SwapTotal: 100,
		LoadAvg5: 1.5,
	}
	cases := []struct {
		metric string
		want   float64
	}{
		{"cpu_pct", 47.3},
		{"mem_pct", 62.1},
		{"swap_pct", 50},
		{"load_5", 1.5},
	}
	for _, c := range cases {
		got, ok := MetricValue(s, c.metric)
		if !ok {
			t.Errorf("%s: not found", c.metric)
			continue
		}
		if got != c.want {
			t.Errorf("%s: got %v want %v", c.metric, got, c.want)
		}
	}
}

func TestMetricValueDotted(t *testing.T) {
	s := types.MetricSample{
		Timestamp:  time.Now(),
		NetIfaces:  []types.NetInterfaceSample{{Name: "eth0", RxRate: 1024, TxRate: 512, RxBytes: 9999}},
		DiskMounts: []types.DiskMountSample{{Mount: "/", Percent: 73.5, Used: 1000, Total: 2000}},
		Temps:      []types.TempSample{{Name: "cpu", Temp: 65.2}, {Name: "nvme", Temp: 40}},
		Processes:  []types.ProcessSample{{Name: "nginx", CPUPct: 12, MemRSS: 4096}, {Name: "nginx", CPUPct: 25, MemRSS: 8192}},
	}
	cases := []struct {
		metric string
		want   float64
		found  bool
	}{
		{"iface.eth0.rx_rate", 1024, true},
		{"iface.eth0.tx_rate", 512, true},
		{"iface.eth1.rx_rate", 0, false},
		{"mount./.pct", 73.5, true},
		{"mount./.used", 1000, true},
		{"temp.cpu.value", 65.2, true},
		{"temp.max", 65.2, true},
		{"proc.nginx.cpu_pct", 25, true}, // max across both processes
		{"proc.missing.cpu_pct", 0, false},
		{"bogus.foo.bar", 0, false},
	}
	for _, c := range cases {
		got, ok := MetricValue(s, c.metric)
		if ok != c.found {
			t.Errorf("%s: found=%v want %v (val=%v)", c.metric, ok, c.found, got)
			continue
		}
		if ok && got != c.want {
			t.Errorf("%s: got %v want %v", c.metric, got, c.want)
		}
	}
}

func TestValidateRule(t *testing.T) {
	good := []string{"cpu_pct", "mem_pct", "iface.eth0.rx_rate", "mount./.pct", "temp.cpu.value", "proc.nginx.cpu_pct"}
	for _, m := range good {
		r := types.AlertRule{Metric: m, Op: ">", Threshold: 1}
		if err := ValidateRule(r); err != nil {
			t.Errorf("ValidateRule(%q) unexpected error: %v", m, err)
		}
	}
	bad := []string{"", "bogus", "iface.eth0", "mount./.nonsense", "temp.x", "category"}
	for _, m := range bad {
		r := types.AlertRule{Metric: m, Op: ">", Threshold: 1}
		if err := ValidateRule(r); err == nil {
			t.Errorf("ValidateRule(%q) expected error, got none", m)
		}
	}
}
