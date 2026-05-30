package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

func (s *StorageService) ListProfiles() ([]StorageProfile, error) {
	profiles, err := s.loadProfiles()
	if err != nil {
		return nil, err
	}
	return s.maskProfiles(profiles), nil
}

func (s *StorageService) GetProfile(id string) (*StorageProfile, error) {
	profiles, err := s.loadProfiles()
	if err != nil {
		return nil, err
	}
	for _, profile := range profiles {
		if profile.ID == id {
			masked := s.maskProfile(profile)
			return &masked, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (s *StorageService) SaveProfile(profile *StorageProfile) (*StorageProfile, error) {
	if profile == nil {
		return nil, fmt.Errorf("存储配置不能为空")
	}

	profiles, err := s.loadProfiles()
	if err != nil {
		return nil, err
	}

	profile.Provider = canonicalizeStorageProvider(profile.Provider)
	meta, ok := s.providerMeta[profile.Provider]
	if !ok {
		return nil, fmt.Errorf("不支持的存储类型")
	}

	existingIndex := -1
	var existing *StorageProfile
	for i := range profiles {
		if profiles[i].ID == profile.ID {
			existingIndex = i
			existing = &profiles[i]
			break
		}
	}

	if existing == nil {
		if profile.ID == "" {
			profile.ID = fmt.Sprintf("%s-%d", profile.Provider, time.Now().UnixNano())
		}
		if profile.Provider == StorageProviderLocal && !meta.CreateAllowed {
			return nil, fmt.Errorf("本地存储不支持新增额外卡片")
		}
		existing = &StorageProfile{
			ID:       profile.ID,
			Provider: profile.Provider,
		}
	} else {
		if existing.Provider != profile.Provider {
			return nil, fmt.Errorf("不允许修改存储类型")
		}
	}

	next := s.normalizeProfile(*existing, *profile)
	if err := s.validateProfile(next); err != nil {
		return nil, err
	}

	if existingIndex >= 0 {
		profiles[existingIndex] = next
	} else {
		profiles = append(profiles, next)
	}

	if err := s.persistProfiles(profiles); err != nil {
		return nil, err
	}

	masked := s.maskProfile(next)
	return &masked, nil
}

func (s *StorageService) DeleteProfile(id string) error {
	profiles, err := s.loadProfiles()
	if err != nil {
		return err
	}

	next := make([]StorageProfile, 0, len(profiles))
	removed := false
	for _, profile := range profiles {
		if profile.ID != id {
			next = append(next, profile)
			continue
		}
		if profile.Builtin {
			return fmt.Errorf("内置存储不允许删除")
		}
		if profile.IsCurrent {
			return fmt.Errorf("当前存储不允许删除")
		}
		removed = true
	}
	if !removed {
		return gorm.ErrRecordNotFound
	}
	return s.persistProfiles(next)
}

func (s *StorageService) ApplyProfile(id string) (*StorageProfile, error) {
	profiles, err := s.loadProfiles()
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
		return nil, gorm.ErrRecordNotFound
	}

	if err := s.ensureProfileReadyForRuntime(*target); err != nil {
		return nil, err
	}

	for i := range profiles {
		profiles[i].IsCurrent = profiles[i].ID == id
	}

	if err := s.persistProfiles(profiles); err != nil {
		return nil, err
	}

	masked := s.maskProfile(*target)
	return &masked, nil
}

func (s *StorageService) loadProfiles() ([]StorageProfile, error) {
	val, err := s.configRepo.Get(StorageProfilesConfigKey)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return s.initializeProfilesConfig()
		}
		return nil, err
	}

	var cfg StorageProfilesConfig
	if err := json.Unmarshal([]byte(val), &cfg); err != nil {
		return nil, fmt.Errorf("解析存储配置失败: %v", err)
	}

	profiles, changed := s.ensureBuiltinProfiles(cfg.Profiles)
	if changed {
		if err := s.persistProfiles(profiles); err != nil {
			return nil, err
		}
	}
	return profiles, nil
}

func (s *StorageService) persistProfiles(profiles []StorageProfile) error {
	normalizedProfiles, _ := s.ensureBuiltinProfiles(profiles)
	cfg := StorageProfilesConfig{Profiles: normalizedProfiles}
	b, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := s.configRepo.Set(StorageProfilesConfigKey, string(b)); err != nil {
		return err
	}
	s.invalidatePreviewURLTransformerCache()
	return nil
}

