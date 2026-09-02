package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"ideaboard/api/internal/platform/database"
)

type storageProfileLoadSecretPolicy int

const (
	storageProfileLoadMasked storageProfileLoadSecretPolicy = iota
	storageProfileLoadPlaintext
)

func (s *StorageService) ListProfiles(ctx context.Context) ([]StorageProfile, error) {
	profiles, err := s.loadProfiles(ctx)
	if err != nil {
		return nil, err
	}
	return s.maskProfiles(profiles), nil
}

func (s *StorageService) GetProfile(ctx context.Context, id string) (*StorageProfile, error) {
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

func (s *StorageService) SaveProfile(ctx context.Context, profile *StorageProfile) (*StorageProfile, error) {
	return s.saveProfile(ctx, profile, false)
}

func (s *StorageService) SaveAndApplyProfile(ctx context.Context, profile *StorageProfile) (*StorageProfile, error) {
	return s.saveProfile(ctx, profile, true)
}

func (s *StorageService) saveProfile(ctx context.Context, profile *StorageProfile, apply bool) (*StorageProfile, error) {
	if profile == nil {
		return nil, fmt.Errorf("存储配置不能为空")
	}

	profiles, err := s.loadProfilesWithSecrets(ctx)
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

func (s *StorageService) resolveProfileSaveTarget(profiles []StorageProfile, profile *StorageProfile) (int, *StorageProfile, error) {
	profile.Provider = canonicalizeStorageProvider(profile.Provider)
	meta, ok := s.providerMeta[profile.Provider]
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
	if profile.Provider == StorageProviderLocal && !meta.CreateAllowed {
		return -1, nil, fmt.Errorf("本地存储不支持新增额外卡片")
	}
	return -1, &StorageProfile{ID: profile.ID, Provider: profile.Provider}, nil
}

func (s *StorageService) prepareProfileSave(ctx context.Context, existing, incoming StorageProfile, existingIndex int, apply bool) (StorageProfile, bool, error) {
	rotateSecret := storageProfileHasRealSecret(incoming)
	next := s.normalizeProfile(existing, incoming)
	if apply {
		next.IsCurrent = true
	}
	if err := s.validateProfile(next); err != nil {
		return StorageProfile{}, false, err
	}
	if next.IsCurrent && !next.Enabled {
		return StorageProfile{}, false, fmt.Errorf("当前存储不允许停用")
	}
	if apply {
		if err := s.ensureProfileReadyForRuntime(ctx, next); err != nil {
			return StorageProfile{}, false, err
		}
	}
	if existingIndex < 0 && next.IsCurrent && !apply {
		next.IsCurrent = false
	}
	return next, rotateSecret, nil
}

func (s *StorageService) persistProfile(ctx context.Context, profile StorageProfile, apply, rotateSecret bool) error {
	options := storageProfileSaveOptions{rotateSecret: rotateSecret}
	if apply {
		return s.profileRepo.saveAndApplyProfile(ctx, profile, options)
	}
	return s.profileRepo.saveProfile(ctx, profile, options)
}

func (s *StorageService) DeleteProfile(ctx context.Context, id string) error {
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
		return s.profileRepo.DeleteProfile(ctx, id)
	}
	return database.ErrNotFound
}

func (s *StorageService) ApplyProfile(ctx context.Context, id string) (*StorageProfile, error) {
	id = strings.TrimSpace(id)
	profiles, err := s.loadProfilesWithSecrets(ctx)
	if err != nil {
		return nil, err
	}

	var target *StorageProfile
	for i := range profiles {
		if profiles[i].ID == id {
			target = &profiles[i]
			break
		}
	}
	if target == nil {
		return nil, database.ErrNotFound
	}

	if err := s.ensureProfileReadyForRuntime(ctx, *target); err != nil {
		return nil, err
	}

	if err := s.profileRepo.ApplyProfile(ctx, id); err != nil {
		return nil, err
	}

	target.IsCurrent = true
	masked := s.maskProfile(*target)
	return &masked, nil
}

func (s *StorageService) loadProfiles(ctx context.Context) ([]StorageProfile, error) {
	return s.loadProfilesBySecretPolicy(ctx, storageProfileLoadMasked)
}

func (s *StorageService) loadProfilesWithSecrets(ctx context.Context) ([]StorageProfile, error) {
	return s.loadProfilesBySecretPolicy(ctx, storageProfileLoadPlaintext)
}

func (s *StorageService) loadProfileForReclamation(ctx context.Context, id string) (*StorageProfile, error) {
	profiles, err := s.profileRepo.ListProfilesWithSecrets(ctx, true)
	if err != nil {
		return nil, err
	}
	return s.findProfile(normalizeLoadedProfiles(profiles), strings.TrimSpace(id))
}

func (s *StorageService) loadProfilesBySecretPolicy(ctx context.Context, secretPolicy storageProfileLoadSecretPolicy) ([]StorageProfile, error) {
	var (
		profiles []StorageProfile
		err      error
	)
	switch secretPolicy {
	case storageProfileLoadPlaintext:
		profiles, err = s.profileRepo.ListProfilesWithSecrets(ctx, false)
	case storageProfileLoadMasked:
		profiles, err = s.profileRepo.ListProfiles(ctx, false)
	default:
		return nil, fmt.Errorf("unknown storage profile load secret policy: %d", secretPolicy)
	}
	if err != nil {
		return nil, err
	}
	if len(profiles) == 0 {
		return []StorageProfile{defaultLocalStorageProfile(true)}, nil
	}
	return normalizeLoadedProfiles(profiles), nil
}

func (s *StorageService) EnsureDefaultProfiles(ctx context.Context) error {
	profiles, err := s.profileRepo.ListProfiles(ctx, false)
	if err != nil {
		return err
	}

	currentExists := false
	localProfile := defaultLocalStorageProfile(false)
	for _, profile := range profiles {
		if profile.IsCurrent {
			currentExists = true
		}
		if profile.ID == builtinLocalProfileID() {
			localProfile = defaultLocalStorageProfile(profile.IsCurrent)
		}
	}

	if !currentExists {
		localProfile.IsCurrent = true
		return s.profileRepo.SaveAndApplyProfile(ctx, localProfile)
	}
	return s.profileRepo.SaveProfile(ctx, localProfile)
}

func defaultLocalStorageProfile(isCurrent bool) StorageProfile {
	return StorageProfile{
		ID:        builtinLocalProfileID(),
		Name:      "本地存储",
		Provider:  StorageProviderLocal,
		Enabled:   true,
		IsCurrent: isCurrent,
		Builtin:   true,
		Config:    map[string]interface{}{},
	}
}

func normalizeLoadedProfiles(profiles []StorageProfile) []StorageProfile {
	for i := range profiles {
		if profiles[i].Config == nil {
			profiles[i].Config = map[string]interface{}{}
		}
	}
	return profiles
}

func (s *StorageService) normalizeProfile(existing StorageProfile, incoming StorageProfile) StorageProfile {
	next := existing
	next.ID = strings.TrimSpace(incoming.ID)
	if next.ID == "" {
		next.ID = existing.ID
	}
	next.Provider = canonicalizeStorageProvider(incoming.Provider)
	next.Name = strings.TrimSpace(incoming.Name)
	if next.Name == "" {
		switch next.Provider {
		case StorageProviderLocal:
			next.Name = "本地存储"
		case StorageProviderS3Compatible:
			next.Name = "S3 兼容存储"
		default:
			next.Name = "存储"
		}
	}
	next.Enabled = incoming.Enabled
	next.IsCurrent = existing.IsCurrent
	next.Builtin = existing.Builtin
	if next.ID == builtinLocalProfileID() {
		next.Enabled = true
		next.Builtin = true
		next.Provider = StorageProviderLocal
	}

	cfg := cloneConfig(existing.Config)
	for key, value := range incoming.Config {
		cfg[key] = value
	}

	switch next.Provider {
	case StorageProviderLocal:
		next.Config = LocalStorageProfileConfig{}.Map()
	case StorageProviderS3Compatible:
		typedConfig := S3CompatibleProfileConfigFromMap(cfg)
		secret := strings.TrimSpace(configString(incoming.Config, storageSecretAccessKeyName))
		if secret == "" || strings.Contains(secret, "*") {
			if oldSecret := configString(existing.Config, storageSecretAccessKeyName); oldSecret != "" {
				typedConfig.SecretAccessKey = oldSecret
			} else {
				typedConfig.SecretAccessKey = ""
			}
		} else {
			typedConfig.SecretAccessKey = secret
		}
		next.Config = typedConfig.Map()
	default:
		next.Config = cfg
	}

	return next
}

func (s *StorageService) validateProfile(profile StorageProfile) error {
	if strings.TrimSpace(profile.Name) == "" {
		return fmt.Errorf("存储名称不能为空")
	}
	if _, ok := s.providerMeta[canonicalizeStorageProvider(profile.Provider)]; !ok {
		return fmt.Errorf("不支持的存储类型")
	}
	return nil
}

func (s *StorageService) findProfile(profiles []StorageProfile, id string) (*StorageProfile, error) {
	for i := range profiles {
		if profiles[i].ID == id {
			return &profiles[i], nil
		}
	}
	return nil, database.ErrNotFound
}

func (s *StorageService) maskProfiles(profiles []StorageProfile) []StorageProfile {
	result := make([]StorageProfile, 0, len(profiles))
	for _, profile := range profiles {
		result = append(result, s.maskProfile(profile))
	}
	return result
}

func (s *StorageService) maskProfile(profile StorageProfile) StorageProfile {
	masked := profile
	masked.Config = cloneConfig(profile.Config)
	switch masked.Provider {
	case StorageProviderS3Compatible:
		secret := configString(masked.Config, storageSecretAccessKeyName)
		masked.SecretConfigured = masked.SecretConfigured || secret != ""
		if secret != "" {
			masked.Config[storageSecretAccessKeyName] = maskSecret(secret)
		}
	}
	return masked
}
