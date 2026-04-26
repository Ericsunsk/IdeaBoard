package storage

import (
	"api/internal/platform/settings"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/qiniu/go-sdk/v7/auth/qbox"
	"github.com/qiniu/go-sdk/v7/storage"
)

const kodoConfigKey = "kodo_config"
const kodoDeleteListLimit = 1000

type KodoService struct {
	configRepo *settings.SystemConfigRepository
}

func NewKodoService(configRepo *settings.SystemConfigRepository) *KodoService {
	return &KodoService{configRepo: configRepo}
}

// GetConfig 获取七牛云配置
func (s *KodoService) GetConfig() (*KodoConfig, error) {
	val, err := s.configRepo.Get(kodoConfigKey)
	if err != nil {
		return &KodoConfig{}, nil
	}
	var cfg KodoConfig
	if err := json.Unmarshal([]byte(val), &cfg); err != nil {
		return &KodoConfig{}, nil
	}
	return &cfg, nil
}

// SaveConfig 保存七牛云配置
func (s *KodoService) SaveConfig(cfg *KodoConfig) error {
	b, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return s.configRepo.Set(kodoConfigKey, string(b))
}

// IsEnabled 检查七牛云是否启用
func (s *KodoService) IsEnabled() bool {
	cfg, err := s.GetConfig()
	if err != nil {
		return false
	}
	return s.IsConfigEnabled(cfg)
}

func (s *KodoService) IsConfigEnabled(cfg *KodoConfig) bool {
	if cfg == nil {
		return false
	}
	return cfg.Enabled && cfg.AccessKey != "" && cfg.SecretKey != "" && cfg.BucketName != ""
}

// IsDirectFetch 检查是否启用直传模式
func (s *KodoService) IsDirectFetch() bool {
	cfg, err := s.GetConfig()
	if err != nil {
		return false
	}
	return cfg.DirectFetch
}

// FetchURL 让七牛云直接从远程 URL 抓取文件到 Bucket（不经过业务服务器）
func (s *KodoService) FetchURL(remoteURL string) (string, error) {
	cfg, err := s.GetConfig()
	if err != nil {
		return "", fmt.Errorf("获取配置失败: %v", err)
	}
	return s.FetchURLWithConfig(remoteURL, cfg)
}

func (s *KodoService) FetchURLWithConfig(remoteURL string, cfg *KodoConfig) (string, error) {
	if !s.IsConfigEnabled(cfg) {
		return "", fmt.Errorf("七牛云配置未启用或不完整")
	}

	mac := qbox.NewMac(cfg.AccessKey, cfg.SecretKey)
	bucketManager := storage.NewBucketManager(mac, nil)

	// 从 URL 推断扩展名
	ext := path.Ext(strings.Split(remoteURL, "?")[0])
	if ext == "" {
		ext = ".jpg"
	}
	key := fmt.Sprintf("uploads/%s/%d%s", time.Now().Format("2006/01/02"), time.Now().UnixNano(), ext)

	fetchRet, err := bucketManager.Fetch(remoteURL, cfg.BucketName, key)
	if err != nil {
		return "", fmt.Errorf("七牛云抓取失败: %v", err)
	}

	// 拼接访问 URL
	domain := strings.TrimRight(cfg.AccessDomain, "/")
	if domain == "" {
		return "", fmt.Errorf("未配置访问域名")
	}
	if !strings.HasPrefix(domain, "http") {
		domain = "https://" + domain
	}

	finalKey := fetchRet.Key
	if finalKey == "" {
		finalKey = key
	}
	return fmt.Sprintf("%s/%s", domain, finalKey), nil
}

// TestConnection 测试连接（获取 Bucket 信息验证 AK/SK 和 Bucket 有效性）
func (s *KodoService) TestConnection() error {
	cfg, err := s.GetConfig()
	if err != nil {
		return fmt.Errorf("获取配置失败: %v", err)
	}
	return s.TestConnectionWithConfig(cfg)
}

func (s *KodoService) TestConnectionWithConfig(cfg *KodoConfig) error {
	if cfg.AccessKey == "" || cfg.SecretKey == "" || cfg.BucketName == "" {
		return fmt.Errorf("请先完整填写七牛云配置")
	}

	mac := qbox.NewMac(cfg.AccessKey, cfg.SecretKey)
	bucketManager := storage.NewBucketManager(mac, nil)

	// 获取 Bucket 的域名列表来验证连接
	_, err := bucketManager.ListBucketDomains(cfg.BucketName)
	if err != nil {
		return fmt.Errorf("验证失败: %v", err)
	}
	return nil
}

// GetBucketInfo 获取 Bucket 信息（供前端自动填充）
func (s *KodoService) GetBucketInfo() (map[string]interface{}, error) {
	cfg, err := s.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("获取配置失败: %v", err)
	}
	return s.GetBucketInfoWithConfig(cfg)
}

