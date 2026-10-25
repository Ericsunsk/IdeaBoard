package notifications

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"ideaboard/api/internal/platform/contract"
)

// MemoryStore is an in-memory test double for Store.
type MemoryStore struct {
	deduplicated map[string]*Notification
	mu           sync.RWMutex
	items        []*Notification
	nextID       int64
}

var _ Store = (*MemoryStore)(nil)

// NewMemoryStore creates an empty in-memory notifications store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		items:        make([]*Notification, 0),
		deduplicated: make(map[string]*Notification),
		nextID:       1,
	}
}

func (m *MemoryStore) CreateNotification(ctx context.Context, input CreateNotificationInput) (*Notification, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%d:%s", input.UserID, input.DeduplicationKey)
	if input.DeduplicationKey != "" {
		if existing := m.deduplicated[key]; existing != nil {
			return cloneNotification(existing), nil
		}
	}

	publicID, err := contract.MintPublicID(contract.PrefixNotification, 12)
	if err != nil {
		return nil, fmt.Errorf("notifications: mint public id: %w", err)
	}

	payload := input.ActionPayload
	if payload == nil {
		payload = make(ActionPayload)
	}

	now := time.Now()
	notif := &Notification{
		InternalID:    m.nextID,
		PublicID:      publicID,
		UserID:        input.UserID,
		Type:          input.Type,
		Title:         input.Title,
		Body:          input.Body,
		ActionPayload: payload,
		Status:        StatusUnread,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	m.nextID++
	m.items = append(m.items, notif)
	if input.DeduplicationKey != "" {
		m.deduplicated[key] = notif
	}

	return cloneNotification(notif), nil
}

func (m *MemoryStore) ListNotifications(ctx context.Context, userID int64, filter ListFilter, limit, offset int32) ([]*Notification, int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var matched []*Notification
	for _, n := range m.items {
		if n.UserID != userID {
			continue
		}
		if !filter.matches(n) {
			continue
		}
		matched = append(matched, n)
	}

	// Sort created_at DESC, internal_id DESC
	sort.Slice(matched, func(i, j int) bool {
		if (matched[i].Status == StatusUnread) != (matched[j].Status == StatusUnread) {
			return matched[i].Status == StatusUnread
		}
		if matched[i].CreatedAt.Equal(matched[j].CreatedAt) {
			return matched[i].InternalID > matched[j].InternalID
		}
		return matched[i].CreatedAt.After(matched[j].CreatedAt)
	})

	total := int64(len(matched))
	start := int(offset)
	if start < 0 {
		start = 0
	}
	if start > len(matched) {
		return []*Notification{}, total, nil
	}

	end := start + int(limit)
	if limit <= 0 {
		end = len(matched)
	}
	if end > len(matched) {
		end = len(matched)
	}

	res := make([]*Notification, 0, end-start)
	for _, item := range matched[start:end] {
		res = append(res, cloneNotification(item))
	}

	return res, total, nil
}

func (m *MemoryStore) CountUnread(ctx context.Context, userID int64) (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var count int64
	for _, n := range m.items {
		if n.UserID == userID && n.Status == StatusUnread {
			count++
		}
	}
	return count, nil
}

func (m *MemoryStore) MarkRead(ctx context.Context, userID int64, publicID string) (*Notification, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for _, n := range m.items {
		if n.UserID == userID && n.PublicID == publicID {
			n.Status = StatusRead
			if n.ReadAt == nil {
				n.ReadAt = &now
			}
			n.UpdatedAt = now
			return cloneNotification(n), nil
		}
	}

	return nil, ErrNotFound
}

func (m *MemoryStore) MarkAllRead(ctx context.Context, userID int64) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var count int64
	now := time.Now()
	for _, n := range m.items {
		if n.UserID == userID && n.Status == StatusUnread {
			n.Status = StatusRead
			if n.ReadAt == nil {
				n.ReadAt = &now
			}
			n.UpdatedAt = now
			count++
		}
	}

	return count, nil
}

func (m *MemoryStore) Archive(ctx context.Context, userID int64, publicID string) (*Notification, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for _, n := range m.items {
		if n.UserID == userID && n.PublicID == publicID {
			n.Status = StatusArchived
			n.UpdatedAt = now
			return cloneNotification(n), nil
		}
	}

	return nil, ErrNotFound
}

func (m *MemoryStore) MarkInviteResolved(ctx context.Context, inviteID string, status string) (int64, error) {
	if inviteID == "" || status == "" {
		return 0, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	var count int64
	now := time.Now()
	for _, n := range m.items {
		if n.Type != TypeWorkspaceInvite && n.Type != TypeOrganizationInvite {
			continue
		}
		if n.ActionPayload == nil {
			continue
		}
		raw, ok := n.ActionPayload[PayloadKeyInviteID]
		if !ok {
			continue
		}
		id, ok := raw.(string)
		if !ok || id != inviteID {
			continue
		}
		if n.ActionPayload == nil {
			n.ActionPayload = make(ActionPayload)
		}
		n.ActionPayload[PayloadKeyInviteStatus] = status
		n.UpdatedAt = now
		count++
	}
	return count, nil
}

func cloneNotification(n *Notification) *Notification {
	if n == nil {
		return nil
	}
	cp := *n
	if n.ReadAt != nil {
		t := *n.ReadAt
		cp.ReadAt = &t
	}
	if n.ActionPayload != nil {
		cp.ActionPayload = make(ActionPayload, len(n.ActionPayload))
		for k, v := range n.ActionPayload {
			cp.ActionPayload[k] = v
		}
	}
	return &cp
}
