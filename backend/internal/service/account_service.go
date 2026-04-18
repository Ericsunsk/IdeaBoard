package service

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"backend/internal/model"
	"backend/internal/repository"
	"backend/pkg/redis"
	"backend/pkg/utils"
)

const (
	MaxFailCount            = 5
	styleFusionFeatureSlug  = "style-fusion"
	canvasExtendFeatureSlug = "canvas-extend"
	hdUpscaleFeatureSlug    = "hd-upscale"
)

const (
	styleFusionHiddenConstraintGeneric  = "【执行语义】图片输入与 Image A / Image B 的来源权重是主导信息。用户 prompt 作为辅助/弱修饰信号，用于描述融合方式、质感、细节、氛围、构图；若与主导信息冲突，以图片来源与滑块权重为准。"
	styleFusionHiddenConstraintWithNum  = "【执行语义】图片输入与 Image A / Image B 的来源权重是主导信息（Image A: %s，Image B: %s）。用户 prompt 作为辅助/弱修饰信号，用于描述融合方式、质感、细节、氛围、构图；若与主导信息冲突，以图片来源与滑块权重为准。"
	canvasExtendHiddenConstraintGeneric = "【执行语义】画布延伸以扩边参数为主导，决定上、右、下、左四个方向的延伸幅度。用户 prompt 只用于描述新扩展区域的内容、纹理、背景和补全方式；若与扩边方向冲突，以扩边参数为准。"
	canvasExtendHiddenConstraintWithNum = "【执行语义】画布延伸以扩边参数为主导（上 %s，右 %s，下 %s，左 %s），决定四个方向的延伸幅度。用户 prompt 只用于描述新扩展区域的内容、纹理、背景和补全方式；若与扩边方向冲突，以扩边参数为准。"
	hdUpscaleMaskedConstraint            = "【执行语义】当前请求为局部/遮罩高清增强（非整图放大）。请以原图（original_image_url）作为全局上下文，仅对蒙版（mask_image_url / mask_image_urls / mask_url）限定区域进行高清增强，未遮罩区域保持原图主体、结构与语义一致。"
)

func buildStyleFusionHiddenConstraint(params map[string]interface{}) string {
	if params == nil {
		return styleFusionHiddenConstraintGeneric
	}

	if imageAWeight, imageBWeight, ok := readStyleFusionWeights(params); ok {
		return fmt.Sprintf(
			styleFusionHiddenConstraintWithNum,
			formatWeightPercent(imageAWeight),
			formatWeightPercent(imageBWeight),
		)
	}

	return styleFusionHiddenConstraintGeneric
}

func buildCanvasExtendHiddenConstraint(params map[string]interface{}) string {
	if params == nil {
		return canvasExtendHiddenConstraintGeneric
	}

	top, hasTop := readCanvasExtendPercent(params["extend_top"])
	right, hasRight := readCanvasExtendPercent(params["extend_right"])
	bottom, hasBottom := readCanvasExtendPercent(params["extend_bottom"])
	left, hasLeft := readCanvasExtendPercent(params["extend_left"])
	if hasTop || hasRight || hasBottom || hasLeft {
		return fmt.Sprintf(
			canvasExtendHiddenConstraintWithNum,
			formatWeightPercent(top),
			formatWeightPercent(right),
			formatWeightPercent(bottom),
			formatWeightPercent(left),
		)
	}

	return canvasExtendHiddenConstraintGeneric
}

func readStyleFusionWeights(params map[string]interface{}) (float64, float64, bool) {
	if params == nil {
		return 0, 0, false
	}

	if rawA, okA := params["image_a_weight"]; okA {
		if rawB, okB := params["image_b_weight"]; okB {
			if imageAWeight, ok := parseWeight(rawA); ok {
				if imageBWeight, ok := parseWeight(rawB); ok {
					return normalizeWeights(imageAWeight, imageBWeight)
				}
			}
		}
	}

	if blendRatio, ok := params["blend_ratio"]; ok {
		if imageAWeight, ok := parseWeight(blendRatio); ok {
			if imageAWeight >= 0 && imageAWeight <= 1 {
				return normalizeWeights(imageAWeight, 1-imageAWeight)
			}
			return normalizeWeights(imageAWeight, 100-imageAWeight)
		}
	}

	return 0, 0, false
}

