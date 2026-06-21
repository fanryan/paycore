package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	registry                         *prometheus.Registry
	httpRequestsTotal                *prometheus.CounterVec
	httpRequestDuration              *prometheus.HistogramVec
	settlementBatchTotal             *prometheus.CounterVec
	settlementBatchDuration          *prometheus.HistogramVec
	settlementPaymentsTotal          prometheus.Counter
	settlementRecoveredBatchesTotal  prometheus.Counter
	outboxClaimedEventsTotal         *prometheus.CounterVec
	outboxPublishAttemptsTotal       *prometheus.CounterVec
	outboxPublishFailuresTotal       *prometheus.CounterVec
	outboxEventsPublishedTotal       *prometheus.CounterVec
	rateLimitAllowedTotal            prometheus.Counter
	rateLimitRejectedTotal           prometheus.Counter
	rateLimitRedisErrorsTotal        prometheus.Counter
	rateLimitCheckDuration           *prometheus.HistogramVec
	idempotencyCacheHitsTotal        prometheus.Counter
	idempotencyCacheMissesTotal      prometheus.Counter
	idempotencyCacheErrorsTotal      prometheus.Counter
	idempotencyPostgresFallbackTotal prometheus.Counter
}

func New() *Metrics {
	registry := prometheus.NewRegistry()
	httpRequestsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "paycore_http_requests_total",
			Help: "Total HTTP requests handled by PayCore.",
		},
		[]string{"method", "route", "status"},
	)
	httpRequestDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "paycore_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "route", "status"},
	)
	settlementBatchTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "paycore_settlement_batch_total",
			Help: "Total settlement batches processed by PayCore.",
		},
		[]string{"status"},
	)
	settlementBatchDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "paycore_settlement_batch_duration_seconds",
			Help:    "Settlement batch processing duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"status"},
	)
	settlementPaymentsTotal := prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "paycore_settlement_payments_total",
			Help: "Total payments settled by PayCore.",
		},
	)
	settlementRecoveredBatchesTotal := prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "paycore_settlement_recovered_batches_total",
			Help: "Total stale settlement batches recovered by PayCore.",
		},
	)
	outboxClaimedEventsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "paycore_outbox_claimed_events_total",
			Help: "Total outbox events claimed by PayCore workers.",
		},
		[]string{"publisher"},
	)
	outboxPublishAttemptsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "paycore_outbox_publish_attempts_total",
			Help: "Total outbox publish attempts by publisher backend.",
		},
		[]string{"publisher"},
	)
	outboxPublishFailuresTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "paycore_outbox_publish_failures_total",
			Help: "Total failed outbox publish attempts by publisher backend.",
		},
		[]string{"publisher"},
	)
	outboxEventsPublishedTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "paycore_outbox_events_published_total",
			Help: "Total outbox events marked published by publisher backend.",
		},
		[]string{"publisher"},
	)
	rateLimitAllowedTotal := prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "paycore_rate_limit_allowed_total",
			Help: "Total rate-limit checks that allowed a request.",
		},
	)
	rateLimitRejectedTotal := prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "paycore_rate_limit_rejected_total",
			Help: "Total rate-limit checks that rejected a request.",
		},
	)
	rateLimitRedisErrorsTotal := prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "paycore_rate_limit_redis_errors_total",
			Help: "Total Redis-backed rate-limit errors.",
		},
	)
	rateLimitCheckDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "paycore_rate_limit_check_duration_seconds",
			Help:    "Rate-limit check duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"result"},
	)
	idempotencyCacheHitsTotal := prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "paycore_idempotency_cache_hits_total",
			Help: "Total idempotency response cache hits.",
		},
	)
	idempotencyCacheMissesTotal := prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "paycore_idempotency_cache_misses_total",
			Help: "Total idempotency response cache misses.",
		},
	)
	idempotencyCacheErrorsTotal := prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "paycore_idempotency_cache_errors_total",
			Help: "Total idempotency response cache errors.",
		},
	)
	idempotencyPostgresFallbackTotal := prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "paycore_idempotency_postgres_fallback_total",
			Help: "Total idempotency requests served from durable records after cache miss or error.",
		},
	)

	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		httpRequestsTotal,
		httpRequestDuration,
		settlementBatchTotal,
		settlementBatchDuration,
		settlementPaymentsTotal,
		settlementRecoveredBatchesTotal,
		outboxClaimedEventsTotal,
		outboxPublishAttemptsTotal,
		outboxPublishFailuresTotal,
		outboxEventsPublishedTotal,
		rateLimitAllowedTotal,
		rateLimitRejectedTotal,
		rateLimitRedisErrorsTotal,
		rateLimitCheckDuration,
		idempotencyCacheHitsTotal,
		idempotencyCacheMissesTotal,
		idempotencyCacheErrorsTotal,
		idempotencyPostgresFallbackTotal,
	)

	return &Metrics{
		registry:                         registry,
		httpRequestsTotal:                httpRequestsTotal,
		httpRequestDuration:              httpRequestDuration,
		settlementBatchTotal:             settlementBatchTotal,
		settlementBatchDuration:          settlementBatchDuration,
		settlementPaymentsTotal:          settlementPaymentsTotal,
		settlementRecoveredBatchesTotal:  settlementRecoveredBatchesTotal,
		outboxClaimedEventsTotal:         outboxClaimedEventsTotal,
		outboxPublishAttemptsTotal:       outboxPublishAttemptsTotal,
		outboxPublishFailuresTotal:       outboxPublishFailuresTotal,
		outboxEventsPublishedTotal:       outboxEventsPublishedTotal,
		rateLimitAllowedTotal:            rateLimitAllowedTotal,
		rateLimitRejectedTotal:           rateLimitRejectedTotal,
		rateLimitRedisErrorsTotal:        rateLimitRedisErrorsTotal,
		rateLimitCheckDuration:           rateLimitCheckDuration,
		idempotencyCacheHitsTotal:        idempotencyCacheHitsTotal,
		idempotencyCacheMissesTotal:      idempotencyCacheMissesTotal,
		idempotencyCacheErrorsTotal:      idempotencyCacheErrorsTotal,
		idempotencyPostgresFallbackTotal: idempotencyPostgresFallbackTotal,
	}
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

