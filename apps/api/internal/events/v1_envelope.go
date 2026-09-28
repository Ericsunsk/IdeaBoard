package events

// Versioned workspace event envelope (issue #807).
//
// One envelope is shared by SSE and signed webhooks; transports differ
// only in delivery. The envelope version is independent from HTTP API
// versions: it versions the event data payload for (type) only.
//
// Frozen registry (type -> version). Additive only: new types arrive with
// a new versioned data schema; existing (type, version) pairs never
// change shape. Internal DB rows and task structs are NEVER exposed:
// producers map domain facts to opaque public ids (task_, board_,
// asset_, lasset_, ws_, usr_) before minting.
//
// Publishing rule (exactly-once-from-domain-boundary): domain services
// stage facts during settlement and emit envelopes ONLY after the commit
// succeeds (see workspacetasks.Service.StagedEvents + TaskStatusV1Type).
// Delivery is at-least-once: NATS publishes with MsgID = envelope id
// (duplicate window), the hot log append is idempotent on id, and webhook
// dispatch dedups per subscription. Durable outbox persistence lands
// with #809; until then the process-local log is the hot replay window
// and NATS JetStream (7-day retention per DESIGN) is the transport.
//
// Control audit records are never exposed here: the registry contains no
// audit/control types by construction (see TestV1RegistryExcludesControl).

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/platform/contract"
)

// Frozen workspace event types (type -> data schema version).
const (
	EventV1TaskCreated           = "task.created"
	EventV1TaskRunning           = "task.running"
	EventV1TaskCompleted         = "task.completed"
	EventV1TaskFailed            = "task.failed"
	EventV1TaskCancelled         = "task.cancelled"
	EventV1MediaAssetReady       = "media.asset_ready"
	EventV1MediaAssetArchived    = "media.asset_archived"
	EventV1BoardDocumentReplaced = "board.document_replaced"
	EventV1BoardMemberChanged    = "board.member_changed"
	EventV1BoardShareRevoked     = "board.share_revoked"
	EventV1BillingSettled        = "billing.settled"
)

// v1EventVersions freezes the data schema version per event type.
var v1EventVersions = map[string]int{
	EventV1TaskCreated:           1,
	EventV1TaskRunning:           1,
	EventV1TaskCompleted:         1,
	EventV1TaskFailed:            1,
	EventV1TaskCancelled:         1,
	EventV1MediaAssetReady:       1,
	EventV1MediaAssetArchived:    1,
	EventV1BoardDocumentReplaced: 1,
	EventV1BoardMemberChanged:    1,
	EventV1BoardShareRevoked:     1,
	EventV1BillingSettled:        1,
}

// V1EventVersion reports the frozen data schema version for typ.
func V1EventVersion(typ string) (int, bool) {
	version, ok := v1EventVersions[strings.TrimSpace(typ)]
	return version, ok
}

// IsV1EventType reports whether typ is a frozen registry member.
func IsV1EventType(typ string) bool {
	_, ok := V1EventVersion(typ)
	return ok
}

// V1EventTypes returns the frozen registry in stable order.
func V1EventTypes() []string {
	return []string{
		EventV1TaskCreated, EventV1TaskRunning, EventV1TaskCompleted,
		EventV1TaskFailed, EventV1TaskCancelled,
		EventV1MediaAssetReady, EventV1MediaAssetArchived,
		EventV1BoardDocumentReplaced, EventV1BoardMemberChanged, EventV1BoardShareRevoked,
		EventV1BillingSettled,
	}
}

// Actor types carried by the envelope. Canonical `service` covers
// service accounts and provider adapters.
const (
	V1ActorUser    = "user"
	V1ActorService = "service"
	V1ActorSystem  = "system"
)

// Resource types carried by the envelope. billing.settled rides on the
// settled entity (task); checkout_session is
// reserved for the #809 Stripe-convergence mapping and is never emitted
// today (Stripe inbound webhooks stay distinct integrations that emit
// internal user events only).
const (
	V1ResourceTask            = "task"
	V1ResourceBoard           = "board"
	V1ResourceMediaAsset      = "media_asset"
	V1ResourceCheckoutSession = "checkout_session"
)

