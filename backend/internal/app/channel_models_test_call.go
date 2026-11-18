// 管理端「测试模型」：按能力构造最小测试请求并真实调用一次上游。

package app

import (
	"strings"
)

// 模型测试必须使用当前模型声明的默认参数，避免固定分辨率 SKU 被通用 1K 测试值误伤。
// videoTestDefaults 从模型能力画像取测试用的比例和分辨率；画像缺失时回退到最通用的 16:9 / 720。
func videoTestDefaults(profile *VideoCapabilityConfig) (string, string) {
	if profile == nil {
		return "16:9", "720"
	}
	ratio := strings.TrimSpace(profile.DefaultRatio)
	if ratio == "" && len(profile.Ratios) > 0 {
		ratio = strings.TrimSpace(profile.Ratios[0])
	}
	if ratio == "" {
		ratio = "16:9"
	}
	resolution := strings.TrimSpace(profile.DefaultResolution)
	if resolution == "" && len(profile.Resolutions) > 0 {
		resolution = strings.TrimSpace(profile.Resolutions[0])
	}
	if resolution == "" {
		resolution = "720"
	}
	return ratio, resolution
}

// videoTestSeconds 取模型能力声明的默认时长（保存时已校验它属于时长范围或固定档位），
// 避免固定 6 秒落在 5/10 秒这类档位之外造成测试假失败；画像缺失时才用 6 秒。
func videoTestSeconds(profile *VideoCapabilityConfig) int {
	if profile != nil && profile.Duration.Default > 0 {
		return profile.Duration.Default
	}
	return 6
}

func imageTestDefaults(profile *ImageCapabilityConfig) (string, string) {
	if profile == nil {
		return "1024x1024", "auto"
	}
	size := ""
	if profile.Size.Parameter != "none" {
		size = strings.TrimSpace(profile.Size.Default)
	}
	quality := ""
	if profile.Quality.Supported {
		quality = strings.TrimSpace(profile.Quality.Default)
	}
	return size, quality
}
