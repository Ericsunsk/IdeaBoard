package httpapi

// Control Plane guard, rate limits, and route registration (issue #808).
//
// The guard is the single choke point every /api/control/v1 route flows
// through: capability check (never IsAdmin) -> reason + Idempotency-Key
// enforcement on mutations -> idempotent execution with response replay
// -> handler -> mandatory audit record (success/read/denied/failure).
// Denied and failed calls are audited too.

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"ideaboard/api/internal/aigateway"
	billinghttp "ideaboard/api/internal/billing/interfaces/http"
	creditsdomain "ideaboard/api/internal/credits/domain"
	credithttp "ideaboard/api/internal/credits/interfaces/http"
	creditsports "ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/identity/users/domain"
	identityports "ideaboard/api/internal/identity/users/ports"
	storagehttp "ideaboard/api/internal/media/storage/interfaces/http"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	"ideaboard/api/internal/platform/adminaction"
	"ideaboard/api/internal/platform/adminbranding"
	"ideaboard/api/internal/platform/audit"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/idempotency"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/prompts"
	rewardhttp "ideaboard/api/internal/rewards/interfaces/http"
	searchhttp "ideaboard/api/internal/searchcatalog/interfaces/http"
	taskapp "ideaboard/api/internal/tasking/application/task"
	tasksdomain "ideaboard/api/internal/tasking/domain"
	statisticshttp "ideaboard/api/internal/tasking/interfaces/http/statistics"
	"ideaboard/api/internal/tenancy"
	tenancypg "ideaboard/api/internal/tenancy/postgres"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// ControlUserService is the narrow user read seam for the Control Plane
// (list/get only; mutations go through IdentityAdminCommands).
type ControlUserService interface {
	ListUsers(ctx context.Context, req *identityports.UserListRequest) (*identityports.UserListResponse, error)
	GetByPublicID(ctx context.Context, publicID string) (*domain.User, error)
}

// ControlCreditCommands is the Control-side subset of the credits.Commands
// seam. Only Credits owns ledger writes; control performs NO direct
// ledger writes and issues no other command kinds.
type ControlCreditCommands interface {
	Grant(ctx context.Context, command creditsports.GrantCommand) (creditsports.CommandResult, error)
	Deduct(ctx context.Context, command creditsports.DeductionCommand) (creditsports.CommandResult, error)
	Correct(ctx context.Context, command creditsports.CorrectionCommand) (creditsports.CommandResult, error)
	Refund(ctx context.Context, command creditsports.RefundCommand) (creditsports.CommandResult, error)
}

// ControlCreditReads is the Credits read side for the Control Plane.
type ControlCreditReads interface {
	GetWallet(ctx context.Context, userID int64) (creditsdomain.WalletDetails, error)
	ListLedger(ctx context.Context, input creditsdomain.ListLedgerInput) (creditsdomain.LedgerPage, error)
}

// ControlCreditDirectory resolves user identity both ways for the
// Control Plane (public id -> internal id for commands, internal id ->
// public identity for read enrichment).
type ControlCreditDirectory interface {
	credithttp.UserPublicIDResolver
	credithttp.UserIDResolver
}

// ControlTaskAdmin is the scoped task read/ops seam for the Control
// Plane: get/list/cancel with operator reason. There is deliberately NO
// task creation path here (no duplicate of the submit surface).
type ControlTaskAdmin interface {
	GetTask(ctx context.Context, taskID string) (*taskapp.AdminMonitorDetail, error)
	ListTasks(ctx context.Context, req *tasksdomain.TaskListRequest) ([]taskapp.AdminMonitorListItem, tasksdomain.TaskListCounts, error)
	ResolveAdminTaskUserID(ctx context.Context, rawPublicID string) (int64, error)
	CancelAdminTask(ctx context.Context, taskID string, adminID int64, reason string) error
}

// ControlBillingAccountStore provisions the implicit per-workspace
// billing account for control reads.
type ControlBillingAccountStore interface {
	EnsureBillingAccountForWorkspace(ctx context.Context, workspaceInternalID int64, workspacePublicID string) (tenancypg.BillingAccount, error)
}

