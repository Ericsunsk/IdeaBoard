package statistics

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgNormalizeStatisticsRange = "range 仅支持 1d、7d、30d 或 1y" // NormalizeStatisticsRange @ service.go
	msgGetTaskStats             = "快速完成"                     // GetTaskStats @ service.go
	msgGetTaskStats2            = "慢速完成"                     // GetTaskStats @ service.go
	msgGetTaskStats3            = "正常排队"                     // GetTaskStats @ service.go
	msgGetTaskStats4            = "积压排队"                     // GetTaskStats @ service.go
	msgGetTaskStats5            = "正常执行"                     // GetTaskStats @ service.go
	msgGetTaskStats6            = "卡住执行"                     // GetTaskStats @ service.go
	msgGetTaskStats7            = "快速失败"                     // GetTaskStats @ service.go
	msgGetTaskStats8            = "慢速失败"                     // GetTaskStats @ service.go
	msgGetTaskStats9            = "已取消"                      // GetTaskStats @ service.go
	msgGetTaskStats10           = "重试中"                      // GetTaskStats @ service.go
	msgGetTaskStats11           = "其他状态"                     // GetTaskStats @ service.go
	msgBuildIntradayTrendItems  = "未识别模型"                    // BuildIntradayTrendItems @ service.go
	msgBottleneckSourceLabel    = "队列等待"                     // BottleneckSourceLabel @ service.go
	msgBottleneckSourceLabel2   = "Worker 调度"                // BottleneckSourceLabel @ service.go
	msgBottleneckSourceLabel3   = "上游模型"                     // BottleneckSourceLabel @ service.go
	msgBottleneckSourceLabel4   = "存储链路"                     // BottleneckSourceLabel @ service.go
	msgBottleneckSourceLabel5   = "结果落库"                     // BottleneckSourceLabel @ service.go
	msgBottleneckSourceLabel6   = "输入校验"                     // BottleneckSourceLabel @ service.go
	msgBottleneckSourceLabel7   = "未知来源"                     // BottleneckSourceLabel @ service.go
	msgFailureCategoryLabel     = "输入无效"                     // FailureCategoryLabel @ service.go
	msgFailureCategoryLabel2    = "认证失败"                     // FailureCategoryLabel @ service.go
	msgFailureCategoryLabel3    = "额度限制"                     // FailureCategoryLabel @ service.go
	msgFailureCategoryLabel4    = "执行超时"                     // FailureCategoryLabel @ service.go
	msgFailureCategoryLabel5    = "上游错误"                     // FailureCategoryLabel @ service.go
	msgFailureCategoryLabel6    = "存储错误"                     // FailureCategoryLabel @ service.go
	msgFailureCategoryLabel7    = "任务取消"                     // FailureCategoryLabel @ service.go
	msgFailureCategoryLabel8    = "未知错误"                     // FailureCategoryLabel @ service.go
)
