package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"ideaboard/api/internal/events"
)

const (
	defaultNATSStreamName       = "IDEABOARD_EVENTS"
	defaultNATSSubjectPrefix    = "ideaboard"
	defaultNATSRetention        = 7 * 24 * time.Hour
	defaultNATSConnectTimeout   = 5 * time.Second
	defaultNATSRequestTimeout   = 5 * time.Second
	defaultNATSDrainTimeout     = 2 * time.Second
	defaultNATSConsumerPrefix   = "ideaboard-api-events"
	taskStatusSubjectSuffix     = "task.status"
	userEventSubjectSuffix      = "user.event"
	workspaceEventSubjectSuffix = "workspace.event"
	natsDuplicateWindow         = 2 * time.Minute
	natsConsumerInactivePeriod  = 24 * time.Hour
)

type NATSJetStreamConfig struct {
	URL            string
	StreamName     string
	SubjectPrefix  string
	ConsumerName   string
	ConnectTimeout time.Duration
	RequestTimeout time.Duration
	DrainTimeout   time.Duration
}

type NATSJetStreamEventBus struct {
	nc                *nats.Conn
	js                jetstream.JetStream
	streamName        string
	taskStatusSubject string
	userEventSubject  string
	workspaceSubject  string
	instanceID        string

	mu       sync.Mutex
	consumes []jetstream.ConsumeContext
}

func NewNATSJetStreamEventBus(ctx context.Context, cfg NATSJetStreamConfig) (*NATSJetStreamEventBus, error) {
	cfg = normalizeNATSJetStreamConfig(cfg)

	nc, err := nats.Connect(
		cfg.URL,
		nats.Name("ideaboard-api-events"),
		nats.Timeout(cfg.ConnectTimeout),
		nats.DrainTimeout(cfg.DrainTimeout),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("connect nats: %w", err)
	}

	js, err := jetstream.New(nc, jetstream.WithDefaultTimeout(cfg.RequestTimeout))
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("create jetstream client: %w", err)
	}

	streamCfg := jetstream.StreamConfig{
		Name:         cfg.StreamName,
		Description:  "IdeaBoard domain events",
		Subjects:     []string{cfg.SubjectPrefix + ".>"},
		Retention:    jetstream.LimitsPolicy,
		MaxConsumers: -1,
		MaxMsgs:      -1,
		MaxBytes:     -1,
		MaxAge:       defaultNATSRetention,
		Storage:      jetstream.FileStorage,
		Replicas:     1,
		Duplicates:   natsDuplicateWindow,
	}
	if _, err := js.CreateOrUpdateStream(ctx, streamCfg); err != nil {
		nc.Close()
		return nil, fmt.Errorf("ensure nats stream %s: %w", cfg.StreamName, err)
	}

	return &NATSJetStreamEventBus{
		nc:                nc,
		js:                js,
		streamName:        cfg.StreamName,
		taskStatusSubject: taskStatusSubject(cfg.SubjectPrefix),
		userEventSubject:  userEventSubject(cfg.SubjectPrefix),
		workspaceSubject:  workspaceEventSubject(cfg.SubjectPrefix),
		instanceID:        cfg.ConsumerName,
	}, nil
}

// Generic NATS envelope bridge.
//
// The three event families (user events, task-status events, workspace v1
// envelopes) share one transport shape: an origin_instance wrapper published
// with a MsgID dedup key, fanned out through a per-instance durable consumer
// (DeliverNew, AckExplicit, 30s AckWait, 5 MaxDeliver, 1024 MaxAckPending),
// and drained by a consume loop with a uniform panic-recovery guard.
// Per-family thin codecs supply the subject, MsgID, envelope marshal/decode,
// and log labels; subject topology, retention, and ack policy stay exactly
// as-is.
//
// bridgeCodec is the publish-side convergence of those thin codecs: each
// family keeps only its subject accessor plus field validation/marshal, and
// shares publishBridgedEvent for the validate -> marshal -> MsgID publish
// path. The subscribe path (ack/retention/consumer policy) is untouched.
type bridgeCodec[T any] struct {
	subject  func(*NATSJetStreamEventBus) string
	msgIDOf  func(T) string
	validate func(T) error
}

func publishBridgedEvent[T any](b *NATSJetStreamEventBus, ctx context.Context, codec bridgeCodec[T], event T, marshal func(string, T) ([]byte, error), op string) error {
	if err := codec.validate(event); err != nil {
		return err
	}
	payload, err := marshal(b.instanceID, event)
	if err != nil {
		return err
	}
	return b.publishBridgeMessage(ctx, codec.subject(b), codec.msgIDOf(event), payload, op)
}