// ContractControlDeps wires the Control Plane handlers. Every field is a
// narrow seam owned by its business context; nil keeps the dependent
// routes fail-closed with 501 (service_unavailable) instead of panicking.
// Proxied legacy handlers (AIGateway, Storage, SearchProviders,
// FeaturePrompts, Monitor) keep their validation and side-effect logic
// single-sourced; the control guard still enforces capability, audit,
// idempotency, and reason uniformly around them.
type ContractControlDeps struct {
	Users           ControlUserService
	Identity        identityports.IdentityAdminCommands
	Impersonator    identityports.AdminUserImpersonator
	Credits         ControlCreditCommands
	CreditUsers     ControlCreditDirectory
	CreditReads     ControlCreditReads
	Reconciliation  credithttp.ReconciliationService
	Tasks           ControlTaskAdmin
	Tenancy         tenancy.Store
	UserDirectory   V1UserDirectory
	BillingAccounts ControlBillingAccountStore
	Catalog         billinghttp.BillableCatalogService
	// ModelPools is the Service Model Pool catalog seam (#849): Admin
	// pool reads and atomic replaces. *catalog.Repository is the
	// production implementation; nil keeps the pool routes fail-closed
	// with 501-style 503 (never a second model catalog).
	ModelPools ControlModelPoolStore
	Payments   paymentcontracts.AdminService
	Settings   *settings.RuntimeSettingsService
	// PaymentWebhookURL is the Stripe webhook endpoint recorded into the
	// provider configuration (composition wires the public API URL).
	PaymentWebhookURL string
	AIGateway         *aigateway.Handler
	Storage           *storagehttp.Handler
	SearchProviders   *searchhttp.Handler
	FeaturePrompts    *prompts.FeaturePromptHandler
	Monitor           *statisticshttp.Handler
	// RewardCommands and RewardReads reuse the Rewards admin handlers
	// under the control guard (single-sourced validation, idempotency,
	// and error mapping; the control-facing render layer translates
	// every response into the unified contract envelope). Nil keeps the
	// reward routes fail-closed with 501.
	RewardCommands *rewardhttp.AdminCommandHandler
	RewardReads    *rewardhttp.AdminReadHandler
	// OAuth is the Auth-owned social-provider seam behind the dedicated
	// control OAuth handlers (contract envelope); nil keeps the OAuth
	// routes fail-closed with 503.
	OAuth ControlOAuthService
	// BrandLogos reuses the platform brand-logo override handler under
	// the control guard (translated into the unified contract envelope);
	// nil keeps the override routes fail-closed with 501.
	BrandLogos *adminbranding.Handler
	// BrandingUpload stores the site branding logo file (multipart)
	// through the Storage service; nil keeps the upload route
	// fail-closed with 503.
	BrandingUpload ControlBrandingUploadService
	Idempotency    *contract.IdempotencyStore
	// IdempotencyDurable is the PostgreSQL authority behind Control
	// mutation idempotency (#891): the guard claims the in-progress
	// fact inside its guard-owned transaction before running the
	// handler, so concurrent same-key claimants elect exactly one
	// winner across processes and restarts, and retries replay the
	// stored 2xx response. The process-local Idempotency store stays a
	// same-process fast-path optimization only. Nil keeps the legacy
	// L1-only path (tests and unwired deployments).
	IdempotencyDurable ControlIdempotencyDurable
	RateLimiter        *ControlRateLimiter
	AuditSink          ControlAuditSink
	// TxScope and TxAudit opt the guarded MUTATIONS into the atomic
	// audit path (step 2): the guard begins one transaction per
	// mutating call, the handler runs inside it, and the audit row
	// joins it before commit, so an audit failure rolls the call back
	// and a business failure still commits its failure audit row (the
	// domain-owned business write already rolled back itself). Both
	// nil keeps the legacy detached path (reads, fail-fast denials,
	// and unwired deployments). Reads never set these: a read performs
	// no business write, so there is nothing for its audit row to be
	// atomic with.
	TxScope ControlTxScope
	TxAudit ControlAuditTxWriter
	// Mail is the Auth-owned transactional mail seam behind the control
	// mail-setting handlers (same instance serving legacy admin mail
	// settings); nil keeps the mail routes fail-closed with 503.
	Mail ControlMailService
}

func (d *ContractControlDeps) idempotencyStore() *contract.IdempotencyStore {
	if d != nil && d.Idempotency != nil {
		return d.Idempotency
	}
	return contract.NewIdempotencyStore(contract.DefaultIdempotencyStoreOptions())
}

func (d *ContractControlDeps) rateLimiter() *ControlRateLimiter {
	if d != nil && d.RateLimiter != nil {
		return d.RateLimiter
	}
	return defaultControlRateLimiter()
}

// ControlRateLimiter is the dedicated Control Plane fixed-window bucket
// (Baseline 2.2: separate from Public limits). Keyed by operator subject;
// production default 120 requests / 60 seconds (mirrors the
// openapi.control.yaml x-rate-limit contract).
type ControlRateLimiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	hits   map[string][]time.Time
}

// NewControlRateLimiter builds a limiter with an explicit budget.
func NewControlRateLimiter(max int, window time.Duration) *ControlRateLimiter {
	if max <= 0 {
		max = 120
	}
	if window <= 0 {
		window = time.Minute
	}
	return &ControlRateLimiter{max: max, window: window, hits: map[string][]time.Time{}}
}

var (
	defaultControlRateLimiterOnce sync.Once
	defaultControlRateLimiterVal  *ControlRateLimiter
)

func defaultControlRateLimiter() *ControlRateLimiter {
	defaultControlRateLimiterOnce.Do(func() {
		defaultControlRateLimiterVal = NewControlRateLimiter(120, time.Minute)
	})
	return defaultControlRateLimiterVal
}