func parseWeight(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func readCanvasExtendPercent(v interface{}) (float64, bool) {
	value, ok := parseWeight(v)
	if !ok {
		return 0, false
	}
	if value < 0 {
		value = 0
	}
	if value > 100 {
		value = 100
	}
	return value, true
}

func normalizeWeights(a, b float64) (float64, float64, bool) {
	if a < 0 || b < 0 {
		return 0, 0, false
	}
	total := a + b
	if total <= 0 {
		return 0, 0, false
	}
	return a / total * 100, b / total * 100, true
}

func formatWeightPercent(weight float64) string {
	value := strconv.FormatFloat(weight, 'f', 1, 64)
	value = strings.TrimSuffix(value, ".0")
	return value + "%"
}

func appendHiddenConstraint(fullPrompt, constraint string) string {
	if strings.TrimSpace(constraint) == "" {
		return fullPrompt
	}
	if strings.TrimSpace(fullPrompt) == "" {
		return constraint
	}
	return fullPrompt + "\n\n" + constraint
}

func parseBoolLike(v interface{}) (bool, bool) {
	switch b := v.(type) {
	case bool:
		return b, true
	case float64:
		return b != 0, true
	case float32:
		return b != 0, true
	case int:
		return b != 0, true
	case int8:
		return b != 0, true
	case int16:
		return b != 0, true
	case int32:
		return b != 0, true
	case int64:
		return b != 0, true
	case uint:
		return b != 0, true
	case uint8:
		return b != 0, true
	case uint16:
		return b != 0, true
	case uint32:
		return b != 0, true
	case uint64:
		return b != 0, true
	case string:
		s := strings.TrimSpace(strings.ToLower(b))
		if s == "" {
			return false, false
		}
		switch s {
		case "1", "true", "yes", "y", "on", "enabled", "masked", "local", "partial":
			return true, true
		case "0", "false", "no", "n", "off", "disabled":
			return false, true
		}
		return false, false
	default:
		return false, false
	}
}

func hasNonEmptyString(v interface{}) bool {
	s, ok := v.(string)
	return ok && strings.TrimSpace(s) != ""
}

func hasNonEmptyStringList(v interface{}) bool {
	switch arr := v.(type) {
	case []string:
		for _, item := range arr {
			if strings.TrimSpace(item) != "" {
				return true
			}
		}
	case []interface{}:
		for _, item := range arr {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				return true
			}
		}
	}
	return false
}

func isMaskedHDUpscaleRequest(params map[string]interface{}) bool {
	if params == nil {
		return false
	}

	flagKeys := []string{
		"is_masked", "masked",
		"use_mask", "mask_enabled",
		"local_enhance", "is_local_enhance",
		"partial_enhance", "is_partial_enhance",
	}
	for _, key := range flagKeys {
		if raw, ok := params[key]; ok {
			if enabled, parsed := parseBoolLike(raw); parsed && enabled {
				return true
			}
		}
	}

	maskURLKeys := []string{"mask_image_url", "mask_url", "mask_image"}
	for _, key := range maskURLKeys {
		if hasNonEmptyString(params[key]) {
			return true
		}
	}

	maskListKeys := []string{"mask_image_urls", "mask_urls"}
	for _, key := range maskListKeys {
		if hasNonEmptyStringList(params[key]) {
			return true
		}
	}

	return hasNonEmptyString(params["original_image_url"]) && hasNonEmptyStringList(params["image_urls"])
}

type AccountService struct {
	accountRepo      *repository.AccountRepository
	taskRepo         *repository.TaskRepository
	redisMgr         *redis.RedisManager
	cardService      *StyleCardService
	lineDraftRepo    *repository.LineDraftModeRepository
	systemConfigRepo *repository.SystemConfigRepository
}

func NewAccountService(
	accountRepo *repository.AccountRepository,
	taskRepo *repository.TaskRepository,
	redisMgr *redis.RedisManager,
	cardService *StyleCardService,
	lineDraftRepo *repository.LineDraftModeRepository,
	systemConfigRepo *repository.SystemConfigRepository,
) *AccountService {
	return &AccountService{
		accountRepo:      accountRepo,
		taskRepo:         taskRepo,
		redisMgr:         redisMgr,
		cardService:      cardService,
		lineDraftRepo:    lineDraftRepo,
		systemConfigRepo: systemConfigRepo,
	}
}

// ============ 账号池管理 ============

