package idempotency

import (
	"context"
	"errors"
	"time"
)

var (
	ErrRequestInProgress = errors.New("idempotency request is already in progress")
)

type Service struct {
	repository Repository
	now        func() time.Time
	ttl        time.Duration
}

type StartRequestInput struct {
	Key         string
	RequestHash string
}

type StartRequestResult struct {
	Record       Record
	Replay       bool
	ResponseCode int
	ResponseBody []byte
}

type CompleteRequestInput struct {
	Key          string
	ResponseCode int
	ResponseBody []byte
}

func NewService(repository Repository, ttl time.Duration) *Service {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}

	return &Service{
		repository: repository,
		now:        time.Now,
		ttl:        ttl,
	}
}

func (s *Service) StartRequest(ctx context.Context, input StartRequestInput) (StartRequestResult, error) {
	now := s.now().UTC()

	record, err := NewRecord(NewRecordInput{
		Key:         input.Key,
		RequestHash: input.RequestHash,
		Now:         now,
		TTL:         s.ttl,
	})
	if err != nil {
		return StartRequestResult{}, err
	}

	created, err := s.repository.CreateRecord(ctx, record)
	if err == nil {
		return StartRequestResult{
			Record: created,
			Replay: false,
		}, nil
	}

	if !errors.Is(err, ErrDuplicateKey) {
		return StartRequestResult{}, err
	}

	existing, err := s.repository.GetRecord(ctx, record.Key)
	if err != nil {
		return StartRequestResult{}, err
	}

	if existing.IsExpired(now) {
		return StartRequestResult{}, ErrExpiredIdempotencyKey
	}

	if existing.RequestHash != record.RequestHash {
		return StartRequestResult{}, ErrRequestHashMismatch
	}

	if existing.Status == StatusInProgress {
		return StartRequestResult{}, ErrRequestInProgress
	}

	if existing.Status != StatusCompleted {
		return StartRequestResult{}, ErrInvalidStatus
	}

	return StartRequestResult{
		Record:       existing,
		Replay:       true,
		ResponseCode: existing.ResponseCode,
		ResponseBody: append([]byte(nil), existing.ResponseBody...),
	}, nil
}

func (s *Service) CompleteRequest(ctx context.Context, input CompleteRequestInput) (Record, error) {
	existing, err := s.repository.GetRecord(ctx, input.Key)
	if err != nil {
		return Record{}, err
	}

	completed, err := existing.Complete(input.ResponseCode, input.ResponseBody, s.now().UTC())
	if err != nil {
		return Record{}, err
	}

	return s.repository.UpdateRecord(ctx, completed)
}
