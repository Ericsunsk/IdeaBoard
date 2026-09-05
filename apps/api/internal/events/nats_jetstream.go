package events

import (
	"context"
	"encoding/json"
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
	defaultNATSStreamName      = "IDEABOARD_EVENTS"
	defaultNATSSubjectPrefix   = "ideaboard"
	defaultNATSRetention       = 7 * 24 * time.Hour
	defaultNATSConnectTimeout  = 5 * time.Second
	defaultNATSRequestTimeout  = 5 * time.Second
	defaultNATSDrainTimeout    = 2 * time.Second
	defaultNATSConsumerPrefix  = "ideaboard-api-events"
	taskStatusSubjectSuffix    = "task.status"
	userEventSubjectSuffix     = "user.event"
	natsDuplicateWindow        = 2 * time.Minute
	natsConsumerInactivePeriod = 24 * time.Hour
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
		instanceID:        cfg.ConsumerName,
	}, nil
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
	if _, err := b.js.Publish(ctx, b.userEventSubject, payload, jetstream.WithMsgID(msgID)); err != nil {
		return fmt.Errorf("publish user event: %w", err)
	}
	return nil
}

func (b *NATSJetStreamEventBus) SubscribeUserEvents(ctx context.Context, handler UserEventHandler) error {
	if b == nil || b.js == nil {
		return nil
	}
	if handler == nil {
		return fmt.Errorf("subscribe user events: handler is nil")
	}
	consumerName := b.instanceID + "-user-events"
	consumer, err := b.js.CreateOrUpdateConsumer(ctx, b.streamName, jetstream.ConsumerConfig{
		Name: consumerName, Durable: consumerName, Description: "IdeaBoard API user event fanout bridge",
		DeliverPolicy: jetstream.DeliverNewPolicy, AckPolicy: jetstream.AckExplicitPolicy,
		AckWait: 30 * time.Second, MaxDeliver: 5, MaxAckPending: 1024,
		FilterSubject: b.userEventSubject, InactiveThreshold: natsConsumerInactivePeriod,
	})
	if err != nil {
		return fmt.Errorf("ensure nats user event consumer %s: %w", consumerName, err)
	}
	consume, err := consumer.Consume(func(msg jetstream.Msg) {
		event, shouldDeliver, err := decodeUserEventForInstance(msg.Data(), b.instanceID)
		if err != nil {
			slog.Warn("discard malformed user event", "subject", msg.Subject(), "err", err)
			_ = msg.Term()
			return
		}
		if !shouldDeliver {
			_ = msg.Ack()
			return
		}
		handlerCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err = handler(handlerCtx, event)
		cancel()
		if err != nil {
			_ = msg.NakWithDelay(time.Second)
			return
		}
		_ = msg.Ack()
	}, jetstream.ConsumeErrHandler(func(_ jetstream.ConsumeContext, err error) {
		slog.Warn("user event consumer error", "err", err)
	}))
	if err != nil {
		return fmt.Errorf("start nats user event consumer %s: %w", consumerName, err)
	}
	b.mu.Lock()
	b.consumes = append(b.consumes, consume)
	b.mu.Unlock()
	return nil
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
	msgID := fmt.Sprintf("%s:%s:%d", event.TaskID, event.Status, event.OccurredAt.UnixNano())
	if _, err := b.js.Publish(ctx, b.taskStatusSubject, payload, jetstream.WithMsgID(msgID)); err != nil {
		return fmt.Errorf("publish task status event: %w", err)
	}
	return nil
}

func (b *NATSJetStreamEventBus) SubscribeTaskStatus(ctx context.Context, handler TaskStatusHandler) error {
	if b == nil || b.js == nil {
		return nil
	}
	if handler == nil {
		return fmt.Errorf("subscribe task status: handler is nil")
	}

	consumer, err := b.js.CreateOrUpdateConsumer(ctx, b.streamName, jetstream.ConsumerConfig{
		Name:              b.instanceID,
		Durable:           b.instanceID,
		Description:       "IdeaBoard API task status fanout bridge",
		DeliverPolicy:     jetstream.DeliverNewPolicy,
		AckPolicy:         jetstream.AckExplicitPolicy,
		AckWait:           30 * time.Second,
		MaxDeliver:        5,
		MaxAckPending:     1024,
		FilterSubject:     b.taskStatusSubject,
		InactiveThreshold: natsConsumerInactivePeriod,
	})
	if err != nil {
		return fmt.Errorf("ensure nats task status consumer %s: %w", b.instanceID, err)
	}

	consume, err := consumer.Consume(func(msg jetstream.Msg) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("NATS consume callback panicked", "subject", msg.Subject(), "panic", r)
				_ = msg.NakWithDelay(time.Second)
			}
		}()
		event, shouldDeliver, err := decodeTaskStatusForInstance(msg.Data(), b.instanceID)
		if err != nil {
			slog.Warn("discard malformed task status event", "subject", msg.Subject(), "err", err)
			_ = msg.Term()
			return
		}
		if !shouldDeliver {
			if err := msg.Ack(); err != nil {
				slog.Warn("ack local-origin task status event failed", "subject", msg.Subject(), "err", err)
			}
			return
		}
		handlerCtx, handlerCancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := handler(handlerCtx, event); err != nil {
			slog.Warn("task status handler failed", "task_id", event.TaskID, "status", event.Status, "err", err)
			handlerCancel()
			_ = msg.NakWithDelay(time.Second)
			return
		}
		handlerCancel()
		if err := msg.Ack(); err != nil {
			slog.Warn("ack task status event failed", "task_id", event.TaskID, "status", event.Status, "err", err)
		}
	}, jetstream.ConsumeErrHandler(func(_ jetstream.ConsumeContext, err error) {
		slog.Warn("task status consumer error", "err", err)
	}))
	if err != nil {
		return fmt.Errorf("start nats task status consumer %s: %w", b.instanceID, err)
	}

	b.mu.Lock()
	b.consumes = append(b.consumes, consume)
	b.mu.Unlock()
	return nil
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
		if err != nil && err != nats.ErrConnectionClosed {
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