func (s *AccountService) CreateAccount(req *model.CreateAccountRequest) (*model.ApiAccount, error) {
	if req.MaxConcurrent <= 0 {
		req.MaxConcurrent = 3
	}
	if req.Timeout <= 0 {
		req.Timeout = 120
	}
	if req.Weight <= 0 {
		req.Weight = 1
	}

	account := &model.ApiAccount{
		Name:          req.Name,
		ChannelID:     req.ChannelID,
		Platform:      req.Platform,
		ApiType:       req.ApiType,
		ModelSet:      req.ModelSet,
		ApiKey:        req.ApiKey,
		ProxyURL:      req.ProxyURL,
		MaxConcurrent: req.MaxConcurrent,
		Timeout:       req.Timeout,
		ExtraConfig:   req.ExtraConfig,
		Weight:        req.Weight,
		Status:        1,
	}

	if err := s.accountRepo.Create(account); err != nil {
		return nil, err
	}

	ctx := context.Background()
	s.redisMgr.InitAccountConcurrent(ctx, account.ID, account.MaxConcurrent)

	return account, nil
}

func (s *AccountService) GetAccount(id uint64) (*model.ApiAccount, error) {
	return s.accountRepo.GetByID(id)
}

func (s *AccountService) ListAccounts(req *model.AccountListRequest) ([]*model.AccountWithLoad, int64, error) {
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 || req.PageSize > 100 {
		req.PageSize = 20
	}

	accounts, total, err := s.accountRepo.List(req.Keyword, req.Platform, req.Page, req.PageSize)
	if err != nil {
		return nil, 0, err
	}

	ctx := context.Background()
	result := make([]*model.AccountWithLoad, len(accounts))

	for i, acc := range accounts {
		concurrent, _ := s.redisMgr.GetAccountConcurrent(ctx, acc.ID)
		waitQueue, _ := s.redisMgr.GetWaitQueueLength(ctx)

		status := "运行中"
		if acc.Status == 0 {
			status = "已停止"
		} else if acc.FailCount >= MaxFailCount {
			status = "故障"
		}

		result[i] = &model.AccountWithLoad{
			ApiAccount:        *acc,
			CurrentConcurrent: concurrent,
			WaitQueueCount:    int(waitQueue),
			RunningStatus:     status,
		}
	}

	return result, total, nil
}

func (s *AccountService) UpdateAccount(id uint64, req *model.UpdateAccountRequest) error {
	account, err := s.accountRepo.GetByID(id)
	if err != nil {
		return fmt.Errorf("账号不存在")
	}

	account.Name = req.Name
	if req.ChannelID > 0 {
		account.ChannelID = req.ChannelID
	}
	account.Platform = req.Platform
	account.ApiType = req.ApiType
	account.ModelSet = req.ModelSet
	account.ProxyURL = req.ProxyURL
	account.MaxConcurrent = req.MaxConcurrent
	account.Timeout = req.Timeout
	account.ExtraConfig = req.ExtraConfig
	account.Weight = req.Weight
	account.Status = req.Status

	if req.ApiKey != "" {
		account.ApiKey = req.ApiKey
	}

	if err := s.accountRepo.Update(account); err != nil {
		return err
	}

	ctx := context.Background()
	s.redisMgr.SetAccountMaxConcurrent(ctx, account.ID, account.MaxConcurrent)

	return nil
}

func (s *AccountService) DeleteAccount(id uint64) error {
	ctx := context.Background()
	s.redisMgr.DeleteAccountConcurrent(ctx, id)
	return s.accountRepo.Delete(id)
}

func (s *AccountService) StartAccount(id uint64) error {
	return s.accountRepo.UpdateStatus(id, 1)
}

func (s *AccountService) StopAccount(id uint64) error {
	// 1. 将该账号下所有 running/pending 的任务标记为 failed
	affected, err := s.taskRepo.FailRunningByAccountID(id, "账号已停用")
	if err != nil {
		log.Printf("[StopAccount] 标记账号 %d 任务失败: %v", id, err)
	} else if affected > 0 {
		log.Printf("[StopAccount] 已将账号 %d 的 %d 个进行中任务标记为失败", id, affected)
	}

	// 2. 清除 Redis 中该账号的并发计数（释放占位）
	ctx := context.Background()
	s.redisMgr.DeleteAccountConcurrent(ctx, id)

	// 3. 停用账号
	return s.accountRepo.UpdateStatus(id, 0)
}

