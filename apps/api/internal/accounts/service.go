package accounts

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"api/internal/platform/redis"
	"api/internal/platform/settings"
	"api/internal/prompts"
	"api/internal/tasks"
	"api/internal/workflow"
)

type AccountService struct {
	accountRepo      *AccountRepository
	taskRepo         *TaskRepository
	redisMgr         *redis.RedisManager
	taskRuntime      *TaskRuntimeService
	systemConfigRepo *settings.SystemConfigRepository
	previewResolver  previewURLResolver
}

type previewURLResolver interface {
	PreviewURLTransformer() func(string) string
}

const defaultAccountMaxConcurrent = 3
const taskCancelTTL = 24 * time.Hour
const defaultAccountTimeoutSeconds = 120
const maxTaskKeywordLength = 200

func NewAccountService(
	accountRepo *AccountRepository,
	taskRepo *TaskRepository,
	redisMgr *redis.RedisManager,
	systemConfigRepo *settings.SystemConfigRepository,
	previewResolver previewURLResolver,
) *AccountService {
	return &AccountService{
		accountRepo:      accountRepo,
		taskRepo:         taskRepo,
		redisMgr:         redisMgr,
		taskRuntime:      NewTaskRuntimeService(accountRepo, taskRepo, redisMgr),
		systemConfigRepo: systemConfigRepo,
		previewResolver:  previewResolver,
	}
}

// ============ 账号池管理 ============