func validateUserEventForPublish(event events.UserEvent) error {
	if event.UserID <= 0 || strings.TrimSpace(event.Type) == "" {
		return fmt.Errorf("publish user event: user_id and type are required")
	}
	return nil
}

func marshalUserEventForPublish(instanceID string, event events.UserEvent) ([]byte, error) {
	payload, err := json.Marshal(userEventEnvelope{OriginInstance: instanceID, Event: event})
	if err != nil {
		return nil, fmt.Errorf("marshal user event: %w", err)
	}
	return payload, nil
}

var userEventBridgeCodec = bridgeCodec[events.UserEvent]{
	subject: func(b *NATSJetStreamEventBus) string { return b.userEventSubject },
	msgIDOf: func(event events.UserEvent) string {
		return fmt.Sprintf("%d:%s:%d", event.UserID, event.Type, event.OccurredAt.UnixNano())
	},
	validate: validateUserEventForPublish,
}

func validateTaskStatusForPublish(event events.TaskStatusEvent) error {
	if event.TaskID == "" || event.Status == "" {
		return fmt.Errorf("publish task status: task_id and status are required")
	}
	return nil
}

func marshalTaskStatusForPublish(instanceID string, event events.TaskStatusEvent) ([]byte, error) {
	payload, err := json.Marshal(taskStatusEnvelope{OriginInstance: instanceID, Event: event})
	if err != nil {
		return nil, fmt.Errorf("marshal task status event: %w", err)
	}
	return payload, nil
}

var taskStatusBridgeCodec = bridgeCodec[events.TaskStatusEvent]{
	subject: func(b *NATSJetStreamEventBus) string { return b.taskStatusSubject },
	msgIDOf: func(event events.TaskStatusEvent) string {
		if msgID := event.EventID; strings.TrimSpace(msgID) != "" {
			return msgID
		}
		return fmt.Sprintf("%s:%s:%d", event.TaskID, event.Status, event.OccurredAt.UnixNano())
	},
	validate: validateTaskStatusForPublish,
}

func marshalWorkspaceEventForPublish(instanceID string, envelope events.V1Envelope) ([]byte, error) {
	payload, err := json.Marshal(workspaceV1Envelope{OriginInstance: instanceID, Envelope: envelope})
	if err != nil {
		return nil, fmt.Errorf("marshal workspace event: %w", err)
	}
	return payload, nil
}

var workspaceEventBridgeCodec = bridgeCodec[events.V1Envelope]{
	subject:  func(b *NATSJetStreamEventBus) string { return b.workspaceSubject },
	msgIDOf:  func(envelope events.V1Envelope) string { return envelope.ID },
	validate: events.ValidateV1Envelope,
}

type natsBridgeSubscribeConfig[T any] struct {
	consumerName   string
	subject        string
	description    string
	ensureOp       string
	startOp        string
	malformedLog   string
	localAckLog    string
	handlerFailLog string
	ackFailLog     string
	consumerErrLog string
	decode         func(payload []byte, instanceID string) (T, bool, error)
	handler        func(context.Context, T) error
	logAttrs       func(T) []any
}

