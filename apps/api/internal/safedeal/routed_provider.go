package safedeal

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"freelance/apps/api/internal/payments"
)

// RoutedProvider bridges the existing Safe Deal domain to Phase 11 routing.
// Existing payment attempts remain pinned; the Safe Deal state machine stays
// authoritative and never depends on provider-specific statuses directly.
type RoutedProvider struct {
	DB            *sql.DB
	Routing       payments.RoutingService
	Payments      payments.Service
	Providers     payments.ProviderSet
	YooKassa      *payments.YooKassa
	TBankNominal  *payments.TBankNominal
	Runtime       *payments.ProviderRuntime
	PublicBaseURL string
}

type routedProviderRef struct {
	DealID            string
	Provider          payments.ProviderName
	ProviderDealID    string
	ProviderPaymentID string
	ProviderPayoutID  string
}

func (p RoutedProvider) yooKassa() *payments.YooKassa {
	if p.Runtime != nil {
		if v := p.Runtime.YooKassa(); v != nil {
			return v
		}
	}
	return p.YooKassa
}
func (p RoutedProvider) tBankNominal() *payments.TBankNominal {
	if p.Runtime != nil {
		if v := p.Runtime.TBankNominal(); v != nil {
			return v
		}
	}
	return p.TBankNominal
}

func (p RoutedProvider) refByPayment(ctx context.Context, providerPaymentID string) (routedProviderRef, error) {
	if p.DB == nil || strings.TrimSpace(providerPaymentID) == "" {
		return routedProviderRef{}, ErrProvider
	}
	var ref routedProviderRef
	err := p.DB.QueryRowContext(ctx, `SELECT deal_id::text,provider,COALESCE(provider_deal_id,''),COALESCE(provider_payment_id,''),COALESCE(provider_payout_id,'') FROM safe_deal_provider_refs WHERE provider_payment_id=$1`, providerPaymentID).Scan(&ref.DealID, &ref.Provider, &ref.ProviderDealID, &ref.ProviderPaymentID, &ref.ProviderPayoutID)
	if err != nil {
		return routedProviderRef{}, ErrProvider
	}
	return ref, nil
}

