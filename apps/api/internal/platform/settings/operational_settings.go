package settings

import (
	"context"
	"encoding/json"
	"net"
	"strconv"
	"strings"

	"ideaboard/api/internal/platform/adminaction"
	"ideaboard/api/internal/platform/validation"
)

const (
	taskRuntimeWorkerConcurrencyKey      = "river_worker_concurrency"
	taskRuntimeTaskTimeoutSecondsKey     = "river_task_timeout_seconds"
	taskRuntimeNoCapacityRetrySecondsKey = "river_no_capacity_retry_seconds"
	taskRuntimeMaxRetryKey               = "river_max_retry"
	outboundPolicyAllowHostsKey          = "api_outbound_allow_hosts"
	eventRetentionNATSDaysKey            = "nats_retention_days"

	defaultTaskRuntimeWorkerConcurrency      = 5
	defaultTaskRuntimeTaskTimeoutSeconds     = 180
	defaultTaskRuntimeNoCapacityRetrySeconds = 5
	defaultTaskRuntimeMaxRetry               = 25
	defaultNATSRetentionDays                 = 7

	minTaskRuntimeWorkerConcurrency      = 1
	maxTaskRuntimeWorkerConcurrency      = 64
	minTaskRuntimeTaskTimeoutSeconds     = 30
	maxTaskRuntimeTaskTimeoutSeconds     = 3600
	minTaskRuntimeNoCapacityRetrySeconds = 1
	maxTaskRuntimeNoCapacityRetrySeconds = 3600
	minTaskRuntimeMaxRetry               = 1
	maxTaskRuntimeMaxRetry               = 100
	minNATSRetentionDays                 = 1
	maxNATSRetentionDays                 = 3650
	maxOutboundAllowHosts                = 128
	maxOutboundAllowHostLength           = 255
)

type TaskRuntimeSetting struct {
	WorkerConcurrency      int `json:"worker_concurrency"`
	TaskTimeoutSeconds     int `json:"task_timeout_seconds"`
	NoCapacityRetrySeconds int `json:"no_capacity_retry_seconds"`
	MaxRetry               int `json:"max_retry"`
}

type OutboundPolicySetting struct {
	AllowHosts []string `json:"allow_hosts"`
}

type EventRetentionSetting struct {
	NATSRetentionDays int `json:"nats_retention_days"`
}

type OperationalSettings struct {
	TaskRuntime    TaskRuntimeSetting    `json:"task_runtime"`
	OutboundPolicy OutboundPolicySetting `json:"outbound_policy"`
	EventRetention EventRetentionSetting `json:"event_retention"`
}

type TaskRuntimeSettingState struct {
	Configured      TaskRuntimeSetting `json:"configured"`
	Effective       TaskRuntimeSetting `json:"effective"`
	RestartRequired bool               `json:"restart_required"`
}

type OutboundPolicySettingState struct {
	Configured      OutboundPolicySetting `json:"configured"`
	Effective       OutboundPolicySetting `json:"effective"`
	RestartRequired bool                  `json:"restart_required"`
}

type EventRetentionSettingState struct {
	Configured      EventRetentionSetting `json:"configured"`
	Effective       EventRetentionSetting `json:"effective"`
	RestartRequired bool                  `json:"restart_required"`
}

type OperationalSettingsState struct {
	TaskRuntime    TaskRuntimeSettingState    `json:"task_runtime"`
	OutboundPolicy OutboundPolicySettingState `json:"outbound_policy"`
	EventRetention EventRetentionSettingState `json:"event_retention"`
}

type OutboundPolicySettingProvider interface {
	GetOutboundPolicy(ctx context.Context) (OutboundPolicySetting, error)
}

type effectiveOperationalSettings struct {
	taskRuntime    TaskRuntimeSetting
	eventRetention EventRetentionSetting
}

