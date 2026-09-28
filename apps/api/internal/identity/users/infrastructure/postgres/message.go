package postgres

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgCreate             = "用户不能为空"       // Create @ repository.go
	msgAllocateStorageUID = "生成唯一用户存储标识失败" // AllocateStorageUID @ repository.go
)
