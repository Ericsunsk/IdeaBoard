package postgres

import (
	"errors"
	"fmt"

	"ideaboard/api/internal/credits/domain"

	"github.com/jackc/pgx/v5/pgconn"
)

func MapCreditDatabaseError(err error) error {
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) {
		return fmt.Errorf("credits service: execute command: %w", err)
	}
	switch postgresError.Code {
	case "IB001":
		return domain.ErrIdempotencyConflict
	case "IB002":
		return domain.ErrWalletNotFound
	case "IB003":
		return domain.ErrWalletFrozen
	case "IB004":
		return domain.ErrInsufficientCredits
	case "IB005":
		return domain.ErrHoldNotFound
	case "IB006":
		return domain.ErrHoldSettled
	case "IB007":
		return domain.ErrChargeNotFound
	case "IB008":
		return domain.ErrRefundExceedsCharge
	case "IB009":
		return domain.ErrInvalidCommand
	case "IB010":
		return domain.ErrWelcomeIdentityKeyMismatch
	case "IB011":
		return domain.ErrWalletRestricted
	case "IB012":
		return domain.ErrWalletRestrictionNotFound
	case "IB013":
		return domain.ErrHoldSettlementExceeds
	default:
		return fmt.Errorf("credits service: execute command: %w", err)
	}
}

type rowScanner interface {
	Scan(...any) error
}