func (s *AccountService) GetAccountQueueInfo(id uint64) (concurrent int, maxConcurrent int, err error) {
	ctx := context.Background()
	concurrent, err = s.redisMgr.GetAccountConcurrent(ctx, id)
	if err != nil {
		return 0, 0, err
	}
	maxConcurrent, err = s.redisMgr.GetAccountMaxConcurrent(ctx, id)
	if err != nil {
		return 0, 0, err
	}
	return concurrent, maxConcurrent, nil
}

// ============ 任务管理 ============

func (s *AccountService) SubmitTask(userID uint64, req *model.SubmitTaskRequest) (string, error) {
	// 获取所有可用的账号
	accounts, err := s.accountRepo.GetAllEnabled()
	if err != nil {
		return "", err
	}

	if len(accounts) == 0 {
		return "", fmt.Errorf("没有可用的 API 账号，请先添加账号")
	}

	// 生成任务ID
	taskID := fmt.Sprintf("task_%d_%d", time.Now().UnixNano(), rand.Intn(10000))

	params := req.Params
	if params == nil {
		params = map[string]interface{}{}
	}
	if req.Feature != "" {
		params["feature"] = req.Feature
	}
	if len(req.ImageURLs) > 0 {
		params["image_urls"] = req.ImageURLs
	}
	req.Params = params

	// 使用风格卡片拼接完整提示词
	fullPrompt := req.Prompt
	if req.CardIDs != nil && len(req.CardIDs) > 0 {
		p, err := s.cardService.BuildPrompt(req.Prompt, req.CardIDs)
		if err == nil {
			fullPrompt = p
		}
	}

	featureSlug := ""
	if req.Params != nil {
		if featureRaw, ok := req.Params["feature"]; ok {
			if feature, ok := featureRaw.(string); ok {
				featureSlug = feature
			}
		}
	}

	// 功能页系统提示词：根据 feature slug 查找系统预设 prompt，支持模板变量替换
	if req.Params != nil {
		if featureSlug != "" && s.systemConfigRepo != nil {
			sysPrompt, err := s.systemConfigRepo.Get("feature_prompt:" + featureSlug)
			if err == nil && sysPrompt != "" {
				// 构建模板变量映射
				varMap := map[string]string{
					"{{user_prompt}}": req.Prompt,
				}
				if req.Params != nil {
					if v, ok := req.Params["color_type"]; ok {
						varMap["{{color_type}}"] = fmt.Sprintf("%v", v)
					}
					if v, ok := req.Params["colors"]; ok {
						switch arr := v.(type) {
						case []interface{}:
							strs := make([]string, 0, len(arr))
							for _, item := range arr {
								strs = append(strs, fmt.Sprintf("%v", item))
							}
							varMap["{{colors}}"] = strings.Join(strs, ", ")
						default:
							varMap["{{colors}}"] = fmt.Sprintf("%v", v)
						}
					}
					if v, ok := req.Params["category"]; ok {
						varMap["{{category}}"] = fmt.Sprintf("%v", v)
					}
					if v, ok := req.Params["variation_strength"]; ok {
						varMap["{{strength}}"] = fmt.Sprintf("%v", v)
					}
					if v, ok := req.Params["blend_ratio"]; ok {
						varMap["{{blend_ratio}}"] = fmt.Sprintf("%v", v)
					}
					if v, ok := req.Params["extend_top"]; ok {
						varMap["{{extend_top}}"] = fmt.Sprintf("%v", v)
					}
					if v, ok := req.Params["extend_right"]; ok {
						varMap["{{extend_right}}"] = fmt.Sprintf("%v", v)
					}
					if v, ok := req.Params["extend_bottom"]; ok {
						varMap["{{extend_bottom}}"] = fmt.Sprintf("%v", v)
					}
					if v, ok := req.Params["extend_left"]; ok {
						varMap["{{extend_left}}"] = fmt.Sprintf("%v", v)
					}
					if v, ok := req.Params["image_a_weight"]; ok {
						varMap["{{image_a_weight}}"] = fmt.Sprintf("%v", v)
					}
					if v, ok := req.Params["image_b_weight"]; ok {
						varMap["{{image_b_weight}}"] = fmt.Sprintf("%v", v)
					}
					if v, ok := req.Params["target_color"]; ok {
						varMap["{{target_color}}"] = fmt.Sprintf("%v", v)
					}
					if v, ok := req.Params["fabric_rotation"]; ok {
						varMap["{{fabric_rotation}}"] = fmt.Sprintf("%v", v)
					}
					if v, ok := req.Params["fabric_opacity"]; ok {
						varMap["{{fabric_opacity}}"] = fmt.Sprintf("%v", v)
					}
					if v, ok := req.Params["fabric_scale"]; ok {
						varMap["{{fabric_scale}}"] = fmt.Sprintf("%v", v)
					}
					if v, ok := req.Params["tile_mode"]; ok {
						varMap["{{tile_mode}}"] = fmt.Sprintf("%v", v)
					}
					if v, ok := req.Params["pattern_mode"]; ok {
						varMap["{{pattern_mode}}"] = fmt.Sprintf("%v", v)
					}
					if v, ok := req.Params["angles"]; ok {
						varMap["{{angles}}"] = fmt.Sprintf("%v", v)
					}
					if v, ok := req.Params["pose"]; ok {
						varMap["{{pose}}"] = fmt.Sprintf("%v", v)
					}
					if v, ok := req.Params["expression"]; ok {
						varMap["{{expression}}"] = fmt.Sprintf("%v", v)
					}
					if v, ok := req.Params["background"]; ok {
						varMap["{{background}}"] = fmt.Sprintf("%v", v)
					}
					if v, ok := req.Params["viewpoints"]; ok {
						varMap["{{viewpoints}}"] = fmt.Sprintf("%v", v)
					}
					if v, ok := req.Params["lighting"]; ok {
						varMap["{{lighting}}"] = fmt.Sprintf("%v", v)
					}
					if v, ok := req.Params["model_url"]; ok {
						varMap["{{model_url}}"] = fmt.Sprintf("%v", v)
					}
					if v, ok := req.Params["pose_url"]; ok {
						varMap["{{pose_url}}"] = fmt.Sprintf("%v", v)
					}
					if v, ok := req.Params["style_model"]; ok {
						varMap["{{style_model}}"] = fmt.Sprintf("%v", v)
					}
				}

				// 检查是否包含模板变量
				if strings.Contains(sysPrompt, "{{") {
					// 模板模式：替换所有 {{var}}
					result := sysPrompt
					for k, v := range varMap {
						result = strings.ReplaceAll(result, k, v)
					}
					fullPrompt = result
				} else {
					// 兼容模式：系统提示词 + 用户提示词
					fullPrompt = sysPrompt + " " + fullPrompt
				}
			}
		}
	}

	// 线稿模式：用模式的 prompt 替换用户 prompt
	if req.Params != nil {
		if modeIDRaw, ok := req.Params["line_draft_mode_id"]; ok && s.lineDraftRepo != nil {
			var modeID uint64
			switch v := modeIDRaw.(type) {
			case float64:
				modeID = uint64(v)
			case int:
				modeID = uint64(v)
			}
			if modeID > 0 {
				mode, err := s.lineDraftRepo.GetByID(modeID)
				if err == nil && mode.Prompt != "" {
					fullPrompt = mode.Prompt
				}
			}
		}
	}

	if featureSlug == styleFusionFeatureSlug {
		fullPrompt = appendHiddenConstraint(fullPrompt, buildStyleFusionHiddenConstraint(req.Params))
	}
	if featureSlug == canvasExtendFeatureSlug {
		fullPrompt = appendHiddenConstraint(fullPrompt, buildCanvasExtendHiddenConstraint(req.Params))
	}
	if featureSlug == hdUpscaleFeatureSlug && isMaskedHDUpscaleRequest(req.Params) {
		fullPrompt = appendHiddenConstraint(fullPrompt, hdUpscaleMaskedConstraint)
	}

	// 序列化参数
	paramsJSON := ""
	if req.Params != nil {
		b, _ := utils.JSONMarshal(req.Params)
		paramsJSON = string(b)
	}

	// 创建任务记录
	task := &model.Task{
		TaskID:     taskID,
		UserID:     userID,
		ChannelID:  0,
		Prompt:     fullPrompt,
		UserPrompt: req.Prompt,
		Params:     paramsJSON,
		Status:     "pending",
	}

	if err := s.taskRepo.Create(task); err != nil {
		return "", err
	}

	// 获取所有可用账号ID
	accountIDs := make([]uint64, len(accounts))
	for i, acc := range accounts {
		accountIDs[i] = acc.ID
	}

	// 推送到全局队列
	queueTask := &redis.GlobalQueueTask{
		TaskID:     taskID,
		UserID:     userID,
		Prompt:     fullPrompt,
		Params:     paramsJSON,
		AccountIDs: accountIDs,
		RetryCount: 0,
		CreatedAt:  time.Now().Unix(),
	}

	if err := s.redisMgr.PushGlobalQueue(context.Background(), queueTask); err != nil {
		_ = s.taskRepo.MarkQueueFailed(taskID, "任务入队失败: "+err.Error())
		return "", err
	}

	return taskID, nil
}

