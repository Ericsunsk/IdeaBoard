package runtime

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgMaterializeOne                   = "素材转存失败，请稍后重试"                            // MaterializeOne @ workspace_task_materializer.go
	msgRenderOutputs                    = "任务结果渲染器未配置"                              // RenderOutputs @ workspace_task_renderer.go
	msgRenderOutputs2                   = "任务标识缺失，无法渲染结果"                           // RenderOutputs @ workspace_task_renderer.go
	msgRenderOutputs3                   = "provider 输出过多: 最多 %d"                    // RenderOutputs @ workspace_task_renderer.go
	msgRenderOutputs4                   = "provider 输出不合法: %w"                      // RenderOutputs @ workspace_task_renderer.go
	msgRenderOutputs5                   = "provider 输出类型不支持: %q"                    // RenderOutputs @ workspace_task_renderer.go
	msgPassthroughAsset                 = "任务输出媒体已归档: %w"                           // PassthroughAsset @ workspace_task_renderer.go
	msgPassthroughAsset2                = "任务输出媒体不存在: %w"                           // PassthroughAsset @ workspace_task_renderer.go
	msgPassthroughAsset3                = "任务输出不符合输出契约: %w"                         // PassthroughAsset @ workspace_task_renderer.go
	msgIngestRemote                     = "任务结果转存未配置: result ingestion unavailable" // IngestRemote @ workspace_task_renderer.go
	msgIngestRemote2                    = "任务结果转存未配置: object storage unavailable"   // IngestRemote @ workspace_task_renderer.go
	msgIngestRemote3                    = "任务结果下载失败: %w"                            // IngestRemote @ workspace_task_renderer.go
	msgPersistWorkspaceTaskResultAsset  = "上游结果为空，无法转存"                             // PersistWorkspaceTaskResultAsset @ workspace_task_renderer.go
	msgPersistWorkspaceTaskResultAsset2 = "上游结果过大: %d 字节"                           // PersistWorkspaceTaskResultAsset @ workspace_task_renderer.go
	msgPersistWorkspaceTaskResultAsset3 = "上游结果不是受支持的图片: %w"                        // PersistWorkspaceTaskResultAsset @ workspace_task_renderer.go
	msgPersistWorkspaceTaskResultAsset4 = "上游结果校验失败: checksum mismatch"             // PersistWorkspaceTaskResultAsset @ workspace_task_renderer.go
	msgPersistWorkspaceTaskResultAsset5 = "任务结果落盘失败: %w"                            // PersistWorkspaceTaskResultAsset @ workspace_task_renderer.go
	msgPersistWorkspaceTaskResultAsset6 = "任务结果字节写入失败: %w"                          // PersistWorkspaceTaskResultAsset @ workspace_task_renderer.go
)
