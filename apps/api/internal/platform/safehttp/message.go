package safehttp

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgResolvePolicyWithTimeout  = "%w: URL 格式错误"           // ResolvePolicyWithTimeout @ safehttp.go
	msgResolvePolicyWithTimeout2 = "%w: 仅支持 http/https URL" // ResolvePolicyWithTimeout @ safehttp.go
	msgResolvePolicyWithTimeout3 = "%w: URL 缺少主机名"          // ResolvePolicyWithTimeout @ safehttp.go
	msgResolvePolicyWithTimeout4 = "解析主机失败"                 // ResolvePolicyWithTimeout @ safehttp.go
	msgResolvePolicyWithTimeout5 = "主机未解析到有效地址"             // ResolvePolicyWithTimeout @ safehttp.go
	msgDoWithAllowHosts          = "创建请求失败"                 // DoWithAllowHosts @ safehttp.go
	msgDoWithAllowHosts2         = "重定向次数过多"                // DoWithAllowHosts @ safehttp.go
	msgDoRequestWithAllowHosts   = "参数错误"                   // DoRequestWithAllowHosts @ safehttp.go
	msgTransportForRequest       = "HTTP 传输配置错误"            // TransportForRequest @ safehttp.go
	msgRestrictedDialContext     = "连接目标主机失败"               // RestrictedDialContext @ safehttp.go
	msgFilterAllowedIPs          = "%w: 不允许访问本地地址"          // FilterAllowedIPs @ safehttp.go
	msgFilterAllowedIPs2         = "%w: 不允许访问内网或本地地址"       // FilterAllowedIPs @ safehttp.go
)