func DefaultTaskRuntimeSetting() TaskRuntimeSetting {
	return TaskRuntimeSetting{
		WorkerConcurrency:      defaultTaskRuntimeWorkerConcurrency,
		TaskTimeoutSeconds:     defaultTaskRuntimeTaskTimeoutSeconds,
		NoCapacityRetrySeconds: defaultTaskRuntimeNoCapacityRetrySeconds,
		MaxRetry:               defaultTaskRuntimeMaxRetry,
	}
}

func DefaultOutboundPolicySetting() OutboundPolicySetting {
	return OutboundPolicySetting{AllowHosts: []string{}}
}

func DefaultEventRetentionSetting() EventRetentionSetting {
	return EventRetentionSetting{NATSRetentionDays: defaultNATSRetentionDays}
}

func DefaultOperationalSettings() OperationalSettings {
	return OperationalSettings{
		TaskRuntime:    DefaultTaskRuntimeSetting(),
		OutboundPolicy: DefaultOutboundPolicySetting(),
		EventRetention: DefaultEventRetentionSetting(),
	}
}

func (s *RuntimeSettingsService) GetTaskRuntime(ctx context.Context) (TaskRuntimeSetting, error) {
	return s.ResolveTaskRuntime(ctx, DefaultTaskRuntimeSetting())
}

// ResolveTaskRuntime prefers persisted settings and only uses fallback for missing or malformed legacy rows.
func (s *RuntimeSettingsService) ResolveTaskRuntime(ctx context.Context, fallback TaskRuntimeSetting) (TaskRuntimeSetting, error) {
	fallback, err := NormalizeTaskRuntimeSetting(fallback)
	if err != nil {
		fallback = DefaultTaskRuntimeSetting()
	}
	values, err := s.getMany(ctx,
		taskRuntimeWorkerConcurrencyKey,
		taskRuntimeTaskTimeoutSecondsKey,
		taskRuntimeNoCapacityRetrySecondsKey,
		taskRuntimeMaxRetryKey,
	)
	if err != nil {
		return fallback, err
	}
	return taskRuntimeSettingFromValues(values, fallback), nil
}

func (s *RuntimeSettingsService) GetOutboundPolicy(ctx context.Context) (OutboundPolicySetting, error) {
	fallback := DefaultOutboundPolicySetting()
	values, err := s.getMany(ctx, outboundPolicyAllowHostsKey)
	if err != nil {
		return fallback, err
	}
	if raw, ok := values[outboundPolicyAllowHostsKey]; ok {
		var hosts []string
		if json.Unmarshal([]byte(raw), &hosts) == nil {
			if normalized, normalizeErr := NormalizeOutboundPolicySetting(OutboundPolicySetting{AllowHosts: hosts}); normalizeErr == nil {
				return normalized, nil
			}
		}
	}
	return fallback, nil
}

// ResolveEventRetention prefers persisted settings and only uses fallback for missing or malformed legacy rows.
func (s *RuntimeSettingsService) ResolveEventRetention(ctx context.Context, fallback EventRetentionSetting) (EventRetentionSetting, error) {
	fallback, err := NormalizeEventRetentionSetting(fallback)
	if err != nil {
		fallback = DefaultEventRetentionSetting()
	}
	values, err := s.getMany(ctx, eventRetentionNATSDaysKey)
	if err != nil {
		return fallback, err
	}
	if parsed, ok := parseBoundedInt(values[eventRetentionNATSDaysKey], minNATSRetentionDays, maxNATSRetentionDays); ok {
		return EventRetentionSetting{NATSRetentionDays: parsed}, nil
	}
	return fallback, nil
}

func (s *RuntimeSettingsService) GetOperationalSettings(ctx context.Context) (OperationalSettingsState, error) {
	configured, err := s.readOperationalSettings(ctx, DefaultOperationalSettings())
	if err != nil {
		return operationalSettingsState(configured, s.effectiveOperationalSettings()), err
	}
	return operationalSettingsState(configured, s.effectiveOperationalSettings()), nil
}