func (m *Metrics) Registry() *prometheus.Registry {
	return m.registry
}

func (m *Metrics) ObserveHTTPRequest(method string, route string, statusCode int, duration time.Duration) {
	status := strconv.Itoa(statusCode)
	m.httpRequestsTotal.WithLabelValues(method, route, status).Inc()
	m.httpRequestDuration.WithLabelValues(method, route, status).Observe(duration.Seconds())
}

func (m *Metrics) ObserveSettlementBatch(status string, payments int, duration time.Duration) {
	m.settlementBatchTotal.WithLabelValues(status).Inc()
	m.settlementBatchDuration.WithLabelValues(status).Observe(duration.Seconds())
	if payments > 0 {
		m.settlementPaymentsTotal.Add(float64(payments))
	}
}

func (m *Metrics) ObserveSettlementRecoveredBatches(count int) {
	if count > 0 {
		m.settlementRecoveredBatchesTotal.Add(float64(count))
	}
}

func (m *Metrics) ObserveOutboxBatch(publisher string, claimed int, published int, failed int) {
	if claimed > 0 {
		m.outboxClaimedEventsTotal.WithLabelValues(publisher).Add(float64(claimed))
	}

	attempts := published + failed
	if attempts > 0 {
		m.outboxPublishAttemptsTotal.WithLabelValues(publisher).Add(float64(attempts))
	}

	if published > 0 {
		m.outboxEventsPublishedTotal.WithLabelValues(publisher).Add(float64(published))
	}

	if failed > 0 {
		m.outboxPublishFailuresTotal.WithLabelValues(publisher).Add(float64(failed))
	}
}

func (m *Metrics) ObserveRateLimit(result string, duration time.Duration) {
	switch result {
	case "allowed":
		m.rateLimitAllowedTotal.Inc()
	case "rejected":
		m.rateLimitRejectedTotal.Inc()
	case "redis_error":
		m.rateLimitRedisErrorsTotal.Inc()
	}

	m.rateLimitCheckDuration.WithLabelValues(result).Observe(duration.Seconds())
}

func (m *Metrics) ObserveIdempotencyCacheHit() {
	m.idempotencyCacheHitsTotal.Inc()
}

func (m *Metrics) ObserveIdempotencyCacheMiss() {
	m.idempotencyCacheMissesTotal.Inc()
}

func (m *Metrics) ObserveIdempotencyCacheError() {
	m.idempotencyCacheErrorsTotal.Inc()
}

func (m *Metrics) ObserveIdempotencyPostgresFallback() {
	m.idempotencyPostgresFallbackTotal.Inc()
}
