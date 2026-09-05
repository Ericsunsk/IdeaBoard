package profiles

import (
	"context"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/media/storage/domain"
	"ideaboard/api/internal/media/storage/ports"
	"ideaboard/api/internal/platform/database"
)

const (
	storageSecretAccessKeyName = "secret_access_key"
	builtinLocalProfileID      = "builtin-local"
)

type DriverResolver interface {
	GetDriver(provider domain.StorageProvider) (ports.StorageDriver, error)
}

type DriverResolverFunc func(provider domain.StorageProvider) (ports.StorageDriver, error)

func (f DriverResolverFunc) GetDriver(provider domain.StorageProvider) (ports.StorageDriver, error) {
	return f(provider)
}

type Service struct {
	repo    ports.ProfileRepository
	drivers DriverResolver
	meta    map[domain.StorageProvider]domain.StorageProviderMeta
}

func NewService(repo ports.ProfileRepository, drivers DriverResolver) *Service {
	return &Service{
		repo:    repo,
		drivers: drivers,
		meta:    DefaultProviderMeta(),
	}
}

func (s *Service) ListProviderMeta() []domain.StorageProviderMeta {
	return ListProviderMeta(s.meta)
}

func (s *Service) ListProfiles(ctx context.Context) ([]domain.StorageProfile, error) {
	profiles, err := s.loadProfiles(ctx)
	if err != nil {
		return nil, err
	}
	return s.maskProfiles(profiles), nil
}

func (s *Service) GetProfile(ctx context.Context, id string) (*domain.StorageProfile, error) {
	profiles, err := s.loadProfiles(ctx)
	if err != nil {
		return nil, err
	}
	for _, profile := range profiles {
		if profile.ID == id {
			masked := s.maskProfile(profile)
			return &masked, nil
		}
	}
	return nil, database.ErrNotFound
}

func (s *Service) SaveProfile(ctx context.Context, profile *domain.StorageProfile) (*domain.StorageProfile, error) {
	return s.saveProfile(ctx, profile, false)
}

func (s *Service) SaveAndApplyProfile(ctx context.Context, profile *domain.StorageProfile) (*domain.StorageProfile, error) {
	return s.saveProfile(ctx, profile, true)
}

func (s *Service) saveProfile(ctx context.Context, profile *domain.StorageProfile, apply bool) (*domain.StorageProfile, error) {
	if profile == nil {
		return nil, fmt.Errorf("存储配置不能为空")
	}

	profiles, err := s.LoadProfilesWithSecrets(ctx)
	if err != nil {
		return nil, err
	}

	existingIndex, existing, err := s.resolveProfileSaveTarget(profiles, profile)
	if err != nil {
		return nil, err
	}
	next, rotateSecret, err := s.prepareProfileSave(ctx, *existing, *profile, existingIndex, apply)
	if err != nil {
		return nil, err
	}
	if err := s.persistProfile(ctx, next, apply, rotateSecret); err != nil {
		return nil, err
	}

	masked := s.maskProfile(next)
	return &masked, nil
}

func (s *Service) resolveProfileSaveTarget(profiles []domain.StorageProfile, profile *domain.StorageProfile) (int, *domain.StorageProfile, error) {
	meta, ok := s.meta[profile.Provider]
	if !ok {
		return -1, nil, fmt.Errorf("不支持的存储类型")
	}

	for i := range profiles {
		if profiles[i].ID != profile.ID {
			continue
		}
		if profiles[i].Provider != profile.Provider {
			return -1, nil, fmt.Errorf("不允许修改存储类型")
		}
		return i, &profiles[i], nil
	}

	if profile.ID == "" {
		profile.ID = fmt.Sprintf("%s-%d", profile.Provider, time.Now().UnixNano())
	}
	if profile.Provider == domain.StorageProviderLocal && !meta.CreateAllowed {
		return -1, nil, fmt.Errorf("本地存储不支持新增额外卡片")
	}
	return -1, &domain.StorageProfile{ID: profile.ID, Provider: profile.Provider}, nil
}

func (s *Service) prepareProfileSave(ctx context.Context, existing, incoming domain.StorageProfile, existingIndex int, apply bool) (domain.StorageProfile, bool, error) {
	rotateSecret := storageProfileHasRealSecret(incoming)
	next := s.normalizeProfile(existing, incoming)
	if apply {
		next.IsCurrent = true
	}
	if err := s.validateProfile(next); err != nil {
		return domain.StorageProfile{}, false, err
	}
	if next.IsCurrent && !next.Enabled {
		return domain.StorageProfile{}, false, fmt.Errorf("当前存储不允许停用")
	}
	if apply {
		if err := s.EnsureProfileReadyForRuntime(ctx, next); err != nil {
			return domain.StorageProfile{}, false, err
		}
	}
	if existingIndex < 0 && next.IsCurrent && !apply {
		next.IsCurrent = false
	}
	return next, rotateSecret, nil
}

func (s *Service) persistProfile(ctx context.Context, profile domain.StorageProfile, apply, rotateSecret bool) error {
	if apply {
		return s.repo.SaveAndApplyProfile(ctx, profile)
	}
	return s.repo.SaveProfile(ctx, profile)
}