func (s *RuntimeSettingsService) UpdateOperationalSettings(ctx context.Context, setting OperationalSettings, action adminaction.Metadata) (OperationalSettingsState, error) {
	normalized, err := NormalizeOperationalSettings(setting)
	if err != nil {
		return OperationalSettingsState{}, err
	}
	if !action.IsValid() {
		return OperationalSettingsState{}, validation.New("管理员操作信息无效")
	}
	metadata, err := json.Marshal(normalized)
	if err != nil {
		return OperationalSettingsState{}, err
	}
	if s == nil || s.repo == nil {
		return OperationalSettingsState{}, ErrRuntimeSettingsRepositoryUnavailable
	}
	if err := s.repo.SetManyWithAdminAction(ctx, operationalSystemConfigValues(normalized), SystemConfigAdminAction{
		ActionType: "update_operational_settings",
		Action:     action,
		Metadata:   metadata,
	}); err != nil {
		return OperationalSettingsState{}, err
	}
	return operationalSettingsState(normalized, s.effectiveOperationalSettings()), nil
}

// SetEffectiveOperationalSettings records the startup snapshot actually used by this process.
func (s *RuntimeSettingsService) SetEffectiveOperationalSettings(taskRuntime TaskRuntimeSetting, eventRetention EventRetentionSetting) {
	if s == nil {
		return
	}
	if normalized, err := NormalizeTaskRuntimeSetting(taskRuntime); err == nil {
		taskRuntime = normalized
	} else {
		taskRuntime = DefaultTaskRuntimeSetting()
	}
	if normalized, err := NormalizeEventRetentionSetting(eventRetention); err == nil {
		eventRetention = normalized
	} else {
		eventRetention = DefaultEventRetentionSetting()
	}
	s.operationalMu.Lock()
	s.effectiveOperational = &effectiveOperationalSettings{taskRuntime: taskRuntime, eventRetention: eventRetention}
	s.operationalMu.Unlock()
}

func (s *RuntimeSettingsService) readOperationalSettings(ctx context.Context, fallback OperationalSettings) (OperationalSettings, error) {
	fallback, err := NormalizeOperationalSettings(fallback)
	if err != nil {
		fallback = DefaultOperationalSettings()
	}
	values, err := s.getMany(ctx,
		taskRuntimeWorkerConcurrencyKey,
		taskRuntimeTaskTimeoutSecondsKey,
		taskRuntimeNoCapacityRetrySecondsKey,
		taskRuntimeMaxRetryKey,
		outboundPolicyAllowHostsKey,
		eventRetentionNATSDaysKey,
	)
	if err != nil {
		return fallback, err
	}
	setting := OperationalSettings{
		TaskRuntime:    taskRuntimeSettingFromValues(values, fallback.TaskRuntime),
		OutboundPolicy: fallback.OutboundPolicy,
		EventRetention: fallback.EventRetention,
	}
	if raw, ok := values[outboundPolicyAllowHostsKey]; ok {
		var hosts []string
		if json.Unmarshal([]byte(raw), &hosts) == nil {
			if normalized, normalizeErr := NormalizeOutboundPolicySetting(OutboundPolicySetting{AllowHosts: hosts}); normalizeErr == nil {
				setting.OutboundPolicy = normalized
			}
		}
	}
	if parsed, ok := parseBoundedInt(values[eventRetentionNATSDaysKey], minNATSRetentionDays, maxNATSRetentionDays); ok {
		setting.EventRetention = EventRetentionSetting{NATSRetentionDays: parsed}
	}
	return setting, nil
}

func (s *RuntimeSettingsService) effectiveOperationalSettings() *effectiveOperationalSettings {
	if s == nil {
		return nil
	}
	s.operationalMu.RLock()
	defer s.operationalMu.RUnlock()
	if s.effectiveOperational == nil {
		return nil
	}
	copy := *s.effectiveOperational
	return &copy
}

