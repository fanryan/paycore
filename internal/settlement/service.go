package settlement

import (
	"context"
	"errors"
	"time"

	"github.com/fanryan/paycore/internal/shared/db"
	"github.com/fanryan/paycore/internal/shared/id"
)

var (
	ErrRepositoryRequired = errors.New("settlement repository is required")
	ErrTransactorRequired = errors.New("settlement transactor is required")
)

const (
	defaultClaimLimit = 100
	defaultLockTTL    = 5 * time.Minute
)

type Service struct {
	repository Repository
	transactor db.Transactor
	workerID   string
	claimLimit int
	lockTTL    time.Duration
	now        func() time.Time
}

type ServiceConfig struct {
	Repository Repository
	Transactor db.Transactor
	WorkerID   string
	ClaimLimit int
	LockTTL    time.Duration
	Now        func() time.Time
}

type CreateBatchInput struct {
	WindowStart time.Time
	WindowEnd   time.Time
}

type CreateBatchResult struct {
	Batch     Batch
	LineItems []LineItem
}

func NewService(config ServiceConfig) (*Service, error) {
	if config.Repository == nil {
		return nil, ErrRepositoryRequired
	}

	if config.Transactor == nil {
		return nil, ErrTransactorRequired
	}

	if config.WorkerID == "" {
		config.WorkerID = "settlement-worker"
	}

	if config.ClaimLimit <= 0 {
		config.ClaimLimit = defaultClaimLimit
	}

	if config.LockTTL <= 0 {
		config.LockTTL = defaultLockTTL
	}

	if config.Now == nil {
		config.Now = time.Now
	}

	return &Service{
		repository: config.Repository,
		transactor: config.Transactor,
		workerID:   config.WorkerID,
		claimLimit: config.ClaimLimit,
		lockTTL:    config.LockTTL,
		now:        config.Now,
	}, nil
}

func (s *Service) CreateBatch(ctx context.Context, input CreateBatchInput) (CreateBatchResult, error) {
	var result CreateBatchResult

	err := s.transactor.WithinTx(ctx, func(ctx context.Context) error {
		now := s.now().UTC()

		batchID, err := id.New("setbat")
		if err != nil {
			return err
		}

		batch, err := NewBatch(NewBatchInput{
			ID:          batchID,
			WindowStart: input.WindowStart,
			WindowEnd:   input.WindowEnd,
			Now:         now,
		})
		if err != nil {
			return err
		}

		batch, err = s.repository.CreateBatch(ctx, batch)
		if err != nil {
			return err
		}

		batch, err = batch.StartProcessing(s.workerID, now.Add(s.lockTTL), now)
		if err != nil {
			return err
		}

		batch, err = s.repository.UpdateBatch(ctx, batch)
		if err != nil {
			return err
		}

		claimed, err := s.repository.ClaimCapturedPayments(ctx, ClaimCapturedPaymentsInput{
			BatchID:     batch.ID,
			WindowStart: batch.WindowStart,
			WindowEnd:   batch.WindowEnd,
			Limit:       s.claimLimit,
			Now:         now,
		})
		if err != nil {
			return err
		}

		lineItems := make([]LineItem, 0, len(claimed))
		for _, payment := range claimed {
			itemID, err := id.New("setitem")
			if err != nil {
				return err
			}

			item, err := NewLineItem(NewLineItemInput{
				ID:              itemID,
				BatchID:         batch.ID,
				MerchantID:      payment.MerchantID,
				PaymentID:       payment.PaymentID,
				AmountMinor:     payment.AmountMinor,
				FeeAmountMinor:  0,
				Currency:        payment.Currency,
				PaymentCaptured: payment.CapturedAt,
				Now:             now,
			})
			if err != nil {
				return err
			}

			item, err = s.repository.CreateLineItem(ctx, item)
			if err != nil {
				return err
			}

			lineItems = append(lineItems, item)
		}

		batch, err = batch.Complete(now)
		if err != nil {
			return err
		}

		batch, err = s.repository.UpdateBatch(ctx, batch)
		if err != nil {
			return err
		}

		result = CreateBatchResult{
			Batch:     batch,
			LineItems: lineItems,
		}

		return nil
	})
	if err != nil {
		return CreateBatchResult{}, err
	}

	return result, nil
}
