package domain

import (
	"errors"

	creditsdomain "ideaboard/api/internal/credits/domain"
	quotadomain "ideaboard/api/internal/memberquotas"
)

var (
	// ErrNotFound reports an invisible task (unknown id, malformed shape
	// at lookup, or cross-workspace id). Handlers render it as 404
	// task_not_found per Baseline 4 anti-enumeration.
	ErrNotFound = errors.New("workspacetasks: not found")
	// ErrForbidden reports a visible-but-forbidden task operation (403
	// task_forbidden).
	ErrForbidden = errors.New("workspacetasks: forbidden")
	// ErrStateConflict reports a terminal-state mutation (409
	// task_state_conflict).
	ErrStateConflict = errors.New("workspacetasks: illegal state transition")
	// ErrValidation reports a 422-class failure (kind/assets/params).
	ErrValidation = errors.New("workspacetasks: validation failed")
	// ErrInsufficientCredits is the canonical credits hold failure
	// (402 task_insufficient_credits). The holder propagates it unchanged.
	ErrInsufficientCredits = creditsdomain.ErrInsufficientCredits
	// ErrQuoteUnavailable reports a server-side price snapshot failure
	// (503 billing_quote_unavailable). It stays a distinct routing
	// marker (never a canonical alias): adapters pair it with the
	// underlying billing cause via multi-%w, and the 422-vs-503 split in
	// renderTaskError depends on that pairing.
	ErrQuoteUnavailable = errors.New("workspacetasks: quote unavailable")
	// ErrSettlementUnavailable reports a missing atomic settlement seam.
	// Production task mutation never falls back to compensating writes.
	ErrSettlementUnavailable = errors.New("workspacetasks: settlement unavailable")
	// ErrWalletRestricted is the canonical credits restriction failure
	// (403 billing_wallet_restricted). The holder propagates it unchanged.
	ErrWalletRestricted = creditsdomain.ErrWalletRestricted
	// ErrMemberQuotaExhausted reports that a submit would exceed the
	// member's remaining cross-workspace consumption quota for the
	// current window (403 member_quota_exhausted, #946). Alias, not a
	// copy: the quota seam pairs it via multi-%w and renderTaskError
	// keeps it distinct from 402 insufficiency and 403 wallet restriction.
	ErrMemberQuotaExhausted = quotadomain.ErrQuotaExhausted
	// ErrMediaNotFound wraps an invisible input/output asset.
	ErrMediaNotFound = errors.New("workspacetasks: media not found")
	// ErrMediaNotReady wraps a non-ready asset (409 media_not_ready).
	ErrMediaNotReady = errors.New("workspacetasks: media not ready")
	// ErrMediaArchived wraps an archived asset (410 media_archived).
	ErrMediaArchived = errors.New("workspacetasks: media archived")
	// ErrBoardNotFound wraps an invisible Board binding (unknown id or no
	// effective Board role for the caller). Handlers render it as 404
	// board_not_found, never leaking Board existence.
	ErrBoardNotFound = errors.New("workspacetasks: board not found")
	// ErrBoardArchived wraps a generation attempt on an archived Board
	// (409 board_archived, matching the Boards write contract).
	ErrBoardArchived = errors.New("workspacetasks: board archived")
	// ErrIdempotencyConflict reports the same (workspace, operation, key) reused
	// with a different request body. Handlers render it as 409
	// task_idempotency_conflict. It is distinct from ErrStateConflict: the task
	// state machine is irrelevant here, the client reused a key for a different
	// request.
	ErrIdempotencyConflict = errors.New("workspacetasks: idempotency key reused with a different request")
)