func (s *AccountService) GetTask(taskID string) (*model.Task, error) {
	return s.taskRepo.GetByTaskID(taskID)
}

func (s *AccountService) GetUserTask(taskID string, userID uint64) (*model.Task, error) {
	return s.taskRepo.GetByTaskIDAndUserID(taskID, userID)
}

func (s *AccountService) ListTasks(req *model.TaskListRequest) ([]*model.Task, int64, error) {
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 || req.PageSize > 100 {
		req.PageSize = 20
	}
	return s.taskRepo.List(req.AccountID, req.Status, req.Page, req.PageSize)
}

func (s *AccountService) DeleteUserTask(taskID string, userID uint64) error {
	return s.taskRepo.DeleteByTaskIDAndUserID(taskID, userID)
}

func (s *AccountService) GetUserTasks(userID uint64, page, pageSize int) ([]*model.Task, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return s.taskRepo.GetByUserID(userID, page, pageSize)
}

// ============ 监控 ============

func (s *AccountService) GetMonitorInfo() (*redis.MonitorInfo, error) {
	ctx := context.Background()
	accounts, _ := s.accountRepo.GetAllEnabled()
	accountIDs := make([]uint64, len(accounts))
	for i, acc := range accounts {
		accountIDs[i] = acc.ID
	}
	return s.redisMgr.GetMonitorInfo(ctx, accountIDs)
}

