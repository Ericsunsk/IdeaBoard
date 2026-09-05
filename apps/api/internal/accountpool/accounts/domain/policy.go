package domain

import (
	"strings"

	"ideaboard/api/internal/platform/validation"
)

const (
	MaxFailCount = 5

	AccountStatusDisabled int16 = 0
	AccountStatusEnabled  int16 = 1

	RunningStatusStopped = "已停止"
	RunningStatusFaulted = "故障"
	RunningStatusRunning = "运行中"
)

func ResolveRunningStatus(status int16, failCount int) string {
	if status == AccountStatusDisabled {
		return RunningStatusStopped
	}
	if failCount >= MaxFailCount {
		return RunningStatusFaulted
	}
	return RunningStatusRunning
}

func NormalizeWeight(weight int) int {
	if weight <= 0 {
		return 1
	}
	return weight
}

func FindModelByName(models []AccountModel, modelName string) (AccountModel, bool) {
	modelKey := strings.ToLower(strings.TrimSpace(modelName))
	for _, model := range models {
		if strings.ToLower(strings.TrimSpace(model.Name)) == modelKey {
			return model, true
		}
	}
	return AccountModel{}, false
}

func NormalizeModelInputs(inputs []AccountModelInput) ([]AccountModel, error) {
	if len(inputs) == 0 {
		return nil, validation.New("models 不能为空，请先检测并选择模型")
	}
	seen := make(map[string]struct{}, len(inputs))
	models := make([]AccountModel, 0, len(inputs))
	for _, input := range inputs {
		name := strings.TrimSpace(input.Name)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		enabled := true
		if input.Enabled != nil {
			enabled = *input.Enabled
		}
		weight := input.Weight
		if weight <= 0 {
			weight = 1
		}
		models = append(models, AccountModel{
			Name:          name,
			Enabled:       enabled,
			Weight:        weight,
			MaxConcurrent: input.MaxConcurrent,
			SortOrder:     len(models),
			Supports2K:    input.Supports2K != nil && *input.Supports2K,
			Supports4K:    input.Supports4K != nil && *input.Supports4K,
		})
	}
	if len(models) == 0 {
		return nil, validation.New("models 不能为空，请先检测并选择模型")
	}
	return models, nil
}
