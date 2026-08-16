package safedeal

import (
	"context"
	"database/sql"
	"fmt"

	"freelance/apps/api/internal/payments"
)

// AttemptTransitioner translates authoritative Phase 11 payment-attempt
// transitions into the existing Safe Deal state machine. It deliberately uses
// the original funding payment id for release/refund events because
// payment_records is keyed by that provider payment.
type AttemptTransitioner struct {
	DB         *sql.DB
	Repository Repository
}

func (t AttemptTransitioner) Apply(ctx context.Context, a payments.Attempt) error {
	if a.Domain != payments.DomainSafeDeal || t.DB == nil || t.Repository == nil {
		return nil
	}
	var provider, fundingPaymentID, currency string
	var gross, payout int64
	err := t.DB.QueryRowContext(ctx, `SELECT r.provider,r.provider_payment_id,d.currency,d.gross_amount_kopecks,d.freelancer_amount_kopecks
FROM safe_deal_provider_refs r JOIN safe_deals d ON d.id=r.deal_id WHERE r.deal_id=$1`, a.InternalReferenceID).
		Scan(&provider, &fundingPaymentID, &currency, &gross, &payout)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}
	eventType, state, amount := "", "", gross
	switch a.OperationType {
	case payments.OperationPayment:
		switch a.Status {
		case payments.StatusSucceeded:
			eventType, state = "FUNDING_CONFIRMED", "FUNDED"
		case payments.StatusCanceled:
			eventType, state = "CANCEL_CONFIRMED", "CANCELED"
		default:
			return nil
		}
	case payments.OperationRefund:
		if a.Status == payments.StatusSucceeded || a.Status == payments.StatusRefunded {
			eventType, state = "REFUND_CONFIRMED", "REFUNDED"
		} else {
			return nil
		}
	case payments.OperationPayout:
		if a.Status == payments.StatusSucceeded {
			eventType, state, amount = "RELEASE_CONFIRMED", "RELEASED", payout
		} else {
			return nil
		}
	default:
		return nil
	}
	_, _, err = t.Repository.ApplyProviderEvent(ctx, VerifiedProviderEvent{
		Provider: provider, ProviderEventID: fmt.Sprintf("attempt:%s:%s", a.ID, a.Status),
		ProviderPaymentID: fundingPaymentID, Type: eventType, State: state,
		Currency: currency, AmountKopecks: amount, Verified: true,
		Payload: map[string]any{"payment_attempt_id": a.ID, "provider_operation_id": a.ProviderOperationID},
	})
	if err == ErrInvalidState || err == ErrConflict {
		// Duplicate or out-of-order terminal provider observations are safe to
		// ignore; the domain repository itself remains authoritative.
		return nil
	}
	return err
}
