package readmodel

import (
	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/payments/application/contracts"
	paymentsdomain "ideaboard/api/internal/payments/domain"
)

func (facts adminPaymentOrderFacts) appendTo(detail *AdminPaymentOrderDetailRecord) {
	for _, refund := range facts.refunds {
		updatedAt := refund.UpdatedAt
		detail.Refunds = append(detail.Refunds, contracts.AdminPaymentRefundSummary{ID: MaskPaymentIdentity(refund.ID), Status: string(refund.Status), IdeaQuantity: refund.IdeaQuantity, AmountMinor: refund.AmountMinor, Currency: refund.Currency, StripeStatus: refund.StripeStatus, ErrorSummary: refund.ErrorSummary, UpdatedAt: &updatedAt})
	}
	for _, dispute := range facts.disputes {
		updatedAt, openedAt := dispute.UpdatedAt, dispute.OpenedAt
		detail.Disputes = append(detail.Disputes, contracts.AdminPaymentDisputeSummary{ID: MaskPaymentIdentity(dispute.StripeDisputeID), Status: string(dispute.Status), ProviderStatus: dispute.ProviderStatus, AmountMinor: dispute.AmountMinor, Currency: dispute.Currency, RatioBasisPoints: DisputeRatioBasisPoints(dispute.AmountMinor, detail.PaidTotalMinor), TargetIdeaQuantity: dispute.TargetIdeaQuantity, DebitedIdeaQuantity: dispute.DebitedIdeaQuantity, UnrecoveredCredits: dispute.UnrecoveredCredits, StripeDashboardURL: StripeDisputeDashboardURL(facts.disputeMode, dispute.StripeDisputeID), OpenedAt: &openedAt, ClosedAt: dispute.ClosedAt, UpdatedAt: &updatedAt})
	}
}

func paymentRefundIDs(refunds []paymentsdomain.PaymentRefund) map[string]struct{} {
	ids := make(map[string]struct{}, len(refunds))
	for _, refund := range refunds {
		if refund.StripeRefundID != "" {
			ids[refund.StripeRefundID] = struct{}{}
		}
	}
	return ids
}

func paymentDisputeIDs(disputes []paymentsdomain.PaymentDispute) map[string]struct{} {
	ids := make(map[string]struct{}, len(disputes))
	for _, dispute := range disputes {
		ids[dispute.StripeDisputeID] = struct{}{}
	}
	return ids
}

func appendAdminPaymentOrderWebhookFacts(detail *AdminPaymentOrderDetailRecord, events []dbsqlc.ListAdminPaymentOrderWebhookEventsRow, refundIDs, disputeIDs map[string]struct{}) {
	for _, event := range events {
		receivedAt := dbutil.TimeValue(event.ReceivedAt)
		updatedAt := &receivedAt
		if event.ProcessedAt.Valid {
			processedAt := event.ProcessedAt.Time
			updatedAt = &processedAt
		}
		switch event.EventType {
		case "refund.updated", "refund.failed":
			if _, durable := refundIDs[event.StripeObjectID]; durable {
				continue
			}
			detail.Refunds = append(detail.Refunds, contracts.AdminPaymentRefundSummary{ID: MaskPaymentIdentity(event.StripeObjectID), Status: event.EventType, UpdatedAt: updatedAt})
		case "charge.dispute.created", "charge.dispute.closed":
			if _, durable := disputeIDs[event.StripeObjectID]; durable {
				continue
			}
			detail.Disputes = append(detail.Disputes, contracts.AdminPaymentDisputeSummary{ID: MaskPaymentIdentity(event.StripeObjectID), Status: event.EventType, UpdatedAt: updatedAt})
		}
	}
}