// V1Actor identifies who caused the event. User ids are usr_ opaque
// ids; service ids are opaque service names; system uses id "system".
type V1Actor struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// V1Resource identifies what the event is about (public ids only).
type V1Resource struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// V1Envelope is the shared SSE + webhook envelope.
type V1Envelope struct {
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	Version     int             `json:"version"`
	OccurredAt  time.Time       `json:"occurred_at"`
	Actor       V1Actor         `json:"actor"`
	Resource    V1Resource      `json:"resource"`
	WorkspaceID string          `json:"workspace_id"`
	Data        json.RawMessage `json:"data"`
}

// Typed data payloads (public ids only, no amounts, no storage URLs).

type V1TaskCreatedData struct {
	TaskID       string `json:"task_id"`
	WorkflowKind string `json:"workflow_kind"`
}

type V1TaskRunningData struct {
	TaskID string `json:"task_id"`
}

type V1TaskCompletedData struct {
	TaskID     string `json:"task_id"`
	OutputsURI string `json:"outputs_uri"`
}

type V1TaskFailedData struct {
	TaskID    string `json:"task_id"`
	ErrorCode string `json:"error_code"`
}

type V1TaskCancelledData struct {
	TaskID       string `json:"task_id"`
	CancelReason string `json:"cancel_reason,omitempty"`
}

type V1MediaAssetReadyData struct {
	AssetID string `json:"asset_id"`
}

type V1MediaAssetArchivedData struct {
	AssetID string `json:"asset_id"`
}

type V1BoardDocumentReplacedData struct {
	BoardID  string `json:"board_id"`
	Revision int64  `json:"revision"`
}

type V1BoardMemberChangedData struct {
	BoardID  string `json:"board_id"`
	MemberID string `json:"member_id"`
}

type V1BoardShareRevokedData struct {
	BoardID      string `json:"board_id"`
	ShareTokenID string `json:"share_token_id"`
}

// V1BillingSettledData carries settlement refs only: quote, hold,
// capture, and release references plus the settled state. Amounts never
// appear here (balance reads stay on the wallet endpoints).
type V1BillingSettledData struct {
	ReferenceType     string `json:"reference_type"`
	ReferenceID       string `json:"reference_id"`
	SettlementState   string `json:"settlement_state"`
	HoldTransactionID string `json:"hold_transaction_id,omitempty"`
}

// MintV1Envelope mints an envelope with an evt_ id (via
// contract.MintPublicID), UTC occurred_at, and the frozen version for
// typ. Unknown types fail closed.
func MintV1Envelope(typ, workspaceID string, actor V1Actor, resource V1Resource, data any) (V1Envelope, error) {
	typ = strings.TrimSpace(typ)
	version, ok := V1EventVersion(typ)
	if !ok {
		return V1Envelope{}, fmt.Errorf("events: unknown workspace event type %q", typ)
	}
	id, err := contract.MintPublicID(contract.PrefixEvent, 12)
	if err != nil {
		return V1Envelope{}, err
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return V1Envelope{}, fmt.Errorf("events: marshal %s data: %w", typ, err)
	}
	envelope := V1Envelope{
		ID: id, Type: typ, Version: version, OccurredAt: time.Now().UTC(),
		Actor:       actor,
		Resource:    resource,
		WorkspaceID: strings.TrimSpace(workspaceID),
		Data:        payload,
	}
	if err := ValidateV1Envelope(envelope); err != nil {
		return V1Envelope{}, err
	}
	return envelope, nil
}

