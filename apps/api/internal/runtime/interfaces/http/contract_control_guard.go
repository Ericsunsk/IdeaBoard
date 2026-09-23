package httpapi

// Control Plane guard, rate limits, and route registration (issue #808).
//
// The guard is the single choke point every /api/control/v1 route flows
// through: capability check (never IsAdmin) -> reason + Idempotency-Key
// enforcement on mutations -> idempotent execution with response replay
// -> handler -> mandatory audit record (success/read/denied/failed).
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
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/prompts"
	searchhttp "ideaboard/api/internal/searchcatalog/interfaces/http"
	taskapp "ideaboard/api/internal/tasking/application/task"
	tasksdomain "ideaboard/api/internal/tasking/domain"
	statisticshttp "ideaboard/api/internal/tasking/interfaces/http/statistics"
	"ideaboard/api/internal/tenancy"
	tenancypg "ideaboard/api/internal/tenancy/postgres"

	"github.com/gin-gonic/gin"
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
	CancelAdminTask(ctx context.Context, taskID string, adminID int64) error
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
	Payments        paymentcontracts.AdminService
	Settings        *settings.RuntimeSettingsService
	// PaymentWebhookURL is the Stripe webhook endpoint recorded into the
	// provider configuration (composition wires the public API URL).
	PaymentWebhookURL string
	AIGateway         *aigateway.Handler
	Storage           *storagehttp.Handler
	SearchProviders   *searchhttp.Handler
	FeaturePrompts    *prompts.FeaturePromptHandler
	Monitor           *statisticshttp.Handler
	Idempotency       *contract.IdempotencyStore
	RateLimiter       *ControlRateLimiter
	AuditSink         ControlAuditSink
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
			contract.RenderError(c, http.StatusTooManyRequests, contract.CodeRateLimited, "操作过于频繁，请稍后重试")
			c.Abort()
			return
		}
		c.Next()
	}
}

// hasAnyControlCapability reports whether p already holds a control
// capability (family grant or wildcard), so the operator middleware can
// preserve least-privilege grants instead of widening them.
func hasAnyControlCapability(p principal.Principal) bool {
	if p.HasCapability(principal.ControlWildcardStar) {
		return true
	}
	for _, capability := range principal.ControlCapabilities() {
		if p.HasCapability(capability) {
			return true
		}
	}
	return false
}

// controlOperatorMiddleware resolves the operator grant AFTER session
// auth (v1AuthMiddleware) and tenant scope (ResolvePrincipal): admin-role
// human sessions receive the explicit control capability grant;
// internal workers pass through with their construction-time
// capabilities; everyone else fails closed with 403 and a denied audit.
func controlOperatorMiddleware(sink ControlAuditSink) gin.HandlerFunc {
	return func(c *gin.Context) {
		p, ok := principal.From(c)
		if !ok {
			contract.RenderError(c, http.StatusUnauthorized, contract.CodeUnauthorized, "未登录")
			c.Abort()
			return
		}
		switch p.SubjectType() {
		case principal.SubjectInternalWorker:
			c.Next()
			return
		case principal.SubjectHumanSession:
			if hasAnyControlCapability(p) {
				// Least-privilege operators minted with explicit family
				// capabilities keep them as-is; the role grant only fills
				// the gap for operators holding no control capability yet
				// (the production session path mints none).
				c.Next()
				return
			}
			granted, err := principal.WithControlGrant(p)
			if err != nil {
				emitControlAudit(c.Request.Context(), sink, deniedControlAudit(c, p, "", "operator.grant", "operator grant"))
				contract.RenderError(c, http.StatusForbidden, contract.CodeForbidden, "权限不足")
				c.Abort()
				return
			}
			if err := principal.Set(c, granted); err != nil {
				contract.RenderError(c, http.StatusForbidden, contract.CodeForbidden, "权限不足")
				c.Abort()
				return
			}
			c.Next()
			return
		default:
			emitControlAudit(c.Request.Context(), sink, deniedControlAudit(c, p, "", "operator.grant", "operator grant"))
			contract.RenderError(c, http.StatusForbidden, contract.CodeForbidden, "权限不足")
			c.Abort()
			return
		}
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

// controlHandler is the guarded endpoint implementation.
type controlHandler func(c *gin.Context, ctx controlContext)

// controlContext carries the guard-resolved request facts handlers need.
type controlContext struct {
	Operator principal.Principal
	Reason   string
	IdemKey  string
	Body     []byte
}

// guard wraps a handler with capability auth, reason + idempotency
// enforcement (mutations), idempotent execution, and the mandatory
// audit record. It returns a gin handler.
func (d *ContractControlDeps) guard(capability string, route controlRoute, handler controlHandler) gin.HandlerFunc {
	return func(c *gin.Context) {
		p, ok := principal.From(c)
		if !ok {
			contract.RenderError(c, http.StatusUnauthorized, contract.CodeUnauthorized, "未登录")
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
		fail := func(status int, code contract.Code, message, outcome string) {
			record.Status = status
			record.Outcome = outcome
			emitControlAudit(c.Request.Context(), d.sink(), record)
			contract.RenderError(c, status, code, message)
		}
		if !principal.HasControlCapability(p, capability) {
			fail(http.StatusForbidden, contract.CodeForbidden, "权限不足", ControlAuditDenied)
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
			if !validControlReason(record.Reason) {
				record.Status = http.StatusBadRequest
				record.Outcome = ControlAuditDenied
				emitControlAudit(c.Request.Context(), d.sink(), record)
				contract.RenderError(c, http.StatusBadRequest, contract.CodeControlReasonRequired, "需要填写操作原因")
				c.Abort()
				return
			}
			key, err := contract.IdempotencyKeyFromHeader(c)
			if err != nil {
				record.Status = http.StatusBadRequest
				record.Outcome = ControlAuditDenied
				emitControlAudit(c.Request.Context(), d.sink(), record)
				contract.RenderError(c, http.StatusBadRequest, contract.CodeIdempotencyKeyRequired, "缺少 Idempotency-Key")
				c.Abort()
				return
			}
			record.IdempotencyKey = key
		} else if reason := extractControlReason(c, nil); validControlReason(reason) {
			record.Reason = reason
		}
		if route.Target != nil {
			record.TargetType, record.TargetID = route.Target(c, body)
		}
		outcome := ControlAuditRead
		if route.Mutation {
			// runIdempotent executes the handler, writes the response,
			// and emits the audit record itself (including replay and
			// conflict paths), so the guard returns here in all cases.
			d.runIdempotent(c, p, route.Operation, record.IdempotencyKey, body, &record, handler, controlContext{
				Operator: p,
				Reason:   record.Reason,
				IdemKey:  record.IdempotencyKey,
				Body:     body,
			})
			return
		}
		handler(c, controlContext{Operator: p, Reason: record.Reason, Body: body})
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

func controlOutcomeFromStatus(status int) string {
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
		contract.RenderError(c, http.StatusConflict, contract.CodeIdempotencyConflict, "Idempotency-Key 已用于不同的请求")
		return
	} else if replayed {
		stored, ok := existing.(controlStoredResponse)
		if !ok {
			record.Status = http.StatusConflict
			record.Outcome = ControlAuditFailed
			emitControlAudit(c.Request.Context(), d.sink(), *record)
			contract.RenderError(c, http.StatusConflict, contract.CodeIdempotencyConflict, "Idempotency-Key 已用于不同的请求")
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
