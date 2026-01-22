package ctarchiveserve

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestMetrics_LowCardinality(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)

	m.ObserveMonitorJSONRequest(120 * time.Millisecond)
	m.ObserveLogRequest("example_log", 50*time.Millisecond)

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}

	// Ensure the per-log series only uses the `log` label.
	assertMetricFamilyLabelNames(t, mfs, "ct_archive_serve_http_log_requests_total", []string{"log"})
	assertMetricFamilyLabelNames(t, mfs, "ct_archive_serve_http_log_request_duration_seconds", []string{"log"})

	// Ensure monitor.json metrics have no labels.
	assertMetricFamilyLabelNames(t, mfs, "ct_archive_serve_http_monitor_json_requests_total", nil)
	assertMetricFamilyLabelNames(t, mfs, "ct_archive_serve_http_monitor_json_request_duration_seconds", nil)
}

func TestMetrics_ResourceObservability_NoLabels(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	_ = NewMetrics(reg)

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}

	assertMetricFamilyLabelNames(t, mfs, "ct_archive_serve_archive_logs_discovered", nil)
	assertMetricFamilyLabelNames(t, mfs, "ct_archive_serve_archive_zip_parts_discovered", nil)
	assertMetricFamilyLabelNames(t, mfs, "ct_archive_serve_zip_cache_open", nil)
	assertMetricFamilyLabelNames(t, mfs, "ct_archive_serve_zip_cache_evictions_total", nil)
	assertMetricFamilyLabelNames(t, mfs, "ct_archive_serve_zip_integrity_passed_total", nil)
	assertMetricFamilyLabelNames(t, mfs, "ct_archive_serve_zip_integrity_failed_total", nil)
}

func assertMetricFamilyLabelNames(t *testing.T, mfs []*dto.MetricFamily, name string, want []string) {
	t.Helper()

	var mf *dto.MetricFamily
	for _, x := range mfs {
		if x.GetName() == name {
			mf = x
			break
		}
	}
	if mf == nil {
		t.Fatalf("metric family %q not found", name)
	}
	if len(mf.Metric) == 0 {
		t.Fatalf("metric family %q has no metrics", name)
	}

	// For families that have multiple series, validate they all use the same label keys.
	for _, m := range mf.Metric {
		got := make([]string, 0, len(m.Label))
		for _, lp := range m.Label {
			got = append(got, lp.GetName())
		}
		if !stringSlicesEqualUnordered(got, want) {
			t.Fatalf("metric family %q label names = %v, want %v", name, got, want)
		}
	}
}

func stringSlicesEqualUnordered(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}

	ma := make(map[string]int, len(a))
	for _, s := range a {
		ma[s]++
	}
	for _, s := range b {
		ma[s]--
		if ma[s] < 0 {
			return false
		}
	}
	for _, v := range ma {
		if v != 0 {
			return false
		}
	}
	return true
}

