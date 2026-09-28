package remote

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgDownloadImageWithLimit  = "下载图片失败: %w"         // DownloadImageWithLimit @ downloader.go
	msgDownloadImageWithLimit2 = "下载图片返回 %d"          // DownloadImageWithLimit @ downloader.go
	msgDownloadImageWithLimit3 = "创建临时文件失败: %w"       // DownloadImageWithLimit @ downloader.go
	msgDownloadImageWithLimit4 = "读取图片数据失败: %w"       // DownloadImageWithLimit @ downloader.go
	msgDownloadImageWithLimit5 = "远程文件超过大小限制 (%d MB)" // DownloadImageWithLimit @ downloader.go
	msgDownloadImageWithLimit6 = "远程图片校验失败: %w"       // DownloadImageWithLimit @ downloader.go
	msgDownloadImageWithLimit7 = "读取临时文件失败: %w"       // DownloadImageWithLimit @ downloader.go
)