func (b *NATSJetStreamEventBus) publishBridgeMessage(ctx context.Context, subject, msgID string, payload []byte, op string) error {
	if _, err := b.js.Publish(ctx, subject, payload, jetstream.WithMsgID(msgID)); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

func subscribeNATSBridge[T any](b *NATSJetStreamEventBus, ctx context.Context, cfg natsBridgeSubscribeConfig[T]) error {
	consumer, err := b.js.CreateOrUpdateConsumer(ctx, b.streamName, jetstream.ConsumerConfig{
		Name: cfg.consumerName, Durable: cfg.consumerName, Description: cfg.description,
		DeliverPolicy: jetstream.DeliverNewPolicy, AckPolicy: jetstream.AckExplicitPolicy,
		AckWait: 30 * time.Second, MaxDeliver: 5, MaxAckPending: 1024,
		FilterSubject: cfg.subject, InactiveThreshold: natsConsumerInactivePeriod,
	})
	if err != nil {
		return fmt.Errorf("%s %s: %w", cfg.ensureOp, cfg.consumerName, err)
	}
	consume, err := consumer.Consume(func(msg jetstream.Msg) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("NATS consume callback panicked", "subject", msg.Subject(), "consumer", cfg.consumerName, "panic", r)
				_ = msg.NakWithDelay(time.Second)
			}
		}()
		event, shouldDeliver, err := cfg.decode(msg.Data(), b.instanceID)
		if err != nil {
			slog.Warn(cfg.malformedLog, "subject", msg.Subject(), "err", err)
			_ = msg.Term()
			return
		}
		if !shouldDeliver {
			if err := msg.Ack(); err != nil {
				slog.Warn(cfg.localAckLog, "subject", msg.Subject(), "err", err)
			}
			return
		}
		handlerCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		handlerErr := cfg.handler(handlerCtx, event)
		cancel()
		if handlerErr != nil {
			attrs := []any{"err", handlerErr}
			if cfg.logAttrs != nil {
				attrs = append(attrs, cfg.logAttrs(event)...)
			}
			slog.Warn(cfg.handlerFailLog, attrs...)
			_ = msg.NakWithDelay(time.Second)
			return
		}
		if err := msg.Ack(); err != nil {
			attrs := []any{"subject", msg.Subject(), "err", err}
			if cfg.logAttrs != nil {
				attrs = append(attrs, cfg.logAttrs(event)...)
			}
			slog.Warn(cfg.ackFailLog, attrs...)
		}
	}, jetstream.ConsumeErrHandler(func(_ jetstream.ConsumeContext, err error) {
		slog.Warn(cfg.consumerErrLog, "err", err)
	}))
	if err != nil {
		return fmt.Errorf("%s %s: %w", cfg.startOp, cfg.consumerName, err)
	}
	b.mu.Lock()
	b.consumes = append(b.consumes, consume)
	b.mu.Unlock()
	return nil
}

type userEventEnvelope struct {
	OriginInstance string           `json:"origin_instance"`
	Event          events.UserEvent `json:"event"`
}

func decodeUserEventForInstance(payload []byte, instanceID string) (events.UserEvent, bool, error) {
	var envelope userEventEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return events.UserEvent{}, false, fmt.Errorf("decode user event envelope: %w", err)
	}
	envelope.OriginInstance = strings.TrimSpace(envelope.OriginInstance)
	envelope.Event.Type = strings.TrimSpace(envelope.Event.Type)
	if envelope.OriginInstance == "" {
		return events.UserEvent{}, false, fmt.Errorf("decode user event envelope: origin_instance is required")
	}
	if envelope.Event.UserID <= 0 || envelope.Event.Type == "" {
		return events.UserEvent{}, false, fmt.Errorf("decode user event envelope: user_id and type are required")
	}
	if envelope.OriginInstance == strings.TrimSpace(instanceID) {
		return events.UserEvent{}, false, nil
	}
	return envelope.Event, true, nil
}

func (b *NATSJetStreamEventBus) PublishUserEvent(ctx context.Context, event events.UserEvent) error {
	if b == nil || b.js == nil {
		return nil
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}
	return publishBridgedEvent(b, ctx, userEventBridgeCodec, event, marshalUserEventForPublish, "publish user event")
}

func (b *NATSJetStreamEventBus) SubscribeUserEvents(ctx context.Context, handler events.UserEventHandler) error {
	if b == nil || b.js == nil {
		return nil
	}
	if handler == nil {
		return fmt.Errorf("subscribe user events: handler is nil")
	}
	return subscribeNATSBridge(b, ctx, natsBridgeSubscribeConfig[events.UserEvent]{
		consumerName: b.instanceID + "-user-events", subject: b.userEventSubject,
		description:    "IdeaBoard API user event fanout bridge",
		ensureOp:       "ensure nats user event consumer",
		startOp:        "start nats user event consumer",
		malformedLog:   "discard malformed user event",
		localAckLog:    "ack local-origin user event failed",
		handlerFailLog: "user event handler failed",
		ackFailLog:     "ack user event failed",
		consumerErrLog: "user event consumer error",
		decode:         decodeUserEventForInstance,
		handler:        handler,
		logAttrs: func(event events.UserEvent) []any {
			return []any{"user_id", event.UserID, "type", event.Type}
		},
	})
}

type taskStatusEnvelope struct {
	OriginInstance string                 `json:"origin_instance"`
	Event          events.TaskStatusEvent `json:"event"`
}

