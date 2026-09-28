package secrets

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgRequireEncryptionEnabled = "加密密钥未配置"              // RequireEncryptionEnabled @ encrypt.go
	msgEncrypt                  = "创建 AES cipher 失败: %w" // Encrypt @ encrypt.go
	msgEncrypt2                 = "创建 GCM 失败: %w"        // Encrypt @ encrypt.go
	msgEncrypt3                 = "生成 nonce 失败: %w"      // Encrypt @ encrypt.go
	msgDecrypt                  = "解码密文失败: %w"           // Decrypt @ encrypt.go
	msgDecrypt2                 = "密文数据不完整"              // Decrypt @ encrypt.go
)
