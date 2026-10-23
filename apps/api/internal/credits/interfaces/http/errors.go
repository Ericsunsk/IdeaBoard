package http

import (
	"net/http"

	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/errorrender"

	"github.com/gin-gonic/gin"
)

// Credit error codes are wire-stable. Do not rename a code without
// coordinating the caller that branches on it. Messages stay
// intentionally safe for public display.
//
// Classification lives here at the HTTP edge so credits domain sentinels
// stay dependency-free; every credits failure below renders through the
// machine envelope. The values below alias the kernel registry
// (platform/contract.CodeCredit*): the registry is the single source of
// truth, these names keep the edge diffs trivial. Wire statuses and copy
// are unchanged from the legacy numeric envelope.
const (
	codeCreditInvalidInput            = string(contract.CodeCreditInvalidInput)
	codeCreditIdempotencyConflict     = string(contract.CodeCreditIdempotencyConflict)
	codeCreditWalletNotFound          = string(contract.CodeCreditWalletNotFound)
	codeCreditWalletFrozen            = string(contract.CodeCreditWalletFrozen)
	codeCreditWalletRestricted        = string(contract.CodeCreditWalletRestricted)
	codeCreditInsufficientCredits     = string(contract.CodeCreditInsufficientCredits)
	codeCreditChargeNotFound          = string(contract.CodeCreditChargeNotFound)
	codeCreditRefundExceedsCharge     = string(contract.CodeCreditRefundExceedsCharge)
	codeCreditReconciliationNotFound  = string(contract.CodeCreditReconciliationNotFound)
	codeCreditReconciliationAvailable = string(contract.CodeCreditReconciliationUnavailable)
	codeCreditOperationFailed         = string(contract.CodeCreditOperationFailed)
)

// creditErrorTable is the single source of truth for credits HTTP error
// rendering; add rows here instead of scattering errors.Is checks across
// handlers. Rows are ordered from the most specific outcome to the most
// generic; the first match wins.
var creditErrorTable = []errorrender.Mapping{
	{Targets: []error{domain.ErrInvalidCommand}, Code: codeCreditInvalidInput, Message: errorrender.MsgCreditsInterfacesHTTPPackage},
	{Targets: []error{domain.ErrIdempotencyConflict}, Code: codeCreditIdempotencyConflict, Message: errorrender.MsgPackage2},
	{Targets: []error{domain.ErrWalletNotFound}, Code: codeCreditWalletNotFound, Message: errorrender.MsgPackage3},
	{Targets: []error{domain.ErrWalletFrozen}, Code: codeCreditWalletFrozen, Message: errorrender.MsgPackage4},
	{Targets: []error{domain.ErrWalletRestricted}, Code: codeCreditWalletRestricted, Message: errorrender.MsgPackage5},
	{Targets: []error{domain.ErrInsufficientCredits}, Code: codeCreditInsufficientCredits, Message: errorrender.MsgPackage6},
	{Targets: []error{domain.ErrChargeNotFound}, Code: codeCreditChargeNotFound, Message: errorrender.MsgPackage7},
	{Targets: []error{domain.ErrRefundExceedsCharge}, Code: codeCreditRefundExceedsCharge, Message: errorrender.MsgPackage8},
	{Targets: []error{domain.ErrReconciliationNotFound}, Code: codeCreditReconciliationNotFound, Message: errorrender.MsgPackage9},
	{Targets: []error{domain.ErrReconciliationUnavailable}, Code: codeCreditReconciliationAvailable, Message: errorrender.MsgPackage10},
}

func writeCreditError(c *gin.Context, err error) {
	errorrender.RenderMapped(c, err, creditErrorTable, errorrender.MappedOptions{
		DefaultStatus:  http.StatusInternalServerError,
		DefaultCode:    codeCreditOperationFailed,
		DefaultMessage: errorrender.MsgWriteCreditError,
		UnmatchedLog:   "credit operation failed",
		// The reconciliation outage row stays server-attributable like
		// the legacy InternalError path; 4xx matches render silently.
		MatchedLogAtOrAbove: 500,
		MatchedLog:          "credit operation failed",
	})
}