func decodeTaskStatusForInstance(payload []byte, instanceID string) (events.TaskStatusEvent, bool, error) {
	var envelope taskStatusEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return events.TaskStatusEvent{}, false, fmt.Errorf("decode task status envelope: %w", err)
	}
	envelope.OriginInstance = strings.TrimSpace(envelope.OriginInstance)
	if envelope.OriginInstance == "" {
		return events.TaskStatusEvent{}, false, fmt.Errorf("decode task status envelope: origin_instance is required")
	}
	if envelope.OriginInstance == instanceID {
		return events.TaskStatusEvent{}, false, nil
	}
	event := events.NormalizeTaskStatusEvent(envelope.Event)
	if event.TaskID == "" || event.Status == "" {
		return events.TaskStatusEvent{}, false, fmt.Errorf("decode task status envelope: task_id and status are required")
	}
	return event, true, nil
}

func (b *NATSJetStreamEventBus) PublishTaskStatus(ctx context.Context, event events.TaskStatusEvent) error {
	if b == nil || b.js == nil {
		return nil
	}
	event = events.NormalizeTaskStatusEvent(event)
	return publishBridgedEvent(b, ctx, taskStatusBridgeCodec, event, marshalTaskStatusForPublish, "publish task status event")
}

func (b *NATSJetStreamEventBus) SubscribeTaskStatus(ctx context.Context, handler events.TaskStatusHandler) error {
	if b == nil || b.js == nil {
		return nil
	}
	if handler == nil {
		return fmt.Errorf("subscribe task status: handler is nil")
	}

	return subscribeNATSBridge(b, ctx, natsBridgeSubscribeConfig[events.TaskStatusEvent]{
		consumerName: b.instanceID, subject: b.taskStatusSubject,
		description:    "IdeaBoard API task status fanout bridge",
		ensureOp:       "ensure nats task status consumer",
		startOp:        "start nats task status consumer",
		malformedLog:   "discard malformed task status event",
		localAckLog:    "ack local-origin task status event failed",
		handlerFailLog: "task status handler failed",
		ackFailLog:     "ack task status event failed",
		consumerErrLog: "task status consumer error",
		decode:         decodeTaskStatusForInstance,
		handler:        handler,
		logAttrs: func(event events.TaskStatusEvent) []any {
			return []any{"task_id", event.TaskID, "status", event.Status}
		},
	})
}

// Workspace v1 envelopes ride the same JetStream stream (7-day MaxAge)
// under <prefix>.workspace.event. The transport is the event layer, NOT
// a task queue: River + PostgreSQL stay the task state fact source.
// MsgID is the envelope evt_ id so redeliveries collapse in the
// duplicate window; consumers additionally dedup on id.
type workspaceV1Envelope struct {
	OriginInstance string            `json:"origin_instance"`
	Envelope       events.V1Envelope `json:"envelope"`
}

// PublishWorkspaceEvent publishes a validated v1 envelope best-effort.
// Nil buses and unconfigured clients are no-ops (single-process tests).
func (b *NATSJetStreamEventBus) PublishWorkspaceEvent(ctx context.Context, envelope events.V1Envelope) error {
	if b == nil || b.js == nil {
		return nil
	}
	return publishBridgedEvent(b, ctx, workspaceEventBridgeCodec, envelope, marshalWorkspaceEventForPublish, "publish workspace event")
}

func decodeWorkspaceEventForInstance(payload []byte, instanceID string) (events.V1Envelope, bool, error) {
	var wrapped workspaceV1Envelope
	if err := json.Unmarshal(payload, &wrapped); err != nil {
		return events.V1Envelope{}, false, fmt.Errorf("decode workspace event envelope: %w", err)
	}
	wrapped.OriginInstance = strings.TrimSpace(wrapped.OriginInstance)
	if wrapped.OriginInstance == "" {
		return events.V1Envelope{}, false, fmt.Errorf("decode workspace event envelope: origin_instance is required")
	}
	if wrapped.OriginInstance == strings.TrimSpace(instanceID) {
		return events.V1Envelope{}, false, nil
	}
	if err := events.ValidateV1Envelope(wrapped.Envelope); err != nil {
		return events.V1Envelope{}, false, err
	}
	return wrapped.Envelope, true, nil
}

