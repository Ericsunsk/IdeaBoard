package workspacetasks

import (
	"context"
	tasks "ideaboard/api/internal/tasking/domain"
	taskmemory "ideaboard/api/internal/tasking/infrastructure/memory"
	taskports "ideaboard/api/internal/tasking/ports/tasks"
)

type InMemoryStore = taskmemory.InMemoryStore

var NewInMemoryStoreForTests = taskmemory.NewInMemoryStoreForTests

type Status = tasks.Status

const (
	StatusPending   = tasks.StatusPending
	StatusRunning   = tasks.StatusRunning
	StatusCompleted = tasks.StatusCompleted
	StatusFailed    = tasks.StatusFailed
	StatusCancelled = tasks.StatusCancelled
)

type WorkspaceTask = tasks.WorkspaceTask
type OutputRef = tasks.OutputRef
type BillingSnapshot = tasks.BillingSnapshot
type AssetInfo = tasks.AssetInfo
type QuoteParams = tasks.QuoteParams
type CreateParams = tasks.CreateParams
type CreateInput = tasks.CreateInput
type TaskEvent = tasks.TaskEvent
type TaskBoardBinding = tasks.TaskBoardBinding
type BoardAccess = tasks.BoardAccess
type BoardOperation = tasks.BoardOperation
type BoardTaskFacts = tasks.BoardTaskFacts
type BoardDecision = tasks.BoardDecision
type SubmissionClaim = tasks.SubmissionClaim
type FieldError = tasks.FieldError
type ValidationError = tasks.ValidationError

var (
	ErrNotFound              = tasks.ErrNotFound
	ErrForbidden             = tasks.ErrForbidden
	ErrStateConflict         = tasks.ErrStateConflict
	ErrValidation            = tasks.ErrValidation
	ErrInsufficientCredits   = tasks.ErrInsufficientCredits
	ErrQuoteUnavailable      = tasks.ErrQuoteUnavailable
	ErrSettlementUnavailable = tasks.ErrSettlementUnavailable
	ErrWalletRestricted      = tasks.ErrWalletRestricted
	ErrMemberQuotaExhausted  = tasks.ErrMemberQuotaExhausted
	ErrMediaNotFound         = tasks.ErrMediaNotFound
	ErrMediaNotReady         = tasks.ErrMediaNotReady
	ErrMediaArchived         = tasks.ErrMediaArchived
	ErrBoardNotFound         = tasks.ErrBoardNotFound
	ErrBoardArchived         = tasks.ErrBoardArchived
	ErrIdempotencyConflict   = tasks.ErrIdempotencyConflict

	BoardPolicy                   = tasks.BoardPolicy
	CanTransition                 = tasks.CanTransition
	ValidateCreate                = tasks.ValidateCreate
	ValidateWorkspacePrompt       = tasks.ValidateWorkspacePrompt
	IsValidMaskAssetID            = tasks.IsValidMaskAssetID
	MaskAssetIDFromParams         = tasks.MaskAssetIDFromParams
	ValidateParams                = tasks.ValidateParams
	ValidateOutputRef             = tasks.ValidateOutputRef
	NewExecutionToken             = tasks.NewExecutionToken
	ShouldRetry                   = tasks.ShouldRetry
	IsTerminalStatus              = tasks.IsTerminalStatus[Status]
	IsValidationError             = tasks.IsValidationError
	BuildTaskCompletedOutputs     = tasks.BuildTaskCompletedOutputs
	ValidateBoardID               = tasks.ValidateBoardID
	AuthorizeTaskRead             = tasks.AuthorizeTaskRead
	AuthorizeTaskCancel           = tasks.AuthorizeTaskCancel
	CheckFundsGate                = tasks.CheckFundsGate
	checkBoardSubmit              = tasks.CheckBoardSubmit
	NormalizeSubmitIdempotencyKey = tasks.NormalizeSubmitIdempotencyKey
	TaskSubmitRequestHash         = tasks.TaskSubmitRequestHash
	TaskSubmitHoldKey             = tasks.TaskSubmitHoldKey
)

const (
	MaxProviderRequeues         = tasks.MaxProviderRequeues
	MaxOutputsPerTask           = tasks.MaxOutputsPerTask
	MaxMediaInputs              = tasks.MaxMediaInputs
	MaxWorkspaceTaskPromptBytes = tasks.MaxWorkspaceTaskPromptBytes
	OutputKindAsset             = tasks.OutputKindAsset
	OutputKindRemote            = tasks.OutputKindRemote
	TaskSubmitOperation         = tasks.TaskSubmitOperation

	BoardOpSubmit = tasks.BoardOpSubmit
	BoardOpRead   = tasks.BoardOpRead
	BoardOpCancel = tasks.BoardOpCancel

	BoardDecisionAllow     = tasks.BoardDecisionAllow
	BoardDecisionNotFound  = tasks.BoardDecisionNotFound
	BoardDecisionArchived  = tasks.BoardDecisionArchived
	BoardDecisionForbidden = tasks.BoardDecisionForbidden
)

type Quoter = taskports.Quoter
type AssetReader = taskports.AssetReader
type BoardAuthorizer = taskports.BoardAuthorizer
type AttemptRecorder = taskports.AttemptRecorder
type TxSubmissionClaimStore = taskports.TxSubmissionClaimStore
type SettlementTransactor = taskports.SettlementTransactor
type TxHolder = taskports.TxHolder
type TxBillingAccountResolver = taskports.TxBillingAccountResolver
type TxMemberQuotaChecker = taskports.TxMemberQuotaChecker
type TxEnqueuer = taskports.TxEnqueuer
type TaskReadStore = taskports.TaskReadStore
type TxTaskLifecycleStore = taskports.TxTaskLifecycleStore
type TaskSettlementStore = taskports.TaskSettlementStore
type TaskLifecycleStore = taskports.TaskLifecycleStore
type TaskClaimStore = taskports.TaskClaimStore
type TaskOutputStore = taskports.TaskOutputStore
type TaskBillingStore = taskports.TaskBillingStore
type RoutingSnapshotSaver = taskports.RoutingSnapshotSaver
type SubmissionAuthorizer = taskports.SubmissionAuthorizer

// Price/routing policy tests exercise the private planner; eligibility tests
// use Readiness.Check, the same interface production callers consume.
func priceQuoteForTest(ctx context.Context, service *Service, params QuoteParams) (BillingSnapshot, error) {
	return NewReadiness(ReadinessDeps{Quoter: service.quoter, Resolver: service.resolver}).priceQuote(ctx, params)
}
