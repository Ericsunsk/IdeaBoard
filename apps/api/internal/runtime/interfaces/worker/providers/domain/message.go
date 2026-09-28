package domain

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgResolveProviderPublicImageURL  = "%s请求缺少原图"            // ResolveProviderPublicImageURL @ image_ref.go
	msgResolveProviderPublicImageURL2 = "%s原图地址无效: %w"        // ResolveProviderPublicImageURL @ image_ref.go
	msgResolveProviderPublicImageURL3 = "无法将上传图片转换为公网 URL"    // ResolveProviderPublicImageURL @ image_ref.go
	msgResolveProviderPublicImageURL4 = "%s原图仅支持公网 HTTPS URL" // ResolveProviderPublicImageURL @ image_ref.go
	msgResolveProviderPublicImageURL5 = "%s原图地址无效"            // ResolveProviderPublicImageURL @ image_ref.go
)
