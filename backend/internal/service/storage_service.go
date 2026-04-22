package service

import (
	"backend/internal/constants"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/pkg/safehttp"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/textproto"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	legacyAliyunOSSProviderID = "aliyun-oss"
)

type StorageStoreOptions struct {
	DirPrefix  string
	AllowHosts []string
}

type StorageService struct {
	configRepo          *repository.SystemConfigRepository
	kodoService         *KodoService
	s3CompatibleService *S3CompatibleService
	providerMeta        map[model.StorageProvider]model.StorageProviderMeta
}

func NewStorageService(configRepo *repository.SystemConfigRepository) *StorageService {
	service := &StorageService{
		configRepo:          configRepo,
		kodoService:         NewKodoService(configRepo),
		s3CompatibleService: NewS3CompatibleService(),
		providerMeta:        make(map[model.StorageProvider]model.StorageProviderMeta),
	}
	for _, meta := range []model.StorageProviderMeta{
		{
			Provider:      model.StorageProviderLocal,
			Label:         "本地存储",
			Description:   "使用服务器本地 /uploads 目录保存文件。",
			Builtin:       true,
			CreateAllowed: false,
		},
		{
			Provider:           model.StorageProviderKodo,
			Label:              "Kodo",
			Description:        "七牛云 Kodo 对象存储，支持直传抓取与 Bucket 域名读取。",
			Builtin:            true,
			CreateAllowed:      true,
			SupportsTest:       true,
			SupportsBucketInfo: true,
			Fields: []model.StorageProviderField{
				{Key: "access_key", Label: "Access Key", Type: model.StorageFieldTypeText, Placeholder: "请输入 Access Key", Required: true},
				{Key: "secret_key", Label: "Secret Key", Type: model.StorageFieldTypePassword, Placeholder: "请输入 Secret Key", Required: true, Secret: true},
				{Key: "bucket_name", Label: "Bucket 名称", Type: model.StorageFieldTypeText, Placeholder: "请输入 Bucket 名称", Required: true},
				{Key: "upload_domain", Label: "上传域名", Type: model.StorageFieldTypeText, Placeholder: "留空则自动检测，例如 up-z2.qiniup.com"},
				{Key: "access_domain", Label: "访问域名", Type: model.StorageFieldTypeText, Placeholder: "例如：https://xxx.clouddn.com", Required: true},
				{Key: "thumbnail_mode", Label: "缩略图模式", Type: model.StorageFieldTypeSwitch, Hint: "列表和画廊优先走缩略图域名参数。"},
				{Key: "direct_fetch", Label: "直传模式", Type: model.StorageFieldTypeSwitch, Hint: "让 Kodo 直接抓取上游 URL，减少业务服务器中转。"},
			},
		},
		{
			Provider:           model.StorageProviderS3Compatible,
			Label:              "S3 兼容存储",
			Description:        "统一接入 Amazon S3、阿里云 OSS、腾讯云 COS、Cloudflare R2、MinIO 等 S3 兼容对象存储。",
			Builtin:            false,
			CreateAllowed:      true,
			SupportsTest:       true,
			SupportsBucketInfo: true,
			Fields: []model.StorageProviderField{
				{Key: "access_key_id", Label: "Access Key ID", Type: model.StorageFieldTypeText, Placeholder: "请输入 Access Key ID", Required: true},
				{Key: "secret_access_key", Label: "Secret Access Key", Type: model.StorageFieldTypePassword, Placeholder: "请输入 Secret Access Key", Required: true, Secret: true},
				{Key: "bucket_name", Label: "Bucket 名称", Type: model.StorageFieldTypeText, Placeholder: "请输入 Bucket 名称", Required: true},
				{Key: "endpoint", Label: "Endpoint", Type: model.StorageFieldTypeText, Placeholder: "例如：s3.us-east-1.amazonaws.com / <account>.r2.cloudflarestorage.com", Required: true},
				{Key: "region_id", Label: "地域 / Region", Type: model.StorageFieldTypeText, Placeholder: "例如：us-east-1，可留空"},
				{Key: "access_domain", Label: "访问域名", Type: model.StorageFieldTypeText, Placeholder: "例如：https://bucket.s3.us-east-1.amazonaws.com", Required: true},
				{Key: "force_path_style", Label: "Path Style", Type: model.StorageFieldTypeSwitch, Hint: "部分兼容存储需要使用 path-style 访问。"},
			},
		},
	} {
		service.providerMeta[meta.Provider] = meta
	}
	return service
}