// allow reports whether the key may proceed and the seconds until the
// window resets. Expired entries are pruned on each call.
func (l *ControlRateLimiter) allow(key string, now time.Time) (allowed bool, remaining int, resetSeconds int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := now.Add(-l.window)
	kept := l.hits[key][:0]
	for _, at := range l.hits[key] {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	l.hits[key] = kept
	if len(kept) >= l.max {
		reset := kept[0].Add(l.window).Sub(now)
		if reset < 0 {
			reset = 0
		}
		return false, 0, int64(reset.Seconds()) + 1
	}
	l.hits[key] = append(kept, now)
	if len(l.hits[key]) > 0 {
		reset := l.hits[key][0].Add(l.window).Sub(now)
		if reset < 0 {
			reset = 0
		}
		resetSeconds = int64(reset.Seconds()) + 1
	}
	return true, l.max - len(l.hits[key]), resetSeconds
}

// ControlRateLimitMiddleware enforces the separate control bucket per
// operator and renders RateLimit-* headers (Retry-After on 429).
func ControlRateLimitMiddleware(limiter *ControlRateLimiter) gin.HandlerFunc {
	if limiter == nil {
		limiter = defaultControlRateLimiter()
	}
	return func(c *gin.Context) {
		key := "anonymous"
		if p, ok := principal.From(c); ok {
			_, subjectID := controlOperatorIdentity(p)
			if subjectID != "" {
				key = subjectID
			}
		}
		allowed, remaining, reset := limiter.allow(key, time.Now())
		contract.SetRateLimit(c, limiter.max, remaining, reset)
		if !allowed {
			contract.SetRetryAfterSeconds(c, reset)
			contract.RenderError(c, http.StatusTooManyRequests, contract.CodeRateLimited, msgRateLimited2)
			c.Abort()
			return
		}
		c.Next()
	}
}

// controlOperatorMiddleware resolves the operator grant AFTER session
// auth (v1AuthMiddleware) and tenant scope (ResolvePrincipal): admin-role
// operators pass through; everyone else fails closed with 403 and a denied
// audit. There is no capability grant here by design: production sessions
// mint no control capabilities at login (identity/auth/middleware.go builds
// principals via principal.New with an empty capability set, and
// principal.NewExtended has no non-test callers), so the guard enforces
// admin-only directly and the family capability strings survive only as
// audit labels. Internal workers pass through only with the admin business
// role; service accounts, API keys, and OAuth apps never pass even with an
// admin role.
func controlOperatorMiddleware(sink ControlAuditSink) gin.HandlerFunc {
	return func(c *gin.Context) {
		p, ok := principal.From(c)
		if !ok {
			contract.RenderError(c, http.StatusUnauthorized, contract.CodeUnauthorized, msgUnauthorized)
			c.Abort()
			return
		}
		if !isControlOperator(p) {
			emitControlAudit(c.Request.Context(), sink, deniedControlAudit(c, p, "", "operator.grant", "operator grant"))
			contract.RenderError(c, http.StatusForbidden, contract.CodeForbidden, msgForbidden)
			c.Abort()
			return
		}
		c.Next()
	}
}

func deniedControlAudit(c *gin.Context, p principal.Principal, capability, operation, route string) ControlAuditRecord {
	operatorType, operatorID := controlOperatorIdentity(p)
	return ControlAuditRecord{
		OperatorType: operatorType,
		OperatorID:   operatorID,
		Capability:   capability,
		Operation:    operation,
		Route:        route,
		RequestID:    contract.RequestID(c),
		Outcome:      ControlAuditDenied,
		Status:       http.StatusForbidden,
		OccurredAt:   time.Now().UTC(),
	}
}

// isControlOperator reports whether p may call the Control Plane: the
// admin business role on a human session or an internal worker. Service
// accounts, API keys, and OAuth apps never pass, even with an admin role.
func isControlOperator(p principal.Principal) bool {
	if !p.IsAdmin() {
		return false
	}
	switch p.SubjectType() {
	case principal.SubjectHumanSession, principal.SubjectInternalWorker:
		return true
	default:
		return false
	}
}

// controlHandler is the guarded endpoint implementation.
type controlHandler func(c *gin.Context, ctx controlContext)

// controlContext carries the guard-resolved request facts handlers need.
type controlContext struct {
	Operator principal.Principal
	Reason   string
	IdemKey  string
	Body     []byte
}

// guard wraps a handler with admin-only auth, reason + idempotency
// enforcement (mutations), idempotent execution, and the mandatory
// audit record. It returns a gin handler. The capability argument is the
// audit label only (family strings survive for the audit record and the
// openapi x-capabilities mirror); authorization is the admin business
// role plus the human/worker subject gate, never a family grant.
func (d *ContractControlDeps) guard(capability string, route controlRoute, handler controlHandler) gin.HandlerFunc {
	return func(c *gin.Context) {
		p, ok := principal.From(c)
		if !ok {
			contract.RenderError(c, http.StatusUnauthorized, contract.CodeUnauthorized, msgUnauthorized)
			c.Abort()
			return
		}
		record := ControlAuditRecord{
			Capability: capability,
			Operation:  route.Operation,
			Route:      route.Route,
			RequestID:  contract.RequestID(c),
			OccurredAt: time.Now().UTC(),
		}
		record.OperatorType, record.OperatorID = controlOperatorIdentity(p)
		fail := func(status int, code contract.Code, message string, outcome audit.Outcome) {
			record.Status = status
			record.Outcome = outcome
			emitControlAudit(c.Request.Context(), d.sink(), record)
			contract.RenderError(c, status, code, message)
		}
		if !isControlOperator(p) {
			fail(http.StatusForbidden, contract.CodeForbidden, msgForbidden, ControlAuditDenied)
			c.Abort()
			return
		}
		var body []byte
		if route.Mutation {
			raw, ok := bufferControlBody(c)
			if !ok {
				record.Status = http.StatusBadRequest
				record.Outcome = ControlAuditFailed
				emitControlAudit(c.Request.Context(), d.sink(), record)
				return
			}
			body = raw
			record.Reason = extractControlReason(c, body)
			if _, err := adminaction.New(p.UserID(), record.Reason); err != nil {
				record.Status = http.StatusBadRequest
				record.Outcome = ControlAuditDenied
				emitControlAudit(c.Request.Context(), d.sink(), record)
				contract.RenderError(c, http.StatusBadRequest, contract.CodeControlReasonRequired, msgControlReasonRequired)
				c.Abort()
				return
			}
			key, err := contract.IdempotencyKeyFromHeader(c)
			if err != nil {
				record.Status = http.StatusBadRequest
				record.Outcome = ControlAuditDenied
				emitControlAudit(c.Request.Context(), d.sink(), record)
				contract.RenderError(c, http.StatusBadRequest, contract.CodeIdempotencyKeyRequired, msgIdempotencyKeyRequired)
				c.Abort()
				return
			}
			record.IdempotencyKey = key
		} else if reason := extractControlReason(c, nil); reason != "" {
			if _, reasonErr := adminaction.New(p.UserID(), reason); reasonErr == nil {
				record.Reason = reason
			}
		}
		if route.Target != nil {
			record.TargetType, record.TargetID = route.Target(c, body)
		}
		outcome := ControlAuditRead
		if route.Mutation {
			// runDetached/runAtomic/runIdempotent execute the handler,
			// write the response, and emit the audit record itself
			// (including replay and conflict paths), so the guard
			// returns here in all cases.
			// With TxScope+TxAudit wired the atomic variant runs
			// for joined mutations: one guard-owned transaction
			// carries the call and the audit row commits or rolls
			// back with it (step 2).
			// Detached mutations (external side effects that cannot
			// join a database transaction by construction: Auth
			// mail/storage/Stripe probes and pre-checked config
			// publishes) run on runDetached: the audit row stays an
			// observer, no long guard transaction wraps the external
			// call, and a rollback never implies the external effect
			// was revoked. runDetached keeps the L1 fast path and
			// adds the durable winner election through SHORT
			// independent transactions (claim before the external
			// call, complete/abandon after): successful settlement
			// replays cross-process, known failures abandon, and
			// failed settlement retains uncertainty without re-sending.
			// Only completed 2xx responses are cached.
			if d.atomicAuditEnabled() && !route.Detached {
				d.runAtomic(c, p, route.Operation, record.IdempotencyKey, body, &record, handler, controlContext{
					Operator: p,
					Reason:   record.Reason,
					IdemKey:  record.IdempotencyKey,
					Body:     body,
				})
				return
			}
			if route.Detached && d.detachedDurableEnabled() {
				d.runDetached(c, p, route.Operation, record.IdempotencyKey, body, &record, handler, controlContext{
					Operator: p,
					Reason:   record.Reason,
					IdemKey:  record.IdempotencyKey,
					Body:     body,
				})
				return
			}
			d.runIdempotent(c, p, route.Operation, record.IdempotencyKey, body, &record, handler, controlContext{
				Operator: p,
				Reason:   record.Reason,
				IdemKey:  record.IdempotencyKey,
				Body:     body,
			})
			return
		}
		handler(c, controlContext{Operator: p, Reason: record.Reason, Body: body})
		if reason := controlReasonFromContext(c); reason != "" {
			if _, reasonErr := adminaction.New(p.UserID(), reason); reasonErr == nil {
				record.Reason = reason
			}
		}
		outcome = controlOutcomeFromStatus(c.Writer.Status())
		record.Status = c.Writer.Status()
		record.Outcome = outcome
		emitControlAudit(c.Request.Context(), d.sink(), record)
	}
}

func (d *ContractControlDeps) sink() ControlAuditSink {
	if d == nil {
		return nil
	}
	return d.AuditSink
}

func controlOutcomeFromStatus(status int) audit.Outcome {
	switch {
	case status < 400:
		return ControlAuditRead
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return ControlAuditDenied
	default:
		return ControlAuditFailed
	}
}

type controlStoredResponse struct {
	Status      int               `json:"status"`
	Header      map[string]string `json:"header"`
	Body        string            `json:"body"`
	RequestHash string            `json:"request_hash"`
}

// runIdempotent executes a mutating control handler under the shared
// idempotency seam: same key + same request hash replays the first
// response; same key + different hash is a 409 conflict. Only 2xx
// responses are cached (failures re-execute so a transient error can
// never poison the key); durable uniqueness facts owned by the business
// contexts (e.g. the credits ledger idempotency key) remain the source
// of truth underneath. It always writes the response and emits the
// audit record.
func (d *ContractControlDeps) runIdempotent(c *gin.Context, p principal.Principal, operation, key string, body []byte, record *ControlAuditRecord, handler controlHandler, ctx controlContext) {
	store := d.idempotencyStore()
	_, operatorID := controlOperatorIdentity(p)
	scope := contract.IdempotencyScope(operatorID, "control:"+operation)
	// The request identity pins the real target (see
	// contract_control_idempotency.go); the process-local store stays a
	// same-process fast path only.
	requestHash := controlDurableClaim(p, operation, key, record, c.Request.Method, c.FullPath(), body).RequestHash
	if existing, replayed, err := store.Lookup(scope, p.UserID(), key, requestHash); err != nil {
		record.Status = http.StatusConflict
		record.Outcome = ControlAuditFailed
		emitControlAudit(c.Request.Context(), d.sink(), *record)
		contract.RenderError(c, http.StatusConflict, contract.CodeIdempotencyConflict, msgIdempotencyConflict)
		return
	} else if replayed {
		stored, ok := existing.(controlStoredResponse)
		if !ok {
			record.Status = http.StatusConflict
			record.Outcome = ControlAuditFailed
			emitControlAudit(c.Request.Context(), d.sink(), *record)
			contract.RenderError(c, http.StatusConflict, contract.CodeIdempotencyConflict, msgIdempotencyConflict)
			return
		}
		if contentType := stored.Header["Content-Type"]; contentType != "" {
			c.Header("Content-Type", contentType)
		}
		c.Data(stored.Status, stored.Header["Content-Type"], []byte(stored.Body))
		record.Status = stored.Status
		record.Outcome = ControlAuditOK
		record.Replayed = true
		emitControlAudit(c.Request.Context(), d.sink(), *record)
		return
	}
	original := c.Writer
	capture := &controlCaptureWriter{ResponseWriter: original, status: http.StatusOK}
	c.Writer = capture
	handler(c, ctx)
	c.Writer = original
	if reason := controlReasonFromContext(c); reason != "" {
		if _, reasonErr := adminaction.New(p.UserID(), reason); reasonErr == nil {
			record.Reason = reason
		}
	}
	status := capture.status
	if status == 0 {
		status = http.StatusOK
	}
	if status >= 200 && status < 300 {
		store.StoreResult(scope, p.UserID(), key, requestHash, controlStoredResponse{
			Status:      status,
			Header:      map[string]string{"Content-Type": capture.Header().Get("Content-Type")},
			Body:        capture.body.String(),
			RequestHash: requestHash,
		})
	}
	record.Status = status
	if status >= 400 {
		record.Outcome = ControlAuditFailed
	} else {
		record.Outcome = ControlAuditOK
	}
	emitControlAudit(c.Request.Context(), d.sink(), *record)
}

// detachedDurableEnabled reports whether detached mutations take the
// durable winner-election path (M1). Short independent transactions
// for the claim/outcome need the transaction scope plus the durable
// authority; the participant audit writer is deliberately NOT required
// (detached audit stays an observer). Nil keeps the legacy L1-only
// path (tests and unwired deployments).
func (d *ContractControlDeps) detachedDurableEnabled() bool {
	return d != nil && d.TxScope != nil && d.IdempotencyDurable != nil
}

// The lease applies only to undispatched claims; elapsed time cannot prove
// whether an external effect happened. Dispatch intent disables takeover.
const detachedDurableStaleLease = 60 * time.Second
const detachedDurableSettlementTimeout = 5 * time.Second
const msgControlSettlementUnknown = "The operation outcome is uncertain. Retry with the same Idempotency-Key; the external operation will not be repeated automatically."

// runDetached commits a fenced dispatch intent before external I/O, then
// buffers the response until settlement commits in a separate transaction.
// Unknown outcomes remain unavailable without re-execution. Audit is an
// observer and cannot roll back an external effect. L1 caches completed 2xx.
func (d *ContractControlDeps) runDetached(c *gin.Context, p principal.Principal, operation, key string, body []byte, record *ControlAuditRecord, handler controlHandler, ctx controlContext) {
	store := d.idempotencyStore()
	_, operatorID := controlOperatorIdentity(p)
	scope := contract.IdempotencyScope(operatorID, "control:"+operation)
	durable := controlDurableClaim(p, operation, key, record, c.Request.Method, c.FullPath(), body)
	requestHash := durable.RequestHash
	if existing, replayed, err := store.Lookup(scope, p.UserID(), key, requestHash); err != nil {
		record.Status = http.StatusConflict
		record.Outcome = ControlAuditFailed
		emitControlAudit(c.Request.Context(), d.sink(), *record)
		contract.RenderError(c, http.StatusConflict, contract.CodeIdempotencyConflict, msgIdempotencyConflict)
		return
	} else if replayed {
		stored, ok := existing.(controlStoredResponse)
		if !ok {
			record.Status = http.StatusConflict
			record.Outcome = ControlAuditFailed
			emitControlAudit(c.Request.Context(), d.sink(), *record)
			contract.RenderError(c, http.StatusConflict, contract.CodeIdempotencyConflict, msgIdempotencyConflict)
			return
		}
		if contentType := stored.Header["Content-Type"]; contentType != "" {
			c.Header("Content-Type", contentType)
		}
		c.Data(stored.Status, stored.Header["Content-Type"], []byte(stored.Body))
		record.Status = stored.Status
		record.Outcome = ControlAuditOK
		record.Replayed = true
		emitControlAudit(c.Request.Context(), d.sink(), *record)
		return
	}
	claimCtx, cancelClaim := context.WithTimeout(c.Request.Context(), detachedDurableSettlementTimeout)
	defer cancelClaim()
	claimTxCtx, beginErr := d.TxScope.Begin(claimCtx)
	if beginErr != nil {
		record.Status = http.StatusServiceUnavailable
		record.Outcome = ControlAuditFailed
		emitControlAudit(c.Request.Context(), d.sink(), *record)
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailableAtomic)
		return
	}
	claimResult, claimErr := d.IdempotencyDurable.Claim(claimTxCtx, controlTxOf(claimTxCtx), durable)
	if claimErr != nil {
		d.TxScope.Rollback(claimTxCtx)
		record.Status = http.StatusServiceUnavailable
		record.Outcome = ControlAuditFailed
		emitControlAudit(c.Request.Context(), d.sink(), *record)
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailableAtomic)
		return
	}
	if !claimResult.Winner {
		d.TxScope.Rollback(claimTxCtx)
		existing := claimResult.Existing
		if existing == nil {
			// Winner rolled back between our INSERT and SELECT: key
			// free again, retryable rather than unfenced.
			record.Status = http.StatusServiceUnavailable
			record.Outcome = ControlAuditFailed
			emitControlAudit(c.Request.Context(), d.sink(), *record)
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailableAtomic)
			return
		}
		if existing.RequestHash != durable.RequestHash {
			record.Status = http.StatusConflict
			record.Outcome = ControlAuditFailed
			emitControlAudit(c.Request.Context(), d.sink(), *record)
			contract.RenderError(c, http.StatusConflict, contract.CodeIdempotencyConflict, msgIdempotencyConflict)
			return
		}
		if !existing.Completed {
			// Active winner running: 503 retryable. Stale orphan
			// (crashed before committed dispatch intent) is taken
			// over only before dispatch; uncertain effects are never
			// repeated automatically.
			durable.AttemptToken = existing.AttemptToken
			if !existing.SettlementUnknown {
				if token, taken := d.tryDetachedStaleTakeover(c.Request.Context(), durable); taken {
					durable.AttemptToken = token
					d.executeDetachedWinner(c, p, operation, key, record, handler, ctx, scope, durable, requestHash, store)
					return
				}
			}
			record.Status = http.StatusServiceUnavailable
			record.Outcome = ControlAuditFailed
			emitControlAudit(c.Request.Context(), d.sink(), *record)
			message := msgServiceUnavailableAtomic
			if existing.SettlementUnknown {
				message = msgControlSettlementUnknown
			}
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, message)
			return
		}
		if existing.Expired {
			// Retention expiry reads as a miss but Claim already
			// takes expired over transparently; reaching here means
			// a race (winner completed expiry takeover concurrently).
			// Retryable rather than unfenced.
			record.Status = http.StatusServiceUnavailable
			record.Outcome = ControlAuditFailed
			emitControlAudit(c.Request.Context(), d.sink(), *record)
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailableAtomic)
			return
		}
		stored := controlStoredResponse{
			Status:      existing.Result.Status,
			Header:      map[string]string{"Content-Type": existing.Result.ContentType},
			Body:        existing.Result.Body,
			RequestHash: requestHash,
		}
		store.StoreResult(scope, p.UserID(), key, requestHash, stored)
		if contentType := existing.Result.ContentType; contentType != "" {
			c.Header("Content-Type", contentType)
		}
		c.Data(existing.Result.Status, existing.Result.ContentType, []byte(existing.Result.Body))
		record.Status = existing.Result.Status
		record.Outcome = ControlAuditOK
		record.Replayed = true
		emitControlAudit(c.Request.Context(), d.sink(), *record)
		return
	}
	durable.AttemptToken = claimResult.AttemptToken
	if err := d.IdempotencyDurable.MarkUnknown(claimTxCtx, controlTxOf(claimTxCtx), durable); err != nil {
		d.TxScope.Rollback(claimTxCtx)
		record.Status = http.StatusServiceUnavailable
		record.Outcome = ControlAuditFailed
		emitControlAudit(c.Request.Context(), d.sink(), *record)
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailableAtomic)
		return
	}
	if commitErr := d.TxScope.Commit(claimTxCtx); commitErr != nil {
		record.Status = http.StatusServiceUnavailable
		record.Outcome = ControlAuditFailed
		emitControlAudit(c.Request.Context(), d.sink(), *record)
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailableAtomic)
		return
	}
	d.executeDetachedWinner(c, p, operation, key, record, handler, ctx, scope, durable, requestHash, store)
}

