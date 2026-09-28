package local

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgLocalPath           = "本地存储路径无效: %w"          // LocalPath @ driver.go
	msgLocalPath2          = "本地存储路径越界"              // LocalPath @ driver.go
	msgGetBucketInfo       = "当前存储类型不支持读取 Bucket 信息" // GetBucketInfo @ driver.go
	msgUploadRef           = "图片校验失败: %w"            // UploadRef @ driver.go
	msgUploadRef2          = "打开文件失败: %w"            // UploadRef @ driver.go
	msgStoreBytesRef       = "读取图片信息失败: %w"          // StoreBytesRef @ driver.go
	msgSaveReaderLocalRef  = "上传对象路径无效: %w"          // SaveReaderLocalRef @ driver.go
	msgSaveReaderLocalRef2 = "创建目录失败: %w"            // SaveReaderLocalRef @ driver.go
	msgSaveReaderLocalRef3 = "保存文件失败: %w"            // SaveReaderLocalRef @ driver.go
	msgRead                = "读取文件失败: %w"            // Read @ driver.go
	msgDelete              = "删除文件失败: %w"            // Delete @ driver.go
	msgDeleteByPrefix      = "按前缀删除本地文件失败: %w"       // DeleteByPrefix @ driver.go
	msgReadStorageObject   = "文件超过大小限制 (%d MB)"      // ReadStorageObject @ driver.go
	msgReadStorageObject2  = "文件内容为空"                // ReadStorageObject @ driver.go
)
