package execution

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgValidateTaskOutputs     = "任务结果包含非本任务的输出"               // ValidateTaskOutputs @ service.go
	msgValidateTaskOutputs2    = "任务结果输出类型不支持"                 // ValidateTaskOutputs @ service.go
	msgValidateTaskOutputs3    = "任务结果输出缺少 URL"                // ValidateTaskOutputs @ service.go
	msgValidateTaskOutputs4    = "任务结果输出缺少 storage_key"        // ValidateTaskOutputs @ service.go
	msgValidateTaskOutputs5    = "remote 任务结果不能包含 storage_key" // ValidateTaskOutputs @ service.go
	msgFormatTaskFailureResult = "\",\"error\":\"任务失败\"}"      // FormatTaskFailureResult @ service.go
)