// executeDetachedWinner runs the handler after winning the detached
// claim (initial claim or stale takeover) and records the outcome in a
// second short transaction. The external call runs with NO guard
// transaction held.
func (d *ContractControlDeps) executeDetachedWinner(c *gin.Context, p principal.Principal, operation, key string, record *ControlAuditRecord, handler controlHandler, ctx controlContext, scope string, durable idempotency.ControlClaim, requestHash string, store *contract.IdempotencyStore) {
	original := c.Writer
	capture := &controlAtomicCaptureWriter{ResponseWriter: original, header: make(http.Header)}
	c.Writer = capture
	handler(c, ctx)
	c.Writer = original
	if reason := controlReasonFromContext(c); reason != "" {
		if _, reasonErr := adminaction.New(p.UserID(), reason); reasonErr == nil {
			record.Reason = reason
		}
	}
	status := capture.status
	if status == 0 {
		status = http.StatusOK
	}
	record.Status = status
	record.Outcome = ControlAuditOK
	if status >= 400 {
		record.Outcome = ControlAuditFailed
	}
	// A disconnect must not prevent settlement, but PostgreSQL writes must
	// remain bounded. The committed dispatch intent already prevents takeover
	// even if Begin/Complete/Commit fails or this process exits now.
	outcomeCtx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), detachedDurableSettlementTimeout)
	defer cancel()
	outcomeTxCtx, err := d.TxScope.Begin(outcomeCtx)
	if err == nil {
		if status >= 200 && status < 300 {
			err = d.IdempotencyDurable.Complete(outcomeTxCtx, controlTxOf(outcomeTxCtx), durable, idempotency.ControlResult{
				Status: status, ContentType: capture.header.Get("Content-Type"), Body: capture.body.String(),
			})
		} else {
			err = d.IdempotencyDurable.Abandon(outcomeTxCtx, controlTxOf(outcomeTxCtx), durable)
		}
		if err != nil {
			d.TxScope.Rollback(outcomeTxCtx)
		} else {
			err = d.TxScope.Commit(outcomeTxCtx)
		}
	}
	if err != nil {
		slog.Warn("control detached settlement failed", "operation", operation)
		record.Status = http.StatusServiceUnavailable
		record.Outcome = ControlAuditFailed
		emitControlAudit(outcomeCtx, d.sink(), *record)
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgControlSettlementUnknown)
		return
	}
	if status >= 200 && status < 300 {
		store.StoreResult(scope, p.UserID(), key, requestHash, controlStoredResponse{
			Status: status, Header: map[string]string{"Content-Type": capture.header.Get("Content-Type")},
			Body: capture.body.String(), RequestHash: requestHash,
		})
		if _, err := d.IdempotencyDurable.PurgeExpired(outcomeCtx); err != nil {
			slog.Warn("control durable idempotency purge failed")
		}
	}
	emitControlAudit(outcomeCtx, d.sink(), *record)
	capture.replayTo(original)
}