func (s *KodoService) GetBucketInfoWithConfig(cfg *KodoConfig) (map[string]interface{}, error) {
	if cfg.AccessKey == "" || cfg.SecretKey == "" || cfg.BucketName == "" {
		return nil, fmt.Errorf("请先填写 AK/SK 和 Bucket 名称")
	}

	mac := qbox.NewMac(cfg.AccessKey, cfg.SecretKey)
	bucketManager := storage.NewBucketManager(mac, nil)

	info := map[string]interface{}{}

	// 获取绑定的域名
	domains, err := bucketManager.ListBucketDomains(cfg.BucketName)
	if err == nil && len(domains) > 0 {
		domainList := []string{}
		for _, d := range domains {
			domainList = append(domainList, d.Domain)
		}
		info["domains"] = domainList
	}

	return info, nil
}

// Upload 上传文件到七牛云，返回可访问的 URL
func (s *KodoService) Upload(file *multipart.FileHeader) (string, error) {
	cfg, err := s.GetConfig()
	if err != nil {
		return "", fmt.Errorf("获取配置失败: %v", err)
	}
	return s.UploadWithConfig(file, cfg, StorageStoreOptions{})
}

func (s *KodoService) UploadWithConfig(file *multipart.FileHeader, cfg *KodoConfig, opts StorageStoreOptions) (string, error) {
	if !s.IsConfigEnabled(cfg) {
		return "", fmt.Errorf("七牛云配置未启用或不完整")
	}

	mac := qbox.NewMac(cfg.AccessKey, cfg.SecretKey)
	putPolicy := storage.PutPolicy{
		Scope: cfg.BucketName,
	}
	upToken := putPolicy.UploadToken(mac)

	// 构建上传配置，自动检测 Bucket 所在区域
	uploadCfg := storage.Config{
		UseHTTPS: true,
	}
	// 自动查询 Bucket 所在区域
	zone, zErr := storage.GetZone(cfg.AccessKey, cfg.BucketName)
	if zErr == nil && zone != nil {
		uploadCfg.Zone = zone
	}
	// 如果用户指定了上传域名则覆盖
	if cfg.UploadDomain != "" {
		uploadDomain := cfg.UploadDomain
		if !strings.HasPrefix(uploadDomain, "http") {
			uploadDomain = "https://" + uploadDomain
		}
		uploadCfg.UpHost = uploadDomain
	}

	formUploader := storage.NewFormUploader(&uploadCfg)

	src, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("打开文件失败: %v", err)
	}
	defer src.Close()

	ext := strings.ToLower(filepath.Ext(file.Filename))
	key := buildObjectKey(opts.DirPrefix, ext)

	ret := storage.PutRet{}
	err = formUploader.Put(context.Background(), &ret, upToken, key, src, file.Size, nil)
	if err != nil {
		return "", fmt.Errorf("上传失败: %v", err)
	}

	// 拼接访问 URL
	domain := strings.TrimRight(cfg.AccessDomain, "/")
	if domain == "" {
		return "", fmt.Errorf("未配置访问域名")
	}
	if !strings.HasPrefix(domain, "http") {
		domain = "https://" + domain
	}
	url := fmt.Sprintf("%s/%s", domain, ret.Key)
	return url, nil
}

func (s *KodoService) DeleteWithConfig(objectKey string, cfg *KodoConfig) error {
	if !s.IsConfigEnabled(cfg) {
		return fmt.Errorf("七牛云配置未启用或不完整")
	}
	key := strings.TrimSpace(objectKey)
	if key == "" {
		return fmt.Errorf("文件路径不能为空")
	}

	mac := qbox.NewMac(cfg.AccessKey, cfg.SecretKey)
	bucketManager := storage.NewBucketManager(mac, nil)
	if err := bucketManager.Delete(cfg.BucketName, key); err != nil {
		return fmt.Errorf("删除失败: %v", err)
	}
	return nil
}

func (s *KodoService) DeletePrefixWithConfig(objectKeyPrefix string, cfg *KodoConfig) error {
	if !s.IsConfigEnabled(cfg) {
		return fmt.Errorf("七牛云配置未启用或不完整")
	}
	prefix, err := normalizeKodoDeletePrefix(objectKeyPrefix)
	if err != nil {
		return err
	}

	mac := qbox.NewMac(cfg.AccessKey, cfg.SecretKey)
	bucketManager := storage.NewBucketManager(mac, nil)
	bucketName := strings.TrimSpace(cfg.BucketName)

	marker := ""
	for {
		entries, _, nextMarker, hasNext, err := bucketManager.ListFiles(bucketName, prefix, "", marker, kodoDeleteListLimit)
		if err != nil {
			return fmt.Errorf("列举前缀对象失败: bucket=%s prefix=%s marker=%s err=%w", bucketName, prefix, marker, err)
		}

		for _, entry := range entries {
			key := strings.TrimSpace(entry.Key)
			if key == "" {
				continue
			}
			if err := bucketManager.Delete(bucketName, key); err != nil {
				return fmt.Errorf("删除对象失败: bucket=%s key=%s err=%w", bucketName, key, err)
			}
		}

		if !hasNext || strings.TrimSpace(nextMarker) == "" {
			break
		}
		marker = nextMarker
	}
	return nil
}

func normalizeKodoDeletePrefix(prefix string) (string, error) {
	normalized := strings.TrimSpace(prefix)
	if normalized == "" {
		return "", fmt.Errorf("文件前缀不能为空")
	}
	return normalized, nil
}
