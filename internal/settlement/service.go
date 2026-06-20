package settlement

import (
	"context"
	"errors"
	"time"

	"github.com/fanryan/paycore/internal/outbox"
	"github.com/fanryan/paycore/internal/payment"
	"github.com/fanryan/paycore/internal/shared/db"
	"github.com/fanryan/paycore/internal/shared/id"
)

var (
	ErrRepositoryRequired        = errors.New("settlement repository is required")
	ErrPaymentRepositoryRequired = errors.New("payment repository is required")
	ErrTransactorRequired        = errors.New("settlement transactor is required")
)

const (
	defaultClaimLimit = 100
	defaultLockTTL    = 5 * time.Minute

	aggregateTypePayment    = "payment"
	eventTypePaymentSettled = "payment.settled"
)

type Service struct {
	repository Repository
	payments   payment.Repository
	outbox     outbox.Repository
	transactor db.Transactor
	workerID   string
	claimLimit int
	lockTTL    time.Duration
	now        func() time.Time
}

type ServiceConfig struct {
	Repository Repository
	Payments   payment.Repository
	Outbox     outbox.Repository
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
	Payments  []payment.Payment
}

type RecoverStaleBatchesInput struct {
	Limit int
}

type RecoverStaleBatchesResult struct {
	Batches   []Batch
	LineItems []LineItem
	Payments  []payment.Payment
}

type paymentSettledPayload struct {
	PaymentID         string    `json:"payment_id"`
	MerchantID        string    `json:"merchant_id"`
	PayerID           string    `json:"payer_id"`
	SettlementBatchID string    `json:"settlement_batch_id"`
	AmountMinor       int64     `json:"amount_minor"`
	Currency          string    `json:"currency"`
	SettledAt         time.Time `json:"settled_at"`
}

func NewService(config ServiceConfig) (*Service, error) {
	if config.Repository == nil {
		return nil, ErrRepositoryRequired
	}

	if config.Payments == nil {
		return nil, ErrPaymentRepositoryRequired
	}

	if config.Transactor == nil {
		return nil, ErrTransactorRequired
	}

	if config.Outbox == nil {
		config.Outbox = outbox.NoopRepository{}
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
		payments:   config.Payments,
		outbox:     config.Outbox,
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
		settledPayments := make([]payment.Payment, 0, len(claimed))
		lineItems, settledPayments, err = s.settleClaimedPayments(ctx, batch.ID, claimed, now)
		if err != nil {
			return err
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
			Payments:  settledPayments,
		}

		return nil
	})
	if err != nil {
		return CreateBatchResult{}, err
	}

	return result, nil
}

func (s *Service) RecoverStaleBatches(ctx context.Context, input RecoverStaleBatchesInput) (RecoverStaleBatchesResult, error) {
	var result RecoverStaleBatchesResult

	limit := input.Limit
	if limit <= 0 {
		limit = s.claimLimit
	}

	err := s.transactor.WithinTx(ctx, func(ctx context.Context) error {
		now := s.now().UTC()

		staleBatches, err := s.repository.ListStaleProcessingBatches(ctx, ListStaleProcessingBatchesInput{
			Now:   now,
			Limit: limit,
		})
		if err != nil {
			return err
		}

		result.Batches = make([]Batch, 0, len(staleBatches))
		result.LineItems = make([]LineItem, 0)
		result.Payments = make([]payment.Payment, 0)

		for _, staleBatch := range staleBatches {
			batch, err := staleBatch.StartProcessing(s.workerID, now.Add(s.lockTTL), now)
			if err != nil {
				return err
			}

			batch, err = s.repository.UpdateBatch(ctx, batch)
			if err != nil {
				return err
			}

			claimed, err := s.repository.ListClaimedPayments(ctx, batch.ID)
			if err != nil {
				return err
			}

			lineItems, settledPayments, err := s.settleClaimedPayments(ctx, batch.ID, claimed, now)
			if err != nil {
				return err
			}

			batch, err = batch.Complete(now)
			if err != nil {
				return err
			}

			batch, err = s.repository.UpdateBatch(ctx, batch)
			if err != nil {
				return err
			}

			result.Batches = append(result.Batches, batch)
			result.LineItems = append(result.LineItems, lineItems...)
			result.Payments = append(result.Payments, settledPayments...)
		}

		return nil
	})
	if err != nil {
		return RecoverStaleBatchesResult{}, err
	}

	return result, nil
}

func (s *Service) settleClaimedPayments(ctx context.Context, batchID string, claimed []ClaimedPayment, now time.Time) ([]LineItem, []payment.Payment, error) {
	lineItems := make([]LineItem, 0, len(claimed))
	settledPayments := make([]payment.Payment, 0, len(claimed))

	for _, claimedPayment := range claimed {
		paymentRecord, err := s.payments.GetPayment(ctx, claimedPayment.PaymentID)
		if err != nil {
			return nil, nil, err
		}

		settledPayment, err := paymentRecord.Settle(now)
		if err != nil {
			return nil, nil, err
		}

		settledPayment, err = s.payments.UpdatePayment(ctx, settledPayment)
		if err != nil {
			return nil, nil, err
		}

		itemID, err := id.New("setitem")
		if err != nil {
			return nil, nil, err
		}

		item, err := NewLineItem(NewLineItemInput{
			ID:              itemID,
			BatchID:         batchID,
			MerchantID:      claimedPayment.MerchantID,
			PaymentID:       claimedPayment.PaymentID,
			AmountMinor:     claimedPayment.AmountMinor,
			FeeAmountMinor:  0,
			Currency:        claimedPayment.Currency,
			PaymentCaptured: claimedPayment.CapturedAt,
			Now:             now,
		})
		if err != nil {
			return nil, nil, err
		}

		item, err = s.repository.CreateLineItem(ctx, item)
		if err != nil {
			return nil, nil, err
		}

		event, err := outbox.NewEvent(outbox.NewEventInput{
			AggregateType: aggregateTypePayment,
			AggregateID:   settledPayment.ID,
			EventType:     eventTypePaymentSettled,
			Payload: paymentSettledPayload{
				PaymentID:         settledPayment.ID,
				MerchantID:        settledPayment.MerchantID,
				PayerID:           settledPayment.PayerID,
				SettlementBatchID: batchID,
				AmountMinor:       settledPayment.AmountMinor,
				Currency:          settledPayment.Currency,
				SettledAt:         now,
			},
			Now: now,
		})
		if err != nil {
			return nil, nil, err
		}

		if _, err := s.outbox.CreateEvent(ctx, event); err != nil {
			return nil, nil, err
		}

		lineItems = append(lineItems, item)
		settledPayments = append(settledPayments, settledPayment)
	}

	return lineItems, settledPayments, nil
}
