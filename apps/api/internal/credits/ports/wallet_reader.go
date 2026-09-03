package ports

import (
	"context"

	"ideaboard/api/internal/credits/domain"
)

type WalletReader interface {
	GetWallet(context.Context, int64) (domain.WalletDetails, error)
	ListLedger(context.Context, domain.ListLedgerInput) (domain.LedgerPage, error)
}
