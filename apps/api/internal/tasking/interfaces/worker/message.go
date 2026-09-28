package worker

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgValidateInput                = "任务输入不合法: %w"                    // ValidateInput @ workspace_task_worker.go
	msgResolveMediaRefs             = "任务媒体引用无法校验: 媒体读取器未配置"           // ResolveMediaRefs @ workspace_task_worker.go
	msgResolveMediaRefs2            = "任务输入媒体 %d 已归档: %w"              // ResolveMediaRefs @ workspace_task_worker.go
	msgResolveMediaRefs3            = "任务输入媒体 %d 未就绪: %w"              // ResolveMediaRefs @ workspace_task_worker.go
	msgResolveMediaRefs4            = "任务输入媒体 %d 不存在: %w"              // ResolveMediaRefs @ workspace_task_worker.go
	msgResolveMediaRefs5            = "任务遮罩引用无法校验: 媒体读取器未配置"           // ResolveMediaRefs @ workspace_task_worker.go
	msgResolveMediaRefs6            = "任务遮罩已归档: %w"                    // ResolveMediaRefs @ workspace_task_worker.go
	msgResolveMediaRefs7            = "任务遮罩未就绪: %w"                    // ResolveMediaRefs @ workspace_task_worker.go
	msgResolveMediaRefs8            = "任务遮罩不存在: %w"                    // ResolveMediaRefs @ workspace_task_worker.go
	msgGenerate                     = "任务执行器未配置: provider unavailable" // Generate @ workspace_task_worker.go
	msgGenerate2                    = "provider 输出不合法: %w"             // Generate @ workspace_task_worker.go
	msgGenerate3                    = "provider 输出过多: 最多 %d"           // Generate @ workspace_task_worker.go
	msgWorkspaceFailureMessage      = "任务执行失败，请稍后重试"                   // WorkspaceFailureMessage @ workspace_task_worker.go
	msgGenerate4                    = "上游图像服务未配置，请联系管理员"               // Generate @ workspace_task_worker.go
	msgGenerate5                    = "任务执行计划缺少模型，请重新提交任务"             // Generate @ workspace_task_worker.go
	msgWorkspaceBridgeTaskPayload   = "任务标识缺失"                         // WorkspaceBridgeTaskPayload @ workspace_task_worker.go
	msgWorkspaceBridgeTaskPayload2  = "任务执行计划缺少 canonical 参数，请重新提交任务"  // WorkspaceBridgeTaskPayload @ workspace_task_worker.go
	msgWorkspaceBridgeTaskPayload3  = "任务 canonical 参数解析失败: %w"        // WorkspaceBridgeTaskPayload @ workspace_task_worker.go
	msgWorkspaceBridgeTaskPayload4  = "任务 canonical 参数编码失败: %w"        // WorkspaceBridgeTaskPayload @ workspace_task_worker.go
	msgMapWorkspaceProviderOutputs  = "上游结果缺少可持久化的输出引用"                // MapWorkspaceProviderOutputs @ workspace_task_worker.go
	msgMapWorkspaceProviderOutputs2 = "上游输出不符合任务输出契约: %w"              // MapWorkspaceProviderOutputs @ workspace_task_worker.go
)