func (s *StorageService) initializeProfilesConfig() ([]StorageProfile, error) {
	profiles := []StorageProfile{
		{
			ID:        builtinLocalProfileID(),
			Name:      "本地存储",
			Provider:  StorageProviderLocal,
			Enabled:   true,
			IsCurrent: true,
			Builtin:   true,
			Config:    map[string]interface{}{},
		},
	}

	if err := s.persistProfiles(profiles); err != nil {
		return nil, err
	}
	return profiles, nil
}

func (s *StorageService) ensureBuiltinProfiles(profiles []StorageProfile) ([]StorageProfile, bool) {
	changed := false
	seenLocal := false
	currentID := ""
	nextProfiles := make([]StorageProfile, 0, len(profiles))

	for i := range profiles {
		if profiles[i].Config == nil {
			profiles[i].Config = map[string]interface{}{}
			changed = true
		}
		canonicalProvider, migratedConfig, providerChanged := canonicalizeStorageProfile(profiles[i].Provider, profiles[i].Config)
		if providerChanged {
			profiles[i].Provider = canonicalProvider
			profiles[i].Config = migratedConfig
			changed = true
		}
		if _, ok := s.providerMeta[profiles[i].Provider]; !ok {
			changed = true
			continue
		}
		switch profiles[i].ID {
		case builtinLocalProfileID():
			if !profiles[i].Builtin {
				profiles[i].Builtin = true
				changed = true
			}
			if profiles[i].Provider != StorageProviderLocal {
				profiles[i].Provider = StorageProviderLocal
				changed = true
			}
			if !profiles[i].Enabled {
				profiles[i].Enabled = true
				changed = true
			}
			if strings.TrimSpace(profiles[i].Name) == "" {
				profiles[i].Name = "本地存储"
				changed = true
			}
			seenLocal = true
		}
		if profiles[i].IsCurrent {
			currentID = profiles[i].ID
		}
		nextProfiles = append(nextProfiles, profiles[i])
	}
	profiles = nextProfiles

	if !seenLocal {
		profiles = append(profiles, StorageProfile{
			ID:        builtinLocalProfileID(),
			Name:      "本地存储",
			Provider:  StorageProviderLocal,
			Enabled:   true,
			IsCurrent: currentID == "",
			Builtin:   true,
			Config:    map[string]interface{}{},
		})
		if currentID == "" {
			currentID = builtinLocalProfileID()
		}
		changed = true
	}

	if currentID == "" || s.indexOfProfile(profiles, currentID) < 0 {
		currentID = builtinLocalProfileID()
	}

	for i := range profiles {
		nextIsCurrent := profiles[i].ID == currentID
		if profiles[i].IsCurrent != nextIsCurrent {
			profiles[i].IsCurrent = nextIsCurrent
			changed = true
		}
	}

	return profiles, changed
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
		next.Config = map[string]interface{}{}
	case StorageProviderS3Compatible:
		secret := strings.TrimSpace(configString(incoming.Config, "secret_access_key"))
		if secret == "" || strings.Contains(secret, "*") {
			if oldSecret := configString(existing.Config, "secret_access_key"); oldSecret != "" {
				cfg["secret_access_key"] = oldSecret
			} else {
				cfg["secret_access_key"] = ""
			}
		} else {
			cfg["secret_access_key"] = secret
		}
		cfg["access_key_id"] = strings.TrimSpace(configString(cfg, "access_key_id"))
		cfg["bucket_name"] = strings.TrimSpace(configString(cfg, "bucket_name"))
		cfg["endpoint"] = strings.TrimSpace(configString(cfg, "endpoint"))
		cfg["region_id"] = strings.TrimSpace(configString(cfg, "region_id"))
		cfg["access_domain"] = strings.TrimSpace(configString(cfg, "access_domain"))
		cfg["force_path_style"] = configBool(cfg, "force_path_style")
		next.Config = cfg
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
	return nil, gorm.ErrRecordNotFound
}

func (s *StorageService) indexOfProfile(profiles []StorageProfile, id string) int {
	for i := range profiles {
		if profiles[i].ID == id {
			return i
		}
	}
	return -1
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
		secret := configString(masked.Config, "secret_access_key")
		masked.SecretConfigured = secret != ""
		if secret != "" {
			masked.Config["secret_access_key"] = maskSecret(secret)
		}
	}
	return masked
}
