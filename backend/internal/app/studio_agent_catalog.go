package app

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"yingce/backend/internal/platformintegration"
)

const studioAgentCatalogPageBytes = 48 << 10

func studioAgentSummaryList(summaries []platformclient.ModelSummary, args studioAgentCatalogArgs, refs []studioNativeReferenceResource) (any, error) {
	items := []map[string]any{}
	matched, unavailable, usedBytes := 0, 0, 1024
	pageFull := false
	for _, summary := range summaries {
		if summary.ModelType == "text" || args.Mode != "" && summary.ModelType != args.Mode || args.Query != "" && !strings.Contains(strings.ToLower(summary.ModelName+" "+summary.Description), strings.ToLower(args.Query)) {
			continue
		}
		if summary.InvocationStatus != "available" {
			unavailable++
			continue
		}
		variants := summary.Variants
		if !summary.VariantsComplete {
			// A truncated summary cannot rule out the unlisted variants.
			variants = []platformclient.ModelVariantSummary{{RequiresDetail: true}}
		}
		for _, variant := range variants {
			slots := []platformintegration.NativeReferenceSlot{}
			for i, ref := range variant.References {
				slots = append(slots, platformintegration.NativeReferenceSlot{ID: fmt.Sprint(i), Role: ref.Role, MinItems: ref.MinItems, MaxItems: ref.MaxItems, MIMETypes: ref.MIMETypes})
			}
			if variant.ReferencesComplete && studioValidateNativeReferences(slots, refs) != nil {
				continue
			}
			matched++
			if matched <= args.Offset || pageFull {
				continue
			}
			selection := map[string]any{"channelId": StudioPlatformChannelID, "channelModelKey": summary.ModelName, "nativeVariantId": variant.ID}
			arguments := map[string]any{"mode": summary.ModelType, "channelId": StudioPlatformChannelID, "channelModelKey": summary.ModelName, "nativeVariantId": variant.ID, "referenceNodeIds": args.ReferenceNodeIDs}
			if args.VideoStartFrameNodeID != "" {
				arguments["videoStartFrameNodeId"] = args.VideoStartFrameNodeID
			}
			if args.VideoEndFrameNodeID != "" {
				arguments["videoEndFrameNodeId"] = args.VideoEndFrameNodeID
			}
			row := map[string]any{"name": summary.ModelName, "capability": summary.ModelType, "selection": selection, "generationArguments": arguments, "referenceInputs": variant.References, "detailsLoaded": false, "requiresDetail": true, "variantsComplete": summary.VariantsComplete, "referencesComplete": variant.ReferencesComplete, "priceLabel": "审批时读取平台精确报价"}
			raw, err := json.Marshal(row)
			if err != nil || len(raw)+1024 > studioAgentCatalogPageBytes {
				return nil, NewAppError(502, "模型摘要超过单次容量，请指定型号查询")
			}
			if len(items) == args.Limit || usedBytes+len(raw) > studioAgentCatalogPageBytes {
				pageFull = true
				continue
			}
			items, usedBytes = append(items, row), usedBytes+len(raw)+1
		}
	}
	if args.Offset > matched {
		return nil, BadAuthRequest("模型目录分页位置无效")
	}
	return map[string]any{"source": "platform", "models": items, "offset": args.Offset, "nextOffset": args.Offset + len(items), "hasMore": args.Offset+len(items) < matched, "totalMatches": matched, "unavailableCount": unavailable,
		"generationGuidance": "列表提供参考角色和数量的初步匹配。用 channelModelKey 和 nativeVariantId 查询选中变体的参数详情，再复制 generationArguments 并补提示词。variantsComplete=false 时先用 channelModelKey 查询全部变体。hasMore=true 可用 nextOffset 续查。生成前核验完整素材限制、版本与报价。"}, nil
}

type studioAgentCatalogArgs struct {
	Mode                  string   `json:"mode"`
	ReferenceNodeIDs      []string `json:"referenceNodeIds"`
	VideoStartFrameNodeID string   `json:"videoStartFrameNodeId"`
	VideoEndFrameNodeID   string   `json:"videoEndFrameNodeId"`
	ChannelModelKey       string   `json:"channelModelKey"`
	NativeVariantID       string   `json:"nativeVariantId"`
	Query                 string   `json:"query"`
	Offset                int      `json:"offset"`
	Limit                 int      `json:"limit"`
}