// SubscribeWorkspaceEvents bridges remote-origin v1 envelopes to the
// handler (local-origin messages ack without delivery).
func (b *NATSJetStreamEventBus) SubscribeWorkspaceEvents(ctx context.Context, handler events.WorkspaceEventHandler) error {
	if b == nil || b.js == nil {
		return nil
	}
	if handler == nil {
		return fmt.Errorf("subscribe workspace events: handler is nil")
	}
	return subscribeNATSBridge(b, ctx, natsBridgeSubscribeConfig[events.V1Envelope]{
		consumerName: b.instanceID + "-workspace-events", subject: b.workspaceSubject,
		description:    "IdeaBoard API workspace event bridge",
		ensureOp:       "ensure nats workspace event consumer",
		startOp:        "start nats workspace event consumer",
		malformedLog:   "discard malformed workspace event",
		localAckLog:    "ack local-origin workspace event failed",
		handlerFailLog: "workspace event handler failed",
		ackFailLog:     "ack workspace event failed",
		consumerErrLog: "workspace event consumer error",
		decode:         decodeWorkspaceEventForInstance,
		handler:        handler,
		logAttrs: func(envelope events.V1Envelope) []any {
			return []any{"id", envelope.ID, "type", envelope.Type}
		},
	})
}

func (b *NATSJetStreamEventBus) Close() error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	consumes := append([]jetstream.ConsumeContext(nil), b.consumes...)
	b.consumes = nil
	b.mu.Unlock()

	for _, consume := range consumes {
		consume.Drain()
		select {
		case <-consume.Closed():
		case <-time.After(2 * time.Second):
			consume.Stop()
		}
	}
	if b.nc != nil {
		err := b.nc.Drain()
		b.nc.Close()
		if err != nil && !errors.Is(err, nats.ErrConnectionClosed) {
			return err
		}
	}
	return nil
}

// IsConnected reports whether the underlying NATS connection is currently connected.
func (b *NATSJetStreamEventBus) IsConnected() bool {
	if b == nil || b.nc == nil {
		return false
	}
	return b.nc.IsConnected()
}

func normalizeNATSJetStreamConfig(cfg NATSJetStreamConfig) NATSJetStreamConfig {
	cfg.URL = strings.TrimSpace(cfg.URL)
	cfg.StreamName = sanitizeJetStreamName(cfg.StreamName, defaultNATSStreamName)
	cfg.SubjectPrefix = normalizeSubjectPrefix(cfg.SubjectPrefix)
	cfg.ConsumerName = sanitizeJetStreamName(cfg.ConsumerName, defaultConsumerName())
	if cfg.ConnectTimeout <= 0 {
		cfg.ConnectTimeout = defaultNATSConnectTimeout
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = defaultNATSRequestTimeout
	}
	if cfg.DrainTimeout <= 0 {
		cfg.DrainTimeout = defaultNATSDrainTimeout
	}
	return cfg
}

func taskStatusSubject(prefix string) string {
	return normalizeSubjectPrefix(prefix) + "." + taskStatusSubjectSuffix
}

func userEventSubject(prefix string) string {
	return normalizeSubjectPrefix(prefix) + "." + userEventSubjectSuffix
}

func workspaceEventSubject(prefix string) string {
	return normalizeSubjectPrefix(prefix) + "." + workspaceEventSubjectSuffix
}

func normalizeSubjectPrefix(raw string) string {
	prefix := strings.Trim(strings.TrimSpace(raw), ".")
	if prefix == "" {
		return defaultNATSSubjectPrefix
	}
	parts := strings.Split(prefix, ".")
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		part = sanitizeSubjectToken(part)
		if part != "" {
			cleaned = append(cleaned, part)
		}
	}
	if len(cleaned) == 0 {
		return defaultNATSSubjectPrefix
	}
	return strings.Join(cleaned, ".")
}

func sanitizeSubjectToken(raw string) string {
	raw = strings.TrimSpace(raw)
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-':
			b.WriteRune(r)
		}
	}
	return b.String()
}

func sanitizeJetStreamName(raw, fallback string) string {
	raw = strings.TrimSpace(raw)
	var b strings.Builder
	lastUnderscore := false
	for _, r := range raw {
		allowed := (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '_' ||
			r == '-'
		if allowed {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	name := strings.Trim(b.String(), "_-")
	if name == "" && raw != fallback {
		return sanitizeJetStreamName(fallback, defaultNATSStreamName)
	}
	if name == "" {
		return defaultNATSStreamName
	}
	return name
}

func defaultConsumerName() string {
	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		return defaultNATSConsumerPrefix
	}
	return defaultNATSConsumerPrefix + "-" + hostname
}
