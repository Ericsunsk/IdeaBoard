package task

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgNormalizeTaskListStatus            = "status 仅支持 pending、running、completed、failed 或 cancelled" // NormalizeTaskListStatus @ admin.go
	msgCompileWorkspaceParams             = "feature 不能为空"                                            // CompileWorkspaceParams @ canonical_intake.go
	msgCompileWorkspaceParams2            = "prompt 不能为空"                                             // CompileWorkspaceParams @ canonical_intake.go
	msgValidateWorkspaceMaskParam         = "mask_asset_id 非法"                                        // ValidateWorkspaceMaskParam @ canonical_intake.go
	msgCompileFeatureExecutionParams      = "output_count 仅支持整数 1-4"                                  // CompileFeatureExecutionParams @ canonical_intake.go
	msgCompileFeatureExecutionParams2     = "output_resolution 必须是字符串"                                // CompileFeatureExecutionParams @ canonical_intake.go
	msgMapFeatureContractValidationError  = "feature 不受支持"                                            // MapFeatureContractValidationError @ canonical_intake.go
	msgMapFeatureContractValidationError2 = "output_count 不符合功能要求"                                    // MapFeatureContractValidationError @ canonical_intake.go
	msgMapFeatureContractValidationError3 = "该功能的 output_resolution 不受支持"                             // MapFeatureContractValidationError @ canonical_intake.go
	msgMapFeatureContractValidationError4 = "该功能的图片输入不符合要求"                                           // MapFeatureContractValidationError @ canonical_intake.go
	msgMapFeatureContractValidationError5 = "功能执行参数无效"                                                // MapFeatureContractValidationError @ canonical_intake.go
	msgMarshalWorkflowTaskParams          = "任务参数层级过深"                                                // MarshalWorkflowTaskParams @ canonical_intake.go
	msgMarshalWorkflowTaskParams2         = "任务参数格式错误"                                                // MarshalWorkflowTaskParams @ canonical_intake.go
	msgMarshalWorkflowTaskParams3         = "任务参数不能超过 %dMB"                                           // MarshalWorkflowTaskParams @ canonical_intake.go
	msgNormalizeFusionParams              = "fusion 节点需要 2 到 4 张输入图"                                  // NormalizeFusionParams @ canonical_intake.go
	msgNormalizeFusionParams2             = "fusion 参数缺少 fusion.input_weights"                        // NormalizeFusionParams @ canonical_intake.go
	msgNormalizeFusionParams3             = "fusion 参数格式错误"                                           // NormalizeFusionParams @ canonical_intake.go
	msgNormalizeFusionParams4             = "fusion 权重必须是数字数组"                                        // NormalizeFusionParams @ canonical_intake.go
	msgNormalizeFusionParams5             = "fusion 权重数量必须与输入图数量一致"                                   // NormalizeFusionParams @ canonical_intake.go
	msgNormalizeFusionInputWeights        = "fusion 权重必须是有限数字"                                        // NormalizeFusionInputWeights @ canonical_intake.go
	msgNormalizeFusionInputWeights2       = "fusion 权重不能为负数"                                          // NormalizeFusionInputWeights @ canonical_intake.go
	msgNormalizeFusionInputWeights3       = "fusion 权重总和必须大于 0"                                       // NormalizeFusionInputWeights @ canonical_intake.go
	msgResolveAdminTaskUserID             = "user_public_id 无效"                                       // ResolveAdminTaskUserID @ user_resolution.go
)