func (p RoutedProvider) selected(ctx context.Context) (payments.ProviderName, error) {
	return p.Routing.Select(ctx, payments.DomainSafeDeal, payments.CapabilitySafeDeal, payments.CapabilityPayoutCard)
}
func (p RoutedProvider) Capabilities() Capabilities {
	// Capability checks are repeated at operation time because durable routing
	// may change. This advertises the product surface without bypassing routing.
	return Capabilities{Funding: true, HoldFunds: true, Release: true, Refund: true, PartialRefund: true, Payout: true, Webhooks: true, Reconciliation: true, PaymentMethods: []string{MethodCard, MethodSBP}}
}
func (p RoutedProvider) CreateFunding(ctx context.Context, in CreateFundingRequest) (CreateFundingResult, error) {
	provider, err := p.selected(ctx)
	if err != nil {
		return CreateFundingResult{}, ErrUnsupported
	}
	yoo := p.yooKassa()
	if provider != payments.ProviderYooKassa || yoo == nil {
		return CreateFundingResult{}, ErrUnsupported
	}

	// Claim the internal financial operation before any external call. The
	// partial unique index added in Phase 11 prevents a second browser/tab with
	// another idempotency key from creating a parallel provider charge.
	attempt, err := p.Payments.Create(ctx, payments.Attempt{Domain: payments.DomainSafeDeal, InternalReferenceID: in.DealID, Provider: provider, OperationType: payments.OperationPayment, AmountKopecks: in.AmountKopecks, Currency: in.Currency, IdempotencyKey: in.IdempotencyKey, PaymentMethod: MethodCard})
	if errors.Is(err, payments.ErrAttemptConflict) && p.DB != nil {
		var existing payments.Attempt
		var terminal sql.NullTime
		err = p.DB.QueryRowContext(ctx, `SELECT id::text,domain,internal_reference_id::text,provider,operation_type,status,amount_kopecks,currency,idempotency_key,COALESCE(payment_method,''),COALESCE(provider_operation_id,''),COALESCE(provider_payment_method_ref,''),COALESCE(provider_confirmation_url,''),COALESCE(provider_raw_status,''),COALESCE(error_category,''),reconciliation_state,created_at,updated_at,terminal_at FROM payment_attempts WHERE domain='SAFE_DEAL' AND internal_reference_id=$1::uuid AND operation_type='PAYMENT' AND status IN('CREATED','PENDING_USER_ACTION','PROCESSING','AUTHORIZED','UNKNOWN_REQUIRES_RECONCILIATION') ORDER BY created_at DESC LIMIT 1`, in.DealID).Scan(&existing.ID, &existing.Domain, &existing.InternalReferenceID, &existing.Provider, &existing.OperationType, &existing.Status, &existing.AmountKopecks, &existing.Currency, &existing.IdempotencyKey, &existing.PaymentMethod, &existing.ProviderOperationID, &existing.ProviderPaymentMethodRef, &existing.ConfirmationURL, &existing.ProviderRawStatus, &existing.ErrorCategory, &existing.ReconciliationState, &existing.CreatedAt, &existing.UpdatedAt, &terminal)
		if err == nil && existing.Provider == provider && existing.AmountKopecks == in.AmountKopecks {
			if existing.ProviderOperationID != "" {
				return CreateFundingResult{Provider: string(provider), ProviderPaymentID: existing.ProviderOperationID, Status: existing.ProviderRawStatus, CheckoutURL: existing.ConfirmationURL}, nil
			}
			// Reuse a claimed-but-not-started attempt. Retrying the provider call
			// with the original idempotency key is safe and prevents a second
			// provider charge after a process crash between DB claim and PSP call.
			attempt = existing
			err = nil
		}
	}
	if err != nil {
		return CreateFundingResult{}, err
	}
	providerKey := strings.TrimSpace(attempt.IdempotencyKey)
	if providerKey == "" {
		providerKey = in.IdempotencyKey
	}
	if attempt.ProviderOperationID != "" {
		return CreateFundingResult{Provider: string(provider), ProviderPaymentID: attempt.ProviderOperationID, Status: attempt.ProviderRawStatus, CheckoutURL: attempt.ConfirmationURL}, nil
	}

	dealID, _, err := yoo.CreateSafeDeal(ctx, payments.SafeDealRequest{IdempotencyKey: providerKey + ":deal", Description: "Naimio Safe Deal " + in.DealID, FeeMoment: "deal_closed"})
	if err != nil {
		_, _ = p.Payments.MarkUnknown(ctx, attempt.ID)
		return CreateFundingResult{}, err
	}
	out, err := yoo.CreateSafeDealPayment(ctx, payments.SafeDealPaymentRequest{DealID: dealID, IdempotencyKey: providerKey, Description: "Naimio Safe Deal funding", ReturnURL: strings.TrimRight(p.PublicBaseURL, "/") + "/dashboard/safe-deals/" + in.DealID, AmountKopecks: in.AmountKopecks, PayoutKopecks: in.PayoutKopecks, Capture: true})
	if err != nil {
		_, _ = p.Payments.MarkUnknown(ctx, attempt.ID)
		return CreateFundingResult{}, err
	}
	attempt.ProviderOperationID = out.ExternalID
	attempt.ConfirmationURL = out.ConfirmationURL
	attempt.ProviderRawStatus = out.Status
	st, _, _ := yoo.GetStatus(ctx, out.ExternalID)
	if st == "" {
		st = payments.StatusPendingUserAction
	}
	if st != payments.StatusCreated {
		_ = attempt.Transition(st, time.Now().UTC())
	}
	if err := p.Payments.Repository.Update(ctx, attempt); err != nil {
		return CreateFundingResult{}, err
	}
	if p.DB != nil {
		_, _ = p.DB.ExecContext(ctx, `INSERT INTO safe_deal_provider_refs(deal_id,provider,provider_deal_id,provider_payment_id) VALUES($1,$2,$3,$4) ON CONFLICT(deal_id) DO UPDATE SET provider=excluded.provider,provider_deal_id=excluded.provider_deal_id,provider_payment_id=excluded.provider_payment_id,updated_at=now()`, in.DealID, provider, dealID, out.ExternalID)
	}
	return CreateFundingResult{Provider: string(provider), ProviderPaymentID: out.ExternalID, Status: out.Status, CheckoutURL: out.ConfirmationURL}, nil
}
func (p RoutedProvider) GetPayment(ctx context.Context, id string) (PaymentState, error) {
	ref, err := p.refByPayment(ctx, id)
	if err != nil {
		return PaymentState{}, err
	}
	provider, providerErr := p.Providers.Status(ref.Provider)
	if providerErr != nil || provider == nil {
		return PaymentState{}, ErrUnsupported
	}
	_, raw, err := provider.GetStatus(ctx, ref.ProviderPaymentID)
	return PaymentState{Status: raw}, err
}
func (p RoutedProvider) CancelPayment(ctx context.Context, in CancelPaymentRequest) error {
	ref, err := p.refByPayment(ctx, in.ProviderPaymentID)
	if err != nil {
		return err
	}
	switch ref.Provider {
	case payments.ProviderYooKassa:
		yoo := p.yooKassa()
		if yoo == nil {
			return ErrUnsupported
		}
		status, _, err := yoo.CancelPayment(ctx, ref.ProviderPaymentID, in.IdempotencyKey)
		if err != nil {
			return err
		}
		if status != payments.StatusCanceled {
			return ErrProvider
		}
		return nil
	default:
		return ErrUnsupported
	}
}
func (p RoutedProvider) Refund(ctx context.Context, in RefundRequest) (RefundResult, error) {
	ref, err := p.refByPayment(ctx, in.ProviderPaymentID)
	if err != nil {
		return RefundResult{}, err
	}
	yoo := p.yooKassa()
	if ref.Provider != payments.ProviderYooKassa || yoo == nil {
		return RefundResult{}, ErrUnsupported
	}
	attempt, err := p.Payments.Create(ctx, payments.Attempt{Domain: payments.DomainSafeDeal, InternalReferenceID: ref.DealID, Provider: ref.Provider, OperationType: payments.OperationRefund, AmountKopecks: in.AmountKopecks, Currency: in.Currency, IdempotencyKey: in.IdempotencyKey, PaymentMethod: MethodCard})
	if err != nil {
		return RefundResult{}, err
	}
	id, err := yoo.RefundSafeDeal(ctx, ref.ProviderPaymentID, ref.ProviderDealID, in.IdempotencyKey, in.AmountKopecks, in.PayoutKopecks)
	if err != nil {
		_, _ = p.Payments.MarkUnknown(ctx, attempt.ID)
		return RefundResult{}, err
	}
	attempt.ProviderOperationID, attempt.ProviderRawStatus = id, "pending"
	_ = attempt.Transition(payments.StatusProcessing, time.Now().UTC())
	if err := p.Payments.Repository.Update(ctx, attempt); err != nil {
		return RefundResult{}, err
	}
	return RefundResult{Status: "PENDING", ProviderOperationID: id}, nil
}
func (p RoutedProvider) Release(ctx context.Context, in ReleaseRequest) (ReleaseResult, error) {
	ref, err := p.refByPayment(ctx, in.ProviderPaymentID)
	if err != nil {
		return ReleaseResult{}, err
	}
	yoo := p.yooKassa()
	if ref.Provider != payments.ProviderYooKassa || yoo == nil || p.DB == nil {
		return ReleaseResult{}, ErrUnsupported
	}
	var freelancerID string
	err = p.DB.QueryRowContext(ctx, `SELECT freelancer_user_id::text FROM safe_deals WHERE id=$1::uuid`, ref.DealID).Scan(&freelancerID)
	if err != nil {
		return ReleaseResult{}, ErrProvider
	}
	var token string
	err = p.DB.QueryRowContext(ctx, `SELECT external_reference FROM payout_recipient_bindings WHERE user_id=$1 AND provider='yookassa' AND status='VERIFIED'`, freelancerID).Scan(&token)
	if errors.Is(err, sql.ErrNoRows) {
		return ReleaseResult{}, ErrUnsupported
	}
	if err != nil {
		return ReleaseResult{}, ErrProvider
	}
	attempt, err := p.Payments.Create(ctx, payments.Attempt{Domain: payments.DomainSafeDeal, InternalReferenceID: ref.DealID, Provider: ref.Provider, OperationType: payments.OperationPayout, AmountKopecks: in.AmountKopecks, Currency: in.Currency, IdempotencyKey: in.IdempotencyKey, PaymentMethod: MethodCard})
	if err != nil {
		return ReleaseResult{}, err
	}
	id, status, err := yoo.CreateSafeDealPayout(ctx, payments.SafeDealPayoutRequest{DealID: ref.ProviderDealID, PayoutToken: token, IdempotencyKey: in.IdempotencyKey, Description: "Naimio Safe Deal payout " + ref.DealID, AmountKopecks: in.AmountKopecks})
	if err != nil {
		_, _ = p.Payments.MarkUnknown(ctx, attempt.ID)
		return ReleaseResult{}, err
	}
	attempt.ProviderOperationID, attempt.ProviderRawStatus = id, status
	_ = attempt.Transition(payments.StatusProcessing, time.Now().UTC())
	if err := p.Payments.Repository.Update(ctx, attempt); err != nil {
		return ReleaseResult{}, err
	}
	_, _ = p.DB.ExecContext(ctx, `UPDATE safe_deal_provider_refs SET provider_payout_id=$2,updated_at=now() WHERE deal_id=$1`, ref.DealID, id)
	return ReleaseResult{Status: status, ProviderOperationID: id}, nil
}
func (p RoutedProvider) VerifyWebhook(context.Context, map[string][]string, []byte) (VerifiedProviderEvent, error) {
	return VerifiedProviderEvent{}, ErrUnsupported
}
