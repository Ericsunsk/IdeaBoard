package v1

import (
	"context"
	"net/http"
	"time"

	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/events"
	eventnats "ideaboard/api/internal/events/nats"
	eventoutbox "ideaboard/api/internal/events/outbox"
	eventwebhook "ideaboard/api/internal/events/webhook"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/notifications"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/platform/settings"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/runtime/interfaces/http/v1/shared"
	v1boards "ideaboard/api/internal/runtime/interfaces/http/v1/boards"
	v1library "ideaboard/api/internal/runtime/interfaces/http/v1/library"
	v1media "ideaboard/api/internal/runtime/interfaces/http/v1/media"
	v1system "ideaboard/api/internal/runtime/interfaces/http/v1/system"
	v1tasks "ideaboard/api/internal/runtime/interfaces/http/v1/tasks"
	v1tenancy "ideaboard/api/internal/runtime/interfaces/http/v1/tenancy"
	"ideaboard/api/internal/media/workspacemedia"
	searchhttp "ideaboard/api/internal/searchcatalog/interfaces/http"
	taskdomain "ideaboard/api/internal/tasking/domain"
	"ideaboard/api/internal/tenancy"
	"ideaboard/api/internal/tenancy/invitations"

	"github.com/gin-gonic/gin"
)

// ContractV1TaskDeps wires workspace task operations, quoting, settlement, and lifecycle.
type ContractV1TaskDeps = v1tasks.ContractV1TaskDeps
type WorkspaceTaskReadStore = v1tasks.WorkspaceTaskReadStore
type WorkspaceTaskLifecycleStore = v1tasks.WorkspaceTaskLifecycleStore
type WorkspaceTaskBillingStore = v1tasks.WorkspaceTaskBillingStore
type WorkspaceTaskQuoter = v1tasks.WorkspaceTaskQuoter
type WorkspaceTaskHolder = v1tasks.WorkspaceTaskHolder
type WorkspaceTaskEnqueuer = v1tasks.WorkspaceTaskEnqueuer
type WorkspaceTaskAssetReader = v1tasks.WorkspaceTaskAssetReader
type WorkspaceTaskSnapshotSaver = v1tasks.WorkspaceTaskSnapshotSaver
type taskQuoteBillingAccountReader = v1tasks.QuoteBillingAccountReader
type taskQuoteWalletReader = v1tasks.QuoteWalletReader
type taskQuoteQuotaReader = v1tasks.QuoteQuotaReader

// ContractV1LibraryDeps wires workspace library store, idempotency, read-through, and sources.
type ContractV1LibraryDeps = v1library.ContractV1LibraryDeps

// ContractV1SearchDeps wires workspace search and vision query operations.
type ContractV1SearchDeps = v1system.ContractV1SearchDeps
type V1BillingQuoter = v1system.V1BillingQuoter
type V1CreditSettler = v1system.V1CreditSettler
type V1VisionExtractor = v1system.V1VisionExtractor
type V1VisionExtractInput = v1system.V1VisionExtractInput
const V1VisionSKU = v1system.V1VisionSKU
var errV1CursorInvalid = v1system.ErrV1CursorInvalid
var errV1BillingRestricted = v1system.ErrV1BillingRestricted
var errV1BillingQuoteUnavailable = v1system.ErrV1BillingQuoteUnavailable
var publicSupportChannelsV1Handler = v1system.PublicSupportChannelsV1Handler
var publicBrandingSettingsV1Handler = v1system.PublicBrandingSettingsV1Handler
var publicSearchV1Handler = v1system.PublicSearchV1Handler
var NewBoardDocumentPublisher = v1system.NewBoardDocumentPublisher

type v1NotificationDTO = v1system.V1NotificationDTO
type v1NotificationListDTO = v1system.V1NotificationListDTO
type v1UnreadCountDTO = v1system.V1UnreadCountDTO
type v1MarkAllReadDTO = v1system.V1MarkAllReadDTO

var v1VisionBillingDeferredObserve func(sku, reason, operationID string)

