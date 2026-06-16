package payment

import (
	"context"
	"errors"
	"time"

	"github.com/fanryan/paycore/internal/merchant"
	"github.com/fanryan/paycore/internal/payer"
	currencycode "github.com/fanryan/paycore/internal/shared/currency"
	"github.com/fanryan/paycore/internal/shared/db"
	"github.com/fanryan/paycore/internal/shared/id"
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
	transactor db.Transactor
	now        func() time.Time
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

func NewService(merchants merchant.MerchantRepository, payers payer.PayerRepository, payments Repository) *Service {
	return NewServiceWithTransactor(merchants, payers, payments, db.NoopTransactor{})
}

func NewServiceWithTransactor(
	merchants merchant.MerchantRepository,
	payers payer.PayerRepository,
	payments Repository,
	transactor db.Transactor,
) *Service {
	if transactor == nil {
		transactor = db.NoopTransactor{}
	}

	return &Service{
		merchants:  merchants,
		payers:     payers,
		payments:   payments,
		transactor: transactor,
		now:        time.Now,
	}
}

func (s *Service) AuthorizePayment(ctx context.Context, input AuthorizePaymentInput) (AuthorizePaymentResult, error) {
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

		result = AuthorizePaymentResult{
			Payment: createdPayment,
			Hold:    createdHold,
			Payer:   updatedPayer,
		}

		return nil
	})
	if err != nil {
		return AuthorizePaymentResult{}, err
	}

	return result, nil
}

func (s *Service) CapturePayment(ctx context.Context, input CapturePaymentInput) (CapturePaymentResult, error) {
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

		result = CapturePaymentResult{
			Payment: capturedPayment,
			Hold:    capturedHold,
			Payer:   updatedPayer,
		}

		return nil
	})
	if err != nil {
		return CapturePaymentResult{}, err
	}

	return result, nil
}