func (s *StorageService) ListProviderMeta() []model.StorageProviderMeta {
	return []model.StorageProviderMeta{
		s.providerMeta[model.StorageProviderLocal],
		s.providerMeta[model.StorageProviderKodo],
		s.providerMeta[model.StorageProviderS3Compatible],
	}
}

func (s *StorageService) ListProfiles() ([]model.StorageProfile, error) {
	profiles, err := s.loadProfiles()
	if err != nil {
		return nil, err
	}
	return s.maskProfiles(profiles), nil
}

func (s *StorageService) GetProfile(id string) (*model.StorageProfile, error) {
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

func (s *StorageService) SaveProfile(profile *model.StorageProfile) (*model.StorageProfile, error) {
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
	var existing *model.StorageProfile
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
		if profile.Provider == model.StorageProviderLocal && !meta.CreateAllowed {
			return nil, fmt.Errorf("本地存储不支持新增额外卡片")
		}
		existing = &model.StorageProfile{
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

	next := make([]model.StorageProfile, 0, len(profiles))
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

func (s *StorageService) ApplyProfile(id string) (*model.StorageProfile, error) {
	profiles, err := s.loadProfiles()
	if err != nil {
		return nil, err
	}

	var target *model.StorageProfile
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

func (s *StorageService) TestProfile(id string) error {
	profiles, err := s.loadProfiles()
	if err != nil {
		return err
	}

	profile, err := s.findProfile(profiles, id)
	if err != nil {
		return err
	}

	switch profile.Provider {
	case model.StorageProviderLocal:
		return nil
	case model.StorageProviderKodo:
		return s.kodoService.TestConnectionWithConfig(s.kodoConfigFromProfile(profile))
	case model.StorageProviderS3Compatible:
		return s.s3CompatibleService.TestConnectionWithConfig(s.s3CompatibleConfigFromProfile(profile))
	default:
		return fmt.Errorf("不支持的存储类型")
	}
}

func (s *StorageService) GetBucketInfo(id string) (map[string]interface{}, error) {
	profiles, err := s.loadProfiles()
	if err != nil {
		return nil, err
	}

	profile, err := s.findProfile(profiles, id)
	if err != nil {
		return nil, err
	}

	switch profile.Provider {
	case model.StorageProviderKodo:
		return s.kodoService.GetBucketInfoWithConfig(s.kodoConfigFromProfile(profile))
	case model.StorageProviderS3Compatible:
		return s.s3CompatibleService.GetBucketInfoWithConfig(s.s3CompatibleConfigFromProfile(profile))
	default:
		return nil, fmt.Errorf("当前存储类型不支持读取 Bucket 信息")
	}
}

func (s *StorageService) Upload(file *multipart.FileHeader) (string, error) {
	profile, err := s.resolveActiveProfile()
	if err != nil {
		return "", err
	}
	return s.uploadWithProfile(*profile, file, StorageStoreOptions{})
}

func (s *StorageService) StoreRemote(ctx context.Context, remoteURL string, opts StorageStoreOptions) (string, error) {
	profile, err := s.resolveActiveProfile()
	if err != nil {
		return "", err
	}
	return s.storeRemoteWithProfile(ctx, *profile, remoteURL, opts)
}

func (s *StorageService) StoreBytes(filename string, data []byte, opts StorageStoreOptions) (string, error) {
	profile, err := s.resolveActiveProfile()
	if err != nil {
		return "", err
	}
	switch profile.Provider {
	case model.StorageProviderLocal:
		return s.saveBytesLocal(filename, data, opts)
	case model.StorageProviderKodo:
		return s.kodoService.UploadWithConfig(createMemoryFileHeader(filename, data), s.kodoConfigFromProfile(profile))
	case model.StorageProviderS3Compatible:
		return s.s3CompatibleService.UploadWithConfig(createMemoryFileHeader(filename, data), s.s3CompatibleConfigFromProfile(profile))
	default:
		return "", fmt.Errorf("不支持的存储类型")
	}
}

func (s *StorageService) UsesObjectStorage() bool {
	profile, err := s.resolveActiveProfile()
	if err != nil || profile == nil {
		return false
	}
	return profile.Provider == model.StorageProviderKodo || profile.Provider == model.StorageProviderS3Compatible
}

func (s *StorageService) IsDirectFetchEnabled() bool {
	profile, err := s.resolveActiveProfile()
	if err != nil || profile == nil || profile.Provider != model.StorageProviderKodo {
		return false
	}
	return configBool(profile.Config, "direct_fetch")
}

func (s *StorageService) loadProfiles() ([]model.StorageProfile, error) {
	val, err := s.configRepo.Get(constants.StorageProfilesConfigKey)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return s.migrateLegacyConfig()
		}
		return nil, err
	}

	var cfg model.StorageProfilesConfig
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

func (s *StorageService) persistProfiles(profiles []model.StorageProfile) error {
	normalizedProfiles, _ := s.ensureBuiltinProfiles(profiles)
	cfg := model.StorageProfilesConfig{Profiles: normalizedProfiles}
	b, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return s.configRepo.Set(constants.StorageProfilesConfigKey, string(b))
}

func (s *StorageService) migrateLegacyConfig() ([]model.StorageProfile, error) {
	profiles := []model.StorageProfile{
		{
			ID:        builtinLocalProfileID(),
			Name:      "本地存储",
			Provider:  model.StorageProviderLocal,
			Enabled:   true,
			IsCurrent: true,
			Builtin:   true,
			Config:    map[string]interface{}{},
		},
		{
			ID:       builtinKodoProfileID(),
			Name:     "Kodo 存储",
			Provider: model.StorageProviderKodo,
			Enabled:  false,
			Builtin:  true,
			Config: map[string]interface{}{
				"access_key":     "",
				"secret_key":     "",
				"bucket_name":    "",
				"upload_domain":  "",
				"access_domain":  "",
				"thumbnail_mode": false,
				"direct_fetch":   false,
			},
		},
	}

	cfg, err := s.kodoService.GetConfig()
	if err == nil && cfg != nil {
		profiles[1].Enabled = cfg.Enabled
		profiles[1].Config = map[string]interface{}{
			"access_key":     strings.TrimSpace(cfg.AccessKey),
			"secret_key":     cfg.SecretKey,
			"bucket_name":    strings.TrimSpace(cfg.BucketName),
			"upload_domain":  strings.TrimSpace(cfg.UploadDomain),
			"access_domain":  strings.TrimSpace(cfg.AccessDomain),
			"thumbnail_mode": cfg.ThumbnailMode,
			"direct_fetch":   cfg.DirectFetch,
		}
		if s.kodoService.IsConfigEnabled(cfg) {
			profiles[0].IsCurrent = false
			profiles[1].IsCurrent = true
		}
	}

	if err := s.persistProfiles(profiles); err != nil {
		return nil, err
	}
	return profiles, nil
}

func (s *StorageService) ensureBuiltinProfiles(profiles []model.StorageProfile) ([]model.StorageProfile, bool) {
	changed := false
	seenLocal := false
	seenKodo := false
	currentID := ""

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
		switch profiles[i].ID {
		case builtinLocalProfileID():
			if !profiles[i].Builtin {
				profiles[i].Builtin = true
				changed = true
			}
			if profiles[i].Provider != model.StorageProviderLocal {
				profiles[i].Provider = model.StorageProviderLocal
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
		case builtinKodoProfileID():
			if !profiles[i].Builtin {
				profiles[i].Builtin = true
				changed = true
			}
			if profiles[i].Provider != model.StorageProviderKodo {
				profiles[i].Provider = model.StorageProviderKodo
				changed = true
			}
			if strings.TrimSpace(profiles[i].Name) == "" {
				profiles[i].Name = "Kodo 存储"
				changed = true
			}
			seenKodo = true
		}
		if profiles[i].IsCurrent {
			currentID = profiles[i].ID
		}
	}

	if !seenLocal {
		profiles = append(profiles, model.StorageProfile{
			ID:        builtinLocalProfileID(),
			Name:      "本地存储",
			Provider:  model.StorageProviderLocal,
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

	if !seenKodo {
		profiles = append(profiles, model.StorageProfile{
			ID:       builtinKodoProfileID(),
			Name:     "Kodo 存储",
			Provider: model.StorageProviderKodo,
			Enabled:  false,
			Builtin:  true,
			Config: map[string]interface{}{
				"access_key":     "",
				"secret_key":     "",
				"bucket_name":    "",
				"upload_domain":  "",
				"access_domain":  "",
				"thumbnail_mode": false,
				"direct_fetch":   false,
			},
		})
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

func (s *StorageService) normalizeProfile(existing model.StorageProfile, incoming model.StorageProfile) model.StorageProfile {
	next := existing
	next.ID = strings.TrimSpace(incoming.ID)
	if next.ID == "" {
		next.ID = existing.ID
	}
	next.Provider = canonicalizeStorageProvider(incoming.Provider)
	next.Name = strings.TrimSpace(incoming.Name)
	if next.Name == "" {
		switch next.Provider {
		case model.StorageProviderLocal:
			next.Name = "本地存储"
		case model.StorageProviderKodo:
			next.Name = "Kodo 存储"
		case model.StorageProviderS3Compatible:
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
		next.Provider = model.StorageProviderLocal
	}
	if next.ID == builtinKodoProfileID() {
		next.Builtin = true
		next.Provider = model.StorageProviderKodo
	}

	cfg := cloneConfig(existing.Config)
	for key, value := range incoming.Config {
		cfg[key] = value
	}

	switch next.Provider {
	case model.StorageProviderLocal:
		next.Config = map[string]interface{}{}
	case model.StorageProviderKodo:
		secret := strings.TrimSpace(configString(incoming.Config, "secret_key"))
		if secret == "" || strings.Contains(secret, "*") {
			if oldSecret := configString(existing.Config, "secret_key"); oldSecret != "" {
				cfg["secret_key"] = oldSecret
			} else {
				cfg["secret_key"] = ""
			}
		} else {
			cfg["secret_key"] = secret
		}
		cfg["access_key"] = strings.TrimSpace(configString(cfg, "access_key"))
		cfg["bucket_name"] = strings.TrimSpace(configString(cfg, "bucket_name"))
		cfg["upload_domain"] = strings.TrimSpace(configString(cfg, "upload_domain"))
		cfg["access_domain"] = strings.TrimSpace(configString(cfg, "access_domain"))
		cfg["thumbnail_mode"] = configBool(cfg, "thumbnail_mode")
		cfg["direct_fetch"] = configBool(cfg, "direct_fetch")
		next.Config = cfg
	case model.StorageProviderS3Compatible:
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

func (s *StorageService) validateProfile(profile model.StorageProfile) error {
	if strings.TrimSpace(profile.Name) == "" {
		return fmt.Errorf("存储名称不能为空")
	}
	if _, ok := s.providerMeta[canonicalizeStorageProvider(profile.Provider)]; !ok {
		return fmt.Errorf("不支持的存储类型")
	}
	return nil
}

func (s *StorageService) ensureProfileReadyForRuntime(profile model.StorageProfile) error {
	switch profile.Provider {
	case model.StorageProviderLocal:
		return nil
	case model.StorageProviderKodo:
		cfg := s.kodoConfigFromProfile(&profile)
		if !s.kodoService.IsConfigEnabled(cfg) {
			return fmt.Errorf("请先完整填写并启用 Kodo 存储")
		}
		if strings.TrimSpace(cfg.AccessDomain) == "" {
			return fmt.Errorf("请先配置访问域名")
		}
		return nil
	case model.StorageProviderS3Compatible:
		cfg := s.s3CompatibleConfigFromProfile(&profile)
		if !s.s3CompatibleService.IsConfigEnabled(cfg) {
			return fmt.Errorf("请先完整填写并启用 S3 兼容存储")
		}
		return nil
	default:
		return fmt.Errorf("不支持的存储类型")
	}
}

func (s *StorageService) resolveActiveProfile() (*model.StorageProfile, error) {
	profiles, err := s.loadProfiles()
	if err != nil {
		return nil, err
	}

	for _, profile := range profiles {
		if !profile.IsCurrent {
			continue
		}
		if s.isProfileReady(profile) {
			return &profile, nil
		}
		break
	}

	local, err := s.findProfile(profiles, builtinLocalProfileID())
	if err != nil {
		return nil, err
	}
	return local, nil
}

func (s *StorageService) isProfileReady(profile model.StorageProfile) bool {
	switch profile.Provider {
	case model.StorageProviderLocal:
		return true
	case model.StorageProviderKodo:
		cfg := s.kodoConfigFromProfile(&profile)
		return s.kodoService.IsConfigEnabled(cfg) && strings.TrimSpace(cfg.AccessDomain) != ""
	case model.StorageProviderS3Compatible:
		cfg := s.s3CompatibleConfigFromProfile(&profile)
		return s.s3CompatibleService.IsConfigEnabled(cfg)
	default:
		return false
	}
}

func (s *StorageService) uploadWithProfile(profile model.StorageProfile, file *multipart.FileHeader, opts StorageStoreOptions) (string, error) {
	switch profile.Provider {
	case model.StorageProviderLocal:
		return s.saveUploadedFileLocal(file, opts)
	case model.StorageProviderKodo:
		return s.kodoService.UploadWithConfig(file, s.kodoConfigFromProfile(&profile))
	case model.StorageProviderS3Compatible:
		return s.s3CompatibleService.UploadWithConfig(file, s.s3CompatibleConfigFromProfile(&profile))
	default:
		return "", fmt.Errorf("不支持的存储类型")
	}
}

func (s *StorageService) storeRemoteWithProfile(ctx context.Context, profile model.StorageProfile, remoteURL string, opts StorageStoreOptions) (string, error) {
	switch {
	case strings.HasPrefix(remoteURL, "data:image/"):
		filename, data, err := decodeDataURL(remoteURL)
		if err != nil {
			return "", err
		}
		fileHeader := createMemoryFileHeader(filename, data)
		return s.uploadWithProfile(profile, fileHeader, opts)
	case profile.Provider == model.StorageProviderKodo:
		cfg := s.kodoConfigFromProfile(&profile)
		if cfg.DirectFetch {
			if url, err := s.kodoService.FetchURLWithConfig(remoteURL, cfg); err == nil {
				return url, nil
			}
		}
		data, filename, err := s.downloadRemoteFile(ctx, remoteURL, opts.AllowHosts)
		if err != nil {
			return "", err
		}
		return s.kodoService.UploadWithConfig(createMemoryFileHeader(filename, data), cfg)
	case profile.Provider == model.StorageProviderS3Compatible:
		data, filename, err := s.downloadRemoteFile(ctx, remoteURL, opts.AllowHosts)
		if err != nil {
			return "", err
		}
		return s.s3CompatibleService.UploadWithConfig(createMemoryFileHeader(filename, data), s.s3CompatibleConfigFromProfile(&profile))
	case profile.Provider == model.StorageProviderLocal:
		data, filename, err := s.downloadRemoteFile(ctx, remoteURL, opts.AllowHosts)
		if err != nil {
			return "", err
		}
		return s.saveBytesLocal(filename, data, opts)
	default:
		return "", fmt.Errorf("不支持的存储类型")
	}
}

func (s *StorageService) saveUploadedFileLocal(file *multipart.FileHeader, opts StorageStoreOptions) (string, error) {
	src, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("打开文件失败: %v", err)
	}
	defer src.Close()

	data, err := io.ReadAll(src)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %v", err)
	}

	return s.saveBytesLocal(file.Filename, data, opts)
}

func (s *StorageService) saveBytesLocal(filename string, data []byte, opts StorageStoreOptions) (string, error) {
	dirParts := []string{"uploads"}
	if opts.DirPrefix != "" {
		dirParts = append(dirParts, opts.DirPrefix)
	}
	dirParts = append(dirParts, time.Now().Format("2006/01/02"))
	dir := filepath.Join(dirParts...)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("创建目录失败: %v", err)
	}

	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		ext = ".png"
	}
	name := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
	dst := filepath.Join(dir, name)
	if err := os.WriteFile(dst, data, 0644); err != nil {
		return "", fmt.Errorf("保存文件失败: %v", err)
	}
	return "/" + strings.ReplaceAll(dst, "\\", "/"), nil
}

func (s *StorageService) downloadRemoteFile(ctx context.Context, remoteURL string, allowHosts []string) ([]byte, string, error) {
	resp, err := safehttp.GetWithAllowHosts(ctx, remoteURL, 30*time.Second, allowHosts...)
	if err != nil {
		return nil, "", fmt.Errorf("下载图片失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("下载图片返回 %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("读取图片数据失败: %v", err)
	}

	ext := path.Ext(strings.Split(remoteURL, "?")[0])
	if ext == "" {
		ct := resp.Header.Get("Content-Type")
		switch {
		case strings.Contains(ct, "png"):
			ext = ".png"
		case strings.Contains(ct, "webp"):
			ext = ".webp"
		case strings.Contains(ct, "gif"):
			ext = ".gif"
		default:
			ext = ".jpg"
		}
	}

	filename := fmt.Sprintf("remote_%d%s", time.Now().UnixNano(), ext)
	return data, filename, nil
}

func (s *StorageService) findProfile(profiles []model.StorageProfile, id string) (*model.StorageProfile, error) {
	for i := range profiles {
		if profiles[i].ID == id {
			return &profiles[i], nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (s *StorageService) indexOfProfile(profiles []model.StorageProfile, id string) int {
	for i := range profiles {
		if profiles[i].ID == id {
			return i
		}
	}
	return -1
}

func (s *StorageService) maskProfiles(profiles []model.StorageProfile) []model.StorageProfile {
	result := make([]model.StorageProfile, 0, len(profiles))
	for _, profile := range profiles {
		result = append(result, s.maskProfile(profile))
	}
	return result
}

func (s *StorageService) maskProfile(profile model.StorageProfile) model.StorageProfile {
	masked := profile
	masked.Config = cloneConfig(profile.Config)
	switch masked.Provider {
	case model.StorageProviderKodo:
		secret := configString(masked.Config, "secret_key")
		masked.SecretConfigured = secret != ""
		if secret != "" {
			masked.Config["secret_key"] = maskSecret(secret)
		}
	case model.StorageProviderS3Compatible:
		secret := configString(masked.Config, "secret_access_key")
		masked.SecretConfigured = secret != ""
		if secret != "" {
			masked.Config["secret_access_key"] = maskSecret(secret)
		}
	}
	return masked
}

func (s *StorageService) kodoConfigFromProfile(profile *model.StorageProfile) *model.KodoConfig {
	if profile == nil {
		return &model.KodoConfig{}
	}
	return &model.KodoConfig{
		Enabled:       profile.Enabled,
		AccessKey:     strings.TrimSpace(configString(profile.Config, "access_key")),
		SecretKey:     configString(profile.Config, "secret_key"),
		BucketName:    strings.TrimSpace(configString(profile.Config, "bucket_name")),
		UploadDomain:  strings.TrimSpace(configString(profile.Config, "upload_domain")),
		AccessDomain:  strings.TrimSpace(configString(profile.Config, "access_domain")),
		ThumbnailMode: configBool(profile.Config, "thumbnail_mode"),
		DirectFetch:   configBool(profile.Config, "direct_fetch"),
	}
}

func (s *StorageService) s3CompatibleConfigFromProfile(profile *model.StorageProfile) *model.S3CompatibleConfig {
	if profile == nil {
		return &model.S3CompatibleConfig{}
	}
	return &model.S3CompatibleConfig{
		Enabled:         profile.Enabled,
		AccessKeyID:     strings.TrimSpace(configString(profile.Config, "access_key_id")),
		SecretAccessKey: configString(profile.Config, "secret_access_key"),
		BucketName:      strings.TrimSpace(configString(profile.Config, "bucket_name")),
		Endpoint:        strings.TrimSpace(configString(profile.Config, "endpoint")),
		RegionID:        strings.TrimSpace(configString(profile.Config, "region_id")),
		AccessDomain:    strings.TrimSpace(configString(profile.Config, "access_domain")),
		ForcePathStyle:  configBool(profile.Config, "force_path_style"),
	}
}

func builtinLocalProfileID() string {
	return "builtin-local"
}

func builtinKodoProfileID() string {
	return "builtin-kodo"
}

func canonicalizeStorageProvider(provider model.StorageProvider) model.StorageProvider {
	switch provider {
	case model.StorageProviderS3Compatible:
		return model.StorageProviderS3Compatible
	case model.StorageProvider(legacyAliyunOSSProviderID):
		return model.StorageProviderS3Compatible
	default:
		return provider
	}
}

func canonicalizeStorageProfile(provider model.StorageProvider, cfg map[string]interface{}) (model.StorageProvider, map[string]interface{}, bool) {
	canonicalProvider := canonicalizeStorageProvider(provider)
	if canonicalProvider != model.StorageProviderS3Compatible {
		return canonicalProvider, cfg, canonicalProvider != provider
	}
	if provider != model.StorageProvider(legacyAliyunOSSProviderID) {
		return canonicalProvider, cfg, canonicalProvider != provider
	}

	migrated := cloneConfig(cfg)
	if secret := strings.TrimSpace(configString(migrated, "access_key_secret")); secret != "" && configString(migrated, "secret_access_key") == "" {
		migrated["secret_access_key"] = secret
	}
	delete(migrated, "access_key_secret")
	if _, ok := migrated["force_path_style"]; !ok {
		migrated["force_path_style"] = false
	}
	return canonicalProvider, migrated, true
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

func configBool(cfg map[string]interface{}, key string) bool {
	if cfg == nil {
		return false
	}
	if val, ok := cfg[key]; ok {
		switch typed := val.(type) {
		case bool:
			return typed
		case string:
			return typed == "true" || typed == "1"
		case float64:
			return typed != 0
		}
	}
	return false
}

func cloneConfig(cfg map[string]interface{}) map[string]interface{} {
	if cfg == nil {
		return map[string]interface{}{}
	}
	cloned := make(map[string]interface{}, len(cfg))
	for key, value := range cfg {
		cloned[key] = value
	}
	return cloned
}

func maskSecret(secret string) string {
	if len(secret) <= 6 {
		return "***"
	}
	return secret[:3] + "***" + secret[len(secret)-3:]
}

func createMemoryFileHeader(filename string, data []byte) *multipart.FileHeader {
	buffer := &multipartBuffer{filename: filename, data: data}
	return buffer.FileHeader()
}

func decodeDataURL(dataURL string) (string, []byte, error) {
	parts := strings.SplitN(dataURL, ",", 2)
	if len(parts) != 2 {
		return "", nil, fmt.Errorf("无效的 base64 图片格式")
	}

	decoded, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", nil, fmt.Errorf("base64 解码失败: %v", err)
	}

	ext := ".png"
	switch {
	case strings.Contains(parts[0], "jpeg"), strings.Contains(parts[0], "jpg"):
		ext = ".jpg"
	case strings.Contains(parts[0], "webp"):
		ext = ".webp"
	case strings.Contains(parts[0], "gif"):
		ext = ".gif"
	}

	return fmt.Sprintf("gen_%d%s", time.Now().UnixNano(), ext), decoded, nil
}

type multipartBuffer struct {
	filename string
	data     []byte
}

func (m *multipartBuffer) FileHeader() *multipart.FileHeader {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, m.filename))
	h.Set("Content-Type", "application/octet-stream")
	part, _ := writer.CreatePart(h)
	_, _ = part.Write(m.data)
	_ = writer.Close()

	reader := multipart.NewReader(body, writer.Boundary())
	form, _ := reader.ReadForm(int64(len(m.data) + 1024))
	if files, ok := form.File["file"]; ok && len(files) > 0 {
		return files[0]
	}
	return nil
}
