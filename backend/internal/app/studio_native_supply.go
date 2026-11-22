package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/model"
	"yingce/backend/internal/platformintegration"
)

func studioPrepareNativeCombination(v *platformintegration.NativeRequestVariant, refs []studioNativeReferenceResource, body map[string]any) ([]byte, error) {
	validateSize := func(size *platformclient.ImageOutputSize, prepared []byte) error {
		if size == nil {
			return nil
		}
		profile, err := json.Marshal(platformclient.ImageSizeProfile{OutputSize: size, SizePresets: v.SizePresets})
		if err != nil || platformclient.ValidateNativeImageSize(profile, prepared) != nil {
			return BadAuthRequest("图片尺寸超出所选规格的范围")
		}
		return nil
	}
	options, err := v.Alternatives()
	if err != nil {
		return nil, BadAuthRequest("模型供给合同无效")
	}
	if _, explicit := body["size"]; !explicit && v.OutputSize != nil {
		if value, published := v.Defaults["size"]; published {
			body["size"] = value
		}
	}
	checkBase := func(candidate map[string]any) ([]byte, error) {
		if candidate["stream"] == true || candidate["async"] == true {
			return nil, BadAuthRequest("生成模式必须使用已发布的非流式协议")
		}
		if err := platformintegration.ValidateSpecifications(v.Schema, candidate); err != nil {
			return nil, BadAuthRequest(err.Error())
		}
		prepared, err := json.Marshal(candidate)
		if err != nil {
			return nil, BadAuthRequest("模型请求参数无效")
		}
		if err := validateSize(v.OutputSize, prepared); err != nil {
			return nil, err
		}
		return prepared, studioValidateMaterialGroup(v.Materials, refs, studioOutputDurationMS(candidate))
	}
	prepared, baseErr := checkBase(body)
	if len(options) == 0 {
		return prepared, baseErr
	}
	checkOption := func(option platformintegration.NativeSupplyAlternative, candidate map[string]any, prepared []byte) error {
		if err := platformintegration.ValidateSpecifications(option.Schema, candidate); err != nil {
			return BadAuthRequest("当前参数与参考素材不属于同一组可用规格")
		}
		if err := validateSize(option.OutputSize, prepared); err != nil {
			return err
		}
		if err := studioValidateNativeReferences(option.References, refs); err != nil {
			return err
		}
		return studioValidateMaterialGroup(option.Materials, refs, studioOutputDurationMS(candidate))
	}
	mismatch := baseErr
	if baseErr == nil {
		for _, option := range options {
			mismatch = checkOption(option, body, prepared)
			if mismatch == nil {
				return prepared, nil
			}
		}
	}
	var result []byte
	for _, option := range options {
		var schema studioNativeSchemaNode
		if json.Unmarshal(option.Schema, &schema) != nil {
			continue
		}
		candidate := studioMergeNativeObjects(nil, body)
		studioFillNativeRequired(schema, candidate, v.Defaults)
		if _, set := candidate["size"]; !set && option.OutputSize != nil {
			if value, published := v.Defaults["size"]; published {
				candidate["size"] = value
			}
		}
		value, err := json.Marshal(candidate)
		if err != nil || bytes.Equal(value, prepared) {
			continue
		}
		if _, err := checkBase(candidate); err != nil || checkOption(option, candidate, value) != nil {
			continue
		}
		if result != nil && !bytes.Equal(value, result) {
			return nil, BadAuthRequest("多个供给规格需要不同的必填值，请明确选择生成参数")
		}
		result = value
	}
	if result != nil {
		return result, nil
	}
	return nil, mismatch
}

func studioOutputDurationMS(body map[string]any) int64 {
	for _, key := range []string{"duration", "seconds"} {
		seconds, err := strconv.ParseInt(fmt.Sprint(body[key]), 10, 64)
		if err == nil && seconds > 0 && seconds <= math.MaxInt64/1000 {
			return seconds * 1000
		}
	}
	return 0
}

func studioNativeChoices(item platformclient.Model) (platformintegration.MediaProfile, bool) {
	var profile platformintegration.MediaProfile
	if json.Unmarshal(item.MediaGenerationProfile, &profile) != nil {
		return profile, false
	}
	return profile, len(profile.Variants) > 1 && slices.ContainsFunc(profile.Variants, func(e platformintegration.NativeEndpoint) bool { return e.NativeRequest != nil })
}

// Hosts may pick between equivalent bodies on the same route. A different
// body or execution path needs an explicit choice before pricing the request.
func studioChooseNativeVariant(item platformclient.Model, profile platformintegration.MediaProfile, prepare func(platformintegration.ModelSelection) ([]byte, error)) (platformintegration.ModelSelection, error) {
	var chosen platformintegration.ModelSelection
	var prepared []byte
	var conflict error
	for _, endpoint := range profile.Variants {
		selected, err := platformintegration.SelectModelFromProfile(item, endpoint.ID, profile)
		if err != nil || selected.NativeRequest == nil {
			continue
		}
		body, err := prepare(selected)
		if err != nil {
			conflict = err
			continue
		}
		if prepared != nil {
			if chosen.GenerationV1 != selected.GenerationV1 || chosen.MediaRoute != selected.MediaRoute || !bytes.Equal(prepared, body) {
				return chosen, BadAuthRequest("有多个不同的生成规格可用，请从模型详情选择变体")
			}
			continue
		}
		chosen, prepared = selected, body
	}
	if prepared == nil {
		if conflict != nil {
			return chosen, conflict
		}
		return chosen, BadAuthRequest("没有支持本次参数和素材的模型规格")
	}
	return chosen, nil
}

