package execution

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgPrepareTaskExecution         = "任务参数无法解析"            // PrepareTaskExecution @ orchestrator.go
	msgPrepareTaskExecution2        = "功能模型未配置，请先在后台配置"     // PrepareTaskExecution @ orchestrator.go
	msgStartTaskAttempt             = "任务状态已收敛，丢弃脏队列任务"     // StartTaskAttempt @ orchestrator.go
	msgCheckCancelledBeforeFinalize = "检查任务取消状态失败"          // CheckCancelledBeforeFinalize @ orchestrator.go
	msgCompleteTask                 = "任务结果保存失败，请稍后重试"      // CompleteTask @ orchestrator.go
	msgSkipCancelledTask            = "任务已取消"               // SkipCancelledTask @ orchestrator.go
	msgUserFacingProviderError      = "上游图像服务响应超时"          // UserFacingProviderError @ orchestrator.go
	msgUserFacingProviderError2     = "上游图像服务调用失败"          // UserFacingProviderError @ orchestrator.go
	msgUserFacingProviderHTTPError  = "请求参数不合法，请检查输入后重试"    // UserFacingProviderHTTPError @ orchestrator.go
	msgUserFacingProviderHTTPError2 = "上游服务鉴权失败，请联系管理员"     // UserFacingProviderHTTPError @ orchestrator.go
	msgUserFacingProviderHTTPError3 = "上游服务额度不足或未开通，请联系管理员" // UserFacingProviderHTTPError @ orchestrator.go
	msgUserFacingProviderHTTPError4 = "上游服务接口或模型不可用，请联系管理员" // UserFacingProviderHTTPError @ orchestrator.go
	msgUserFacingProviderHTTPError5 = "上游服务请求过于频繁，请稍后重试"    // UserFacingProviderHTTPError @ orchestrator.go
	msgUserFacingFailureCategory    = "图片结果保存失败，请检查存储配置"    // UserFacingFailureCategory @ orchestrator.go
)