// tryDetachedStaleTakeover replaces only an undispatched, observed attempt.
// Dispatch intent excludes lease recovery: an external outcome cannot safely
// be inferred from elapsed time. The new attempt is marked before commit.
func (d *ContractControlDeps) tryDetachedStaleTakeover(ctx context.Context, durable idempotency.ControlClaim) (string, bool) {
	if d == nil || d.TxScope == nil || d.IdempotencyDurable == nil {
		return "", false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	takeoverCtx, cancel := context.WithTimeout(ctx, detachedDurableSettlementTimeout)
	defer cancel()
	txCtx, err := d.TxScope.Begin(takeoverCtx)
	if err != nil {
		return "", false
	}
	taken, err := d.IdempotencyDurable.TryAbandonStale(txCtx, controlTxOf(txCtx), durable, time.Now().UTC().Add(-detachedDurableStaleLease))
	if err != nil || !taken {
		d.TxScope.Rollback(txCtx)
		return "", false
	}
	claimed, err := d.IdempotencyDurable.Claim(txCtx, controlTxOf(txCtx), durable)
	if err != nil || !claimed.Winner {
		d.TxScope.Rollback(txCtx)
		return "", false
	}
	durable.AttemptToken = claimed.AttemptToken
	if err := d.IdempotencyDurable.MarkUnknown(txCtx, controlTxOf(txCtx), durable); err != nil {
		d.TxScope.Rollback(txCtx)
		return "", false
	}
	if err := d.TxScope.Commit(txCtx); err != nil {
		return "", false
	}
	return durable.AttemptToken, true
}

// atomicAuditEnabled reports whether the guarded mutations take the
// atomic path (step 2). Both seams are required: a transaction scope
// without a participant writer (or vice versa) is a wiring error, and
// the guard fails safe back to the legacy detached path rather than
// half-joining a transaction.
func (d *ContractControlDeps) atomicAuditEnabled() bool {
	return d != nil && d.TxScope != nil && d.TxAudit != nil
}

// runAtomic executes a mutating control handler inside ONE guard-owned
// transaction that the audit row joins before commit. It mirrors
// runIdempotent's idempotency contract (same key + same hash replays,
// same key + different hash conflicts, only 2xx cached) with two
// deliberate differences:
//
//   - The handler response is BUFFERED (controlAtomicCaptureWriter),
//     never streamed: the bytes reach the wire only after the
//     transaction commits, so an audit failure can still fail the
//     request instead of following an already-sent 200.
//   - The audit row is a co-committer, not an observer. Audit failure
//     (or commit failure) rolls the transaction back and renders 500:
//     the business call fails BECAUSE its audit trail did. This is the
//     step-2 semantic inversion versus the step-1 warn-and-continue
//     sink, and it is what "atomic" means here.
//
// A handler (business) failure does NOT roll the audit back: the
// domain-owned business write already rolled back in its own
// transaction, so the only durable write left is the failure audit
// row, which commits so the failed call stays on the trail. A replayed
// (idempotent) call commits its replay audit row the same way.
//
// STEP 3 (carrier): the domain ports below now accept the guard-owned
// transaction from the context carrier (dbutil.AmbientTxFromContext,
// bridged here as ControlTxFromContext) and join it instead of opening
// their own: credits commands and wallet restrictions, identity
// provisioning and lifecycle, workspace task settlement, payments
// refund/dispute writes, settings config writes, and tenancy mutations.
// A handler that joins shares one commit with the audit row, so the
// crash window between the domain commit and this commit is closed for
// every joined port. Ports that still commit outside the carrier are the
// documented remainder: single-statement pool writes (each self-atomic,
// e.g. refund provider-result markers, deletion phase reads), external
// side effects that cannot join a database
// transaction by construction (Auth service calls, storage cleanup,
// Stripe I/O, River/NATS/event publication), and the multi-phase
// deletion saga steps between external calls. The seam stays ready: any
// future tx-aware seam reads ControlTxFromContext and joins with no
// guard change.
func (d *ContractControlDeps) runAtomic(c *gin.Context, p principal.Principal, operation, key string, body []byte, record *ControlAuditRecord, handler controlHandler, ctx controlContext) {
	store := d.idempotencyStore()
	_, operatorID := controlOperatorIdentity(p)
	scope := contract.IdempotencyScope(operatorID, "control:"+operation)
	// Durable scope (operator/operation/target/key) plus the canonical
	// request hash, computed once and shared by the L1 fast path and
	// the durable claim below.
	durable := controlDurableClaim(p, operation, key, record, c.Request.Method, c.FullPath(), body)
	requestHash := durable.RequestHash
	failTx := func(txCtx context.Context, status int, code contract.Code, message string, outcome audit.Outcome) {
		record.Status = status
		record.Outcome = outcome
		d.TxScope.Rollback(txCtx)
		// The guard transaction is gone, so the failure audit must use the
		// detached sink. Keeping it outside the rolled-back mutation
		// transaction preserves the failure trail without reviving a
		// business write that already failed.
		d.emitAtomicFailureAudit(c.Request.Context(), record)
		contract.RenderError(c, status, code, message)
	}
	if existing, replayed, err := store.Lookup(scope, p.UserID(), key, requestHash); err != nil {
		txCtx, beginErr := d.TxScope.Begin(c.Request.Context())
		if beginErr != nil {
			record.Status = http.StatusConflict
			record.Outcome = ControlAuditFailed
			d.emitAtomicFailureAudit(c.Request.Context(), record)
			contract.RenderError(c, http.StatusConflict, contract.CodeIdempotencyConflict, msgIdempotencyConflict)
			return
		}
		failTx(txCtx, http.StatusConflict, contract.CodeIdempotencyConflict, msgIdempotencyConflict, ControlAuditFailed)
		return
	} else if replayed {
		stored, ok := existing.(controlStoredResponse)
		if !ok {
			txCtx, beginErr := d.TxScope.Begin(c.Request.Context())
			if beginErr != nil {
				record.Status = http.StatusConflict
				record.Outcome = ControlAuditFailed
				d.emitAtomicFailureAudit(c.Request.Context(), record)
				contract.RenderError(c, http.StatusConflict, contract.CodeIdempotencyConflict, msgIdempotencyConflict)
				return
			}
			failTx(txCtx, http.StatusConflict, contract.CodeIdempotencyConflict, msgIdempotencyConflict, ControlAuditFailed)
			return
		}
		txCtx, beginErr := d.TxScope.Begin(c.Request.Context())
		if beginErr != nil {
			record.Status = http.StatusServiceUnavailable
			record.Outcome = ControlAuditFailed
			record.Replayed = true
			d.emitAtomicFailureAudit(c.Request.Context(), record)
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailableAtomic)
			return
		}
		record.Status = stored.Status
		record.Outcome = ControlAuditOK
		record.Replayed = true
		if _, auditErr := d.TxAudit.AppendControlAuditTx(txCtx, controlTxOf(txCtx), *record); auditErr != nil {
			record.Status = http.StatusServiceUnavailable
			record.Outcome = ControlAuditFailed
			d.TxScope.Rollback(txCtx)
			d.emitAtomicFailureAudit(c.Request.Context(), record)
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailableAtomic)
			return
		}
		if commitErr := d.TxScope.Commit(txCtx); commitErr != nil {
			record.Status = http.StatusServiceUnavailable
			record.Outcome = ControlAuditFailed
			d.emitAtomicFailureAudit(c.Request.Context(), record)
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailableAtomic)
			return
		}
		emitControlAudit(txCtx, nil, *record)
		if contentType := stored.Header["Content-Type"]; contentType != "" {
			c.Header("Content-Type", contentType)
		}
		c.Data(stored.Status, stored.Header["Content-Type"], []byte(stored.Body))
		return
	}
	txCtx, beginErr := d.TxScope.Begin(c.Request.Context())
	if beginErr != nil {
		record.Status = http.StatusServiceUnavailable
		record.Outcome = ControlAuditFailed
		d.emitAtomicFailureAudit(c.Request.Context(), record)
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailableAtomic)
		return
	}
	// Post-commit announcements (#889): participants on the borrowed
	// transaction defer publication to hooks run after the commit below,
	// so an audit or commit failure drops them instead of announcing a
	// rolled-back fact. Replay/conflict paths above return before any
	// handler runs, so they never register hooks.
	txCtx = dbutil.ContextWithAmbientPostCommit(txCtx)
	// Durable single-winner election: the in-progress fact lands in the
	// guard transaction before the handler runs, so a concurrent
	// same-key claimant blocks on the primary key until this
	// transaction commits or rolls back. Nil keeps the legacy L1-only
	// path (tests and unwired deployments).
	durableWinner := false
	if d.IdempotencyDurable != nil {
		claimResult, claimErr := d.IdempotencyDurable.Claim(txCtx, controlTxOf(txCtx), durable)
		if claimErr != nil {
			failTx(txCtx, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailableAtomic, ControlAuditFailed)
			return
		}
		if !claimResult.Winner {
			durableReplay := claimResult.Existing
			if durableReplay == nil || durableReplay.RequestHash != durable.RequestHash {
				failTx(txCtx, http.StatusConflict, contract.CodeIdempotencyConflict, msgIdempotencyConflict, ControlAuditFailed)
				return
			}
			if !durableReplay.Completed {
				// Unreachable in production: in-progress facts only
				// become visible after the winner commits, and winners
				// flip to completed (or delete the claim) before commit.
				// Fail retryable rather than execute unfenced.
				failTx(txCtx, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailableAtomic, ControlAuditFailed)
				return
			}
			record.Status = durableReplay.Result.Status
			record.Outcome = ControlAuditOK
			record.Replayed = true
			if _, auditErr := d.TxAudit.AppendControlAuditTx(txCtx, controlTxOf(txCtx), *record); auditErr != nil {
				d.TxScope.Rollback(txCtx)
				d.emitAtomicFailureAudit(c.Request.Context(), record)
				contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailableAtomic)
				return
			}
			if commitErr := d.TxScope.Commit(txCtx); commitErr != nil {
				d.emitAtomicFailureAudit(c.Request.Context(), record)
				contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailableAtomic)
				return
			}
			emitControlAudit(txCtx, nil, *record)
			store.StoreResult(scope, p.UserID(), key, requestHash, controlStoredResponse{
				Status:      durableReplay.Result.Status,
				Header:      map[string]string{"Content-Type": durableReplay.Result.ContentType},
				Body:        durableReplay.Result.Body,
				RequestHash: requestHash,
			})
			if contentType := durableReplay.Result.ContentType; contentType != "" {
				c.Header("Content-Type", contentType)
			}
			c.Data(durableReplay.Result.Status, durableReplay.Result.ContentType, []byte(durableReplay.Result.Body))
			return
		}
		durable.AttemptToken = claimResult.AttemptToken
		durableWinner = true
	}
	original := c.Writer
	capture := &controlAtomicCaptureWriter{ResponseWriter: original, header: make(http.Header)}
	c.Writer = capture
	c.Request = c.Request.WithContext(txCtx)
	handler(c, ctx)
	c.Writer = original
	if reason := controlReasonFromContext(c); reason != "" {
		if _, reasonErr := adminaction.New(p.UserID(), reason); reasonErr == nil {
			record.Reason = reason
		}
	}
	status := capture.status
	if status == 0 {
		status = http.StatusOK
	}
	record.Status = status
	if status >= 400 {
		record.Outcome = ControlAuditFailed
	} else {
		record.Outcome = ControlAuditOK
	}
	// The handler already decided the business outcome; the audit row
	// now joins the same transaction. durable=false keeps the slog
	// trail only (service subjects, step-1 skip parity) and still
	// commits; err rolls everything back and fails the request.
	if _, auditErr := d.TxAudit.AppendControlAuditTx(txCtx, controlTxOf(txCtx), *record); auditErr != nil {
		d.TxScope.Rollback(txCtx)
		d.emitAtomicFailureAudit(c.Request.Context(), record)
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailableAtomic)
		return
	}
	// The dedup fact joins the same transaction: a 2xx flips the claim
	// to completed with the response (retained for replay), while any
	// other outcome abandons the claim so the retry re-executes instead
	// of replaying a failure.
	if durableWinner {
		if status >= 200 && status < 300 {
			if err := d.IdempotencyDurable.Complete(txCtx, controlTxOf(txCtx), durable, idempotency.ControlResult{
				Status:      status,
				ContentType: capture.header.Get("Content-Type"),
				Body:        capture.body.String(),
			}); err != nil {
				d.TxScope.Rollback(txCtx)
				d.emitAtomicFailureAudit(c.Request.Context(), record)
				contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailableAtomic)
				return
			}
		} else if err := d.IdempotencyDurable.Abandon(txCtx, controlTxOf(txCtx), durable); err != nil {
			d.TxScope.Rollback(txCtx)
			d.emitAtomicFailureAudit(c.Request.Context(), record)
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailableAtomic)
			return
		}
	}
	if commitErr := d.TxScope.Commit(txCtx); commitErr != nil {
		d.emitAtomicFailureAudit(c.Request.Context(), record)
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailableAtomic)
		return
	}
	// #889: the guard-owned transaction (business plus audit) committed:
	// run deferred post-commit announcements now. Every failure path
	// above returns before this line, so audit/commit failures drop
	// their hooks with txCtx and publish nothing.
	dbutil.RunAmbientPostCommit(txCtx)
	emitControlAudit(txCtx, nil, *record)
	if durableWinner && status >= 200 && status < 300 {
		// Best-effort retention hygiene in autocommit, outside the
		// committed guard transaction: a failure never fails the call.
		if _, err := d.IdempotencyDurable.PurgeExpired(c.Request.Context()); err != nil {
			slog.Warn("control durable idempotency purge failed", "err", err)
		}
	}
	if status >= 200 && status < 300 {
		store.StoreResult(scope, p.UserID(), key, requestHash, controlStoredResponse{
			Status:      status,
			Header:      map[string]string{"Content-Type": capture.header.Get("Content-Type")},
			Body:        capture.body.String(),
			RequestHash: requestHash,
		})
	}
	capture.replayTo(original)
}

// emitAtomicFailureAudit records a failure after the guard transaction has
// rolled back. The normal ControlAuditSink is intentionally used here: the
// production durable sink owns its own detached transaction, while tests can
// observe that the failure row is emitted outside the rolled-back participant
// transaction. WithoutCancel keeps the audit trail durable when the client
// disconnects while the failed request is being rendered.
func (d *ContractControlDeps) emitAtomicFailureAudit(ctx context.Context, record *ControlAuditRecord) {
	if d == nil || record == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	emitControlAudit(context.WithoutCancel(ctx), d.sink(), *record)
}

// controlTxOf extracts the guard-owned transaction for the participant
// writer. A missing transaction (miswired scope) yields nil, which the
// participant rejects as a wiring error.
func controlTxOf(txCtx context.Context) pgx.Tx {
	extracted, _ := ControlTxFromContext(txCtx)
	return extracted
}
