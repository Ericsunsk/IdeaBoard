package testsupport

import (
	"context"

	creditsdomain "ideaboard/api/internal/credits/domain"
)

// FakeMissingWalletReader is the unified test-only credits WalletReader:
// every lookup reports a missing wallet. It replaces the byte-identical
// testWalletReader (payments/tests) and stubWalletReader
// (payments/application/readmodel) duplicates; stateful wallet fakes
// (e.g. runtime v1 fakeBillingWallets) stay where behavior needs them.
type FakeMissingWalletReader struct{}

func (FakeMissingWalletReader) GetWallet(context.Context, int64) (creditsdomain.WalletDetails, error) {
	return creditsdomain.WalletDetails{}, creditsdomain.ErrWalletNotFound
}

func (FakeMissingWalletReader) ResolvePersonalAccount(context.Context, int64) (int64, error) {
	return 0, creditsdomain.ErrWalletNotFound
}

func (FakeMissingWalletReader) ListLedger(context.Context, creditsdomain.ListLedgerInput) (creditsdomain.LedgerPage, error) {
	return creditsdomain.LedgerPage{}, creditsdomain.ErrWalletNotFound
}

// Structural compatibility with credits/ports.WalletReader is enforced at
// every call site (the fake is passed directly as a WalletReader); no
// direct import of the ports package here so this module stays on stdlib
// plus credits/domain.
