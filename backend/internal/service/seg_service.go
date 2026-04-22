package service

import (
	"backend/internal/constants"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	imageenhan "github.com/alibabacloud-go/imageenhan-20190930/v3/client"
	imageseg "github.com/alibabacloud-go/imageseg-20191230/v2/client"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/dara"
	"github.com/alibabacloud-go/tea/tea"

	"backend/internal/model"
	"backend/internal/repository"
	"backend/pkg/safehttp"
)

type SegService struct {
	configRepo *repository.SystemConfigRepository
}

func NewSegService(configRepo *repository.SystemConfigRepository) *SegService {
	return &SegService{configRepo: configRepo}
}

func (s *SegService) GetConfig() (*model.AliyunSegConfig, error) {
	val, err := s.configRepo.Get(constants.AliyunSegConfigKey)
	if err != nil {
		return &model.AliyunSegConfig{RegionID: "cn-shanghai"}, nil
	}
	var cfg model.AliyunSegConfig
	if err := json.Unmarshal([]byte(val), &cfg); err != nil {
		return &model.AliyunSegConfig{RegionID: "cn-shanghai"}, nil
	}
	return &cfg, nil
}

func (s *SegService) SaveConfig(req *model.AliyunSegConfigUpdateRequest) error {
	currentCfg, err := s.GetConfig()
	if err != nil {
		return err
	}
	if currentCfg == nil {
		currentCfg = &model.AliyunSegConfig{}
	}

	nextCfg := *currentCfg
	nextCfg.Enabled = req.Enabled
	nextCfg.AccessKeyID = req.AccessKeyID
	nextCfg.RegionID = req.RegionID
	if nextCfg.RegionID == "" {
		nextCfg.RegionID = "cn-shanghai"
	}
	if req.AccessKeySecret != nil {
		nextCfg.AccessKeySecret = *req.AccessKeySecret
	}

	data, _ := json.Marshal(&nextCfg)
	return s.configRepo.Set(constants.AliyunSegConfigKey, string(data))
}

// createClient 创建阿里云分割抠图 SDK 客户端
func (s *SegService) createClient(cfg *model.AliyunSegConfig) (*imageseg.Client, error) {
	regionID := cfg.RegionID
	if regionID == "" {
		regionID = "cn-shanghai"
	}
	config := &openapi.Config{
		AccessKeyId:     tea.String(cfg.AccessKeyID),
		AccessKeySecret: tea.String(cfg.AccessKeySecret),
		Endpoint:        tea.String(fmt.Sprintf("imageseg.%s.aliyuncs.com", regionID)),
	}
	return imageseg.NewClient(config)
}

// fetchImageStream 从 URL 获取图片流
func fetchImageStream(imageURL string) (io.ReadCloser, error) {
	resp, err := safehttp.Get(context.Background(), imageURL, 20*time.Second)
	if err != nil {
		return nil, fmt.Errorf("获取图片失败: %v", err)
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		return nil, fmt.Errorf("获取图片返回 %d", resp.StatusCode)
	}
	return resp.Body, nil
}

// createImageEnhanClient 创建阿里云图像生产 SDK 客户端（图像超分等）
func (s *SegService) createImageEnhanClient(cfg *model.AliyunSegConfig) (*imageenhan.Client, error) {
	regionID := cfg.RegionID
	if regionID == "" {
		regionID = "cn-shanghai"
	}
	config := &openapi.Config{
		AccessKeyId:     tea.String(cfg.AccessKeyID),
		AccessKeySecret: tea.String(cfg.AccessKeySecret),
		Endpoint:        tea.String(fmt.Sprintf("imageenhan.%s.aliyuncs.com", regionID)),
	}
	return imageenhan.NewClient(config)
}