func init() {
	defaultObserver := v1system.V1VisionBillingDeferredObserve
	v1system.V1VisionBillingDeferredObserve = func(sku, reason, operationID string) {
		if v1VisionBillingDeferredObserve != nil {
			v1VisionBillingDeferredObserve(sku, reason, operationID)
			return
		}
		defaultObserver(sku, reason, operationID)
	}
}

// ContractV1MediaDeps wires workspace media storage, upload sessions, and image delivery.
type ContractV1MediaDeps = v1media.ContractV1MediaDeps
type WorkspaceMediaStore = workspacemedia.WorkspaceMediaStore
type WorkspaceMediaObjects = workspacemedia.MediaObjects

// ContractV1AvatarDeps wires avatar storage UID resolution and avatar object persistence.
type ContractV1AvatarDeps = v1media.ContractV1AvatarDeps
type V1AvatarUIDDirectory = v1media.V1AvatarUIDDirectory
type V1AvatarObjects = v1media.V1AvatarObjects
var NewV1AvatarDirectory = v1media.NewV1AvatarDirectory
var toMediaAssetDTO = v1media.ToMediaAssetDTO
var v1SniffedImageMIME = v1media.SniffedImageMIME
var v1StagedChecksum = v1media.StagedChecksum
var errV1AvatarUIDUnavailable = v1media.ErrV1AvatarUIDUnavailable
const v1AvatarMaxBytes = v1media.AvatarMaxBytes
const maxV1MediaBatchAssets = v1media.MaxMediaBatchAssets

// ContractV1MemberDeps wires workspace-member display facts and invite lookups.
type ContractV1MemberDeps = v1tenancy.MemberDeps
type V1MemberLookupProvider = v1tenancy.V1MemberLookupProvider
type V1MemberDisplayProvider = shared.MemberDisplayProvider
type V1MemberDisplay = shared.MemberDisplay
var batchDisplayFacts = shared.BatchDisplayFacts
var NewMemberLookupService = v1tenancy.NewMemberLookupService
var NewMemberDisplayService = v1tenancy.NewMemberDisplayService
var NewInvitationReader = v1tenancy.NewInvitationReader
var NewInvitationIdentityReader = v1tenancy.NewInvitationIdentityReader

type v1UserDTO = v1tenancy.UserDTO
type v1MemberDTO = v1tenancy.MemberDTO
type v1OrganizationDTO = v1tenancy.OrganizationDTO
type v1OrganizationMemberDTO = v1tenancy.OrganizationMemberDTO
type v1WorkspaceMemberInviteDTO = v1tenancy.WorkspaceMemberInviteDTO
type v1OrganizationMemberInviteDTO = v1tenancy.OrganizationMemberInviteDTO
type v1WorkspaceInvitePreviewDTO = v1tenancy.WorkspaceInvitePreviewDTO
type v1WorkspaceInviteAckDTO = v1tenancy.WorkspaceInviteAckDTO
type v1OrganizationInviteAckDTO = v1tenancy.OrganizationInviteAckDTO
type v1DeletedDTO = shared.DeletedDTO