func (s *Service) DeleteProfile(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	profiles, err := s.loadProfiles(ctx)
	if err != nil {
		return err
	}

	for _, profile := range profiles {
		if profile.ID != id {
			continue
		}
		if profile.Builtin {
			return fmt.Errorf("内置存储不允许删除")
		}
		if profile.IsCurrent {
			return fmt.Errorf("当前存储不允许删除")
		}
		return s.repo.DeleteProfile(ctx, id)
	}
	return database.ErrNotFound
}

func (s *Service) ApplyProfile(ctx context.Context, id string) (*domain.StorageProfile, error) {
	id = strings.TrimSpace(id)
	profiles, err := s.LoadProfilesWithSecrets(ctx)
	if err != nil {
		return nil, err
	}

	var target *domain.StorageProfile
	for i := range profiles {
		if profiles[i].ID == id {
			target = &profiles[i]
			break
		}
	}
	if target == nil {
		return nil, database.ErrNotFound
	}

	if err := s.EnsureProfileReadyForRuntime(ctx, *target); err != nil {
		return nil, err
	}

	if err := s.repo.ApplyProfile(ctx, id); err != nil {
		return nil, err
	}

	target.IsCurrent = true
	masked := s.maskProfile(*target)
	return &masked, nil
}

func (s *Service) TestProfile(ctx context.Context, id string) error {
	profile, err := s.ResolveProfileByID(ctx, id)
	if err != nil {
		return err
	}
	driver, err := s.drivers.GetDriver(profile.Provider)
	if err != nil {
		return err
	}
	return driver.TestProfile(ctx, profile)
}

func (s *Service) GetBucketInfo(ctx context.Context, id string) (map[string]interface{}, error) {
	profile, err := s.ResolveProfileByID(ctx, id)
	if err != nil {
		return nil, err
	}
	driver, err := s.drivers.GetDriver(profile.Provider)
	if err != nil {
		return nil, err
	}
	return driver.GetBucketInfo(ctx, profile)
}

func (s *Service) EnsureProfileReadyForRuntime(ctx context.Context, profile domain.StorageProfile) error {
	driver, err := s.drivers.GetDriver(profile.Provider)
	if err != nil {
		return err
	}
	if !driver.IsProfileReady(profile) {
		return fmt.Errorf("存储未启用或未配置完整")
	}
	return driver.TestProfile(ctx, &profile)
}

func (s *Service) ResolveProfileByID(ctx context.Context, id string) (*domain.StorageProfile, error) {
	id = strings.TrimSpace(id)
	profiles, err := s.LoadProfilesWithSecrets(ctx)
	if err != nil {
		return nil, err
	}
	return s.findProfile(profiles, id)
}

func (s *Service) ResolveActiveProfile(ctx context.Context) (*domain.StorageProfile, error) {
	profiles, err := s.LoadProfilesWithSecrets(ctx)
	if err != nil {
		return nil, err
	}
	for i := range profiles {
		if profiles[i].IsCurrent {
			return &profiles[i], nil
		}
	}
	for i := range profiles {
		if profiles[i].ID == builtinLocalProfileID {
			return &profiles[i], nil
		}
	}
	if len(profiles) > 0 {
		return &profiles[0], nil
	}
	return nil, fmt.Errorf("未找到可用存储配置")
}

func (s *Service) loadProfiles(ctx context.Context) ([]domain.StorageProfile, error) {
	profiles, err := s.repo.ListProfiles(ctx, false)
	if err != nil {
		return nil, err
	}
	if len(profiles) == 0 {
		return []domain.StorageProfile{s.DefaultLocalStorageProfile(true)}, nil
	}
	return normalizeLoadedProfiles(profiles), nil
}

func (s *Service) LoadProfilesWithSecrets(ctx context.Context) ([]domain.StorageProfile, error) {
	profiles, err := s.repo.ListProfilesWithSecrets(ctx, false)
	if err != nil {
		return nil, err
	}
	if len(profiles) == 0 {
		return []domain.StorageProfile{s.DefaultLocalStorageProfile(true)}, nil
	}
	return normalizeLoadedProfiles(profiles), nil
}

func (s *Service) LoadProfileForReclamation(ctx context.Context, id string) (*domain.StorageProfile, error) {
	profiles, err := s.repo.ListProfilesWithSecrets(ctx, true)
	if err != nil {
		return nil, err
	}
	return s.findProfile(normalizeLoadedProfiles(profiles), strings.TrimSpace(id))
}

func (s *Service) EnsureDefaultProfiles(ctx context.Context) error {
	profiles, err := s.repo.ListProfiles(ctx, false)
	if err != nil {
		return err
	}

	currentExists := false
	localProfile := s.DefaultLocalStorageProfile(false)
	for _, profile := range profiles {
		if profile.IsCurrent {
			currentExists = true
		}
		if profile.ID == builtinLocalProfileID {
			localProfile = s.DefaultLocalStorageProfile(profile.IsCurrent)
		}
	}

	if !currentExists {
		localProfile.IsCurrent = true
		return s.repo.SaveAndApplyProfile(ctx, localProfile)
	}
	return s.repo.SaveProfile(ctx, localProfile)
}

