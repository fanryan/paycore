package metrics_test

import (
	"strings"
	"testing"
	"time"

	"github.com/fanryan/paycore/internal/shared/metrics"
	"github.com/prometheus/common/expfmt"
)

func TestMetricsExposeSettlementCollectors(t *testing.T) {
	appMetrics := metrics.New()

	appMetrics.ObserveSettlementBatch("COMPLETED", 2, 25*time.Millisecond)
	appMetrics.ObserveSettlementRecoveredBatches(1)

	metricFamilies, err := appMetrics.Registry().Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	var output strings.Builder
	encoder := expfmt.NewEncoder(&output, expfmt.NewFormat(expfmt.TypeTextPlain))
	for _, metricFamily := range metricFamilies {
		if err := encoder.Encode(metricFamily); err != nil {
			t.Fatalf("encode metric family: %v", err)
		}
	}

	body := output.String()
	expectedMetrics := []string{
		"paycore_settlement_batch_total",
		"paycore_settlement_batch_duration_seconds",
		"paycore_settlement_payments_total 2",
		"paycore_settlement_recovered_batches_total 1",
	}

	for _, expected := range expectedMetrics {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected metric output to contain %q, got:\n%s", expected, body)
		}
	}
}

func TestMetricsExposeOutboxCollectors(t *testing.T) {
	appMetrics := metrics.New()

	appMetrics.ObserveOutboxBatch("kafka", 3, 2, 1)

	metricFamilies, err := appMetrics.Registry().Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	var output strings.Builder
	encoder := expfmt.NewEncoder(&output, expfmt.NewFormat(expfmt.TypeTextPlain))
	for _, metricFamily := range metricFamilies {
		if err := encoder.Encode(metricFamily); err != nil {
			t.Fatalf("encode metric family: %v", err)
		}
	}

	body := output.String()
	expectedMetrics := []string{
		`paycore_outbox_claimed_events_total{publisher="kafka"} 3`,
		`paycore_outbox_publish_attempts_total{publisher="kafka"} 3`,
		`paycore_outbox_publish_failures_total{publisher="kafka"} 1`,
		`paycore_outbox_events_published_total{publisher="kafka"} 2`,
	}

	for _, expected := range expectedMetrics {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected metric output to contain %q, got:\n%s", expected, body)
		}
	}
}
