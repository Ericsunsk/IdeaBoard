package media

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgValidateSubmitMediaAssetIDs  = "无法校验媒体资产权限"               // ValidateSubmitMediaAssetIDs @ media.go
	msgValidateSubmitMediaAssetIDs2 = "媒体 AssetID 无效: index=%d"  // ValidateSubmitMediaAssetIDs @ media.go
	msgValidateSubmitMediaAssetIDs3 = "无权使用媒体 AssetID: index=%d" // ValidateSubmitMediaAssetIDs @ media.go
	msgValidateSubmitMediaRef       = "仅支持画布上传资源"                // ValidateSubmitMediaRef @ media.go
	msgValidateSubmitMediaRef2      = "无法校验画布上传资源权限"             // ValidateSubmitMediaRef @ media.go
	msgValidateSubmitMediaRef3      = "无权使用该画布上传资源"              // ValidateSubmitMediaRef @ media.go
)
