package outbox

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNewEventCreatesPendingEvent(t *testing.T) {
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.FixedZone("SGT", 8*60*60))

	event, err := NewEvent(NewEventInput{
		AggregateType: "payment",
		AggregateID:   "pay_123",
		EventType:     "payment.authorized",
		Payload: map[string]any{
			"payment_id": "pay_123",
			"amount":     4000,
			"currency":   "USD",
		},
		Now: now,
	})
	if err != nil {
		t.Fatalf("expected event creation to succeed, got error: %v", err)
	}

	if !strings.HasPrefix(event.ID, "evt_") {
		t.Fatalf("expected event id prefix evt_, got %q", event.ID)
	}

	if event.AggregateType != "payment" {
		t.Fatalf("expected aggregate type payment, got %q", event.AggregateType)
	}

	if event.AggregateID != "pay_123" {
		t.Fatalf("expected aggregate id pay_123, got %q", event.AggregateID)
	}

	if event.EventType != "payment.authorized" {
		t.Fatalf("expected event type payment.authorized, got %q", event.EventType)
	}

	if event.Status != StatusPending {
		t.Fatalf("expected pending status, got %q", event.Status)
	}

	if event.Attempts != 0 {
		t.Fatalf("expected attempts 0, got %d", event.Attempts)
	}

	expectedTime := now.UTC()
	if !event.AvailableAt.Equal(expectedTime) {
		t.Fatalf("expected available_at %s, got %s", expectedTime, event.AvailableAt)
	}

	if !event.CreatedAt.Equal(expectedTime) {
		t.Fatalf("expected created_at %s, got %s", expectedTime, event.CreatedAt)
	}

	if !event.UpdatedAt.Equal(expectedTime) {
		t.Fatalf("expected updated_at %s, got %s", expectedTime, event.UpdatedAt)
	}

	if event.LockedAt != nil {
		t.Fatalf("expected locked_at nil, got %v", event.LockedAt)
	}

	if event.LockedBy != nil {
		t.Fatalf("expected locked_by nil, got %v", event.LockedBy)
	}

	if event.PublishedAt != nil {
		t.Fatalf("expected published_at nil, got %v", event.PublishedAt)
	}

	if event.LastError != nil {
		t.Fatalf("expected last_error nil, got %v", event.LastError)
	}

	var payload map[string]any
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("expected payload to be valid JSON, got error: %v", err)
	}

	if payload["payment_id"] != "pay_123" {
		t.Fatalf("expected payload payment_id pay_123, got %v", payload["payment_id"])
	}
}

func TestNewEventTrimsRequiredFields(t *testing.T) {
	event, err := NewEvent(NewEventInput{
		AggregateType: " payment ",
		AggregateID:   " pay_123 ",
		EventType:     " payment.captured ",
		Payload:       map[string]any{"payment_id": "pay_123"},
		Now:           testNow(),
	})
	if err != nil {
		t.Fatalf("expected event creation to succeed, got error: %v", err)
	}

	if event.AggregateType != "payment" {
		t.Fatalf("expected trimmed aggregate type payment, got %q", event.AggregateType)
	}

	if event.AggregateID != "pay_123" {
		t.Fatalf("expected trimmed aggregate id pay_123, got %q", event.AggregateID)
	}

	if event.EventType != "payment.captured" {
		t.Fatalf("expected trimmed event type payment.captured, got %q", event.EventType)
	}
}

func TestNewEventRejectsMissingRequiredFields(t *testing.T) {
	tests := []struct {
		name  string
		input NewEventInput
	}{
		{
			name: "missing aggregate type",
			input: NewEventInput{
				AggregateID: "pay_123",
				EventType:   "payment.authorized",
				Payload:     map[string]any{},
				Now:         testNow(),
			},
		},
		{
			name: "missing aggregate id",
			input: NewEventInput{
				AggregateType: "payment",
				EventType:     "payment.authorized",
				Payload:       map[string]any{},
				Now:           testNow(),
			},
		},
		{
			name: "missing event type",
			input: NewEventInput{
				AggregateType: "payment",
				AggregateID:   "pay_123",
				Payload:       map[string]any{},
				Now:           testNow(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewEvent(tt.input); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestNewEventRejectsUnmarshalablePayload(t *testing.T) {
	_, err := NewEvent(NewEventInput{
		AggregateType: "payment",
		AggregateID:   "pay_123",
		EventType:     "payment.authorized",
		Payload:       func() {},
		Now:           testNow(),
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func testNow() time.Time {
	return time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
}
