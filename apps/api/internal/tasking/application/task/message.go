package task

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgNormalizeTaskListStatus                        = "status 仅支持 pending、running、completed、failed 或 cancelled" // NormalizeTaskListStatus @ admin.go
	msgCompileWorkspaceParams                         = "feature 不能为空"                                            // CompileWorkspaceParams @ canonical_intake.go
	msgCompileWorkspaceParams2                        = "prompt 不能为空"                                             // CompileWorkspaceParams @ canonical_intake.go
	msgValidateWorkspaceMaskParam                     = "mask_asset_id 非法"                                        // ValidateWorkspaceMaskParam @ canonical_intake.go
	msgAssembleCanonicalTaskRequest                   = "AI 生图任务必须提供 client_action_id"                            // AssembleCanonicalTaskRequest @ canonical_intake.go
	msgCompileFeatureExecutionParams                  = "output_count 仅支持整数 1-4"                                  // CompileFeatureExecutionParams @ canonical_intake.go
	msgCompileFeatureExecutionParams2                 = "output_resolution 必须是字符串"                                // CompileFeatureExecutionParams @ canonical_intake.go
	msgMapFeatureContractValidationError              = "feature 不受支持"                                            // MapFeatureContractValidationError @ canonical_intake.go
	msgMapFeatureContractValidationError2             = "output_count 不符合功能要求"                                    // MapFeatureContractValidationError @ canonical_intake.go
	msgMapFeatureContractValidationError3             = "该功能的 output_resolution 不受支持"                             // MapFeatureContractValidationError @ canonical_intake.go
	msgMapFeatureContractValidationError4             = "该功能的图片输入不符合要求"                                           // MapFeatureContractValidationError @ canonical_intake.go
	msgMapFeatureContractValidationError5             = "功能执行参数无效"                                                // MapFeatureContractValidationError @ canonical_intake.go
	msgMarshalWorkflowTaskParams                      = "任务参数层级过深"                                                // MarshalWorkflowTaskParams @ canonical_intake.go
	msgMarshalWorkflowTaskParams2                     = "任务参数格式错误"                                                // MarshalWorkflowTaskParams @ canonical_intake.go
	msgMarshalWorkflowTaskParams3                     = "任务参数不能超过 %dMB"                                           // MarshalWorkflowTaskParams @ canonical_intake.go
	msgNormalizeFusionParams                          = "fusion 节点需要 2 到 4 张输入图"                                  // NormalizeFusionParams @ canonical_intake.go
	msgNormalizeFusionParams2                         = "fusion 参数缺少 fusion.input_weights"                        // NormalizeFusionParams @ canonical_intake.go
	msgNormalizeFusionParams3                         = "fusion 参数格式错误"                                           // NormalizeFusionParams @ canonical_intake.go
	msgNormalizeFusionParams4                         = "fusion 权重必须是数字数组"                                        // NormalizeFusionParams @ canonical_intake.go
	msgNormalizeFusionParams5                         = "fusion 权重数量必须与输入图数量一致"                                   // NormalizeFusionParams @ canonical_intake.go
	msgNormalizeFusionInputWeights                    = "fusion 权重必须是有限数字"                                        // NormalizeFusionInputWeights @ canonical_intake.go
	msgNormalizeFusionInputWeights2                   = "fusion 权重不能为负数"                                          // NormalizeFusionInputWeights @ canonical_intake.go
	msgNormalizeFusionInputWeights3                   = "fusion 权重总和必须大于 0"                                       // NormalizeFusionInputWeights @ canonical_intake.go
	msgPackage                                        = "用户取消任务"                                                  // Package @ commands.go
	msgPackage2                                       = "管理员取消任务"                                                 // Package @ commands.go
	msgSubmitTask                                     = "任务提交失败，请稍后重试"                                            // SubmitTask @ commands.go
	msgCheckSubmitTaskReadiness                       = "任务提交检查暂不可用，请稍后重试"                                        // CheckSubmitTaskReadiness @ commands.go
	msgValidateImageFillDimensions                    = "图像填充需要提供正整数的原图宽高和目标宽高"                                   // ValidateImageFillDimensions @ image_fill_validation.go
	msgValidateImageFillDimensions2                   = "原图"                                                      // ValidateImageFillDimensions @ image_fill_validation.go
	msgValidateOutpaintDimensions                     = "扩图需要提供目标尺寸和原图偏移"                                         // ValidateOutpaintDimensions @ image_fill_validation.go
	msgValidateOutpaintDimensions2                    = "目标图片"                                                    // ValidateOutpaintDimensions @ image_fill_validation.go
	msgValidateOutpaintDimensions3                    = "扩图目标尺寸不能小于原图尺寸"                                          // ValidateOutpaintDimensions @ image_fill_validation.go
	msgValidateOutpaintDimensions4                    = "扩图偏移不能让原图超出目标画布"                                         // ValidateOutpaintDimensions @ image_fill_validation.go
	msgValidateOutpaintDimensions5                    = "扩图目标尺寸必须至少扩展一个方向"                                        // ValidateOutpaintDimensions @ image_fill_validation.go
	msgValidateInpaintDimensions                      = "局部重绘的目标尺寸必须与原图尺寸一致"                                      // ValidateInpaintDimensions @ image_fill_validation.go
	msgValidateImageFillSize                          = "%s宽高必须是正整数"                                              // ValidateImageFillSize @ image_fill_validation.go
	msgValidateImageFillSize2                         = "%s超过图像填充服务的 4096 边长或约 4MP 限制"                            // ValidateImageFillSize @ image_fill_validation.go
	msgValidateSubmitTaskPrompt                       = "%s长度不能超过 %dKB"                                           // ValidateSubmitTaskPrompt @ submit_validation.go
	msgValidateSubmitTaskImageURLsWithRefValidator    = "图片数量不能超过 %d 张"                                           // ValidateSubmitTaskImageURLsWithRefValidator @ submit_validation.go
	msgValidateSubmitTaskImageURLsWithRefValidator2   = "图片地址不能为空"                                                // ValidateSubmitTaskImageURLsWithRefValidator @ submit_validation.go
	msgValidateSubmitTaskImageURLsWithRefValidator3   = "图片地址长度不能超过 %d 字符"                                        // ValidateSubmitTaskImageURLsWithRefValidator @ submit_validation.go
	msgValidateSubmitTaskMediaParamsWithRefValidator  = "%s 数量不能超过 %d 张"                                          // ValidateSubmitTaskMediaParamsWithRefValidator @ submit_validation.go
	msgValidateSubmitTaskMediaParamsWithRefValidator2 = "%s 无效: %w"                                               // ValidateSubmitTaskMediaParamsWithRefValidator @ submit_validation.go
	msgValidateSubmitTaskContractKeys                 = "任务参数必须使用结构化字段"                                           // ValidateSubmitTaskContractKeys @ submit_validation.go
	msgCompileReadiness                               = "参数错误"                                                    // CompileReadiness @ task_intake.go
	msgNormalizeCompileRequest                        = "priority 仅支持 1-4"                                        // NormalizeCompileRequest @ task_intake.go
	msgNormalizeCompileRequest2                       = "提示词"                                                     // NormalizeCompileRequest @ task_intake.go
	msgCompileTaskPrompts                             = "系统提示词暂不可用"                                               // CompileTaskPrompts @ task_intake.go
	msgCompileTaskPrompts2                            = "该功能未配置 System Prompt"                                    // CompileTaskPrompts @ task_intake.go
	msgCompileTaskPrompts3                            = "Prompt Node 需要系统模板或用户描述"                                 // CompileTaskPrompts @ task_intake.go
	msgCompileTaskPrompts4                            = "最终提示词"                                                   // CompileTaskPrompts @ task_intake.go
	msgPrepareMedia                                   = "请求用户与任务不匹配"                                              // PrepareMedia @ task_intake.go
	msgPrepareMedia2                                  = "参考图片准备失败，请稍后重试"                                          // PrepareMedia @ task_intake.go
	msgPrepareMedia3                                  = "参考图片仍在处理中，请稍后重试"                                         // PrepareMedia @ task_intake.go
	msgValidateImageAssetIDs                          = "媒体资产校验服务未配置"                                             // ValidateImageAssetIDs @ task_intake.go
	msgValidateMediaRef                               = "任务媒体校验服务未配置"                                             // ValidateMediaRef @ task_intake.go
	msgNormalizeSubmitTaskClientActionID              = "client_action_id 过长"                                     // NormalizeSubmitTaskClientActionID @ task_intake.go
	msgNormalizeSubmitTaskClientActionID2             = "client_action_id 只能包含字母、数字、横线、下划线、点或冒号"                  // NormalizeSubmitTaskClientActionID @ task_intake.go
	msgResolveAdminTaskUserID                         = "user_public_id 无效"                                       // ResolveAdminTaskUserID @ user_resolution.go
)