// ValidateV1Envelope checks envelope shape: evt_ id, frozen type +
// version match, RFC3339-capable timestamp, actor/resource/typed ids,
// ws_ workspace, and non-empty typed data.
func ValidateV1Envelope(envelope V1Envelope) error {
	if !contract.IsValidPublicID(contract.PrefixEvent, strings.TrimSpace(envelope.ID)) {
		return fmt.Errorf("events: envelope id must be evt_ opaque")
	}
	version, ok := V1EventVersion(envelope.Type)
	if !ok {
		return fmt.Errorf("events: unknown workspace event type %q", envelope.Type)
	}
	if envelope.Version != version {
		return fmt.Errorf("events: %s version=%d, want %d", envelope.Type, envelope.Version, version)
	}
	if envelope.OccurredAt.IsZero() {
		return fmt.Errorf("events: occurred_at is required")
	}
	switch strings.TrimSpace(envelope.Actor.Type) {
	case V1ActorUser, V1ActorService, V1ActorSystem:
	default:
		return fmt.Errorf("events: actor type must be user|service|system")
	}
	if strings.TrimSpace(envelope.Actor.ID) == "" {
		return fmt.Errorf("events: actor id is required")
	}
	if strings.TrimSpace(envelope.Actor.Type) == V1ActorUser &&
		!contract.IsValidPublicID(contract.PrefixUser, strings.TrimSpace(envelope.Actor.ID)) {
		return fmt.Errorf("events: user actor id must be usr_ opaque")
	}
	switch strings.TrimSpace(envelope.Resource.Type) {
	case V1ResourceTask, V1ResourceBoard, V1ResourceMediaAsset,
		V1ResourceCheckoutSession:
	default:
		return fmt.Errorf("events: unknown resource type %q", envelope.Resource.Type)
	}
	if strings.TrimSpace(envelope.Resource.ID) == "" {
		return fmt.Errorf("events: resource id is required")
	}
	if err := validateV1ResourceID(envelope.Resource.Type, strings.TrimSpace(envelope.Resource.ID)); err != nil {
		return err
	}
	if !contract.IsValidPublicID(contract.PrefixWorkspace, strings.TrimSpace(envelope.WorkspaceID)) {
		return fmt.Errorf("events: workspace_id must be ws_ opaque")
	}
	if len(envelope.Data) == 0 || strings.TrimSpace(string(envelope.Data)) == "null" {
		return fmt.Errorf("events: data payload is required")
	}
	var probe map[string]any
	if err := json.Unmarshal(envelope.Data, &probe); err != nil {
		return fmt.Errorf("events: data must be a JSON object: %w", err)
	}
	return validateV1Data(envelope.Type, envelope.Data)
}

func validateV1ResourceID(resourceType, id string) error {
	switch resourceType {
	case V1ResourceTask:
		if !contract.IsValidPublicID(contract.PrefixTask, id) {
			return fmt.Errorf("events: task resource id must be task_ opaque")
		}
	case V1ResourceBoard:
		if !contract.IsValidPublicID(contract.PrefixBoard, id) {
			return fmt.Errorf("events: board resource id must be board_ opaque")
		}
	case V1ResourceMediaAsset:
		if !contract.IsValidPublicID(contract.PrefixAsset, id) {
			return fmt.Errorf("events: media_asset resource id must be asset_ opaque")
		}
	case V1ResourceCheckoutSession:
		if id == "" {
			return fmt.Errorf("events: checkout_session resource id is required")
		}
	}
	return nil
}

