package idempotency

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

type Status string

const (
	StatusInProgress Status = "IN_PROGRESS"
	StatusCompleted  Status = "COMPLETED"
	StatusFailed     Status = "FAILED"
)

var (
	ErrInvalidKey         = errors.New("idempotency key is required")
	ErrInvalidRequestHash = errors.New("idempotency request hash is required")
	ErrInvalidStatus      = errors.New("invalid idempotency status")
)

type Record struct {
	Key          string
	RequestHash  string
	Status       Status
	ResponseCode int
	ResponseBody []byte
	CreatedAt    time.Time
	UpdatedAt    time.Time
	ExpiresAt    time.Time
}

type NewRecordInput struct {
	Key         string
	RequestHash string
	Now         time.Time
	TTL         time.Duration
}

func NewRecord(input NewRecordInput) (Record, error) {
	key := strings.TrimSpace(input.Key)
	if key == "" {
		return Record{}, ErrInvalidKey
	}

	requestHash := strings.TrimSpace(input.RequestHash)
	if requestHash == "" {
		return Record{}, ErrInvalidRequestHash
	}

	now := input.Now.UTC()
	ttl := input.TTL
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}

	return Record{
		Key:          key,
		RequestHash:  requestHash,
		Status:       StatusInProgress,
		ResponseBody: []byte{},
		CreatedAt:    now,
		UpdatedAt:    now,
		ExpiresAt:    now.Add(ttl),
	}, nil
}

func (r Record) Complete(responseCode int, responseBody []byte, now time.Time) (Record, error) {
	if r.Status != StatusInProgress {
		return Record{}, ErrInvalidStatus
	}

	r.Status = StatusCompleted
	r.ResponseCode = responseCode
	r.ResponseBody = append([]byte(nil), responseBody...)
	r.UpdatedAt = now.UTC()

	return r, nil
}

func (r Record) IsExpired(now time.Time) bool {
	return !r.ExpiresAt.IsZero() && now.UTC().After(r.ExpiresAt)
}

func HashRequest(method string, path string, body []byte) string {
	hash := sha256.New()

	hash.Write([]byte(method))
	hash.Write([]byte("\n"))
	hash.Write([]byte(path))
	hash.Write([]byte("\n"))
	hash.Write(body)

	return hex.EncodeToString(hash.Sum(nil))
}

func HashRequestBody(body []byte) string {
	return HashRequest("", "", body)
}
