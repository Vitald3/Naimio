package payments

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type AdminOperationsHandler struct {
	Repository PostgresRepository
	Reconciler Reconciler
	Providers  ProviderSet
	ActorID    func(context.Context) (string, bool)
}

func (h AdminOperationsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.ActorID(r.Context())
	if !ok {
		routingError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/payments"), "/")
	if path == "" && r.Method == http.MethodGet {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		from, err := parseAdminDate(r.URL.Query().Get("from"), false)
		if err != nil {
			routingError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid from date")
			return
		}
		to, err := parseAdminDate(r.URL.Query().Get("to"), true)
		if err != nil {
			routingError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid to date")
			return
		}
		items, err := h.Repository.ListAdminAttempts(r.Context(), AdminAttemptFilter{
			Provider: ParseProvider(r.URL.Query().Get("provider")), Operation: OperationType(strings.TrimSpace(r.URL.Query().Get("operation"))),
			Domain: Domain(strings.TrimSpace(r.URL.Query().Get("domain"))), Status: Status(strings.TrimSpace(r.URL.Query().Get("status"))),
			Reference: strings.TrimSpace(r.URL.Query().Get("reference")), From: from, To: to, Limit: limit,
		})
		if err != nil {
			routingError(w, r, 500, "INTERNAL_ERROR", "payment operations unavailable")
			return
		}
		views := make([]adminAttemptView, 0, len(items))
		for _, item := range items {
			views = append(views, newAdminAttemptView(item))
		}
		routingReply(w, 200, map[string]any{"data": views})
		return
	}
	parts := strings.Split(path, "/")
	if len(parts) == 1 && r.Method == http.MethodGet {
		a, err := h.Repository.Get(r.Context(), parts[0])
		if err != nil {
			routingError(w, r, 404, "NOT_FOUND", "payment attempt not found")
			return
		}
		events, err := h.Repository.ListAttemptEvents(r.Context(), a.ID)
		if err != nil {
			routingError(w, r, 500, "INTERNAL_ERROR", "payment operation unavailable")
			return
		}
		related, err := h.Repository.ListRelatedAttempts(r.Context(), a)
		if err != nil {
			routingError(w, r, 500, "INTERNAL_ERROR", "payment operation unavailable")
			return
		}
		audit, err := h.Repository.ListPaymentAudit(r.Context(), a.ID)
		if err != nil {
			routingError(w, r, 500, "INTERNAL_ERROR", "payment operation unavailable")
			return
		}
		relatedViews := make([]adminAttemptView, 0, len(related))
		for _, item := range related {
			relatedViews = append(relatedViews, newAdminAttemptView(item))
		}
		_, providerErr := h.Providers.Status(a.Provider)
		reconciliationAvailable := a.ProviderOperationID != "" && providerErr == nil
		if !reconciliationAvailable && h.Reconciler.statusProvider(a.Provider) != nil {
			reconciliationAvailable = a.ProviderOperationID != ""
		}
		routingReply(w, 200, map[string]any{"data": map[string]any{"attempt": newAdminAttemptView(a), "events": newAdminEventViews(events), "related_attempts": relatedViews, "audit": audit, "reconciliation_available": reconciliationAvailable}})
		return
	}
	if len(parts) != 2 || r.Method != http.MethodPost {
		routingError(w, r, 404, "NOT_FOUND", "payment operation route not found")
		return
	}
	attempt, err := h.Repository.Get(r.Context(), parts[0])
	if err != nil {
		routingError(w, r, 404, "NOT_FOUND", "payment attempt not found")
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body)
	reason := strings.TrimSpace(body.Reason)
	if reason == "" {
		routingError(w, r, 422, "VALIDATION_ERROR", "reason is required")
		return
	}
	switch parts[1] {
	case "reconcile":
		out, err := h.Reconciler.One(r.Context(), attempt)
		if err != nil {
			routingError(w, r, 503, "RECONCILIATION_UNAVAILABLE", "payment reconciliation unavailable")
			return
		}
		_ = h.Repository.AuditPaymentAdminAction(r.Context(), actor, "payment.reconciled", attempt.ID, reason, r.Header.Get("X-Request-ID"), string(out.Status))
		routingReply(w, 200, map[string]any{"data": newAdminAttemptView(out)})
	case "refund":
		if attempt.Domain != DomainPlatformPayment || attempt.OperationType != OperationPayment || attempt.Status != StatusSucceeded || attempt.ProviderOperationID == "" {
			routingError(w, r, 409, "INVALID_STATE", "refund must use the owning financial domain; only standalone platform payments are refundable here")
			return
		}
		provider, err := h.Providers.Purchase(attempt.Provider)
		if err != nil {
			routingError(w, r, 422, "UNSUPPORTED_ROUTE", "provider does not support refunds")
			return
		}
		key := "admin-refund:" + attempt.ID
		refundAttempt, err := h.Reconciler.Service.Create(r.Context(), Attempt{Domain: attempt.Domain, InternalReferenceID: attempt.InternalReferenceID, Provider: attempt.Provider, OperationType: OperationRefund, AmountKopecks: attempt.AmountKopecks, Currency: attempt.Currency, IdempotencyKey: key, PaymentMethod: attempt.PaymentMethod})
		if err != nil {
			routingError(w, r, 409, "CONFLICT", "refund already exists or cannot be created")
			return
		}
		if refundAttempt.ProviderOperationID == "" {
			external, callErr := provider.RefundPurchase(r.Context(), attempt.ProviderOperationID, key, attempt.AmountKopecks)
			if callErr != nil {
				_, _ = h.Reconciler.Service.MarkUnknown(r.Context(), refundAttempt.ID)
				routingError(w, r, 503, "PROVIDER_UNAVAILABLE", "refund status requires reconciliation")
				return
			}
			refundAttempt.ProviderOperationID = external
			refundAttempt.ProviderRawStatus = "PENDING"
			_ = refundAttempt.Transition(StatusProcessing, time.Now().UTC())
			if err := h.Repository.Update(r.Context(), refundAttempt); err != nil {
				routingError(w, r, 500, "INTERNAL_ERROR", "refund could not be stored")
				return
			}
		}
		_ = h.Repository.AuditPaymentAdminAction(r.Context(), actor, "payment.refund_requested", attempt.ID, reason, r.Header.Get("X-Request-ID"), refundAttempt.ID)
		routingReply(w, 202, map[string]any{"data": newAdminAttemptView(refundAttempt)})
	case "cancel":
		if attempt.Domain != DomainPlatformPayment || attempt.OperationType != OperationPayment || attempt.Terminal() || attempt.ProviderOperationID == "" {
			routingError(w, r, 409, "INVALID_STATE", "only a non-terminal standalone platform payment can be canceled here")
			return
		}
		provider, err := h.Providers.Purchase(attempt.Provider)
		if err != nil {
			routingError(w, r, 422, "UNSUPPORTED_ROUTE", "provider does not support cancellation")
			return
		}
		canceler, ok := provider.(CancellationProvider)
		if !ok {
			routingError(w, r, 422, "UNSUPPORTED_ROUTE", "provider does not support cancellation")
			return
		}
		status, raw, err := canceler.CancelPurchase(r.Context(), attempt.ProviderOperationID, "admin-cancel:"+attempt.ID, attempt.AmountKopecks)
		if err != nil {
			unknown, _ := h.Reconciler.Service.MarkUnknown(r.Context(), attempt.ID)
			_ = h.Repository.AuditPaymentAdminAction(r.Context(), actor, "payment.cancel_unknown", attempt.ID, reason, r.Header.Get("X-Request-ID"), string(unknown.Status))
			routingError(w, r, 503, "PROVIDER_UNAVAILABLE", "cancellation status requires reconciliation")
			return
		}
		if status != StatusCanceled {
			routingError(w, r, 409, "INVALID_PROVIDER_STATE", "provider did not confirm cancellation")
			return
		}
		attempt.ProviderRawStatus = raw
		if err := attempt.Transition(StatusCanceled, time.Now().UTC()); err != nil {
			routingError(w, r, 409, "INVALID_STATE", "payment cannot be canceled from its current state")
			return
		}
		if err := h.Repository.Update(r.Context(), attempt); err != nil {
			routingError(w, r, 500, "INTERNAL_ERROR", "cancellation could not be stored")
			return
		}
		_ = h.Repository.AuditPaymentAdminAction(r.Context(), actor, "payment.canceled", attempt.ID, reason, r.Header.Get("X-Request-ID"), raw)
		routingReply(w, 200, map[string]any{"data": newAdminAttemptView(attempt)})
	default:
		routingError(w, r, 404, "NOT_FOUND", "payment operation route not found")
	}
}

type adminAttemptView struct {
	ID                  string              `json:"id"`
	Domain              Domain              `json:"domain"`
	InternalReferenceID string              `json:"internal_reference_id"`
	Provider            ProviderName        `json:"provider"`
	OperationType       OperationType       `json:"operation_type"`
	Status              Status              `json:"status"`
	AmountKopecks       int64               `json:"amount_kopecks"`
	Currency            string              `json:"currency"`
	PaymentMethod       string              `json:"payment_method,omitempty"`
	ProviderOperationID string              `json:"provider_operation_id,omitempty"`
	ProviderRawStatus   string              `json:"provider_raw_status,omitempty"`
	ErrorCategory       string              `json:"error_category,omitempty"`
	ReconciliationState ReconciliationState `json:"reconciliation_state"`
	CreatedAt           time.Time           `json:"created_at"`
	UpdatedAt           time.Time           `json:"updated_at"`
	TerminalAt          *time.Time          `json:"terminal_at,omitempty"`
}

func newAdminAttemptView(a Attempt) adminAttemptView {
	return adminAttemptView{
		ID: a.ID, Domain: a.Domain, InternalReferenceID: a.InternalReferenceID, Provider: a.Provider,
		OperationType: a.OperationType, Status: a.Status, AmountKopecks: a.AmountKopecks, Currency: a.Currency,
		PaymentMethod: a.PaymentMethod, ProviderOperationID: a.ProviderOperationID, ProviderRawStatus: a.ProviderRawStatus,
		ErrorCategory: a.ErrorCategory, ReconciliationState: a.ReconciliationState, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt, TerminalAt: a.TerminalAt,
	}
}

type adminWebhookEventView struct {
	Provider          ProviderName `json:"provider"`
	ProviderEventID   string       `json:"provider_event_id"`
	EventType         string       `json:"event_type"`
	ExternalReference string       `json:"external_reference,omitempty"`
	VerifiedAt        *time.Time   `json:"verified_at,omitempty"`
	ProcessedAt       *time.Time   `json:"processed_at,omitempty"`
	Attempts          int          `json:"attempts"`
	ProcessingResult  string       `json:"processing_result,omitempty"`
}

func newAdminEventViews(events []WebhookEvent) []adminWebhookEventView {
	out := make([]adminWebhookEventView, 0, len(events))
	for _, e := range events {
		out = append(out, adminWebhookEventView{Provider: e.Provider, ProviderEventID: e.EventID, EventType: e.Type, ExternalReference: e.ExternalReference, VerifiedAt: e.VerifiedAt, ProcessedAt: e.ProcessedAt, Attempts: e.Attempts, ProcessingResult: e.ProcessingResult})
	}
	return out
}

func parseAdminDate(raw string, endExclusive bool) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		t = t.UTC()
		return &t, nil
	}
	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return nil, err
	}
	if endExclusive {
		t = t.AddDate(0, 0, 1)
	}
	t = t.UTC()
	return &t, nil
}
