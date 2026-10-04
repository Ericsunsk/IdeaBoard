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
	"net/http"
	"sync"
	"time"

	"ideaboard/api/internal/aigateway"
	billinghttp "ideaboard/api/internal/billing/interfaces/http"
	creditsdomain "ideaboard/api/internal/credits/domain"
	credithttp "ideaboard/api/internal/credits/interfaces/http"
	creditsports "ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/identity/users/domain"
	identityports "ideaboard/api/internal/identity/users/ports"
	storagehttp "ideaboard/api/internal/media/storage/interfaces/http"
	paymentcontracts "ideaboard/api/internal/payments/application/contracts"
	"ideaboard/api/internal/platform/adminaction"
	"ideaboard/api/internal/platform/adminbranding"
	"ideaboard/api/internal/platform/audit"
	"ideaboard/api/internal/platform/contract"
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
	RateLimiter    *ControlRateLimiter
	AuditSink      ControlAuditSink
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
			// runIdempotent executes the handler, writes the response,
			// and emits the audit record itself (including replay and
			// conflict paths), so the guard returns here in all cases.
			// With TxScope+TxAudit wired the atomic variant runs
			// instead: one guard-owned transaction carries the call and
			// the audit row commits or rolls back with it (step 2).
			if d.atomicAuditEnabled() {
				d.runAtomic(c, p, route.Operation, record.IdempotencyKey, body, &record, handler, controlContext{
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
	requestHash := contract.HashIdempotencyRequest(c.Request.Method, c.FullPath(), string(body))
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
	requestHash := contract.HashIdempotencyRequest(c.Request.Method, c.FullPath(), string(body))
	failTx := func(txCtx context.Context, status int, code contract.Code, message string, outcome audit.Outcome) {
		record.Status = status
		record.Outcome = outcome
		d.appendAtomicAudit(txCtx, record)
		d.TxScope.Rollback(txCtx)
		contract.RenderError(c, status, code, message)
	}
	if existing, replayed, err := store.Lookup(scope, p.UserID(), key, requestHash); err != nil {
		txCtx, beginErr := d.TxScope.Begin(c.Request.Context())
		if beginErr != nil {
			record.Status = http.StatusConflict
			record.Outcome = ControlAuditFailed
			emitControlAudit(c.Request.Context(), nil, *record)
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
				emitControlAudit(c.Request.Context(), nil, *record)
				contract.RenderError(c, http.StatusConflict, contract.CodeIdempotencyConflict, msgIdempotencyConflict)
				return
			}
			failTx(txCtx, http.StatusConflict, contract.CodeIdempotencyConflict, msgIdempotencyConflict, ControlAuditFailed)
			return
		}
		txCtx, beginErr := d.TxScope.Begin(c.Request.Context())
		if beginErr != nil {
			record.Status = stored.Status
			record.Outcome = ControlAuditOK
			record.Replayed = true
			emitControlAudit(c.Request.Context(), nil, *record)
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailableAtomic)
			return
		}
		record.Status = stored.Status
		record.Outcome = ControlAuditOK
		record.Replayed = true
		if _, auditErr := d.TxAudit.AppendControlAuditTx(txCtx, controlTxOf(txCtx), *record); auditErr != nil {
			emitControlAudit(txCtx, nil, *record)
			d.TxScope.Rollback(txCtx)
			contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailableAtomic)
			return
		}
		if commitErr := d.TxScope.Commit(txCtx); commitErr != nil {
			emitControlAudit(txCtx, nil, *record)
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
		emitControlAudit(c.Request.Context(), nil, *record)
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailableAtomic)
		return
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
		emitControlAudit(txCtx, nil, *record)
		d.TxScope.Rollback(txCtx)
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailableAtomic)
		return
	}
	if commitErr := d.TxScope.Commit(txCtx); commitErr != nil {
		emitControlAudit(txCtx, nil, *record)
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, msgServiceUnavailableAtomic)
		return
	}
	emitControlAudit(txCtx, nil, *record)
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

// appendAtomicAudit best-effort joins a failure-path audit row into
// txCtx before the guard rolls back: the row is discarded with the
// rollback, but the attempt keeps the slog trail complete for the
// failed call.
func (d *ContractControlDeps) appendAtomicAudit(txCtx context.Context, record *ControlAuditRecord) {
	if d == nil || d.TxAudit == nil || record == nil {
		return
	}
	_, _ = d.TxAudit.AppendControlAuditTx(txCtx, controlTxOf(txCtx), *record)
	emitControlAudit(txCtx, nil, *record)
}

// controlTxOf extracts the guard-owned transaction for the participant
// writer. A missing transaction (miswired scope) yields nil, which the
// participant rejects as a wiring error.
func controlTxOf(txCtx context.Context) pgx.Tx {
	extracted, _ := ControlTxFromContext(txCtx)
	return extracted
}
