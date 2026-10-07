package events

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
	OriginInstance string    `json:"origin_instance"`
	Event          UserEvent `json:"event"`
}

func decodeUserEventForInstance(payload []byte, instanceID string) (UserEvent, bool, error) {
	var envelope userEventEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return UserEvent{}, false, fmt.Errorf("decode user event envelope: %w", err)
	}
	envelope.OriginInstance = strings.TrimSpace(envelope.OriginInstance)
	envelope.Event.Type = strings.TrimSpace(envelope.Event.Type)
	if envelope.OriginInstance == "" {
		return UserEvent{}, false, fmt.Errorf("decode user event envelope: origin_instance is required")
	}
	if envelope.Event.UserID <= 0 || envelope.Event.Type == "" {
		return UserEvent{}, false, fmt.Errorf("decode user event envelope: user_id and type are required")
	}
	if envelope.OriginInstance == strings.TrimSpace(instanceID) {
		return UserEvent{}, false, nil
	}
	return envelope.Event, true, nil
}

func (b *NATSJetStreamEventBus) PublishUserEvent(ctx context.Context, event UserEvent) error {
	if b == nil || b.js == nil {
		return nil
	}
	if event.UserID <= 0 || strings.TrimSpace(event.Type) == "" {
		return fmt.Errorf("publish user event: user_id and type are required")
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}
	payload, err := json.Marshal(userEventEnvelope{OriginInstance: b.instanceID, Event: event})
	if err != nil {
		return fmt.Errorf("marshal user event: %w", err)
	}
	msgID := fmt.Sprintf("%d:%s:%d", event.UserID, event.Type, event.OccurredAt.UnixNano())
	return b.publishBridgeMessage(ctx, b.userEventSubject, msgID, payload, "publish user event")
}

func (b *NATSJetStreamEventBus) SubscribeUserEvents(ctx context.Context, handler UserEventHandler) error {
	if b == nil || b.js == nil {
		return nil
	}
	if handler == nil {
		return fmt.Errorf("subscribe user events: handler is nil")
	}
	return subscribeNATSBridge(b, ctx, natsBridgeSubscribeConfig[UserEvent]{
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
		logAttrs: func(event UserEvent) []any {
			return []any{"user_id", event.UserID, "type", event.Type}
		},
	})
}

type taskStatusEnvelope struct {
	OriginInstance string          `json:"origin_instance"`
	Event          TaskStatusEvent `json:"event"`
}

func decodeTaskStatusForInstance(payload []byte, instanceID string) (TaskStatusEvent, bool, error) {
	var envelope taskStatusEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return TaskStatusEvent{}, false, fmt.Errorf("decode task status envelope: %w", err)
	}
	envelope.OriginInstance = strings.TrimSpace(envelope.OriginInstance)
	if envelope.OriginInstance == "" {
		return TaskStatusEvent{}, false, fmt.Errorf("decode task status envelope: origin_instance is required")
	}
	if envelope.OriginInstance == instanceID {
		return TaskStatusEvent{}, false, nil
	}
	event := NormalizeTaskStatusEvent(envelope.Event)
	if event.TaskID == "" || event.Status == "" {
		return TaskStatusEvent{}, false, fmt.Errorf("decode task status envelope: task_id and status are required")
	}
	return event, true, nil
}

func (b *NATSJetStreamEventBus) PublishTaskStatus(ctx context.Context, event TaskStatusEvent) error {
	if b == nil || b.js == nil {
		return nil
	}
	event = NormalizeTaskStatusEvent(event)
	if event.TaskID == "" || event.Status == "" {
		return fmt.Errorf("publish task status: task_id and status are required")
	}
	payload, err := json.Marshal(taskStatusEnvelope{
		OriginInstance: b.instanceID,
		Event:          event,
	})
	if err != nil {
		return fmt.Errorf("marshal task status event: %w", err)
	}
	msgID := event.EventID
	if strings.TrimSpace(msgID) == "" {
		msgID = fmt.Sprintf("%s:%s:%d", event.TaskID, event.Status, event.OccurredAt.UnixNano())
	}
	return b.publishBridgeMessage(ctx, b.taskStatusSubject, msgID, payload, "publish task status event")
}

func (b *NATSJetStreamEventBus) SubscribeTaskStatus(ctx context.Context, handler TaskStatusHandler) error {
	if b == nil || b.js == nil {
		return nil
	}
	if handler == nil {
		return fmt.Errorf("subscribe task status: handler is nil")
	}

	return subscribeNATSBridge(b, ctx, natsBridgeSubscribeConfig[TaskStatusEvent]{
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
		logAttrs: func(event TaskStatusEvent) []any {
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
	OriginInstance string     `json:"origin_instance"`
	Envelope       V1Envelope `json:"envelope"`
}

// WorkspaceEventHandler handles a remote-origin v1 envelope.
type WorkspaceEventHandler func(ctx context.Context, envelope V1Envelope) error

// PublishWorkspaceEvent publishes a validated v1 envelope best-effort.
// Nil buses and unconfigured clients are no-ops (single-process tests).
func (b *NATSJetStreamEventBus) PublishWorkspaceEvent(ctx context.Context, envelope V1Envelope) error {
	if b == nil || b.js == nil {
		return nil
	}
	if err := ValidateV1Envelope(envelope); err != nil {
		return err
	}
	payload, err := json.Marshal(workspaceV1Envelope{
		OriginInstance: b.instanceID,
		Envelope:       envelope,
	})
	if err != nil {
		return fmt.Errorf("marshal workspace event: %w", err)
	}
	return b.publishBridgeMessage(ctx, b.workspaceSubject, envelope.ID, payload, "publish workspace event")
}

func decodeWorkspaceEventForInstance(payload []byte, instanceID string) (V1Envelope, bool, error) {
	var wrapped workspaceV1Envelope
	if err := json.Unmarshal(payload, &wrapped); err != nil {
		return V1Envelope{}, false, fmt.Errorf("decode workspace event envelope: %w", err)
	}
	wrapped.OriginInstance = strings.TrimSpace(wrapped.OriginInstance)
	if wrapped.OriginInstance == "" {
		return V1Envelope{}, false, fmt.Errorf("decode workspace event envelope: origin_instance is required")
	}
	if wrapped.OriginInstance == strings.TrimSpace(instanceID) {
		return V1Envelope{}, false, nil
	}
	if err := ValidateV1Envelope(wrapped.Envelope); err != nil {
		return V1Envelope{}, false, err
	}
	return wrapped.Envelope, true, nil
}

// SubscribeWorkspaceEvents bridges remote-origin v1 envelopes to the
// handler (local-origin messages ack without delivery).
func (b *NATSJetStreamEventBus) SubscribeWorkspaceEvents(ctx context.Context, handler WorkspaceEventHandler) error {
	if b == nil || b.js == nil {
		return nil
	}
	if handler == nil {
		return fmt.Errorf("subscribe workspace events: handler is nil")
	}
	return subscribeNATSBridge(b, ctx, natsBridgeSubscribeConfig[V1Envelope]{
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
		logAttrs: func(envelope V1Envelope) []any {
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
