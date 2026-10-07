package events

// Workspace hot event log (issue #807).
//
// The log is the process-local replay window behind
// GET /api/v1/workspaces/{workspace_id}/events (SSE): Append stores the
// envelope (idempotent on evt_ id) and broadcasts to live subscribers;
// ReplayAfter resumes a Last-Event-ID cursor; Subscribe streams live
// envelopes filtered by type.
//
// Retention/replay boundaries: NATS JetStream MaxAge (7 days per DESIGN)
// is the authoritative retention; this log is a hot window capped at
// maxPerWorkspace envelopes per workspace (oldest-first eviction). A
// cursor that names an evicted or unknown id resumes from the oldest
// retained envelope with gap=true so the client refetches state instead
// of silently missing events. Task status events can be replayed from the
// durable task-status outbox; other event types remain bounded by this hot
// window and the NATS retention policy.

import (
	"fmt"
	"strings"
	"sync"
)

const (
	defaultWorkspaceLogMaxPerWorkspace = 1000
	defaultWorkspaceLogMaxSubscribers  = 256
)

// WorkspaceEventFilter narrows replay + live delivery to a subset of
// frozen types. Empty means all types.
type WorkspaceEventFilter struct {
	Types map[string]struct{}
}

// NewWorkspaceEventFilter builds a filter; unknown types fail closed.
func NewWorkspaceEventFilter(types []string) (WorkspaceEventFilter, error) {
	filter := WorkspaceEventFilter{Types: map[string]struct{}{}}
	for _, typ := range types {
		trimmed := strings.TrimSpace(typ)
		if trimmed == "" {
			continue
		}
		if !IsV1EventType(trimmed) {
			return WorkspaceEventFilter{}, fmt.Errorf("events: unknown event type filter %q", trimmed)
		}
		filter.Types[trimmed] = struct{}{}
	}
	return filter, nil
}

// Matches reports whether typ passes the filter.
func (f WorkspaceEventFilter) Matches(typ string) bool {
	if len(f.Types) == 0 {
		return true
	}
	_, ok := f.Types[strings.TrimSpace(typ)]
	return ok
}

// Fingerprint binds a cursor to its filter set: replay with a different
// filter set than the cursor was issued under still resumes correctly
// (filters narrow, never widen), but subscribers record the fingerprint
// so mixed-filter sharing is observable.
func (f WorkspaceEventFilter) Fingerprint() string {
	if len(f.Types) == 0 {
		return "*"
	}
	names := make([]string, 0, len(f.Types))
	for typ := range f.Types {
		names = append(names, typ)
	}
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && names[j] < names[j-1]; j-- {
			names[j], names[j-1] = names[j-1], names[j]
		}
	}
	return strings.Join(names, ",")
}

type workspaceLogSubscription struct {
	filter  WorkspaceEventFilter
	channel chan V1Envelope
	done    chan struct{}
}

// WorkspaceEventLog is the hot per-workspace envelope window + live fanout.
type WorkspaceEventLog struct {
	mu           sync.RWMutex
	maxPer       int
	events       map[string][]V1Envelope
	ids          map[string]map[string]struct{}
	subscribers  map[string][]*workspaceLogSubscription
	subscriberCt int
}

// NewWorkspaceEventLog builds the production hot log.
func NewWorkspaceEventLog() *WorkspaceEventLog {
	return NewWorkspaceEventLogWithLimits(defaultWorkspaceLogMaxPerWorkspace, defaultWorkspaceLogMaxSubscribers)
}

// NewWorkspaceEventLogWithLimits builds a log with test-tunable bounds.
func NewWorkspaceEventLogWithLimits(maxPerWorkspace, maxSubscribers int) *WorkspaceEventLog {
	if maxPerWorkspace <= 0 {
		maxPerWorkspace = defaultWorkspaceLogMaxPerWorkspace
	}
	if maxSubscribers <= 0 {
		maxSubscribers = defaultWorkspaceLogMaxSubscribers
	}
	return &WorkspaceEventLog{
		maxPer: maxPerWorkspace, events: map[string][]V1Envelope{},
		ids: map[string]map[string]struct{}{}, subscribers: map[string][]*workspaceLogSubscription{},
		subscriberCt: maxSubscribers,
	}
}

