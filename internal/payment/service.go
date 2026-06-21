package payment

import (
	"context"
	"errors"
	"time"

	"github.com/fanryan/paycore/internal/merchant"
	"github.com/fanryan/paycore/internal/outbox"
	"github.com/fanryan/paycore/internal/payer"
	currencycode "github.com/fanryan/paycore/internal/shared/currency"
	"github.com/fanryan/paycore/internal/shared/db"
	"github.com/fanryan/paycore/internal/shared/id"
)

const (
	aggregateTypePayment       = "payment"
	eventTypePaymentAuthorized = "payment.authorized"
	eventTypePaymentCaptured   = "payment.captured"
	eventTypePaymentExpired    = "payment.expired"
)

var (
	ErrMerchantCannotCreatePayments = errors.New("merchant cannot create payments")
	ErrPayerCurrencyMismatch        = errors.New("payer currency does not match payment currency")
	ErrInsufficientAvailableBalance = errors.New("payer has insufficient available balance")
	ErrPaymentNotCapturable         = errors.New("payment is not capturable")
	ErrAuthorizationExpired         = errors.New("authorization has expired")
)

type Service struct {
	merchants  merchant.MerchantRepository
	payers     payer.PayerRepository
	payments   Repository
	outbox     outbox.Repository
	transactor db.Transactor
	metrics    MetricsRecorder
	now        func() time.Time
}

type MetricsRecorder interface {
	ObserveAuthorization(result string, duration time.Duration)
	ObserveCapture(result string, duration time.Duration)
	ObservePayerVersionConflict()
}

type AuthorizePaymentInput struct {
	MerchantID  string
	PayerID     string
	AmountMinor int64
	Currency    string
}

type AuthorizePaymentResult struct {
	Payment Payment
	Hold    Hold
	Payer   payer.Payer
}

type CapturePaymentInput struct {
	PaymentID string
}

type CapturePaymentResult struct {
	Payment Payment
	Hold    Hold
	Payer   payer.Payer
}

type ExpireAuthorizedPaymentsInput struct {
	Limit int
}

type ExpireAuthorizedPaymentsResult struct {
	Payments []Payment
	Holds    []Hold
	Payers   []payer.Payer
}