// ContractV1Deps wires the Public Data Plane v1 handlers (issue #801).
//
// Store is the tenancy persistence seam: production uses PostgreSQL, and
// tests pass tenancy.NewInMemoryStoreForTests(). The in-memory fallback
// must never be wired in production.
type ContractV1Deps struct {
	Store       tenancy.Store
	Users       runtimehttp.V1UserDirectory
	Idempotency *contract.IdempotencyStore
	// Boards is the workspace-rooted board collaboration store (issue
	// #802). Nil keeps the boards surface fail-closed with 501 until the
	// postgres workspace-board mapping lands (#809); tests inject the
	// in-memory implementation. Role facts leave this store only through
	// the narrow boards.RoleReader port (#934: h.boardRoles()); Media /
	// Tasking / Events must not copy the role mapping or reach past it
	// into Boards storage.
	Boards boards.WorkspaceBoardStore
	// Library is the workspace-scoped library + materialization surface
	// (issue #806). Zero value keeps the surface fail-closed (501/503);
	// tests inject the in-memory store plus fakes.
	Library ContractV1LibraryDeps
	// Search is the workspace-scoped search + vision-query surface
	// (issue #806). Zero value keeps the surface fail-closed (501/503);
	// tests inject fakes.
	Search ContractV1SearchDeps
	// Tasks is the workspace-scoped async job surface (issue #805).
	// Production wires the postgres store and settlement adapters; tests
	// inject the in-memory implementation plus quoter/holder/queue/asset fakes.
	Tasks ContractV1TaskDeps
	// Media is the workspace-scoped upload-session + media-asset
	// surface (issue #803, restored in #820). Production wires the
	// workspace postgres store; tests inject the in-memory implementation.
	Media ContractV1MediaDeps
	// Avatar resolves caller storage UIDs and persists avatar bytes.
	Avatar ContractV1AvatarDeps
	// Members resolves workspace-member display facts and registered user lookups.
	Members ContractV1MemberDeps
	// Events is the workspace event hot log + SSE replay window (issue
	// #807). Nil normalizes to a process-local log; tests inject a
	// fresh instance per engine for isolation.
	Events *events.WorkspaceEventLog
	// Webhooks owns signed webhook subscriptions + dispatch (issue
	// #807). Nil normalizes to a process-local store with the
	// production HTTPS sender; tests inject fakes.
	Webhooks *eventwebhook.WebhookStore
	// EventBus publishes v1 envelopes to NATS JetStream (event layer,
	// not a queue). Nil disables cross-instance fanout (single-process
	// tests); production wires the shared bus.
	EventBus *eventnats.NATSJetStreamEventBus
	// BoardAccessOutbox acknowledges immediate delivery of transaction-committed
	// access signals. Pending facts remain retryable by the River sweeper.
	// Nil is reserved for in-memory tests without a durable store.
	BoardAccessOutbox   *eventoutbox.BoardAccessOutboxStore
	SupportChannels     settings.SupportChannelsSettingProvider
	Branding            settings.BrandingSettingProvider
	PublicSearchHandler *searchhttp.SearchHandler
	// UserProfile persists profile updates and projects the user record on session reads.
	// Nil omits the read projection and fails avatar uploads closed with 503.
	UserProfile   runtimehttp.V1UserProfileDirectory
	// RealtimeTicketSecret is the HMAC secret for board presence tickets
	// (RT_TICKET_SECRET). Empty fails the realtime-ticket endpoint
	// closed with 500 (no fallback default); Load records the missing
	// key in Config.InvalidEnv so production startup fails fast.
	RealtimeTicketSecret string
	// BillingAccounts resolves the stable funds identity for creation-time
	// account display and authorization (#944). Production wires the
	// tenancy billing-account store; nil keeps legacy workspace creation
	// (no explicit linkage) so existing in-memory fixtures stay green.
	// Explicit billing_account_id/new_billing_account requests with a nil
	// store fail closed with 503, never silently unlinked.
	BillingAccounts billingAccountStore
	// Notifications is the explicitly wired user notification command boundary
	// (#958, ADR-0040). Nil fails the notification endpoints closed with 503.
	Notifications notifications.Store
	// UserEvents serves the recipient-addressed SSE subscription.
	UserEvents *events.UserEventHub
	// InvitationReader owns optional invitation participant enrichment and projection.
	InvitationReader invitations.Reader
	// Invitations owns atomic membership decisions, notifications, and mail enqueue.
	Invitations *invitations.Service
}

// contractV1UserDirectoryNote: runtimehttp.V1UserDirectory, runtimehttp.V1UserProfileDirectory,
// usersRepositoryDirectory, runtimehttp.NewUsersRepositoryDirectory and
// runtimehttp.NewV1UserProfileDirectory live in the runtimehttp kernel (shared.go)
// so v1 and control share them without a control -> v1 edge.

func (d *ContractV1Deps) normalize() {
	if d.Idempotency == nil {
		d.Idempotency = contract.NewIdempotencyStore(contract.DefaultIdempotencyStoreOptions())
	}
	if d.Events == nil {
		d.Events = events.NewWorkspaceEventLog()
	}
	if d.Webhooks == nil {
		d.Webhooks = eventwebhook.NewWebhookStore(eventwebhook.NewHTTPSender(), nil)
	}
}

// contractV1AuthNote: session auth rides the kernel runtimehttp.V1AuthMiddleware
// (shared.go); runtimehttp.ResolvePrincipal is kernel runtimehttp.ResolvePrincipal.

