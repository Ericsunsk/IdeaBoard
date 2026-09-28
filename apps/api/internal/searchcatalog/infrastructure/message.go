package infrastructure

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgTest   = "请求构造失败"     // Test @ providers.go
	msgTest2  = "请求超时"       // Test @ providers.go
	msgTest3  = "网络不可达"      // Test @ providers.go
	msgTest4  = "响应读取失败"     // Test @ providers.go
	msgTest5  = "凭证无效或无权访问"  // Test @ providers.go
	msgTest6  = "请求频率或配额受限"  // Test @ providers.go
	msgTest7  = "供应商拒绝了测试请求" // Test @ providers.go
	msgTest8  = "供应商返回空响应"   // Test @ providers.go
	msgTest9  = "供应商响应格式无效"  // Test @ providers.go
	msgTest10 = "连接测试通过"     // Test @ providers.go
)