// Append stores the envelope (idempotent on evt_ id: duplicates return
// the stored copy without fanout) and broadcasts to live subscribers.
func (l *WorkspaceEventLog) Append(envelope V1Envelope) V1Envelope {
	if l == nil {
		return envelope
	}
	if err := ValidateV1Envelope(envelope); err != nil {
		return envelope
	}
	l.mu.Lock()
	workspace := strings.TrimSpace(envelope.WorkspaceID)
	if l.ids[workspace] == nil {
		l.ids[workspace] = map[string]struct{}{}
	}
	if _, dup := l.ids[workspace][envelope.ID]; dup {
		for _, stored := range l.events[workspace] {
			if stored.ID == envelope.ID {
				l.mu.Unlock()
				return stored
			}
		}
		l.mu.Unlock()
		return envelope
	}
	l.ids[workspace][envelope.ID] = struct{}{}
	l.events[workspace] = append(l.events[workspace], envelope)
	for len(l.events[workspace]) > l.maxPer {
		evicted := l.events[workspace][0]
		l.events[workspace] = append([]V1Envelope(nil), l.events[workspace][1:]...)
		delete(l.ids[workspace], evicted.ID)
	}
	subs := append([]*workspaceLogSubscription(nil), l.subscribers[workspace]...)
	l.mu.Unlock()
	for _, sub := range subs {
		if !sub.filter.Matches(envelope.Type) {
			continue
		}
		select {
		case <-sub.done:
			continue
		default:
		}
		select {
		case sub.channel <- envelope:
		default:
			select {
			case <-sub.channel:
			default:
			}
			select {
			case sub.channel <- envelope:
			default:
			}
		}
	}
	return envelope
}

// ReplayAfter returns envelopes after cursor (exclusive) matching the
// filter. Empty cursor means live-only (no replay). Unknown or evicted
// cursor ids resume from the oldest retained envelope with gap=true.
// found=false means the cursor named no retained envelope.
func (l *WorkspaceEventLog) ReplayAfter(workspaceID, cursor string, filter WorkspaceEventFilter) (missed []V1Envelope, gap, found bool) {
	if l == nil {
		return nil, false, false
	}
	workspace := strings.TrimSpace(workspaceID)
	cursor = strings.TrimSpace(cursor)
	l.mu.RLock()
	stored := append([]V1Envelope(nil), l.events[workspace]...)
	l.mu.RUnlock()
	if cursor == "" {
		return nil, false, true
	}
	start := -1
	for i, envelope := range stored {
		if envelope.ID == cursor {
			start = i
			break
		}
	}
	if start < 0 {
		for _, envelope := range stored {
			if filter.Matches(envelope.Type) {
				missed = append(missed, envelope)
			}
		}
		return missed, true, false
	}
	for _, envelope := range stored[start+1:] {
		if filter.Matches(envelope.Type) {
			missed = append(missed, envelope)
		}
	}
	return missed, false, true
}

// Subscribe streams live envelopes for the workspace+filter. The channel
// is bounded (16) with drop-oldest delivery, mirroring UserEventHub.
func (l *WorkspaceEventLog) Subscribe(workspaceID string, filter WorkspaceEventFilter) (<-chan V1Envelope, func(), bool) {
	if l == nil || strings.TrimSpace(workspaceID) == "" {
		return nil, nil, false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	total := 0
	for _, subs := range l.subscribers {
		total += len(subs)
	}
	if total >= l.subscriberCt {
		return nil, nil, false
	}
	sub := &workspaceLogSubscription{filter: filter, channel: make(chan V1Envelope, 16), done: make(chan struct{})}
	workspace := strings.TrimSpace(workspaceID)
	l.subscribers[workspace] = append(l.subscribers[workspace], sub)
	return sub.channel, func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		select {
		case <-sub.done:
			return
		default:
		}
		close(sub.done)
		entries := l.subscribers[workspace]
		for i, entry := range entries {
			if entry != sub {
				continue
			}
			l.subscribers[workspace] = append(entries[:i], entries[i+1:]...)
			break
		}
		if len(l.subscribers[workspace]) == 0 {
			delete(l.subscribers, workspace)
		}
	}, true
}

// Len reports retained envelopes for the workspace (tests/diagnostics).
func (l *WorkspaceEventLog) Len(workspaceID string) int {
	if l == nil {
		return 0
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.events[strings.TrimSpace(workspaceID)])
}
