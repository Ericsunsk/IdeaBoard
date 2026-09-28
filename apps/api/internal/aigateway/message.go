package aigateway

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgNormalizeBrandLogoProvider    = "provider 非法：仅支持小写字母、数字与连字符（1-64 字符）"                       // NormalizeBrandLogoProvider @ brandlogo.go
	msgValidateUpdateInput           = "网关 account id 必须是 32 位小写十六进制"                              // ValidateUpdateInput @ config.go
	msgValidateUpdateInput2          = "网关 id 仅允许小写字母、数字与连字符"                                      // ValidateUpdateInput @ config.go
	msgValidateUpdateInput3          = "网关 Base URL 必须是合法的 http 或 https 地址"                        // ValidateUpdateInput @ config.go
	msgValidateUpdateInput4          = "网关 Base URL 不得包含凭据、query 或 fragment"                       // ValidateUpdateInput @ config.go
	msgValidateUpdateInput5          = "网关 token 不能为空"                                             // ValidateUpdateInput @ config.go
	msgResolveControlPlaneIdentity   = "网关缺少 Account/Gateway 身份，请先在网关设置中补全后再探测"                    // ResolveControlPlaneIdentity @ detect.go
	msgFetchHTTPBody                 = "请求地址无效"                                                    // FetchHTTPBody @ detect.go
	msgDetectModels                  = "读取网关已配置 Provider 失败: %w"                                   // DetectModels @ detect.go
	msgDetectModels2                 = "全部"                                                        // DetectModels @ detect.go
	msgDetectModels3                 = "未知 Provider %q"                                            // DetectModels @ detect.go
	msgWorkersAIAvailable            = "网关地址无效"                                                    // WorkersAIAvailable @ detect.go
	msgFetchCommunityCatalog         = "获取社区模型目录失败: %w"                                            // FetchCommunityCatalog @ detect.go
	msgFetchCommunityCatalog2        = "社区模型目录返回 %d"                                               // FetchCommunityCatalog @ detect.go
	msgFetchCommunityCatalog3        = "社区模型目录内容为空"                                                // FetchCommunityCatalog @ detect.go
	msgFetchCommunityCatalog4        = "社区模型目录格式变化，解析失败"                                           // FetchCommunityCatalog @ detect.go
	msgFetchCommunityCatalog5        = "社区模型目录无有效条目"                                               // FetchCommunityCatalog @ detect.go
	msgModelsDevCapabilities         = "文本"                                                        // ModelsDevCapabilities @ detect.go
	msgModelsDevCapabilities2        = "图像"                                                        // ModelsDevCapabilities @ detect.go
	msgModelsDevCapabilities3        = "音频"                                                        // ModelsDevCapabilities @ detect.go
	msgModelsDevCapabilities4        = "视频"                                                        // ModelsDevCapabilities @ detect.go
	msgDescribeModelsDevFact         = "知识截止 "                                                     // DescribeModelsDevFact @ detect.go
	msgSummarizeModelsDevModalities  = "输入："                                                       // SummarizeModelsDevModalities @ detect.go
	msgSummarizeModelsDevModalities2 = "输出："                                                       // SummarizeModelsDevModalities @ detect.go
	msgSplitGatewayModel             = "模型网关路径无效"                                                  // SplitGatewayModel @ gateway_contract.go
	msgSplitGatewayModel2            = "模型必须是 provider/model 格式且不含空白"                              // SplitGatewayModel @ gateway_contract.go
	msgGet                           = "网关配置服务暂不可用"                                                // Get @ handler.go
	msgGet2                          = "读取网关配置失败"                                                  // Get @ handler.go
	msgUpdate                        = "参数错误"                                                      // Update @ handler.go
	msgListMappings                  = "读取网关模型映射失败"                                                // ListMappings @ handler.go
	msgPreviewGatewayRoute           = "模型标识不能为空"                                                  // PreviewGatewayRoute @ handler.go
	msgTestConnection                = "读取网关模型映射失败"                                                // TestConnection @ handler.go
	msgListCatalog                   = "读取模型库失败"                                                   // ListCatalog @ handler.go
	msgSaveCatalog                   = "参数错误：含未知字段，目录单价字段已下线，请改用 models.dev 参照接口"                  // SaveCatalog @ handler.go
	msgValidateModelMapping          = "模型标识过长"                                                    // ValidateModelMapping @ model.go
	msgValidateModelMapping2         = "模型缺少 provider 前缀"                                          // ValidateModelMapping @ model.go
	msgProbeGatewayConnection        = "网关地址为空"                                                    // ProbeGatewayConnection @ model.go
	msgProbeGatewayConnection2       = "网关地址必须是 http(s) URL"                                       // ProbeGatewayConnection @ model.go
	msgProbeGatewayConnection3       = "必须提供 provider 和 provider-native path"                      // ProbeGatewayConnection @ model.go
	msgProbeGatewayConnection4       = "provider-native path 无效"                                   // ProbeGatewayConnection @ model.go
	msgProbeGatewayConnection5       = "网关连接失败: "                                                  // ProbeGatewayConnection @ model.go
	msgProbeGatewayConnection6       = "网关连接正常 (%dms, HTTP %d)"                                    // ProbeGatewayConnection @ model.go
	msgBuildSKUMargin                = "暂无数据：网关回执与本地日志均无实际成本字段"                                    // BuildSKUMargin @ observed.go
	msgListModelsObserved            = "模型观测服务暂不可用"                                                // ListModelsObserved @ observed_handler.go
	msgListModelsObserved2           = "读取模型观测数据失败"                                                // ListModelsObserved @ observed_handler.go
	msgPackage                       = "参照价，非结算依据"                                                 // Package @ reference.go
	msgGetModelReference             = "需要指定 model_id（或 provider+model）"                           // GetModelReference @ reference.go
	msgGetModelReference2            = "models.dev 未在 egress allowlist 中，请在出站策略中放行 models.dev 后重试" // GetModelReference @ reference.go
	msgGetModelReference3            = "models.dev 暂不可用，参照价为空，不影响目录主流程"                            // GetModelReference @ reference.go
	msgGetModelReference4            = "models.dev 未收录模型 %q"                                       // GetModelReference @ reference.go
	msgLoadModelsReferenceFacts      = "models.dev 返回 %d"                                          // LoadModelsReferenceFacts @ reference.go
	msgLoadModelsReferenceFacts2     = "models.dev 内容为空"                                           // LoadModelsReferenceFacts @ reference.go
	msgLoadModelsReferenceFacts3     = "models.dev 格式变化，解析失败"                                      // LoadModelsReferenceFacts @ reference.go
	msgLoadModelsReferenceFacts4     = "models.dev 无有效条目"                                          // LoadModelsReferenceFacts @ reference.go
)