func operationalSettingsState(configured OperationalSettings, effective *effectiveOperationalSettings) OperationalSettingsState {
	effectiveTaskRuntime := configured.TaskRuntime
	effectiveEventRetention := configured.EventRetention
	if effective != nil {
		effectiveTaskRuntime = effective.taskRuntime
		effectiveEventRetention = effective.eventRetention
	}
	return OperationalSettingsState{
		TaskRuntime: TaskRuntimeSettingState{
			Configured:      configured.TaskRuntime,
			Effective:       effectiveTaskRuntime,
			RestartRequired: configured.TaskRuntime != effectiveTaskRuntime,
		},
		OutboundPolicy: OutboundPolicySettingState{
			Configured:      configured.OutboundPolicy,
			Effective:       configured.OutboundPolicy,
			RestartRequired: false,
		},
		EventRetention: EventRetentionSettingState{
			Configured:      configured.EventRetention,
			Effective:       effectiveEventRetention,
			RestartRequired: configured.EventRetention != effectiveEventRetention,
		},
	}
}

func NormalizeOperationalSettings(setting OperationalSettings) (OperationalSettings, error) {
	taskRuntime, err := NormalizeTaskRuntimeSetting(setting.TaskRuntime)
	if err != nil {
		return OperationalSettings{}, err
	}
	outboundPolicy, err := NormalizeOutboundPolicySetting(setting.OutboundPolicy)
	if err != nil {
		return OperationalSettings{}, err
	}
	eventRetention, err := NormalizeEventRetentionSetting(setting.EventRetention)
	if err != nil {
		return OperationalSettings{}, err
	}
	return OperationalSettings{TaskRuntime: taskRuntime, OutboundPolicy: outboundPolicy, EventRetention: eventRetention}, nil
}

func NormalizeTaskRuntimeSetting(setting TaskRuntimeSetting) (TaskRuntimeSetting, error) {
	if setting.WorkerConcurrency < minTaskRuntimeWorkerConcurrency || setting.WorkerConcurrency > maxTaskRuntimeWorkerConcurrency {
		return TaskRuntimeSetting{}, validation.New("worker_concurrency 必须在 1 到 64 之间")
	}
	if setting.TaskTimeoutSeconds < minTaskRuntimeTaskTimeoutSeconds || setting.TaskTimeoutSeconds > maxTaskRuntimeTaskTimeoutSeconds {
		return TaskRuntimeSetting{}, validation.New("task_timeout_seconds 必须在 30 到 3600 秒之间")
	}
	if setting.NoCapacityRetrySeconds < minTaskRuntimeNoCapacityRetrySeconds || setting.NoCapacityRetrySeconds > maxTaskRuntimeNoCapacityRetrySeconds {
		return TaskRuntimeSetting{}, validation.New("no_capacity_retry_seconds 必须在 1 到 3600 秒之间")
	}
	if setting.MaxRetry < minTaskRuntimeMaxRetry || setting.MaxRetry > maxTaskRuntimeMaxRetry {
		return TaskRuntimeSetting{}, validation.New("max_retry 必须在 1 到 100 之间")
	}
	return setting, nil
}

func NormalizeOutboundPolicySetting(setting OutboundPolicySetting) (OutboundPolicySetting, error) {
	if len(setting.AllowHosts) > maxOutboundAllowHosts {
		return OutboundPolicySetting{}, validation.New("allow_hosts 最多支持 128 条")
	}
	normalized := make([]string, 0, len(setting.AllowHosts))
	seen := make(map[string]struct{}, len(setting.AllowHosts))
	for _, raw := range setting.AllowHosts {
		host, err := normalizeOutboundAllowHost(raw)
		if err != nil {
			return OutboundPolicySetting{}, err
		}
		if _, exists := seen[host]; exists {
			continue
		}
		seen[host] = struct{}{}
		normalized = append(normalized, host)
	}
	return OutboundPolicySetting{AllowHosts: normalized}, nil
}

