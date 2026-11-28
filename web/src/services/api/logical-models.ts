import { http } from "@/services/api/request";
import type { ModelTag } from "@/lib/model-tags";
import type { ImageResolutionOption } from "@/lib/image-resolution-tiers";
export type InputConstraint = { min: number; max: number };
export type OptionConstraint = { values?: unknown[]; min?: number; max?: number; step?: number };
export type CapabilityImageSizePreset = ImageResolutionOption;
export type CapabilityImageSize = {
    parameter?: "size" | "aspect_ratio";
    allowCustom?: boolean;
    presets?: CapabilityImageSizePreset[];
};
export type CapabilitySpec = {
    imageParameters?: import("@/lib/image-parameters").ImageParameter[];
    version: 1;
    capability: "text" | "image" | "video" | "audio";
    operations?: string[];
    inputs?: Record<string, InputConstraint>;
    options?: Record<string, OptionConstraint>;
    imageSize?: CapabilityImageSize;
};

export type PublicLogicalModel = {
    id: string;
    code: string;
    name: string;
    icon?: string;
    description: string;
    capability: CapabilitySpec["capability"];
    sortOrder: number;

    legacyModelIds: string[];
    capabilitySpec: CapabilitySpec;
    capabilityProfiles: CapabilitySpec[];
    defaultOptions: Record<string, unknown>;
    available: boolean;
};

export type ModelCatalogSource = "system" | "platform";

export type PublicChannelCatalog = {
    platformGroups?: import("@/lib/platform-models").PlatformModelGroup[];
    id: string;
    name: string;
    displayName: string;
    sortOrder?: number;
    models: PublicChannelModel[];
};

export type PublicChannelModel = {
    platform?: import("@/lib/platform-models").PlatformModel;
    unavailableReason?: string;
    id: string;
    modelKey: string;
    displayName: string;
    channelLabel?: string;
    tags?: ModelTag[];
    description?: string;
    sortOrder?: number;
    icon: string;
    capability: string;
    protocol?: string;
    /** 文本模型协议按插件 create 路径识别出的请求格式；插件自定义接口时缺省。 */
    textWire?: string;
    capabilityConfig?: Record<string, any>;
    defaultOptions?: Record<string, unknown>;

    pricingMode: string;
    displayPrice?: number;
    priceLabel: string;
    available: boolean;
    availability?: PublicChannelModelAvailability;
    /** 仅视频模型：挂参考视频的请求是否按带参考视频价格另计费。 */
};

export type PublicChannelModelAvailability = {
    rate24h: number | null;
    sampleCount24h: number;
    trend7d: PublicChannelModelAvailabilityDay[];
    recentResults?: boolean[];
    dataState: "ready" | "insufficient" | "no_data";
    computedAt: string;
    dataThrough?: string;
};

export type PublicChannelModelAvailabilityDay = {
    day: string;
    rate: number | null;
    sampleCount: number;
    dataState: "ready" | "insufficient" | "no_data";
};

export type ModelCatalogResponse = {
    source: ModelCatalogSource;
    models?: PublicLogicalModel[];
    channels?: PublicChannelCatalog[];
    catalogRevision?: string;
    nextCursor?: string;
    hasMore?: boolean;
};

// 创作目录直接读取系统渠道模型，不使用逻辑模型及其功能开关。
export function getModelCatalog() {
    return http.get<ModelCatalogResponse>("/model-catalog");
}

export function getModelCatalogPage(cursor = "", filter?: { model_type: "text"; query?: string; defaults_only?: boolean }) {
    return http.get<ModelCatalogResponse>("/model-catalog/summaries", { params: { limit: 50, ...filter, ...(cursor ? { cursor } : {}) } });
}

export function getModelCatalogDetail(model: import("@/lib/platform-models").PlatformModel) {
    return http.get<{ model: PublicChannelModel }>("/model-catalog/detail", {
        params: {
            model_name: model.model_name,
            expected_price_version: model.price_version || model.price.version,
            ...(model.model_type === "text" ? {} : { expected_capability_revision: model.capability_revision, expected_connection_revision: model.connection_revision }),
        },
    });
}
