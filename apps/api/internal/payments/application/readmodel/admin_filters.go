package readmodel

import (
	"regexp"
	"strings"
	"unicode/utf8"

	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	paymentsdomain "ideaboard/api/internal/payments/domain"
	"ideaboard/api/internal/platform/pagination"

	"github.com/google/uuid"
)

const (
	maxAdminPaymentOrderUserLen = 50
)

var adminPaymentOrderNumber = regexp.MustCompile(`^IBP-[0-9]{8}-[A-Z0-9]{12}$`)

// NormalizeAdminPaymentOrderFilters validates and canonicalizes an Admin
// payment-order query before it reaches a persistence adapter.
func NormalizeAdminPaymentOrderFilters(input paymentcontracts.AdminPaymentOrderListInput) (AdminPaymentOrderFilters, error) {
	filters := AdminPaymentOrderFilters{OrderNumber: strings.TrimSpace(input.OrderNumber), Status: string(input.Status), AnomalyOnly: input.AnomalyOnly, Limit: input.Limit}
	if filters.OrderNumber != "" && !adminPaymentOrderNumber.MatchString(filters.OrderNumber) {
		return AdminPaymentOrderFilters{}, paymentcontracts.ErrAdminPaymentOrderFilter
	}
	if filters.Status != "" && !validAdminPaymentOrderStatus(paymentsdomain.PaymentOrderStatus(filters.Status)) {
		return AdminPaymentOrderFilters{}, paymentcontracts.ErrAdminPaymentOrderFilter
	}
	limit, err := pagination.NormalizeLimit(filters.Limit)
	if err != nil {
		return AdminPaymentOrderFilters{}, paymentcontracts.ErrAdminPaymentOrderFilter
	}
	filters.Limit = limit
	user := strings.TrimSpace(input.User)
	if utf8.RuneCountInString(user) > maxAdminPaymentOrderUserLen {
		return AdminPaymentOrderFilters{}, paymentcontracts.ErrAdminPaymentOrderFilter
	}
	if user != "" {
		if publicID, err := uuid.Parse(user); err == nil {
			filters.UserPublicID = publicID
		} else if validAdminPaymentOrderDisplayName(user) {
			filters.DisplayName = user
		} else {
			return AdminPaymentOrderFilters{}, paymentcontracts.ErrAdminPaymentOrderFilter
		}
	}
	cursor, err := DecodeAdminPaymentOrderCursor(input.Cursor)
	if err != nil {
		return AdminPaymentOrderFilters{}, err
	}
	filters.Cursor = cursor
	return filters, nil
}

// IsValidAdminPaymentOrderNumber validates the public order number format.
func IsValidAdminPaymentOrderNumber(value string) bool {
	return adminPaymentOrderNumber.MatchString(strings.TrimSpace(value))
}

func validAdminPaymentOrderStatus(status paymentsdomain.PaymentOrderStatus) bool {
	switch status {
	case paymentsdomain.PaymentOrderCreating, paymentsdomain.PaymentOrderCheckoutOpen, paymentsdomain.PaymentOrderAsyncPaymentPending,
		paymentsdomain.PaymentOrderPaidPendingCredit, paymentsdomain.PaymentOrderCredited, paymentsdomain.PaymentOrderCancelled,
		paymentsdomain.PaymentOrderExpired, paymentsdomain.PaymentOrderFailed:
		return true
	default:
		return false
	}
}

func validAdminPaymentOrderDisplayName(value string) bool {
	return strings.TrimSpace(value) != "" && !strings.ContainsAny(value, "\x00\r\n")
}