// RegisterContractV1Routes mounts the real v1 handlers on group (exported
// for tests; production calls it from RegisterRoutes with the full auth
// deps). Unknown v1 paths still fail closed with 501.
func RegisterContractV1Routes(group *gin.RouterGroup, authDeps runtimehttp.AuthDeps, deps ContractV1Deps) {
	deps.normalize()
	handler := &contractV1Handler{deps: deps}
	handler.systemHandler().MountPublic(group)
	handler.mediaHandler().MountPublic(group)
	handler.tenancyHandler().MountPublic(group)

	group.Use(runtimehttp.V1AuthMiddleware(authDeps), runtimehttp.ResolvePrincipal())

	handler.tenancyHandler().Mount(group)
	handler.mediaHandler().Mount(group)
	handler.registerBoardRoutes(group)
	handler.registerLibraryRoutes(group)
	handler.registerTaskRoutes(group)
	handler.systemHandler().Mount(group)
}

func (h *contractV1Handler) registerBoardRoutes(group *gin.RouterGroup) {
	boardHandler := v1boards.NewHandler(v1boards.Deps{
		Store:                     h.deps.Store,
		Users:                     h.deps.Users,
		Boards:                    h.deps.Boards,
		Idempotency:               h.deps.Idempotency,
		MemberDisplay:             h.deps.Members.MemberDisplay,
		RealtimeTicketSecret:      h.deps.RealtimeTicketSecret,
		EmitWorkspaceEvent:        h.emitWorkspaceEvent,
		DeliverBoardAccessSignals: h.deliverBoardAccessSignals,
	})
	boardHandler.Mount(group)
}

func (h *contractV1Handler) authorizeBoardByID(c *gin.Context, workspace *tenancy.Workspace, boardID, workspaceRole string, userID int64, cap boards.Capability) (*boards.V1Board, boards.AccessRole, bool) {
	return v1boards.AuthorizeBoardByID(c, h.deps.Boards, workspace, boardID, workspaceRole, userID, cap)
}

func (h *contractV1Handler) requireBoardsStore(c *gin.Context) bool {
	if h.deps.Boards == nil {
		contract.RenderError(c, http.StatusNotImplemented, contract.CodeContractNotImplemented, "The v1 boards surface is not implemented yet")
		c.Abort()
		return false
	}
	return true
}

func (h *contractV1Handler) libraryHandler() *v1library.Handler {
	return v1library.NewHandler(v1library.Deps{
		Store:                     h.deps.Store,
		Library:                   h.deps.Library,
		Tasks:                     h.deps.Tasks.Tasks,
		ResolveTaskBoardAccess:    h.resolveTaskBoardAccess,
		MediaPreviews:             h.deps.Media.MediaPreviews,
		MediaRenderer:             h.deps.Media.MediaRenderer,
		AuthorizeMediaBinaryAsset: h.v1MediaBinaryAsset,
		RenderMediaAsset: func(c *gin.Context, workspacePublicID, mediaAssetID string, userID int64, workspaceRole string) bool {
			if asset, ok := h.authorizeMediaAsset(c, workspacePublicID, mediaAssetID, userID, workspaceRole); ok {
				contract.RenderSuccess(c, toMediaAssetDTO(workspacePublicID, asset, shared.FormatTime(asset.CreatedAt)))
				return true
			}
			return false
		},
	})
}

func (h *contractV1Handler) registerLibraryRoutes(group *gin.RouterGroup) {
	h.libraryHandler().Mount(group)
}

func (h *contractV1Handler) getLibraryMaterializedMediaAsset(c *gin.Context, workspacePublicID, assetPublicID string, userID int64, workspaceRole string) {
	h.libraryHandler().GetLibraryMaterializedMediaAsset(c, workspacePublicID, assetPublicID, userID, workspaceRole)
}

func (h *contractV1Handler) mediaHandler() *v1media.Handler {
	return v1media.NewHandler(v1media.Deps{
		Store:                            h.deps.Store,
		Media:                            h.deps.Media,
		Avatar:                           h.deps.Avatar,
		UserProfile:                      h.deps.UserProfile,
		Boards:                           h.deps.Boards,
		Idempotency:                      h.deps.Idempotency,
		EmitWorkspaceEvent:               h.emitWorkspaceEvent,
		GetLibraryMaterializedMediaAsset: h.getLibraryMaterializedMediaAsset,
	})
}