func (s *AccountService) GetTaskStats() (map[string]int64, error) {
	pending, _ := s.taskRepo.CountByStatus("pending")
	running, _ := s.taskRepo.CountByStatus("running")
	completed, _ := s.taskRepo.CountByStatus("completed")
	failed, _ := s.taskRepo.CountByStatus("failed")

	return map[string]int64{
		"pending":   pending,
		"running":   running,
		"completed": completed,
		"failed":    failed,
	}, nil
}

// ============ 消费服务调用 ============

func (s *AccountService) GetNextTaskFromQueue() (*redis.GlobalQueueTask, error) {
	ctx := context.Background()
	return s.redisMgr.PopGlobalQueue(ctx)
}

func (s *AccountService) AssignTaskToAccount(task *redis.GlobalQueueTask) (uint64, string, error) {
	ctx := context.Background()
	return s.redisMgr.AssignTask(ctx, task.AccountIDs, task)
}

func (s *AccountService) ReleaseTask(accountID uint64, taskID string) error {
	return s.redisMgr.ReleaseTask(context.Background(), accountID, taskID)
}

func (s *AccountService) MarkTaskRunning(taskID string, accountID uint64) error {
	task, err := s.taskRepo.GetByTaskID(taskID)
	if err != nil {
		return err
	}
	task.Status = "running"
	task.AccountID = accountID
	return s.taskRepo.Update(task)
}

func (s *AccountService) MarkTaskCompleted(taskID string, result string) error {
	task, err := s.taskRepo.GetByTaskID(taskID)
	if err != nil {
		return err
	}
	task.Status = "completed"
	task.Result = result
	now := time.Now()
	task.CompletedAt = &now
	return s.taskRepo.Update(task)
}

func (s *AccountService) MarkTaskFailed(taskID string, accountID uint64, errMsg string) error {
	task, err := s.taskRepo.GetByTaskID(taskID)
	if err != nil {
		return err
	}

	task.RetryCount++
	task.ErrorMsg = errMsg

	if task.RetryCount >= 3 {
		task.Status = "failed"
		now := time.Now()
		task.CompletedAt = &now
	} else {
		task.Status = "pending"
	}

	s.accountRepo.IncrementFailCount(accountID)

	account, _ := s.accountRepo.GetByID(accountID)
	if account != nil && account.FailCount >= MaxFailCount {
		s.accountRepo.UpdateStatus(accountID, 0)
	}

	return s.taskRepo.Update(task)
}