// Upscale 执行图像超分，返回结果图片 URL
func (s *SegService) Upscale(imageURL string, mode string, upscaleFactor int64, outputFormat string, outputQuality int64) (string, error) {
	cfg, err := s.GetConfig()
	if err != nil || !cfg.Enabled {
		return "", fmt.Errorf("阿里云视觉AI服务未启用")
	}

	client, err := s.createImageEnhanClient(cfg)
	if err != nil {
		return "", fmt.Errorf("创建SDK客户端失败: %v", err)
	}

	stream, err := fetchImageStream(imageURL)
	if err != nil {
		return "", err
	}
	defer stream.Close()

	req := &imageenhan.MakeSuperResolutionImageAdvanceRequest{
		UrlObject:     stream,
		Mode:          dara.String(mode),
		UpscaleFactor: dara.Int64(upscaleFactor),
		OutputFormat:  dara.String(outputFormat),
		OutputQuality: dara.Int64(outputQuality),
	}

	runtime := &dara.RuntimeOptions{}
	resp, err := client.MakeSuperResolutionImageAdvance(req, runtime)
	if err != nil {
		return "", fmt.Errorf("图像超分失败: %v", err)
	}

	if resp.Body == nil || resp.Body.Data == nil || resp.Body.Data.Url == nil {
		return "", fmt.Errorf("图像超分未返回结果")
	}
	return dara.StringValue(resp.Body.Data.Url), nil
}

// Segment 执行分割，返回结果图片 URL
func (s *SegService) Segment(imageURL string, segType string, returnForm string) (string, error) {
	cfg, err := s.GetConfig()
	if err != nil || !cfg.Enabled {
		return "", fmt.Errorf("分割抠图服务未启用")
	}

	client, err := s.createClient(cfg)
	if err != nil {
		return "", fmt.Errorf("创建SDK客户端失败: %v", err)
	}

	// 从 URL 获取图片流
	stream, err := fetchImageStream(imageURL)
	if err != nil {
		return "", err
	}
	defer stream.Close()

	runtime := &util.RuntimeOptions{}

	switch segType {
	case "body":
		req := &imageseg.SegmentBodyAdvanceRequest{ImageURLObject: stream}
		if returnForm != "" {
			req.ReturnForm = tea.String(returnForm)
		}
		resp, err := client.SegmentBodyAdvance(req, runtime)
		if err != nil {
			return "", fmt.Errorf("人体分割失败: %v", err)
		}
		return tea.StringValue(resp.Body.Data.ImageURL), nil

	case "head":
		req := &imageseg.SegmentHeadAdvanceRequest{ImageURLObject: stream}
		resp, err := client.SegmentHeadAdvance(req, runtime)
		if err != nil {
			return "", fmt.Errorf("头像分割失败: %v", err)
		}
		if resp.Body.Data.Elements != nil && len(resp.Body.Data.Elements) > 0 {
			return tea.StringValue(resp.Body.Data.Elements[0].ImageURL), nil
		}
		return "", fmt.Errorf("头像分割未返回结果")

	case "clothing":
		req := &imageseg.SegmentClothAdvanceRequest{ImageURLObject: stream}
		resp, err := client.SegmentClothAdvance(req, runtime)
		if err != nil {
			return "", fmt.Errorf("服饰分割失败: %v", err)
		}
		if resp.Body.Data.Elements != nil && len(resp.Body.Data.Elements) > 0 {
			return tea.StringValue(resp.Body.Data.Elements[0].ImageURL), nil
		}
		return "", fmt.Errorf("服饰分割未返回结果")

	case "commodity":
		req := &imageseg.SegmentCommodityAdvanceRequest{ImageURLObject: stream}
		resp, err := client.SegmentCommodityAdvance(req, runtime)
		if err != nil {
			return "", fmt.Errorf("商品分割失败: %v", err)
		}
		return tea.StringValue(resp.Body.Data.ImageURL), nil

	case "common":
		req := &imageseg.SegmentCommonImageAdvanceRequest{ImageURLObject: stream}
		resp, err := client.SegmentCommonImageAdvance(req, runtime)
		if err != nil {
			return "", fmt.Errorf("通用分割失败: %v", err)
		}
		return tea.StringValue(resp.Body.Data.ImageURL), nil

	default:
		return "", fmt.Errorf("不支持的分割类型: %s", segType)
	}
}
