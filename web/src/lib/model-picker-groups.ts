import { groupModelsByDisplayName, isDirectSystemModel, type DisplayModelGroup } from "@/lib/model-selection";
import { modelIcon, modelOptionName, PUBLIC_MODEL_CATALOG_ID, resolveModelChannel, type AiConfig } from "@/stores/use-config-store";
import type { PlatformModelGroup } from "@/lib/platform-models";

export type ModelPickerGroup = {
    key: string;
    label: string;
    icon: string;
    scope: string;
    kind: "product" | "channel";
    models: DisplayModelGroup[];
    platform?: PlatformModelGroup;
    platformOwned?: boolean;
};

export { isDirectSystemModel } from "@/lib/model-selection";

export function filterModelPickerGroups(config: AiConfig, groups: ModelPickerGroup[], query: string): ModelPickerGroup[] {
    const needle = query.trim().toLocaleLowerCase();
    if (!needle) return groups;
    const matches = (parts: (string | undefined)[]) => parts.filter(Boolean).join(" ").toLocaleLowerCase().includes(needle);
    return groups.flatMap((group) => {
        if (matches([group.label, group.scope])) return [group];
        const models = group.models.filter((item) =>
            item.models.some((value) => {
                const cost = resolveModelChannel(config, value).modelCosts?.find((entry) => entry.model === modelOptionName(value));
                const presentation = cost?.platform?.presentation;
                return matches([item.label, modelOptionName(value), cost?.displayName, presentation?.summary, presentation?.description, ...(presentation?.tags || [])]);
            }),
        );
        return models.length ? [{ ...group, models }] : [];
    });
}

export function modelChannelLabel(config: AiConfig, value: string) {
    const channel = resolveModelChannel(config, value);
    const cost = channel.modelCosts?.find((item) => item.model === modelOptionName(value));
    if (cost?.platform) return cost.platform.content?.source_label?.trim() || cost.platform.content?.display_name?.trim() || cost.platform.model_name;
    if (channel.scope === "system") return cost?.channelLabel?.trim() || modelOptionName(value);
    return cost?.channelLabel?.trim() || channel.name || "未命名渠道";
}

// Platform owns grouping identities. Display labels never establish membership.
export function groupModelsForPicker(config: AiConfig, options: string[]): ModelPickerGroup[] {
    const groups = new Map<string, ModelPickerGroup>();
    for (const channel of config.channels) {
        const models = options.filter((value) => resolveModelChannel(config, value).id === channel.id);
        const directModels = models.filter((value) => isDirectSystemModel(config, value));
        for (const value of directModels) {
            const cost = channel.modelCosts?.find((item) => item.model === modelOptionName(value));
            if (cost?.platform) {
                const item = cost.platform;
                const suppliedGroup = channel.platformGroups?.find((group) => group.model_names.includes(item.model_name));
                const series = item.model_series?.trim();
                const logical = item.logical_model?.trim();
                const identity = item.model_type === "text" ? series : series && logical;
                const label = suppliedGroup?.display_name || identity || item.model_name;
                const key = suppliedGroup?.key || (identity ? JSON.stringify(["platform", item.model_type, series || "", identity]) : JSON.stringify(["platform-single", value]));
                let group = groups.get(key);
                if (!group) {
                    group = { key, label, icon: modelIcon(config, value), scope: series || "平台服务", kind: "product", models: [], platform: suppliedGroup, platformOwned: true };
                    groups.set(key, group);
                }
                group.models.push({ key: value, label: modelChannelLabel(config, value), models: [value] });
                continue;
            }
            const label = cost?.displayName?.trim() || "未分类";
            const key = JSON.stringify(["product", label]);
            let group = groups.get(key);
            if (!group) {
                group = { key, label, icon: modelIcon(config, value), scope: "平台服务", kind: "product", models: [] };
                groups.set(key, group);
            }
            group.models.push({ key: value, label: modelChannelLabel(config, value), models: [value] });
        }
        const otherModels = models.filter((value) => !isDirectSystemModel(config, value));
        if (otherModels.length) {
            const key = JSON.stringify(["channel", channel.id]);
            groups.set(key, {
                key,
                label: channel.scope === "system" ? "系统模型" : channel.name || "未命名渠道",
                icon: modelIcon(config, otherModels[0]),
                scope: channel.id === PUBLIC_MODEL_CATALOG_ID ? "" : "我的模型",
                kind: "channel",
                models: groupModelsByDisplayName(config, otherModels),
            });
        }
    }
    return Array.from(groups.values());
}
