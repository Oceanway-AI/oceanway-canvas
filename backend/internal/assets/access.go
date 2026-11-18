package assets

import (
	"net/http"
	"time"

	"yingce/backend/internal/kernel"
	"yingce/backend/internal/model"
)

type AccessPurpose string
type ResourceVariant string
type DeliveryMode string

const (
	PurposeDisplay         AccessPurpose   = "display"
	PurposeCopy            AccessPurpose   = "copy"
	PurposeDownload        AccessPurpose   = "download"
	PurposeProcess         AccessPurpose   = "browser-process"
	PurposeEcommerceExport AccessPurpose   = "ecommerce-export"
	PurposeProvider        AccessPurpose   = "provider-input"
	VariantOriginal        ResourceVariant = "original"
	VariantPlayback        ResourceVariant = "playback"
	// VariantThumbnail 是画布展示档位；缩略图未生成前回落原件，保证请求不会因变体失败而退回平台重定向。
	VariantThumbnail ResourceVariant = "thumbnail"
	DeliveryCDN      DeliveryMode    = "cdn"
	DeliveryOrigin   DeliveryMode    = "origin"
	DeliveryLocal    DeliveryMode    = "platform-local"
	DeliveryProxy    DeliveryMode    = "platform-proxy"
)

// AccessOptions contains use intent, never a client-controlled authorization or transport override.
type AccessOptions struct {
	Purpose      AccessPurpose   `json:"purpose"`
	Variant      ResourceVariant `json:"variant"`
	DownloadName string          `json:"downloadName,omitempty"`
	ExpiresAt    time.Time       `json:"-"`
	DescribeOnly bool            `json:"-"`
}

type AccessRequest struct {
	ResourceID string `json:"resourceId"`
	AccessOptions
}

type ResourceAccess struct {
	ResourceID       string          `json:"resourceId"`
	RequestedVariant ResourceVariant `json:"requestedVariant"`
	ActualVariant    ResourceVariant `json:"actualVariant"`
	URL              string          `json:"url"`
	Delivery         DeliveryMode    `json:"delivery"`
	IssuedAt         time.Time       `json:"issuedAt"`
	ExpiresAt        *time.Time      `json:"expiresAt"`
	RefreshAt        time.Time       `json:"refreshAt"`
	Revision         string          `json:"revision"`
	FallbackReason   string          `json:"fallbackReason,omitempty"`
}

func AccessError(status int, reason, message string) *kernel.AppError {
	err := kernel.NewAppError(status, message)
	err.Reason = kernel.ErrorReason(reason)
	return err
}

func NormalizeAccessOptions(options AccessOptions, allowProvider bool) (AccessOptions, error) {
	if options.Purpose == "" {
		options.Purpose = PurposeDisplay
	}
	if options.Variant == "" {
		options.Variant = VariantOriginal
	}
	switch options.Purpose {
	case PurposeDisplay, PurposeCopy, PurposeDownload, PurposeProcess, PurposeEcommerceExport:
	case PurposeProvider:
		if !allowProvider {
			return options, kernel.Forbidden("不允许请求模型输入凭据")
		}
	default:
		return options, kernel.BadAuthRequest("资源访问用途无效")
	}
	if options.Variant != VariantOriginal && options.Variant != VariantPlayback && options.Variant != VariantThumbnail {
		return options, kernel.BadAuthRequest("资源变体无效")
	}
	if options.Purpose == PurposeCopy || options.Purpose == PurposeDownload || options.Purpose == PurposeProvider || options.Purpose == PurposeEcommerceExport {
		options.Variant = VariantOriginal
	}
	if options.Purpose != PurposeDownload {
		options.DownloadName = ""
	}
	return options, nil
}

type AccessPolicy struct {
	AccessURLTTL         time.Duration
	ProviderAccessURLTTL time.Duration
}

func (p AccessPolicy) normalized() AccessPolicy {
	if p.AccessURLTTL <= 0 {
		p.AccessURLTTL = 4 * time.Hour
	}
	if p.ProviderAccessURLTTL <= 0 {
		p.ProviderAccessURLTTL = 4 * time.Hour
	}
	return p
}