func (s *Service) DefaultLocalStorageProfile(isCurrent bool) domain.StorageProfile {
	return domain.StorageProfile{
		ID:        builtinLocalProfileID,
		Name:      "本地存储",
		Provider:  domain.StorageProviderLocal,
		Enabled:   true,
		IsCurrent: isCurrent,
		Builtin:   true,
		Config:    map[string]interface{}{},
	}
}

func normalizeLoadedProfiles(profiles []domain.StorageProfile) []domain.StorageProfile {
	for i := range profiles {
		if profiles[i].Config == nil {
			profiles[i].Config = map[string]interface{}{}
		}
	}
	return profiles
}

func (s *Service) normalizeProfile(existing domain.StorageProfile, incoming domain.StorageProfile) domain.StorageProfile {
	next := existing
	next.ID = strings.TrimSpace(incoming.ID)
	if next.ID == "" {
		next.ID = existing.ID
	}
	next.Provider = incoming.Provider
	next.Name = strings.TrimSpace(incoming.Name)
	if next.Name == "" {
		switch next.Provider {
		case domain.StorageProviderLocal:
			next.Name = "本地存储"
		case domain.StorageProviderS3Compatible:
			next.Name = "S3 兼容存储"
		default:
			next.Name = "存储"
		}
	}
	next.Enabled = incoming.Enabled
	next.IsCurrent = existing.IsCurrent
	next.Builtin = existing.Builtin
	if next.ID == builtinLocalProfileID {
		next.Enabled = true
		next.Builtin = true
		next.Provider = domain.StorageProviderLocal
	}

	cfg := cloneConfig(existing.Config)
	for key, value := range incoming.Config {
		cfg[key] = value
	}

	switch next.Provider {
	case domain.StorageProviderLocal:
		next.Config = map[string]interface{}{}
	case domain.StorageProviderS3Compatible:
		secret := strings.TrimSpace(configString(incoming.Config, storageSecretAccessKeyName))
		if secret == "" || strings.Contains(secret, "*") {
			if oldSecret := configString(existing.Config, storageSecretAccessKeyName); oldSecret != "" {
				cfg[storageSecretAccessKeyName] = oldSecret
			} else {
				delete(cfg, storageSecretAccessKeyName)
			}
		} else {
			cfg[storageSecretAccessKeyName] = secret
		}
		next.Config = cfg
	default:
		next.Config = cfg
	}

	return next
}

func (s *Service) validateProfile(profile domain.StorageProfile) error {
	if strings.TrimSpace(profile.Name) == "" {
		return fmt.Errorf("存储名称不能为空")
	}
	if _, ok := s.meta[profile.Provider]; !ok {
		return fmt.Errorf("不支持的存储类型")
	}
	return nil
}

func (s *Service) findProfile(profiles []domain.StorageProfile, id string) (*domain.StorageProfile, error) {
	for i := range profiles {
		if profiles[i].ID == id {
			return &profiles[i], nil
		}
	}
	return nil, database.ErrNotFound
}

func (s *Service) maskProfiles(profiles []domain.StorageProfile) []domain.StorageProfile {
	result := make([]domain.StorageProfile, 0, len(profiles))
	for _, profile := range profiles {
		result = append(result, s.maskProfile(profile))
	}
	return result
}

func (s *Service) maskProfile(profile domain.StorageProfile) domain.StorageProfile {
	masked := profile
	masked.Config = cloneConfig(profile.Config)
	switch masked.Provider {
	case domain.StorageProviderS3Compatible:
		secret := configString(masked.Config, storageSecretAccessKeyName)
		masked.SecretConfigured = masked.SecretConfigured || secret != ""
		if secret != "" {
			masked.Config[storageSecretAccessKeyName] = maskSecret(secret)
		}
	}
	return masked
}

func maskSecret(secret string) string {
	if len(secret) <= 6 {
		return "***"
	}
	return secret[:3] + "***" + secret[len(secret)-3:]
}

func cloneConfig(cfg map[string]interface{}) map[string]interface{} {
	if cfg == nil {
		return map[string]interface{}{}
	}
	cloned := make(map[string]interface{}, len(cfg))
	for k, v := range cfg {
		cloned[k] = v
	}
	return cloned
}

func configString(cfg map[string]interface{}, key string) string {
	if cfg == nil {
		return ""
	}
	if val, ok := cfg[key]; ok {
		switch typed := val.(type) {
		case string:
			return typed
		case fmt.Stringer:
			return typed.String()
		}
	}
	return ""
}

func storageProfileHasRealSecret(profile domain.StorageProfile) bool {
	if profile.Provider != domain.StorageProviderS3Compatible {
		return false
	}
	secret := strings.TrimSpace(configString(profile.Config, storageSecretAccessKeyName))
	return secret != "" && !strings.Contains(secret, "*")
}