func (h *contractV1Handler) publicAvatar(c *gin.Context) {
	h.mediaHandler().PublicAvatar(c)
}

func (h *contractV1Handler) publicBrandingLogo(c *gin.Context) {
	h.mediaHandler().PublicBrandingLogo(c)
}

func (h *contractV1Handler) v1MediaBinaryAsset(c *gin.Context, workspacePublicID, assetID string, userID int64, workspaceRole string) (*workspacemedia.Asset, bool) {
	return h.mediaHandler().MediaBinaryAsset(c, workspacePublicID, assetID, userID, workspaceRole)
}

func (h *contractV1Handler) authorizeMediaAsset(c *gin.Context, workspacePublicID, assetID string, userID int64, workspaceRole string) (*workspacemedia.Asset, bool) {
	return h.mediaHandler().AuthorizeMediaAsset(c, workspacePublicID, assetID, userID, workspaceRole)
}

func (h *contractV1Handler) systemHandler() *v1system.Handler {
	return v1system.NewHandler(v1system.Deps{
		Store:               h.deps.Store,
		Users:               h.deps.Users,
		Boards:              h.deps.Boards,
		Media:               h.deps.Media,
		Tasks:               h.deps.Tasks,
		Search:              h.deps.Search,
		Notifications:       h.deps.Notifications,
		UserEvents:          h.deps.UserEvents,
		Events:              h.deps.Events,
		Webhooks:            h.deps.Webhooks,
		EventBus:            h.deps.EventBus,
		BoardAccessOutbox:   h.deps.BoardAccessOutbox,
		SupportChannels:     h.deps.SupportChannels,
		Branding:            h.deps.Branding,
		PublicSearchHandler: h.deps.PublicSearchHandler,
		Idempotency:         h.deps.Idempotency,
		TaskEventVisibleToViewer: func(ctx context.Context, workspacePublicID string, viewerID int64, viewerRole string, envelope events.V1Envelope) bool {
			return h.taskEventVisibleToViewer(ctx, workspacePublicID, workspaceEventViewer{userID: viewerID, workspaceRole: viewerRole}, envelope)
		},
	})
}

func (h *contractV1Handler) emitWorkspaceEvent(ctx context.Context, identityPrincipal principal.Principal, workspaceID, typ string, resource events.V1Resource, data any) {
	h.systemHandler().EmitWorkspaceEvent(ctx, identityPrincipal, workspaceID, typ, resource, data)
}

func (h *contractV1Handler) emitWorkspaceEventWithID(ctx context.Context, identityPrincipal principal.Principal, workspaceID, typ string, resource events.V1Resource, data any, eventID string, occurredAt time.Time) {
	h.systemHandler().EmitWorkspaceEventWithID(ctx, identityPrincipal, workspaceID, typ, resource, data, eventID, occurredAt)
}

func (h *contractV1Handler) emitTaskBillingSettled(ctx context.Context, identityPrincipal principal.Principal, workspaceID, taskID, settlementState, holdTransactionID string) {
	h.systemHandler().EmitTaskBillingSettled(ctx, identityPrincipal, workspaceID, taskID, settlementState, holdTransactionID)
}

func (h *contractV1Handler) deliverBoardAccessSignals(ctx context.Context, records []eventoutbox.BoardAccessOutboxRecord) {
	h.systemHandler().DeliverBoardAccessSignals(ctx, records)
}

type workspaceEventViewer struct {
	userID        int64
	userPublicID  string
	workspaceRole string
}

func (h *contractV1Handler) eventViewerFor(ctx context.Context, identityPrincipal principal.Principal, workspaceRole string) (workspaceEventViewer, bool) {
	v, ok := h.systemHandler().EventViewerFor(ctx, identityPrincipal, workspaceRole)
	if !ok {
		return workspaceEventViewer{}, false
	}
	return workspaceEventViewer{userID: v.UserID, userPublicID: v.UserPublicID, workspaceRole: v.WorkspaceRole}, true
}