type paymentAuthorizedPayload struct {
	PaymentID    string    `json:"payment_id"`
	MerchantID   string    `json:"merchant_id"`
	PayerID      string    `json:"payer_id"`
	AmountMinor  int64     `json:"amount_minor"`
	Currency     string    `json:"currency"`
	HoldID       string    `json:"hold_id"`
	AuthorizedAt time.Time `json:"authorized_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type paymentCapturedPayload struct {
	PaymentID      string    `json:"payment_id"`
	MerchantID     string    `json:"merchant_id"`
	PayerID        string    `json:"payer_id"`
	CapturedAmount int64     `json:"captured_amount"`
	Currency       string    `json:"currency"`
	HoldID         string    `json:"hold_id"`
	CapturedAt     time.Time `json:"captured_at"`
}

type paymentExpiredPayload struct {
	PaymentID   string    `json:"payment_id"`
	MerchantID  string    `json:"merchant_id"`
	PayerID     string    `json:"payer_id"`
	AmountMinor int64     `json:"amount_minor"`
	Currency    string    `json:"currency"`
	HoldID      string    `json:"hold_id"`
	ExpiredAt   time.Time `json:"expired_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

func NewService(merchants merchant.MerchantRepository, payers payer.PayerRepository, payments Repository) *Service {
	return NewServiceWithTransactorAndOutbox(
		merchants,
		payers,
		payments,
		db.NoopTransactor{},
		outbox.NoopRepository{},
	)
}

func NewServiceWithTransactor(
	merchants merchant.MerchantRepository,
	payers payer.PayerRepository,
	payments Repository,
	transactor db.Transactor,
) *Service {
	return NewServiceWithTransactorAndOutbox(
		merchants,
		payers,
		payments,
		transactor,
		outbox.NoopRepository{},
	)
}

func NewServiceWithTransactorAndOutbox(
	merchants merchant.MerchantRepository,
	payers payer.PayerRepository,
	payments Repository,
	transactor db.Transactor,
	outboxRepository outbox.Repository,
) *Service {
	return NewServiceWithTransactorOutboxAndMetrics(
		merchants,
		payers,
		payments,
		transactor,
		outboxRepository,
		nil,
	)
}

func NewServiceWithTransactorOutboxAndMetrics(
	merchants merchant.MerchantRepository,
	payers payer.PayerRepository,
	payments Repository,
	transactor db.Transactor,
	outboxRepository outbox.Repository,
	metrics MetricsRecorder,
) *Service {
	if transactor == nil {
		transactor = db.NoopTransactor{}
	}

	if outboxRepository == nil {
		outboxRepository = outbox.NoopRepository{}
	}

	return &Service{
		merchants:  merchants,
		payers:     payers,
		payments:   payments,
		outbox:     outboxRepository,
		transactor: transactor,
		metrics:    metrics,
		now:        time.Now,
	}
}

func (s *Service) SetNowForTest(now time.Time) {
	s.now = func() time.Time {
		return now
	}
}

func (s *Service) AuthorizePayment(ctx context.Context, input AuthorizePaymentInput) (AuthorizePaymentResult, error) {
	startedAt := time.Now()
	var result AuthorizePaymentResult

	err := s.transactor.WithinTx(ctx, func(ctx context.Context) error {
		merchantRecord, err := s.merchants.GetMerchant(ctx, input.MerchantID)
		if err != nil {
			return err
		}

		if !merchantRecord.CanCreatePayments() {
			return ErrMerchantCannotCreatePayments
		}

		payerRecord, err := s.payers.GetPayer(ctx, input.PayerID)
		if err != nil {
			return err
		}

		if payerRecord.Currency != currencycode.NormalizeCurrency(input.Currency) {
			return ErrPayerCurrencyMismatch
		}

		if !payerRecord.CanAuthorize(input.AmountMinor, input.Currency) {
			return ErrInsufficientAvailableBalance
		}

		now := s.now().UTC()

		paymentID, err := id.New("pay")
		if err != nil {
			return err
		}

		holdID, err := id.New("hold")
		if err != nil {
			return err
		}

		hold, err := NewHold(NewHoldInput{
			ID:          holdID,
			PaymentID:   paymentID,
			PayerID:     payerRecord.ID,
			AmountMinor: input.AmountMinor,
			Currency:    input.Currency,
			Now:         now,
		})
		if err != nil {
			return err
		}

		paymentRecord, err := NewAuthorizedPayment(NewAuthorizedPaymentInput{
			ID:                  paymentID,
			MerchantID:          merchantRecord.ID,
			PayerID:             payerRecord.ID,
			AmountMinor:         input.AmountMinor,
			Currency:            input.Currency,
			AuthorizationHoldID: hold.ID,
			Now:                 now,
			ExpiresAt:           now.Add(15 * time.Minute),
		})
		if err != nil {
			return err
		}

		updatedPayer, err := payerRecord.Reserve(input.AmountMinor, input.Currency, now)
		if err != nil {
			return err
		}

		updatedPayer, err = s.payers.UpdatePayer(ctx, updatedPayer)
		if err != nil {
			return err
		}

		createdPayment, err := s.payments.CreatePayment(ctx, paymentRecord)
		if err != nil {
			return err
		}

		createdHold, err := s.payments.CreateHold(ctx, hold)
		if err != nil {
			return err
		}

		event, err := outbox.NewEvent(outbox.NewEventInput{
			AggregateType: aggregateTypePayment,
			AggregateID:   createdPayment.ID,
			EventType:     eventTypePaymentAuthorized,
			Payload: paymentAuthorizedPayload{
				PaymentID:    createdPayment.ID,
				MerchantID:   createdPayment.MerchantID,
				PayerID:      createdPayment.PayerID,
				AmountMinor:  createdPayment.AmountMinor,
				Currency:     createdPayment.Currency,
				HoldID:       createdHold.ID,
				AuthorizedAt: createdPayment.AuthorizedAt,
				ExpiresAt:    createdPayment.ExpiresAt,
			},
			Now: now,
		})
		if err != nil {
			return err
		}

		if _, err := s.outbox.CreateEvent(ctx, event); err != nil {
			return err
		}

		result = AuthorizePaymentResult{
			Payment: createdPayment,
			Hold:    createdHold,
			Payer:   updatedPayer,
		}

		return nil
	})
	if err != nil {
		s.observePayerVersionConflict(err)
		s.observeAuthorization(paymentMetricResult(err), time.Since(startedAt))
		return AuthorizePaymentResult{}, err
	}

	s.observeAuthorization("success", time.Since(startedAt))
	return result, nil
}

func (s *Service) CapturePayment(ctx context.Context, input CapturePaymentInput) (CapturePaymentResult, error) {
	startedAt := time.Now()
	var result CapturePaymentResult

	err := s.transactor.WithinTx(ctx, func(ctx context.Context) error {
		paymentRecord, err := s.payments.GetPayment(ctx, input.PaymentID)
		if err != nil {
			return err
		}

		hold, err := s.payments.GetHoldByPaymentID(ctx, paymentRecord.ID)
		if err != nil {
			return err
		}

		payerRecord, err := s.payers.GetPayer(ctx, paymentRecord.PayerID)
		if err != nil {
			return err
		}

		now := s.now().UTC()

		if paymentRecord.Status != StatusAuthorized {
			return ErrPaymentNotCapturable
		}

		if now.After(paymentRecord.ExpiresAt) {
			return ErrAuthorizationExpired
		}

		capturedPayment, err := paymentRecord.Capture(now)
		if err != nil {
			return err
		}

		capturedHold, err := hold.Capture(now)
		if err != nil {
			return err
		}

		updatedPayer, err := payerRecord.CaptureHeld(paymentRecord.AmountMinor, paymentRecord.Currency, now)
		if err != nil {
			return err
		}

		updatedPayer, err = s.payers.UpdatePayer(ctx, updatedPayer)
		if err != nil {
			return err
		}

		capturedHold, err = s.payments.UpdateHold(ctx, capturedHold)
		if err != nil {
			return err
		}

		capturedPayment, err = s.payments.UpdatePayment(ctx, capturedPayment)
		if err != nil {
			return err
		}

		event, err := outbox.NewEvent(outbox.NewEventInput{
			AggregateType: aggregateTypePayment,
			AggregateID:   capturedPayment.ID,
			EventType:     eventTypePaymentCaptured,
			Payload: paymentCapturedPayload{
				PaymentID:      capturedPayment.ID,
				MerchantID:     capturedPayment.MerchantID,
				PayerID:        capturedPayment.PayerID,
				CapturedAmount: capturedPayment.AmountMinor,
				Currency:       capturedPayment.Currency,
				HoldID:         capturedHold.ID,
				CapturedAt:     now,
			},
			Now: now,
		})
		if err != nil {
			return err
		}

		if _, err := s.outbox.CreateEvent(ctx, event); err != nil {
			return err
		}

		result = CapturePaymentResult{
			Payment: capturedPayment,
			Hold:    capturedHold,
			Payer:   updatedPayer,
		}

		return nil
	})
	if err != nil {
		s.observePayerVersionConflict(err)
		s.observeCapture(paymentMetricResult(err), time.Since(startedAt))
		return CapturePaymentResult{}, err
	}

	s.observeCapture("success", time.Since(startedAt))
	return result, nil
}

func (s *Service) ExpireAuthorizedPayments(ctx context.Context, input ExpireAuthorizedPaymentsInput) (ExpireAuthorizedPaymentsResult, error) {
	limit := input.Limit
	if limit <= 0 {
		limit = 100
	}

	var result ExpireAuthorizedPaymentsResult

	err := s.transactor.WithinTx(ctx, func(ctx context.Context) error {
		now := s.now().UTC()
		expiredPayments, err := s.payments.ListExpiredAuthorizedPayments(ctx, now, limit)
		if err != nil {
			return err
		}

		result.Payments = make([]Payment, 0, len(expiredPayments))
		result.Holds = make([]Hold, 0, len(expiredPayments))
		result.Payers = make([]payer.Payer, 0, len(expiredPayments))

		for _, paymentRecord := range expiredPayments {
			hold, err := s.payments.GetHoldByPaymentID(ctx, paymentRecord.ID)
			if err != nil {
				return err
			}

			payerRecord, err := s.payers.GetPayer(ctx, paymentRecord.PayerID)
			if err != nil {
				return err
			}

			updatedPayer, err := payerRecord.Release(paymentRecord.AmountMinor, paymentRecord.Currency, now)
			if err != nil {
				return err
			}

			updatedPayer, err = s.payers.UpdatePayer(ctx, updatedPayer)
			if err != nil {
				return err
			}

			releasedHold, err := hold.Release(now)
			if err != nil {
				return err
			}

			releasedHold, err = s.payments.UpdateHold(ctx, releasedHold)
			if err != nil {
				return err
			}

			expiredPayment, err := paymentRecord.Expire(now)
			if err != nil {
				return err
			}

			expiredPayment, err = s.payments.UpdatePayment(ctx, expiredPayment)
			if err != nil {
				return err
			}

			event, err := outbox.NewEvent(outbox.NewEventInput{
				AggregateType: aggregateTypePayment,
				AggregateID:   expiredPayment.ID,
				EventType:     eventTypePaymentExpired,
				Payload: paymentExpiredPayload{
					PaymentID:   expiredPayment.ID,
					MerchantID:  expiredPayment.MerchantID,
					PayerID:     expiredPayment.PayerID,
					AmountMinor: expiredPayment.AmountMinor,
					Currency:    expiredPayment.Currency,
					HoldID:      releasedHold.ID,
					ExpiredAt:   now,
					ExpiresAt:   expiredPayment.ExpiresAt,
				},
				Now: now,
			})
			if err != nil {
				return err
			}

			if _, err := s.outbox.CreateEvent(ctx, event); err != nil {
				return err
			}

			result.Payments = append(result.Payments, expiredPayment)
			result.Holds = append(result.Holds, releasedHold)
			result.Payers = append(result.Payers, updatedPayer)
		}

		return nil
	})
	if err != nil {
		s.observePayerVersionConflict(err)
		return ExpireAuthorizedPaymentsResult{}, err
	}

	return result, nil
}

func (s *Service) observeAuthorization(result string, duration time.Duration) {
	if s.metrics == nil {
		return
	}

	s.metrics.ObserveAuthorization(result, duration)
}

func (s *Service) observeCapture(result string, duration time.Duration) {
	if s.metrics == nil {
		return
	}

	s.metrics.ObserveCapture(result, duration)
}

func (s *Service) observePayerVersionConflict(err error) {
	if s.metrics == nil || !errors.Is(err, payer.ErrPayerVersionConflict) {
		return
	}

	s.metrics.ObservePayerVersionConflict()
}

func paymentMetricResult(err error) string {
	switch {
	case errors.Is(err, merchant.ErrMerchantNotFound):
		return "merchant_not_found"
	case errors.Is(err, ErrMerchantCannotCreatePayments):
		return "merchant_cannot_create_payments"
	case errors.Is(err, payer.ErrPayerNotFound):
		return "payer_not_found"
	case errors.Is(err, payer.ErrPayerVersionConflict):
		return "payer_version_conflict"
	case errors.Is(err, ErrPayerCurrencyMismatch):
		return "currency_mismatch"
	case errors.Is(err, ErrInsufficientAvailableBalance):
		return "insufficient_balance"
	case errors.Is(err, ErrPaymentNotFound):
		return "payment_not_found"
	case errors.Is(err, ErrHoldNotFound):
		return "hold_not_found"
	case errors.Is(err, ErrPaymentNotCapturable):
		return "not_capturable"
	case errors.Is(err, ErrAuthorizationExpired):
		return "authorization_expired"
	default:
		return "error"
	}
}
