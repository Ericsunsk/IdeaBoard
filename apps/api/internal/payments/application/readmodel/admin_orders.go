package readmodel

import (
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"

	"github.com/google/uuid"
)

// AdminPaymentOrderFilters is the validated keyset query used by the Admin
// payment-order read model. It contains no persistence implementation detail.
type AdminPaymentOrderFilters struct {
	OrderNumber  string
	UserPublicID uuid.UUID
	DisplayName  string
	Status       string
	AnomalyOnly  bool
	Cursor       *AdminPaymentOrderCursor
	Limit        int
}

// AdminPaymentOrderDetailRecord carries the sanitized Admin detail together
// with the internal user key needed for application-side wallet reads. The
// internal key is never serialized by the HTTP adapter.
type AdminPaymentOrderDetailRecord struct {
	paymentcontracts.AdminPaymentOrderDetail
	InternalUserID int64
}