func (s *Service) studioAgentCatalogReferences(userID string, state *cloudAgentRuntime, args studioAgentCatalogArgs) ([]studioNativeReferenceResource, error) {
	if len(args.ReferenceNodeIDs) == 0 && args.VideoStartFrameNodeID == "" && args.VideoEndFrameNodeID == "" {
		return nil, nil
	}
	canvas, err := s.repo.CanvasProjectForUser(userID, state.Request.CanvasID)
	if err != nil {
		return nil, err
	}
	doc, err := creationDocument(canvas.PayloadJSON)
	if err != nil {
		return nil, err
	}
	nodes, err := creationObjects(doc["nodes"])
	if err != nil {
		return nil, err
	}
	input := map[string]any{"config": map[string]any{"channelId": StudioPlatformChannelID}, "metadata": map[string]any{
		"videoStartFrameNodeId": args.VideoStartFrameNodeID, "videoEndFrameNodeId": args.VideoEndFrameNodeID,
	}}
	seen := map[string]bool{}
	for _, id := range args.ReferenceNodeIDs {
		if id == "" || seen[id] || nodes[id] == nil {
			return nil, BadAuthRequest("参考节点不存在或重复")
		}
		seen[id] = true
		ref, field, err := cloudAgentMediaReference(s.repo, userID, canvas.ProjectID, nodes[id])
		if err != nil {
			return nil, err
		}
		list, _ := input[field].([]any)
		input[field] = append(list, ref)
	}
	if args.Mode != "" {
		if err := validateCloudAgentMediaReferences(args.Mode, input); err != nil {
			return nil, err
		}
	}
	decoded, err := studioDecodeInput(CreateTaskRequest{Input: input})
	if err != nil {
		return nil, err
	}
	return s.studioNativeReferenceResources(context.Background(), userID, decoded)
}

func studioAgentReferenceSlots(selected platformintegration.ModelSelection) []platformintegration.NativeReferenceSlot {
	if selected.NativeRequest != nil {
		return selected.NativeRequest.References
	}
	slots := []platformintegration.NativeReferenceSlot{}
	for index, ref := range selected.References {
		minimum, maximum := int64(ref.MinItems), int64(ref.MaxItems)
		for _, role := range ref.Roles {
			slot := platformintegration.NativeReferenceSlot{ID: fmt.Sprint(index), Pointer: ref.Field, Role: role, MinItems: &minimum, MaxItems: &maximum, MIMETypes: ref.MIMETypes}
			if ref.MaxItemBytes > 0 {
				slot.MaxItemBytes = &ref.MaxItemBytes
			}
			if ref.MaxTotalBytes > 0 {
				slot.MaxTotalBytes = &ref.MaxTotalBytes
			}
			slots = append(slots, slot)
		}
	}
	return slots
}

