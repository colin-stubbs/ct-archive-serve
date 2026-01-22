package ctarchiveserve

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics provides low-cardinality Prometheus metrics for ct-archive-serve.
//
// Per specs/001-ct-archive-serve/spec.md (NFR-009), metrics are limited to:
// - `/monitor.json` aggregate
// - per-`<log>` aggregates for all `/<log>/...` requests combined
//
// Metrics MUST NOT be labeled by status code, endpoint name, or full request path.
type Metrics struct {
	monitorJSONRequestsTotal   prometheus.Counter
	monitorJSONRequestDuration prometheus.Histogram

	logRequestsTotal   *prometheus.CounterVec
	logRequestDuration *prometheus.HistogramVec

	archiveLogsDiscovered     prometheus.Gauge
	archiveZipPartsDiscovered prometheus.Gauge

	zipCacheOpen       prometheus.Gauge
	zipCacheEvictions  prometheus.Counter
	zipIntegrityPassed prometheus.Counter
	zipIntegrityFailed prometheus.Counter
}

// NewMetrics constructs and registers the service's metrics.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}

	m := &Metrics{
		monitorJSONRequestsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "ct_archive_serve",
			Subsystem: "http",
			Name:      "monitor_json_requests_total",
			Help:      "Total number of /monitor.json requests.",
		}),
		monitorJSONRequestDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "ct_archive_serve",
			Subsystem: "http",
			Name:      "monitor_json_request_duration_seconds",
			Help:      "Duration of /monitor.json requests in seconds.",
			Buckets:   prometheus.DefBuckets,
		}),
		logRequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "ct_archive_serve",
			Subsystem: "http",
			Name:      "log_requests_total",
			Help:      "Total number of requests under /<log>/... aggregated by log.",
		}, []string{"log"}),
		logRequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "ct_archive_serve",
			Subsystem: "http",
			Name:      "log_request_duration_seconds",
			Help:      "Duration of requests under /<log>/... in seconds aggregated by log.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"log"}),

		archiveLogsDiscovered: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "ct_archive_serve",
			Name:      "archive_logs_discovered",
			Help:      "Number of archive logs currently discovered by the archive index.",
		}),
		archiveZipPartsDiscovered: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "ct_archive_serve",
			Name:      "archive_zip_parts_discovered",
			Help:      "Number of zip parts currently discovered across all logs by the archive index.",
		}),

		zipCacheOpen: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "ct_archive_serve",
			Name:      "zip_cache_open",
			Help:      "Current number of open zip parts held by the zip cache.",
		}),
		zipCacheEvictions: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "ct_archive_serve",
			Name:      "zip_cache_evictions_total",
			Help:      "Total number of zip cache evictions.",
		}),
		zipIntegrityPassed: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "ct_archive_serve",
			Name:      "zip_integrity_passed_total",
			Help:      "Total number of zip parts that passed structural integrity checks.",
		}),
		zipIntegrityFailed: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "ct_archive_serve",
			Name:      "zip_integrity_failed_total",
			Help:      "Total number of zip parts that failed structural integrity checks.",
		}),
	}

	reg.MustRegister(
		m.monitorJSONRequestsTotal,
		m.monitorJSONRequestDuration,
		m.logRequestsTotal,
		m.logRequestDuration,
		m.archiveLogsDiscovered,
		m.archiveZipPartsDiscovered,
		m.zipCacheOpen,
		m.zipCacheEvictions,
		m.zipIntegrityPassed,
		m.zipIntegrityFailed,
	)

	return m
}

func (m *Metrics) ObserveMonitorJSONRequest(d time.Duration) {
	if m == nil {
		return
	}
	m.monitorJSONRequestsTotal.Inc()
	m.monitorJSONRequestDuration.Observe(d.Seconds())
}

func (m *Metrics) ObserveLogRequest(log string, d time.Duration) {
	if m == nil {
		return
	}
	m.logRequestsTotal.WithLabelValues(log).Inc()
	m.logRequestDuration.WithLabelValues(log).Observe(d.Seconds())
}

func (m *Metrics) SetArchiveDiscovered(logCount, zipPartCount int) {
	if m == nil {
		return
	}
	m.archiveLogsDiscovered.Set(float64(logCount))
	m.archiveZipPartsDiscovered.Set(float64(zipPartCount))
}

func (m *Metrics) SetZipCacheOpen(n int) {
	if m == nil {
		return
	}
	m.zipCacheOpen.Set(float64(n))
}

func (m *Metrics) IncZipCacheEvictions() {
	if m == nil {
		return
	}
	m.zipCacheEvictions.Inc()
}

func (m *Metrics) IncZipIntegrityPassed() {
	if m == nil {
		return
	}
	m.zipIntegrityPassed.Inc()
}

func (m *Metrics) IncZipIntegrityFailed() {
	if m == nil {
		return
	}
	m.zipIntegrityFailed.Inc()
}