func (s *AccountService) CreateAccount(req *CreateAccountRequest) (*ApiAccount, error) {
	req.MaxConcurrent = normalizeMaxConcurrent(req.MaxConcurrent)
	req.Timeout = NormalizeAccountTimeout(req.Timeout)
	if req.Weight <= 0 {
		req.Weight = 1
	}

	account := &ApiAccount{
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

	if s.redisMgr != nil {
		ctx := context.Background()
		s.redisMgr.InitAccountConcurrent(ctx, account.ID, account.MaxConcurrent)
	}

	return account, nil
}

func (s *AccountService) GetAccount(id uint64) (*ApiAccount, error) {
	return s.accountRepo.GetByID(id)
}

func (s *AccountService) ListAccounts(req *AccountListRequest) ([]*AccountWithLoad, int64, error) {
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
	result := make([]*AccountWithLoad, len(accounts))

	for i, acc := range accounts {
		concurrent, _ := s.redisMgr.GetAccountConcurrent(ctx, acc.ID)
		waitQueue, _ := s.redisMgr.GetWaitQueueLength(ctx)

		status := "运行中"
		if acc.Status == 0 {
			status = "已停止"
		} else if acc.FailCount >= MaxFailCount {
			status = "故障"
		}

		result[i] = &AccountWithLoad{
			ApiAccount:        *acc,
			CurrentConcurrent: concurrent,
			WaitQueueCount:    int(waitQueue),
			RunningStatus:     status,
		}
	}

	return result, total, nil
}

func (s *AccountService) UpdateAccount(id uint64, req *UpdateAccountRequest) error {
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
	account.MaxConcurrent = normalizeMaxConcurrent(req.MaxConcurrent)
	account.Timeout = NormalizeAccountTimeout(req.Timeout)
	account.ExtraConfig = req.ExtraConfig
	account.Weight = req.Weight
	account.Status = req.Status

	if req.ApiKey != "" {
		account.ApiKey = req.ApiKey
	}

	if err := s.accountRepo.Update(account); err != nil {
		return err
	}

	if s.redisMgr != nil {
		ctx := context.Background()
		s.redisMgr.SetAccountMaxConcurrent(ctx, account.ID, account.MaxConcurrent)
	}

	return nil
}

func (s *AccountService) DeleteAccount(id uint64) error {
	ctx := context.Background()
	s.redisMgr.DeleteAccountConcurrent(ctx, id)
	return s.accountRepo.Delete(id)
}

func (s *AccountService) StartAccount(id uint64) error {
	account, err := s.accountRepo.GetByID(id)
	if err != nil {
		return fmt.Errorf("账号不存在")
	}

	account.MaxConcurrent = normalizeMaxConcurrent(account.MaxConcurrent)
	if err := s.accountRepo.UpdateStatus(id, 1); err != nil {
		return err
	}

	ctx := context.Background()
	return s.redisMgr.EnsureAccountConcurrent(ctx, id, account.MaxConcurrent)
}

func (s *AccountService) StopAccount(id uint64) error {
	ctx := context.Background()

	taskIDs, err := s.taskRepo.ListTaskIDsByAccountIDAndStatuses(id, tasks.StatusPending, tasks.StatusRunning)
	if err != nil {
		log.Printf("[StopAccount] 查询账号 %d 进行中任务失败: %v", id, err)
	} else {
		for _, taskID := range taskIDs {
			if cancelErr := s.redisMgr.MarkTaskCancelled(ctx, taskID, taskCancelTTL); cancelErr != nil {
				log.Printf("[StopAccount] 标记任务取消失败: account=%d task=%s err=%v", id, taskID, cancelErr)
			}
		}
		if len(taskIDs) > 0 {
			log.Printf("[StopAccount] 账号 %d 已标记 %d 个任务取消", id, len(taskIDs))
		}
	}

	// 1. 将该账号下所有 running/pending 的任务标记为 failed
	affected, err := s.taskRepo.FailRunningByAccountID(id, "账号已停用")
	if err != nil {
		log.Printf("[StopAccount] 标记账号 %d 任务失败: %v", id, err)
	} else if affected > 0 {
		log.Printf("[StopAccount] 已将账号 %d 的 %d 个进行中任务标记为失败", id, affected)
	}

	// 2. 清除 Redis 中该账号的并发计数（释放占位）
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

func (s *AccountService) SubmitTask(userID uint64, req *tasks.SubmitTaskRequest) (string, error) {
	// 获取所有可用的账号
	accounts, err := s.accountRepo.GetAllEnabled()
	if err != nil {
		return "", fmt.Errorf("任务提交失败，请稍后重试")
	}

	if len(accounts) == 0 {
		return "", fmt.Errorf("没有可用的 API 账号，请先添加账号")
	}

	// 生成任务ID
	taskID := generateSecureTaskID("task")

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

	fullPrompt := req.Prompt

	featureSlug := ""
	if req.Params != nil {
		featureSlug, err = workflow.ResolveWorkflowFeatureSlug(req.Params)
		if err != nil {
			return "", err
		}
		if featureSlug != "" {
			req.Params["feature"] = featureSlug
		}
	}
	accounts = filterAccountsByFeature(accounts, featureSlug)
	if len(accounts) == 0 {
		return "", fmt.Errorf("没有可用的 API 账号，请先添加账号")
	}

	fullPrompt = applyTaskSystemPrompt(fullPrompt, req.Prompt, featureSlug, req.Params, func(slug string) (string, error) {
		if s.systemConfigRepo == nil {
			return "", fmt.Errorf("system config repo 未配置")
		}
		return s.systemConfigRepo.Get(prompts.FeaturePromptConfigKey(slug))
	})
	if err := validateExecutablePrompt(fullPrompt); err != nil {
		return "", err
	}

	// 序列化参数
	paramsJSON := ""
	if req.Params != nil {
		b, _ := json.Marshal(req.Params)
		paramsJSON = string(b)
	}

	// 创建任务记录
	task := &tasks.Task{
		TaskID:     taskID,
		UserID:     userID,
		ChannelID:  0,
		Prompt:     fullPrompt,
		UserPrompt: req.Prompt,
		Params:     paramsJSON,
		Status:     tasks.StatusPending,
	}

	if err := s.taskRepo.Create(task); err != nil {
		return "", fmt.Errorf("任务提交失败，请稍后重试")
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
		if failedTask, loadErr := s.taskRepo.GetByTaskID(taskID); loadErr == nil {
			if statErr := s.taskRepo.UpsertDailyStatByTask(failedTask); statErr != nil {
				log.Printf("[AccountService] upsert daily stat failed after queue fail, task_id=%s err=%v", taskID, statErr)
			}
		} else {
			log.Printf("[AccountService] load task failed after queue fail, task_id=%s err=%v", taskID, loadErr)
		}
		return "", fmt.Errorf("任务提交失败，请稍后重试")
	}

	return taskID, nil
}

func (s *AccountService) GetTask(taskID string) (*tasks.Task, error) {
	task, err := s.taskRepo.GetByTaskID(taskID)
	if err != nil {
		return nil, err
	}
	s.applyPreviewForTasks([]*tasks.Task{task})
	return task, nil
}

func (s *AccountService) GetUserTask(taskID string, userID uint64) (*tasks.Task, error) {
	task, err := s.taskRepo.GetByTaskIDAndUserID(taskID, userID)
	if err != nil {
		return nil, err
	}
	s.applyPreviewForTasks([]*tasks.Task{task})
	return task, nil
}

func (s *AccountService) ListTasks(req *tasks.TaskListRequest) ([]*tasks.Task, int64, error) {
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 || req.PageSize > 100 {
		req.PageSize = 20
	}
	req.Keyword = strings.TrimSpace(req.Keyword)
	if len(req.Keyword) > maxTaskKeywordLength {
		req.Keyword = req.Keyword[:maxTaskKeywordLength]
	}
	items, total, err := s.taskRepo.List(req.UserID, req.AccountID, req.Status, req.Keyword, req.Page, req.PageSize)
	if err != nil {
		return nil, 0, err
	}
	s.applyPreviewForTasks(items)
	return items, total, nil
}

func (s *AccountService) DeleteUserTask(taskID string, userID uint64) error {
	if _, err := s.taskRepo.GetByTaskIDAndUserID(taskID, userID); err != nil {
		return err
	}
	if err := s.redisMgr.MarkTaskCancelled(context.Background(), taskID, taskCancelTTL); err != nil {
		return err
	}
	return s.taskRepo.DeleteByTaskIDAndUserID(taskID, userID)
}

func (s *AccountService) GetUserTasks(userID uint64, page, pageSize int) ([]*tasks.Task, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	items, total, err := s.taskRepo.GetByUserID(userID, page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	s.applyPreviewForTasks(items)
	return items, total, nil
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

func normalizeMaxConcurrent(value int) int {
	if value <= 0 {
		return defaultAccountMaxConcurrent
	}
	return value
}

func NormalizeAccountTimeout(value int) int {
	if value <= 0 {
		return defaultAccountTimeoutSeconds
	}
	return value
}

func (s *AccountService) GetTaskStats() (map[string]int64, error) {
	pending, _ := s.taskRepo.CountByStatus(tasks.StatusPending)
	running, _ := s.taskRepo.CountByStatus(tasks.StatusRunning)
	completed, _ := s.taskRepo.CountByStatus(tasks.StatusCompleted)
	failed, _ := s.taskRepo.CountByStatus(tasks.StatusFailed)

	return map[string]int64{
		tasks.StatusPending:   pending,
		tasks.StatusRunning:   running,
		tasks.StatusCompleted: completed,
		tasks.StatusFailed:    failed,
	}, nil
}

func normalizeStatisticsRange(raw string) (StatisticsRange, int) {
	switch StatisticsRange(strings.TrimSpace(strings.ToLower(raw))) {
	case StatisticsRange30D:
		return StatisticsRange30D, 30
	case StatisticsRange1Y:
		return StatisticsRange1Y, 365
	default:
		return StatisticsRange7D, 7
	}
}

func (s *AccountService) GetStatistics(rawRange string) (*AdminStatisticsResponse, error) {
	_, days := normalizeStatisticsRange(rawRange)
	now := time.Now()
	loc := now.Location()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	yesterdayStart := todayStart.AddDate(0, 0, -1)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	rangeStart := todayStart.AddDate(0, 0, -(days - 1))

	totalCalls, err := s.taskRepo.CountAllTasks()
	if err != nil {
		return nil, err
	}

	stats, err := s.GetTaskStats()
	if err != nil {
		return nil, err
	}

	rangeStartDate := rangeStart.Format("2006-01-02")
	rangeEndDate := todayStart.Format("2006-01-02")
	backfilledDates, err := s.taskRepo.ListBackfilledDatesByRange(rangeStartDate, rangeEndDate)
	if err != nil {
		return nil, err
	}
	missingDays := buildMissingStatisticsDaysByMarker(rangeStart, todayStart, days, backfilledDates)

	rangeDailyRows, err := s.taskRepo.ListDailyStatsByDateRange(rangeStartDate, rangeEndDate)
	if err != nil {
		return nil, err
	}

	yesterdayDate := yesterdayStart.Format("2006-01-02")
	yesterdayRows, err := s.taskRepo.ListDailyStatsByDateRange(yesterdayDate, yesterdayDate)
	if err != nil {
		return nil, err
	}
	monthRows, err := s.taskRepo.ListDailyStatsByDateRange(monthStart.Format("2006-01-02"), rangeEndDate)
	if err != nil {
		return nil, err
	}

	type trendBucket struct {
		date   string
		label  string
		total  int64
		models map[string]int64
	}
	buckets := make(map[string]*trendBucket, days)
	orderedDays := make([]string, 0, days)
	for i := 0; i < days; i++ {
		d := rangeStart.AddDate(0, 0, i)
		dateKey := d.Format("2006-01-02")
		buckets[dateKey] = &trendBucket{
			date:   dateKey,
			label:  d.Format("01/02"),
			total:  0,
			models: map[string]int64{},
		}
		orderedDays = append(orderedDays, dateKey)
	}

	modelCounter := map[string]int64{}
	userCounter := map[string]int64{}
	type latencyAcc struct {
		total float64
		count int64
	}
	latencyCounter := map[string]*latencyAcc{}
	healthCells := make([]AdminStatisticsHealthCell, days)
	for i := range healthCells {
		healthCells[i] = AdminStatisticsHealthCell{Status: 0, Success: 0, Fail: 0, Total: 0}
	}
	dayIndexByDate := make(map[string]int, len(orderedDays))
	for i, dayKey := range orderedDays {
		dayIndexByDate[dayKey] = i
	}

	var todayCalls, todaySuccess, todayFailed int64
	for _, row := range rangeDailyRows {
		dateKey := row.StatDate
		model := row.ModelName
		user := row.UserKey
		if bucket, ok := buckets[dateKey]; ok {
			bucket.total += row.Total
			bucket.models[model] += row.Total
		}
		modelCounter[model] += row.Total
		userCounter[user] += row.Total

		if row.LatencyCount > 0 {
			acc := latencyCounter[model]
			if acc == nil {
				acc = &latencyAcc{}
				latencyCounter[model] = acc
			}
			acc.total += row.LatencySecondsSum
			acc.count += row.LatencyCount
		}

		if cellIdx, ok := dayIndexByDate[dateKey]; ok && cellIdx >= 0 && cellIdx < len(healthCells) {
			healthCells[cellIdx].Total += row.Total
			healthCells[cellIdx].Success += row.Completed
			healthCells[cellIdx].Fail += row.Failed
		}

		if dateKey == todayStart.Format("2006-01-02") {
			todayCalls += row.Total
			todaySuccess += row.Completed
			todayFailed += row.Failed
		}
	}

	for i := range healthCells {
		cell := &healthCells[i]
		if cell.Total == 0 {
			cell.Status = 0
			continue
		}
		ratio := float64(cell.Success) / float64(cell.Total)
		if ratio < 0.5 {
			cell.Status = 1
		} else if ratio < 0.9 {
			cell.Status = 2
		} else {
			cell.Status = 3
		}
	}

	var yesterdayCalls, yesterdaySuccess, yesterdayFailed int64
	for _, row := range yesterdayRows {
		yesterdayCalls += row.Total
		yesterdaySuccess += row.Completed
		yesterdayFailed += row.Failed
	}

	trend := make([]AdminStatisticsTrendItem, 0, len(orderedDays))
	for _, dayKey := range orderedDays {
		b := buckets[dayKey]
		segments := make([]AdminStatisticsTrendItemModel, 0, len(b.models))
		for model, value := range b.models {
			segments = append(segments, AdminStatisticsTrendItemModel{Label: model, Value: value})
		}
		sort.Slice(segments, func(i, j int) bool {
			if segments[i].Value == segments[j].Value {
				return segments[i].Label < segments[j].Label
			}
			return segments[i].Value > segments[j].Value
		})
		if len(segments) > 3 {
			segments = segments[:3]
		}
		trend = append(trend, AdminStatisticsTrendItem{
			Date:     b.date,
			Label:    b.label,
			Total:    b.total,
			Segments: segments,
		})
	}

	modelRanking := make([]AdminStatisticsModelItem, 0, len(modelCounter))
	var rangeTotalCalls int64
	for _, row := range rangeDailyRows {
		rangeTotalCalls += row.Total
	}
	for name, value := range modelCounter {
		percent := int64(0)
		if rangeTotalCalls > 0 {
			percent = int64((float64(value) / float64(rangeTotalCalls)) * 100)
		}
		modelRanking = append(modelRanking, AdminStatisticsModelItem{Name: name, Value: value, Percent: percent})
	}
	sort.Slice(modelRanking, func(i, j int) bool {
		if modelRanking[i].Value == modelRanking[j].Value {
			return modelRanking[i].Name < modelRanking[j].Name
		}
		return modelRanking[i].Value > modelRanking[j].Value
	})
	if len(modelRanking) > 5 {
		modelRanking = modelRanking[:5]
	}

	userRank := make([]AdminStatisticsUserItem, 0, len(userCounter))
	for name, value := range userCounter {
		userRank = append(userRank, AdminStatisticsUserItem{Name: name, Value: value})
	}
	sort.Slice(userRank, func(i, j int) bool {
		if userRank[i].Value == userRank[j].Value {
			return userRank[i].Name < userRank[j].Name
		}
		return userRank[i].Value > userRank[j].Value
	})
	if len(userRank) > 6 {
		userRank = userRank[:6]
	}

	latency := make([]AdminStatisticsLatencyItem, 0, len(latencyCounter))
	for name, acc := range latencyCounter {
		if acc.count == 0 {
			continue
		}
		latency = append(latency, AdminStatisticsLatencyItem{
			Name:       name,
			AvgSeconds: acc.total / float64(acc.count),
		})
	}
	sort.Slice(latency, func(i, j int) bool {
		if latency[i].AvgSeconds == latency[j].AvgSeconds {
			return latency[i].Name < latency[j].Name
		}
		return latency[i].AvgSeconds < latency[j].AvgSeconds
	})

	return &AdminStatisticsResponse{
		Summary: AdminStatisticsSummary{
			TotalCalls:       totalCalls,
			TodayCalls:       todayCalls,
			TodaySuccess:     todaySuccess,
			TodayFailed:      todayFailed,
			YesterdayCalls:   yesterdayCalls,
			YesterdaySuccess: yesterdaySuccess,
			YesterdayFailed:  yesterdayFailed,
			ThisMonthCalls:   sumDailyTotal(monthRows),
			Pending:          stats[tasks.StatusPending],
			Running:          stats[tasks.StatusRunning],
			Completed:        stats[tasks.StatusCompleted],
			Failed:           stats[tasks.StatusFailed],
		},
		Trend:        trend,
		ModelRanking: modelRanking,
		UserRank:     userRank,
		Latency:      latency,
		HealthCells:  healthCells,
		Backfill: AdminStatisticsBackfillState{
			RangeStart:     rangeStartDate,
			RangeEnd:       rangeEndDate,
			MissingDays:    len(missingDays),
			BackfilledDays: len(backfilledDates),
		},
	}, nil
}

func (s *AccountService) BackfillStatistics(rawRange string) (*AdminStatisticsBackfillResult, error) {
	normalizedRange, days := normalizeStatisticsRange(rawRange)
	now := time.Now()
	loc := now.Location()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	rangeStart := todayStart.AddDate(0, 0, -(days - 1))
	rangeStartDate := rangeStart.Format("2006-01-02")
	rangeEndDate := todayStart.Format("2006-01-02")

	backfilledDates, err := s.taskRepo.ListBackfilledDatesByRange(rangeStartDate, rangeEndDate)
	if err != nil {
		return nil, fmt.Errorf("list backfilled dates: %w", err)
	}
	missingDays := buildMissingStatisticsDaysByMarker(rangeStart, todayStart, days, backfilledDates)
	backfilledCount, err := s.backfillDailyStatsByMissingDays(missingDays)
	if err != nil {
		return nil, fmt.Errorf("backfill daily stats: %w", err)
	}

	return &AdminStatisticsBackfillResult{
		Range:          normalizedRange,
		RangeStart:     rangeStartDate,
		RangeEnd:       rangeEndDate,
		ScannedDays:    len(missingDays),
		BackfilledDays: backfilledCount,
	}, nil
}

func sumDailyTotal(rows []DailyStatAggregate) int64 {
	var total int64
	for _, row := range rows {
		total += row.Total
	}
	return total
}

func buildMissingStatisticsDaysByMarker(rangeStart, todayStart time.Time, days int, markerDates []string) []time.Time {
	markers := make(map[string]struct{}, len(markerDates))
	for _, date := range markerDates {
		markers[date] = struct{}{}
	}
	missing := make([]time.Time, 0)
	for i := 0; i < days; i++ {
		dayStart := rangeStart.AddDate(0, 0, i)
		dayKey := dayStart.Format("2006-01-02")
		if dayKey == todayStart.Format("2006-01-02") {
			// 今天始终允许回填，覆盖部署前同一天历史终态任务；事件幂等保证不会重复计数。
			missing = append(missing, dayStart)
			continue
		}
		if _, ok := markers[dayKey]; ok {
			continue
		}
		missing = append(missing, dayStart)
	}
	return missing
}

func (s *AccountService) backfillDailyStatsByMissingDays(days []time.Time) (int, error) {
	if len(days) == 0 {
		return 0, nil
	}
	loc := time.Now().Location()
	todayDate := time.Now().In(loc).Format("2006-01-02")
	backfilledCount := 0
	for _, dayStart := range days {
		dayEnd := dayStart.AddDate(0, 0, 1)
		rows, err := s.taskRepo.ListStatisticsByCreatedAtRange(dayStart, dayEnd)
		if err != nil {
			return backfilledCount, err
		}
		accountIDs := make([]uint64, 0, len(rows))
		for _, row := range rows {
			if row.AccountID != 0 {
				accountIDs = append(accountIDs, row.AccountID)
			}
		}
		modelSetMap, err := s.taskRepo.listSingleModelSetByAccountIDs(accountIDs)
		if err != nil {
			return backfilledCount, err
		}
		for _, row := range rows {
			if row.Status != tasks.StatusCompleted && row.Status != tasks.StatusFailed {
				continue
			}
			task := &tasks.Task{
				TaskID:      row.TaskID,
				UserID:      row.UserID,
				AccountID:   row.AccountID,
				Status:      row.Status,
				Result:      row.Result,
				CreatedAt:   row.CreatedAt.In(loc),
				CompletedAt: row.CompletedAt,
			}
			if err := s.taskRepo.UpsertDailyStatByTaskWithFallbackModel(task, modelSetMap[row.AccountID]); err != nil {
				return backfilledCount, err
			}
		}
		dayDate := dayStart.In(loc).Format("2006-01-02")
		if dayDate != todayDate {
			if err := s.taskRepo.MarkDailyBackfillDone(dayDate); err != nil {
				return backfilledCount, err
			}
			backfilledCount++
		}
	}
	return backfilledCount, nil
}

func parseResultMap(raw string) map[string]interface{} {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil
	}
	return result
}

func extractTaskModel(rawResult string) string {
	return extractTaskModelWithFallback(rawResult, "")
}

func extractTaskModelWithFallback(rawResult string, fallbackModelSet string) string {
	result := parseResultMap(rawResult)
	if result != nil {
		if model := readResultString(result, "model"); model != "" {
			return model
		}
		if modelSet := singleModelFromSet(readResultString(result, "model_set")); modelSet != "" {
			return modelSet
		}
	}
	if fallback := singleModelFromSet(fallbackModelSet); fallback != "" {
		return fallback
	}
	return "未识别模型"
}

func singleModelFromSet(modelSet string) string {
	model := strings.TrimSpace(modelSet)
	if model == "" || strings.Contains(model, ",") {
		return ""
	}
	return model
}

func readResultString(result map[string]interface{}, key string) string {
	v, ok := result[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func extractTaskUser(rawResult string, fallbackUserID uint64) string {
	result := parseResultMap(rawResult)
	if result != nil {
		for _, key := range []string{"username", "email", "user_id"} {
			if v, ok := result[key]; ok {
				switch vv := v.(type) {
				case string:
					if strings.TrimSpace(vv) != "" {
						return strings.TrimSpace(vv)
					}
				case float64:
					if vv > 0 {
						return fmt.Sprintf("%.0f", vv)
					}
				}
			}
		}
	}
	if fallbackUserID == 0 {
		return "unknown"
	}
	return fmt.Sprintf("%d", fallbackUserID)
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
	return s.taskRuntime.MarkTaskRunning(taskID, accountID)
}

func (s *AccountService) MarkTaskCompleted(taskID string, result string) error {
	return s.taskRuntime.MarkTaskCompleted(taskID, result)
}

func (s *AccountService) MarkTaskFailed(taskID string, accountID uint64, errMsg string) error {
	return s.taskRuntime.MarkTaskFailedWithRetry(taskID, accountID, errMsg)
}

// generateSecureTaskID produces a non-guessable task identifier using crypto/rand.
func generateSecureTaskID(prefix string) string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		// Extremely unlikely; fall back to timestamp only.
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(b))
}

func applyTaskSystemPrompt(
	basePrompt string,
	userPrompt string,
	featureSlug string,
	params map[string]interface{},
	loadPrompt func(string) (string, error),
) string {
	// 管理员后台配置是系统提示词唯一来源；这里不再追加代码层隐藏约束。
	return prompts.ApplyFeatureSystemPrompt(basePrompt, userPrompt, featureSlug, params, loadPrompt)
}

func validateExecutablePrompt(fullPrompt string) error {
	if strings.TrimSpace(fullPrompt) == "" {
		return fmt.Errorf("缺少提示词，请填写 Prompt 或配置后台 system prompt")
	}
	return nil
}

func (s *AccountService) applyPreviewForTasks(items []*tasks.Task) {
	if len(items) == 0 || s.previewResolver == nil {
		return
	}
	transform := s.previewResolver.PreviewURLTransformer()
	if transform == nil {
		return
	}
	for _, item := range items {
		s.applyPreviewForTask(item, transform)
	}
}

func (s *AccountService) applyPreviewForTask(task *tasks.Task, transform func(string) string) {
	if task == nil || strings.TrimSpace(task.Result) == "" {
		return
	}
	if !strings.Contains(task.Result, "image_url") && !strings.Contains(task.Result, "image_urls") {
		return
	}

	var resultMap map[string]interface{}
	if err := json.Unmarshal([]byte(task.Result), &resultMap); err != nil {
		return
	}

	changed := false
	if raw, ok := resultMap["image_url"].(string); ok {
		next := transform(strings.TrimSpace(raw))
		if next != raw {
			resultMap["image_url"] = next
			changed = true
		}
	}

	if rawList, ok := resultMap["image_urls"].([]interface{}); ok {
		for i, item := range rawList {
			raw, ok := item.(string)
			if !ok {
				continue
			}
			next := transform(strings.TrimSpace(raw))
			if next != raw {
				rawList[i] = next
				changed = true
			}
		}
		resultMap["image_urls"] = rawList
	}

	if !changed {
		return
	}
	if encoded, err := json.Marshal(resultMap); err == nil {
		task.Result = string(encoded)
	}
}
