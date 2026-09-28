package statisticshttp

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgGetMonitor                    = "获取监控数据失败"        // GetMonitor @ handler.go
	msgTaskMonitorConcurrencyLimited = "监控连接数已达上限，请稍后重试" // CodeTaskMonitorConcurrencyLimited
	msgGetStatistics                 = "获取统计数据失败"        // GetStatistics @ handler.go
	msgTaskStatisticsRequestInvalid  = "参数错误"            // CodeTaskStatisticsRequestInvalid
	msgBackfillStatistics            = "补录统计失败"          // BackfillStatistics @ handler.go
	msgValidationMessage             = "参数错误"            // ValidationMessage @ handler.go
)