func validateV1Data(typ string, raw json.RawMessage) error {
	require := func(fields map[string]any, names ...string) error {
		for _, name := range names {
			value, ok := fields[name]
			if !ok {
				return fmt.Errorf("events: %s data requires %q", typ, name)
			}
			if str, isStr := value.(string); isStr && strings.TrimSpace(str) == "" {
				return fmt.Errorf("events: %s data requires non-empty %q", typ, name)
			}
		}
		return nil
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		return fmt.Errorf("events: %s data must decode: %w", typ, err)
	}
	stringField := func(name string) string {
		value, _ := fields[name].(string)
		return strings.TrimSpace(value)
	}
	switch typ {
	case EventV1TaskCreated:
		if err := require(fields, "task_id", "workflow_kind"); err != nil {
			return err
		}
		if !contract.IsValidPublicID(contract.PrefixTask, stringField("task_id")) {
			return fmt.Errorf("events: task.created task_id must be task_ opaque")
		}
	case EventV1TaskRunning, EventV1TaskCancelled:
		if err := require(fields, "task_id"); err != nil {
			return err
		}
		if !contract.IsValidPublicID(contract.PrefixTask, stringField("task_id")) {
			return fmt.Errorf("events: %s task_id must be task_ opaque", typ)
		}
	case EventV1TaskCompleted:
		if err := require(fields, "task_id", "outputs_uri"); err != nil {
			return err
		}
		if !contract.IsValidPublicID(contract.PrefixTask, stringField("task_id")) {
			return fmt.Errorf("events: task.completed task_id must be task_ opaque")
		}
		uri := stringField("outputs_uri")
		if !strings.HasPrefix(uri, "/api/v1/workspaces/") {
			return fmt.Errorf("events: task.completed outputs_uri must be a workspace API URI")
		}
	case EventV1TaskFailed:
		if err := require(fields, "task_id", "error_code"); err != nil {
			return err
		}
		if !contract.IsValidPublicID(contract.PrefixTask, stringField("task_id")) {
			return fmt.Errorf("events: task.failed task_id must be task_ opaque")
		}
	case EventV1MediaAssetReady, EventV1MediaAssetArchived:
		if err := require(fields, "asset_id"); err != nil {
			return err
		}
		if !contract.IsValidPublicID(contract.PrefixAsset, stringField("asset_id")) {
			return fmt.Errorf("events: %s asset_id must be asset_ opaque", typ)
		}
	case EventV1BoardDocumentReplaced:
		if err := require(fields, "board_id", "revision"); err != nil {
			return err
		}
		if !contract.IsValidPublicID(contract.PrefixBoard, stringField("board_id")) {
			return fmt.Errorf("events: board.document_replaced board_id must be board_ opaque")
		}
	case EventV1BoardMemberChanged:
		if err := require(fields, "board_id", "member_id"); err != nil {
			return err
		}
		if !contract.IsValidPublicID(contract.PrefixBoard, stringField("board_id")) {
			return fmt.Errorf("events: board.member_changed board_id must be board_ opaque")
		}
		if !contract.IsValidPublicID(contract.PrefixUser, stringField("member_id")) {
			return fmt.Errorf("events: board.member_changed member_id must be usr_ opaque")
		}
	case EventV1BoardShareRevoked:
		if err := require(fields, "board_id", "share_token_id"); err != nil {
			return err
		}
		if !contract.IsValidPublicID(contract.PrefixBoard, stringField("board_id")) {
			return fmt.Errorf("events: board.share_revoked board_id must be board_ opaque")
		}
	case EventV1BillingSettled:
		if err := require(fields, "reference_type", "reference_id", "settlement_state"); err != nil {
			return err
		}
		switch stringField("reference_type") {
		case V1ResourceTask, V1ResourceCheckoutSession:
		default:
			return fmt.Errorf("events: billing.settled reference_type must be task|checkout_session")
		}
		switch stringField("settlement_state") {
		case "quoted", "free", "held", "captured", "released":
		default:
			return fmt.Errorf("events: billing.settled settlement_state must be quoted|free|held|captured|released")
		}
	}
	return nil
}

// taskStatusToV1 maps workspacetasks staged statuses to v1 types:
// pending (fresh submit) -> task.created; running -> task.running;
// completed/failed/cancelled map 1:1. Unknown statuses fail closed.
func taskStatusToV1(status TaskStatus) (string, bool) {
	switch strings.TrimSpace(string(status)) {
	case string(TaskStatusPending):
		return EventV1TaskCreated, true
	case string(TaskStatusRunning):
		return EventV1TaskRunning, true
	case string(TaskStatusCompleted):
		return EventV1TaskCompleted, true
	case string(TaskStatusFailed):
		return EventV1TaskFailed, true
	case string(TaskStatusCancelled):
		return EventV1TaskCancelled, true
	default:
		return "", false
	}
}

// TaskStatusV1Type maps a staged task status to its v1 event type
// (exported for the HTTP emission seam and the #809 worker flush; the
// table itself stays frozen here).
func TaskStatusV1Type(status TaskStatus) (string, bool) {
	return taskStatusToV1(status)
}

// V1Deduper is an evt_-id exactly-once guard for delivery paths:
// SSE resume, webhook per-subscription dispatch, and log append all
// consult it so duplicate deliveries collapse to one.
type V1Deduper struct {
	seen  map[string]struct{}
	order []string
	max   int
}

// NewV1Deduper builds a deduper retaining up to max ids (oldest evicted).
func NewV1Deduper(max int) *V1Deduper {
	if max <= 0 {
		max = 4096
	}
	return &V1Deduper{seen: make(map[string]struct{}), max: max}
}

// Seen reports whether id was already observed (and records it).
func (d *V1Deduper) Seen(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	if d == nil {
		return false
	}
	if _, ok := d.seen[id]; ok {
		return true
	}
	d.seen[id] = struct{}{}
	d.order = append(d.order, id)
	if len(d.order) > d.max {
		oldest := d.order[0]
		d.order = append([]string(nil), d.order[1:]...)
		delete(d.seen, oldest)
	}
	return false
}