func (s *Service) studioAgentModelList(userID string, state *cloudAgentRuntime, call cloudAgentCall) (any, error) {
	var args studioAgentCatalogArgs
	if err := decodeCloudAgentJSONObject(call.Function.Arguments, &args); err != nil {
		return nil, cloudAgentJSONArgumentError(err)
	}
	if args.Mode != "" && args.Mode != "image" && args.Mode != "video" || len(args.ReferenceNodeIDs) > 16 || args.Offset < 0 || args.Limit < 0 || args.Limit > 24 || args.NativeVariantID != "" && args.ChannelModelKey == "" {
		return nil, BadAuthRequest("模型查询需使用图片或视频模式、有效分页及明确型号")
	}
	if (args.VideoStartFrameNodeID != "" || args.VideoEndFrameNodeID != "") && args.Mode != "video" {
		return nil, BadAuthRequest("首尾帧只适用于视频模型查询")
	}
	if args.Limit == 0 {
		args.Limit = 12
	}
	if args.ReferenceNodeIDs == nil {
		args.ReferenceNodeIDs = []string{}
	}
	refs, err := s.studioAgentCatalogReferences(userID, state, args)
	if err != nil {
		return nil, err
	}
	root, err := s.repo.StudioExecution(userID, state.RuntimeRunID)
	if err != nil {
		return nil, err
	}
	var envelope studioExecutionEnvelope
	if err := s.decryptStudioValue(root.EnvelopeEncrypted, &envelope); err != nil {
		return nil, err
	}
	if envelope.CatalogVersion != "" {
		if args.ChannelModelKey == "" {
			return studioAgentSummaryList(envelope.CatalogSummaries, args, refs)
		}
		detail, err := s.studioAgentCatalogDetail(userID, state.RuntimeRunID, args.ChannelModelKey, args.NativeVariantID)
		if err != nil {
			return nil, err
		}
		envelope.Catalog = []platformclient.Model{detail.Model}
	}
	items := []map[string]any{}
	matched, unavailable, usedBytes := 0, 0, 1024
	detail := args.ChannelModelKey != "" && args.NativeVariantID != ""
	pageFull := false
	for _, item := range envelope.Catalog {
		if item.ModelType == "text" || args.Mode != "" && args.Mode != item.ModelType || args.ChannelModelKey != "" && args.ChannelModelKey != item.ModelName || args.Query != "" && !strings.Contains(strings.ToLower(item.ModelName+" "+item.Description), strings.ToLower(args.Query)) {
			continue
		}
		var profile platformintegration.MediaProfile
		if json.Unmarshal(item.MediaGenerationProfile, &profile) != nil {
			unavailable++
			continue
		}
		variants := []string{""}
		if len(profile.Variants) > 0 {
			variants = nil
			for _, variant := range profile.Variants {
				variants = append(variants, variant.ID)
			}
		}
		for _, variant := range variants {
			if args.NativeVariantID != "" && args.NativeVariantID != variant {
				continue
			}
			selected, err := platformintegration.SelectModelFromProfile(item, variant, profile)
			if err != nil {
				unavailable++
				continue
			}
			slots := studioAgentReferenceSlots(selected)
			if studioValidateNativeReferences(slots, refs) != nil {
				continue
			}
			if v := selected.NativeRequest; v != nil {
				envelope := map[string]any{}
				if studioWriteNativeEnvelope(envelope, nil, selected, refs) != nil || !platformintegration.MaterialSchemaCompatible(v.Schema, envelope, slots) {
					continue
				}
				options, err := v.Alternatives()
				if err != nil || studioValidateMaterialGroup(v.Materials, refs, 0) != nil {
					continue
				}
				if len(options) > 0 && !slices.ContainsFunc(options, func(o platformintegration.NativeSupplyAlternative) bool {
					return studioValidateNativeReferences(o.References, refs) == nil && studioValidateMaterialGroup(o.Materials, refs, 0) == nil && platformintegration.MaterialSchemaCompatible(o.Schema, envelope, slots)
				}) {
					continue
				}
			}
			matched++
			if matched <= args.Offset || pageFull {
				continue
			}
			selection := map[string]any{"channelId": StudioPlatformChannelID, "channelModelKey": selected.Name, "nativeVariantId": selected.ID}
			arguments := map[string]any{"mode": selected.Kind, "channelId": StudioPlatformChannelID, "channelModelKey": selected.Name, "nativeVariantId": selected.ID, "referenceNodeIds": args.ReferenceNodeIDs}
			if args.VideoStartFrameNodeID != "" {
				arguments["videoStartFrameNodeId"] = args.VideoStartFrameNodeID
			}
			if args.VideoEndFrameNodeID != "" {
				arguments["videoEndFrameNodeId"] = args.VideoEndFrameNodeID
			}
			row := map[string]any{"name": selected.Name, "capability": selected.Kind, "selection": selection, "generationArguments": arguments, "referenceInputs": slots, "priceLabel": "审批时读取平台精确报价", "detailsLoaded": detail}
			if detail {
				row["defaults"], row["specificationsSchema"] = selected.Defaults, json.RawMessage(selected.Schema)
				if selected.NativeRequest != nil {
					arguments["nativeParameters"] = "{}"
					settings := cloudAgentMediaArgs{Mode: selected.Kind, NativeVariantID: selected.ID}
					if studioApplyAgentMediaOptions(&settings, state, selected) != nil {
						row["requiresParameters"] = true
					}
				}
			}
			raw, err := json.Marshal(row)
			if err != nil || len(raw)+1024 > studioAgentCatalogPageBytes {
				return nil, NewAppError(502, "所选模型的参数目录超过单次容量，请在模型设置中查看")
			}
			if len(items) == args.Limit || usedBytes+len(raw) > studioAgentCatalogPageBytes {
				pageFull = true
				continue
			}
			items, usedBytes = append(items, row), usedBytes+len(raw)+1
		}
	}
	if args.Offset > matched {
		return nil, BadAuthRequest("模型目录分页位置无效")
	}
	return map[string]any{
		"source": "platform", "models": items, "offset": args.Offset, "nextOffset": args.Offset + len(items),
		"hasMore": args.Offset+len(items) < matched, "totalMatches": matched, "unavailableCount": unavailable,
		"generationGuidance": "列表按真实参考与角色筛选。用 channelModelKey 和 nativeVariantId 查询所选变体详情，复制 generationArguments 并补提示词。requiresParameters=true 时请根据详情选择可用参数并填写 nativeParameters。hasMore=true 可用 nextOffset 续查。生成前会重新核验版本和报价。",
	}, nil
}