func NormalizeEventRetentionSetting(setting EventRetentionSetting) (EventRetentionSetting, error) {
	if setting.NATSRetentionDays < minNATSRetentionDays || setting.NATSRetentionDays > maxNATSRetentionDays {
		return EventRetentionSetting{}, validation.New("nats_retention_days 必须在 1 到 3650 天之间")
	}
	return setting, nil
}

func taskRuntimeSettingFromValues(values map[string]string, fallback TaskRuntimeSetting) TaskRuntimeSetting {
	setting := fallback
	if parsed, ok := parseBoundedInt(values[taskRuntimeWorkerConcurrencyKey], minTaskRuntimeWorkerConcurrency, maxTaskRuntimeWorkerConcurrency); ok {
		setting.WorkerConcurrency = parsed
	}
	if parsed, ok := parseBoundedInt(values[taskRuntimeTaskTimeoutSecondsKey], minTaskRuntimeTaskTimeoutSeconds, maxTaskRuntimeTaskTimeoutSeconds); ok {
		setting.TaskTimeoutSeconds = parsed
	}
	if parsed, ok := parseBoundedInt(values[taskRuntimeNoCapacityRetrySecondsKey], minTaskRuntimeNoCapacityRetrySeconds, maxTaskRuntimeNoCapacityRetrySeconds); ok {
		setting.NoCapacityRetrySeconds = parsed
	}
	if parsed, ok := parseBoundedInt(values[taskRuntimeMaxRetryKey], minTaskRuntimeMaxRetry, maxTaskRuntimeMaxRetry); ok {
		setting.MaxRetry = parsed
	}
	return setting
}

func operationalSystemConfigValues(setting OperationalSettings) []SystemConfigValue {
	allowHosts, _ := json.Marshal(setting.OutboundPolicy.AllowHosts)
	return []SystemConfigValue{
		{Key: taskRuntimeWorkerConcurrencyKey, Value: strconv.Itoa(setting.TaskRuntime.WorkerConcurrency)},
		{Key: taskRuntimeTaskTimeoutSecondsKey, Value: strconv.Itoa(setting.TaskRuntime.TaskTimeoutSeconds)},
		{Key: taskRuntimeNoCapacityRetrySecondsKey, Value: strconv.Itoa(setting.TaskRuntime.NoCapacityRetrySeconds)},
		{Key: taskRuntimeMaxRetryKey, Value: strconv.Itoa(setting.TaskRuntime.MaxRetry)},
		{Key: outboundPolicyAllowHostsKey, Value: string(allowHosts)},
		{Key: eventRetentionNATSDaysKey, Value: strconv.Itoa(setting.EventRetention.NATSRetentionDays)},
	}
}

func normalizeOutboundAllowHost(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxOutboundAllowHostLength || strings.Contains(raw, "://") || strings.ContainsAny(raw, " /?#@\\\\") {
		return "", validation.New("allow_hosts 仅支持 hostname、IP 或 host:port")
	}

	host := raw
	port := ""
	if parsedHost, parsedPort, err := net.SplitHostPort(raw); err == nil {
		host, port = parsedHost, parsedPort
		portValue, parseErr := strconv.Atoi(port)
		if parseErr != nil || portValue < 1 || portValue > 65535 {
			return "", validation.New("allow_hosts 端口必须在 1 到 65535 之间")
		}
	} else if strings.HasPrefix(raw, "[") || strings.Count(raw, ":") > 1 {
		if net.ParseIP(raw) == nil {
			return "", validation.New("allow_hosts 包含无效 IP 地址")
		}
	}

	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "" {
		return "", validation.New("allow_hosts 包含空主机")
	}
	if net.ParseIP(host) == nil && !validOutboundHostname(host) {
		return "", validation.New("allow_hosts 包含无效 hostname")
	}
	if port != "" {
		return net.JoinHostPort(host, port), nil
	}
	return host, nil
}

func validOutboundHostname(host string) bool {
	if len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, char := range label {
			if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
				return false
			}
		}
	}
	return true
}