// ResolveAccess signs system-owned local resources after ownership authorization.
func ResolveAccess(resource *model.Resource, policy AccessPolicy, options AccessOptions, now time.Time,
	platformURL func(ResourceVariant, time.Time) (string, error)) (*ResourceAccess, error) {
	options, err := NormalizeAccessOptions(options, true)
	if err != nil {
		return nil, err
	}
	if resource == nil {
		return nil, kernel.NotFound("资源不存在")
	}
	if resource.Provider != "local" {
		return nil, AccessError(400, "platform_material_required", "此资源必须通过 Platform 素材授权访问")
	}
	if resource.Status != model.ResourceStatusReady {
		return nil, AccessError(http.StatusConflict, "resource_not_ready", "资源尚未上传完成")
	}
	if options.Purpose == PurposeEcommerceExport && (resource.Kind != "image" || resource.Size > 32*1024*1024 || resource.Size <= 0 || (resource.MimeType != "image/png" && resource.MimeType != "image/jpeg" && resource.MimeType != "image/webp")) {
		return nil, AccessError(http.StatusBadRequest, "ecommerce_export_invalid", "电商导出仅支持32 MB以内的PNG、JPEG和WebP商品图片")
	}
	// 系统素材签名采用运行时策略，默认有效期为四小时。
	runtime := policy.normalized()
	ttl := runtime.AccessURLTTL
	if options.Purpose == PurposeProvider {
		ttl = runtime.ProviderAccessURLTTL
	}
	// 浏览器读取的签名地址按固定时间窗对齐：窗口内同一对象的签名地址逐字节相同，
	// 刷新页面或多实例签发可以复用浏览器 HTTP 缓存。
	signedAt := time.Time{}
	if browserCacheable(options.Purpose) {
		signedAt = alignedSigningTime(now, ttl)
	}
	expires := now.Add(ttl)
	if !signedAt.IsZero() {
		expires = signedAt.Add(ttl)
	}
	if !options.ExpiresAt.IsZero() && options.ExpiresAt.Before(expires) {
		expires = options.ExpiresAt
	}
	if !expires.After(now) {
		return nil, kernel.Forbidden("资源授权已过期")
	}
	access := &ResourceAccess{ResourceID: resource.ID, RequestedVariant: options.Variant, ActualVariant: VariantOriginal,
		IssuedAt: now, ExpiresAt: &expires, RefreshAt: now.Add(expires.Sub(now) * 4 / 5), Revision: resource.ETag + ":" + resource.PlaybackStatus}
	if options.Variant == VariantPlayback {
		if resource.PlaybackStatus == model.PlaybackStatusReady && resource.PlaybackObjectKey != "" {
			access.ActualVariant = VariantPlayback
		} else {
			access.FallbackReason = "playback_not_ready"
		}
	}
	if options.Variant == VariantThumbnail {
		access.FallbackReason = "thumbnail_not_ready"
	}
	if options.Purpose == PurposeEcommerceExport {
		access.Delivery = DeliveryProxy
		access.FallbackReason = "ecommerce_export_processing"
		access.ExpiresAt = nil
	} else {
		access.Delivery = DeliveryLocal
	}

	access.URL, err = platformURL(access.ActualVariant, expires)
	if err != nil {
		return nil, err
	}
	return access, nil
}

// browserCacheable 标记浏览器直接读取字节、值得复用 HTTP 缓存的用途。
// 下载地址携带一次性的 Content-Disposition，模型输入由上游服务读取，二者仍按当前时间签发。
func browserCacheable(purpose AccessPurpose) bool {
	return purpose == PurposeDisplay || purpose == PurposeProcess || purpose == PurposeCopy
}

// alignedSigningTime 把签名起始时间对齐到 ttl/4 的时间窗（按 Unix 纪元对齐，多实例结果一致）。
// 地址过期时间为「窗口起点 + ttl」，任何时刻签出的地址至少还剩 3/4 有效期，且总有效期不超过后台配置的 ttl。
func alignedSigningTime(now time.Time, ttl time.Duration) time.Time {
	window := ttl / 4
	if window < time.Second {
		return now.UTC()
	}
	unix := now.UTC().UnixNano()
	return time.Unix(0, unix-unix%int64(window)).UTC()
}
