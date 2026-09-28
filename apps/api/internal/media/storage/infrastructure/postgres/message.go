package postgres

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgSaveProfileTx  = "加密存储密钥失败: %w" // SaveProfileTx @ repository.go
	msgProfileFromRow = "解密存储密钥失败: %w" // ProfileFromRow @ repository.go
)
