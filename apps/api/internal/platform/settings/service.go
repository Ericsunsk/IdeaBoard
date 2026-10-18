// Package settings owns the runtime system-config themes served by
// RuntimeSettingsService. Each theme lives in its own file so branding,
// outbound, and policy concerns stay single-column:
//
//	branding.go + branding_settings.go  project name, logo, support channels
//	outbound_policy.go                  egress allowlist (outbound)
//	rate_limit.go, login_lockout.go     request policy (policy)
//	registration.go, media_upload.go    remaining runtime themes
//	service.go                          shared service core (this file)
//	repository.go                       system_configs persistence seam
package settings

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"ideaboard/api/internal/db/dbutil"
)

type RuntimeSettingsService struct {
	repo           *SystemConfigRepository
	mu             sync.RWMutex
	rateLimitCache *cachedRateLimitSetting
}

type cachedRateLimitSetting struct {
	setting   RateLimitSetting
	expiresAt time.Time
}

var ErrRuntimeSettingsRepositoryUnavailable = errors.New("runtime settings repository unavailable")

const runtimeSettingsCacheTTL = 5 * time.Second

func NewRuntimeSettingsService(repo *SystemConfigRepository) *RuntimeSettingsService {
	return &RuntimeSettingsService{repo: repo}
}

func (s *RuntimeSettingsService) getMany(ctx context.Context, keys ...string) (map[string]string, error) {
	if s == nil || s.repo == nil {
		return nil, ErrRuntimeSettingsRepositoryUnavailable
	}
	return s.repo.GetMany(ctx, keys)
}

func (s *RuntimeSettingsService) setMany(ctx context.Context, values []SystemConfigValue) error {
	if s == nil || s.repo == nil {
		return ErrRuntimeSettingsRepositoryUnavailable
	}
	return s.repo.SetMany(ctx, values)
}

func (s *RuntimeSettingsService) cachedRateLimit(now time.Time) (RateLimitSetting, bool) {
	if s == nil {
		return RateLimitSetting{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.rateLimitCache == nil || now.After(s.rateLimitCache.expiresAt) {
		return RateLimitSetting{}, false
	}
	return s.rateLimitCache.setting, true
}

func (s *RuntimeSettingsService) writeCachedRateLimit(setting RateLimitSetting) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rateLimitCache = &cachedRateLimitSetting{
		setting:   setting,
		expiresAt: time.Now().Add(runtimeSettingsCacheTTL),
	}
}

// publishRateLimit refreshes the in-memory entry only when the database
// write already committed. Inside an ambient (guard-owned) transaction
// the rows are still uncommitted when SetMany returns, so caching the
// new value there would serve rolled-back data until the TTL expires;
// invalidate instead and let the next read reload committed state from
// the database. With no carrier SetMany committed before returning, so
// the write-through below is already post-commit and stays.
func (s *RuntimeSettingsService) publishRateLimit(ctx context.Context, setting RateLimitSetting) {
	if _, ok := dbutil.AmbientTxFromContext(ctx); ok {
		s.invalidateCachedRateLimit()
		return
	}
	s.writeCachedRateLimit(setting)
}

// invalidateCachedRateLimit drops the entry so the next read falls
// through to the database. Invalidation (unlike a value write) is safe
// before commit: on rollback the reload sees the old rows, on commit it
// sees the new rows, and on crash the empty cache reloads from disk.
func (s *RuntimeSettingsService) invalidateCachedRateLimit() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rateLimitCache = nil
}

func parseBoundedInt(raw string, min int, max int) (int, bool) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < min || value > max {
		return 0, false
	}
	return value, true
}