func studioExceeds[T int64 | float64](value T, minValue, maxValue *T) bool {
	return minValue != nil && value < *minValue || maxValue != nil && value > *maxValue
}

func studioTotalWithin(total *int64, value int64, maximum *int64) bool {
	if value < 0 || value > math.MaxInt64-*total {
		return false
	}
	*total += value
	return maximum == nil || *total <= *maximum
}

func studioValidateMaterialGroup(c *platformintegration.NativeMaterialConstraints, refs []studioNativeReferenceResource, outputDuration int64) error {
	if c == nil {
		return nil
	}
	if studioExceeds(int64(len(refs)), nil, c.MaxItems) {
		return BadAuthRequest("参考素材总数超出所选规格的限制")
	}
	var bytes int64
	videoDuration := outputDuration
	var visual, audio bool
	previous := 0
	for _, ref := range refs {
		rank := 0
		switch ref.Role {
		case "reference_image":
			visual, rank = true, 1
		case "first_frame", "last_frame":
			visual = true
		case "reference_video":
			visual, rank = true, 2
		case "reference_audio":
			audio, rank = true, 3
		}
		if c.ReferenceOrder == "images_videos_audios" && rank < previous {
			return BadAuthRequest("参考素材须按图片、视频、音频的顺序提交")
		}
		previous = rank
		if ref.Resource == nil || !studioTotalWithin(&bytes, ref.Resource.Size, c.MaxTotalBytes) {
			return BadAuthRequest("参考素材总大小超出所选规格的限制")
		}
		if ref.Role == "reference_video" && !studioTotalWithin(&videoDuration, ref.Resource.DurationMs, c.MaxVideoAndOutputDurationMS) {
			return BadAuthRequest("参考视频和输出视频总时长超出所选规格的限制")
		}
	}
	if studioExceeds(videoDuration, nil, c.MaxVideoAndOutputDurationMS) {
		return BadAuthRequest("参考视频和输出视频总时长超出所选规格的限制")
	}
	if c.AudioRequiresVisual && audio && !visual {
		return BadAuthRequest("参考音频需要搭配图片或视频")
	}
	return nil
}

func studioValidateNativeReferences(slots []platformintegration.NativeReferenceSlot, refs []studioNativeReferenceResource) error {
	byRole := map[string][]*model.Resource{}
	for _, ref := range refs {
		if ref.Resource == nil {
			return BadAuthRequest("缺少参考素材的信息")
		}
		byRole[ref.Role] = append(byRole[ref.Role], ref.Resource)
	}
	roles, ids := map[string]bool{}, map[string]string{}
	paths := map[string][]string{}
	for _, slot := range slots {
		if roles[slot.Role] {
			return BadAuthRequest("模型参考角色声明不明确")
		}
		roles[slot.Role], ids[slot.ID] = true, slot.Role
		paths[slot.Pointer] = append(paths[slot.Pointer], slot.Role)
		items := byRole[slot.Role]
		if studioExceeds(int64(len(items)), slot.MinItems, slot.MaxItems) {
			return BadAuthRequest("参考素材数量不符合所选规格")
		}
		var bytes, duration int64
		for _, item := range items {
			if studioExceeds(item.Size, nil, slot.MaxItemBytes) || !studioTotalWithin(&bytes, item.Size, slot.MaxTotalBytes) || len(slot.MIMETypes) > 0 && !slices.Contains(slot.MIMETypes, item.MimeType) {
				return BadAuthRequest("参考素材的格式或大小不符合所选规格")
			}
			if item.DurationMs > 0 && (studioExceeds(item.DurationMs, slot.MinDurationMS, slot.MaxDurationMS) || !studioTotalWithin(&duration, item.DurationMs, slot.MaxTotalDurationMS)) {
				return BadAuthRequest("参考素材时长不符合所选规格")
			}
			w, h := int64(item.Width), int64(item.Height)
			if w > 0 && studioExceeds(w, slot.MinWidth, slot.MaxWidth) || h > 0 && studioExceeds(h, slot.MinHeight, slot.MaxHeight) {
				return BadAuthRequest("参考素材宽高不符合所选规格")
			}
			if w > 0 && h > 0 && (w > math.MaxInt64/h || studioExceeds(w*h, slot.MinPixels, slot.MaxPixels) || studioExceeds(float64(w)/float64(h), slot.MinAspectRatio, slot.MaxAspectRatio)) {
				return BadAuthRequest("参考素材像素或比例不符合所选规格")
			}
		}
	}
	for role := range byRole {
		if !roles[role] {
			return BadAuthRequest("所选模型不支持当前参考素材角色")
		}
	}
	for _, slot := range slots {
		if slot.SameDimensionsAs == "" || len(byRole[slot.Role]) == 0 {
			continue
		}
		target, found := ids[slot.SameDimensionsAs]
		if !found && strings.HasPrefix(slot.SameDimensionsAs, "/") && len(paths[slot.SameDimensionsAs]) == 1 {
			target = paths[slot.SameDimensionsAs][0]
		}
		if len(byRole[target]) == 0 {
			return BadAuthRequest("参考素材尺寸约束缺少明确的目标")
		}
		for _, item := range byRole[slot.Role] {
			for _, other := range byRole[target] {
				if item.Width <= 0 || item.Height <= 0 || item.Width != other.Width || item.Height != other.Height {
					return BadAuthRequest("参考素材尺寸不一致或未完成尺寸校验")
				}
			}
		}
	}
	return nil
}