func (h *contractV1Handler) eventVisibleToViewer(ctx context.Context, workspacePublicID string, viewer workspaceEventViewer, envelope events.V1Envelope) bool {
	return h.systemHandler().EventVisibleToViewer(ctx, workspacePublicID, v1system.WorkspaceEventViewer{
		UserID:        viewer.userID,
		UserPublicID:  viewer.userPublicID,
		WorkspaceRole: viewer.workspaceRole,
	}, envelope)
}

func (h *contractV1Handler) registerTaskRoutes(group *gin.RouterGroup) {
	taskHandler := v1tasks.NewHandler(v1tasks.Deps{
		Store:                    h.deps.Store,
		Users:                    h.deps.Users,
		Boards:                   h.deps.Boards,
		Idempotency:              h.deps.Idempotency,
		Tasks:                    h.deps.Tasks,
		EmitWorkspaceEventWithID: h.emitWorkspaceEventWithID,
		EmitTaskBillingSettled:   h.emitTaskBillingSettled,
	})
	taskHandler.Mount(group)
}

func (h *contractV1Handler) taskEventVisibleToViewer(ctx context.Context, workspacePublicID string, viewer workspaceEventViewer, envelope events.V1Envelope) bool {
	taskHandler := v1tasks.NewHandler(v1tasks.Deps{
		Store:  h.deps.Store,
		Boards: h.deps.Boards,
		Tasks:  h.deps.Tasks,
	})
	return taskHandler.TaskEventVisibleToViewer(ctx, workspacePublicID, viewer.userID, viewer.workspaceRole, envelope)
}

func (h *contractV1Handler) resolveTaskBoardAccess(ctx context.Context, workspace *tenancy.Workspace, workspaceRole string, userID int64, task *taskdomain.WorkspaceTask) *taskdomain.BoardAccess {
	taskHandler := v1tasks.NewHandler(v1tasks.Deps{
		Store:  h.deps.Store,
		Boards: h.deps.Boards,
		Tasks:  h.deps.Tasks,
	})
	return taskHandler.ResolveTaskBoardAccess(ctx, workspace, workspaceRole, userID, task)
}

var isTaskContentEvent = v1tasks.IsTaskContentEvent
var reconcileTaskResolution = v1tasks.ReconcileTaskResolution

type contractV1Handler struct {
	deps ContractV1Deps
}

// v1ListCursor is the opaque keyset position for v1 list endpoints.
type v1ListCursor = shared.ListCursor
type v1BoardBundleDTO = v1boards.BoardBundleDTO

func (h *contractV1Handler) tenancyHandler() *v1tenancy.Handler {
	return v1tenancy.NewHandler(v1tenancy.Deps{
		Store:                     h.deps.Store,
		Users:                     h.deps.Users,
		Idempotency:               h.deps.Idempotency,
		UserProfile:               h.deps.UserProfile,
		BillingAccounts:           h.deps.BillingAccounts,
		Members:                   h.deps.Members,
		InvitationReader:          h.deps.InvitationReader,
		Invitations:               h.deps.Invitations,
		Boards:                    h.deps.Boards,
		DeliverBoardAccessSignals: h.deliverBoardAccessSignals,
	})
}

func (h *contractV1Handler) getWorkspaceInviteByToken(c *gin.Context) {
	h.tenancyHandler().GetWorkspaceInviteByToken(c)
}

// RegisterContractV1RoutesOnEngine mounts the additive v1 group on the
// engine: real handlers for the workspace/organization/member lifecycle
// (issue #801), boards + collaboration (#802), uploads + media (#803)
// with the shared kernel PublicContractChain. (Moved here from the
// runtimehttp kernel during the plane split so the kernel stays import-free
// of v1; composition calls it alongside the billing/control mounts.)
func RegisterContractV1RoutesOnEngine(r *gin.Engine, authDeps runtimehttp.AuthDeps, deps ContractV1Deps) {
	v1group := r.Group(runtimehttp.ContractV1GroupPath)
	v1group.Use(runtimehttp.PublicContractChain()...)
	RegisterContractV1Routes(v1group, authDeps, deps)
}
