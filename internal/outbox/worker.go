package outbox

import (
	"context"
	"errors"
	"time"

	"github.com/fanryan/paycore/internal/shared/db"
)

var (
	ErrPublisherRequired  = errors.New("publisher is required")
	ErrRepositoryRequired = errors.New("outbox repository is required")
	ErrTransactorRequired = errors.New("transactor is required")
)

type Worker struct {
	repository    Repository
	publisher     Publisher
	transactor    db.Transactor
	metrics       MetricsRecorder
	workerID      string
	batchSize     int
	publisherName string
	now           func() time.Time
}

type WorkerConfig struct {
	Repository    Repository
	Publisher     Publisher
	Transactor    db.Transactor
	Metrics       MetricsRecorder
	WorkerID      string
	BatchSize     int
	PublisherName string
	Now           func() time.Time
}

type MetricsRecorder interface {
	ObserveOutboxBatch(publisher string, claimed int, published int, failed int)
}

type ProcessBatchResult struct {
	Claimed   int
	Published int
	Failed    int
}

func NewWorker(config WorkerConfig) (*Worker, error) {
	if config.Repository == nil {
		return nil, ErrRepositoryRequired
	}

	if config.Publisher == nil {
		return nil, ErrPublisherRequired
	}

	if config.Transactor == nil {
		return nil, ErrTransactorRequired
	}

	if config.BatchSize <= 0 {
		config.BatchSize = 100
	}

	if config.WorkerID == "" {
		config.WorkerID = "outbox-worker"
	}

	if config.PublisherName == "" {
		config.PublisherName = "unknown"
	}

	if config.Now == nil {
		config.Now = time.Now
	}

	return &Worker{
		repository:    config.Repository,
		publisher:     config.Publisher,
		transactor:    config.Transactor,
		metrics:       config.Metrics,
		workerID:      config.WorkerID,
		batchSize:     config.BatchSize,
		publisherName: config.PublisherName,
		now:           config.Now,
	}, nil
}

func (w *Worker) ProcessBatch(ctx context.Context) (ProcessBatchResult, error) {
	var claimed []Event

	err := w.transactor.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		claimed, err = w.repository.ClaimPendingEvents(ctx, ClaimPendingEventsInput{
			WorkerID: w.workerID,
			Limit:    w.batchSize,
			Now:      w.now().UTC(),
		})

		return err
	})
	if err != nil {
		return ProcessBatchResult{}, err
	}

	result := ProcessBatchResult{
		Claimed: len(claimed),
	}

	for _, event := range claimed {
		if err := w.publisher.Publish(ctx, event); err != nil {
			_, markErr := w.repository.MarkEventFailed(ctx, MarkEventFailedInput{
				EventID:       event.ID,
				ErrorMessage:  err.Error(),
				NextAvailable: w.now().UTC().Add(time.Minute),
				Now:           w.now().UTC(),
			})
			if markErr != nil {
				return result, markErr
			}

			result.Failed++
			continue
		}

		if _, err := w.repository.MarkEventPublished(ctx, event.ID, w.now().UTC()); err != nil {
			return result, err
		}

		result.Published++
	}

	w.observeOutboxBatch(result)

	return result, nil
}

func (w *Worker) observeOutboxBatch(result ProcessBatchResult) {
	if w.metrics == nil {
		return
	}

	w.metrics.ObserveOutboxBatch(w.publisherName, result.Claimed, result.Published, result.Failed)
}
