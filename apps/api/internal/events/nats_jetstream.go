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
	natsDuplicateWindow        = 2 * time.Minute
	natsConsumerInactivePeriod = 24 * time.Hour
)

type NATSJetStreamConfig struct {
	URL            string
	StreamName     string
	SubjectPrefix  string
	ConsumerName   string
	Retention      time.Duration
	ConnectTimeout time.Duration
	RequestTimeout time.Duration
	DrainTimeout   time.Duration
}

type NATSJetStreamEventBus struct {
	nc                *nats.Conn
	js                jetstream.JetStream
	streamName        string
	taskStatusSubject string
	consumerName      string

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
		MaxAge:       cfg.Retention,
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
		consumerName:      cfg.ConsumerName,
	}, nil
}

func (b *NATSJetStreamEventBus) PublishTaskStatus(ctx context.Context, event TaskStatusEvent) error {
	if b == nil || b.js == nil {
		return nil
	}
	event = NormalizeTaskStatusEvent(event)
	if event.TaskID == "" || event.Status == "" {
		return fmt.Errorf("publish task status: task_id and status are required")
	}
	payload, err := json.Marshal(event)
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
		Name:              b.consumerName,
		Durable:           b.consumerName,
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
		return fmt.Errorf("ensure nats task status consumer %s: %w", b.consumerName, err)
	}

	consume, err := consumer.Consume(func(msg jetstream.Msg) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("NATS consume callback panicked", "subject", msg.Subject(), "panic", r)
				_ = msg.NakWithDelay(time.Second)
			}
		}()
		var event TaskStatusEvent
		if err := json.Unmarshal(msg.Data(), &event); err != nil {
			slog.Warn("discard malformed task status event", "subject", msg.Subject(), "err", err)
			_ = msg.Term()
			return
		}
		event = NormalizeTaskStatusEvent(event)
		if event.TaskID == "" || event.Status == "" {
			slog.Warn("discard incomplete task status event", "subject", msg.Subject(), "task_id", event.TaskID, "status", event.Status)
			_ = msg.Term()
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
		return fmt.Errorf("start nats task status consumer %s: %w", b.consumerName, err)
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

func normalizeNATSJetStreamConfig(cfg NATSJetStreamConfig) NATSJetStreamConfig {
	cfg.URL = strings.TrimSpace(cfg.URL)
	cfg.StreamName = sanitizeJetStreamName(cfg.StreamName, defaultNATSStreamName)
	cfg.SubjectPrefix = normalizeSubjectPrefix(cfg.SubjectPrefix)
	cfg.ConsumerName = sanitizeJetStreamName(cfg.ConsumerName, defaultConsumerName())
	if cfg.Retention <= 0 {
		cfg.Retention = defaultNATSRetention
	}
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
