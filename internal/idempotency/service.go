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
	cache      Cache
	metrics    MetricsRecorder
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

type MetricsRecorder interface {
	ObserveIdempotencyCacheHit()
	ObserveIdempotencyCacheMiss()
	ObserveIdempotencyCacheError()
	ObserveIdempotencyPostgresFallback()
}

func NewService(repository Repository, ttl time.Duration) *Service {
	return NewServiceWithCache(repository, NoopCache{}, ttl)
}

func NewServiceWithCache(repository Repository, cache Cache, ttl time.Duration) *Service {
	return NewServiceWithCacheAndMetrics(repository, cache, nil, ttl)
}

func NewServiceWithCacheAndMetrics(repository Repository, cache Cache, metrics MetricsRecorder, ttl time.Duration) *Service {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}

	if cache == nil {
		cache = NoopCache{}
	}

	return &Service{
		repository: repository,
		cache:      cache,
		metrics:    metrics,
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

	cached, err := s.cache.GetResponse(ctx, existing.Key, existing.RequestHash)
	if err == nil {
		s.observeIdempotencyCacheHit()
		return StartRequestResult{
			Record:       existing,
			Replay:       true,
			ResponseCode: cached.ResponseCode,
			ResponseBody: append([]byte(nil), cached.ResponseBody...),
		}, nil
	}

	if errors.Is(err, ErrCachedResponseNotFound) {
		s.observeIdempotencyCacheMiss()
	} else {
		s.observeIdempotencyCacheError()
	}
	s.observeIdempotencyPostgresFallback()

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

	now := s.now().UTC()

	completed, err := existing.Complete(input.ResponseCode, input.ResponseBody, now)
	if err != nil {
		return Record{}, err
	}

	updated, err := s.repository.UpdateRecord(ctx, completed)
	if err != nil {
		return Record{}, err
	}

	cacheTTL := updated.ExpiresAt.Sub(now)
	if cacheTTL <= 0 {
		cacheTTL = s.ttl
	}

	_ = s.cache.SetResponse(ctx, CachedResponse{
		Key:          updated.Key,
		RequestHash:  updated.RequestHash,
		ResponseCode: updated.ResponseCode,
		ResponseBody: append([]byte(nil), updated.ResponseBody...),
	}, cacheTTL)

	return updated, nil
}

func (s *Service) observeIdempotencyCacheHit() {
	if s.metrics == nil {
		return
	}

	s.metrics.ObserveIdempotencyCacheHit()
}

func (s *Service) observeIdempotencyCacheMiss() {
	if s.metrics == nil {
		return
	}

	s.metrics.ObserveIdempotencyCacheMiss()
}

func (s *Service) observeIdempotencyCacheError() {
	if s.metrics == nil {
		return
	}

	s.metrics.ObserveIdempotencyCacheError()
}

func (s *Service) observeIdempotencyPostgresFallback() {
	if s.metrics == nil {
		return
	}

	s.metrics.ObserveIdempotencyPostgresFallback()
}
