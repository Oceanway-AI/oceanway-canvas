const unavailableRoutes: Record<string, string> = {
    admin: "管理后台",
    projects: "短剧创作",
    ecommerce: "电商创作",
    toolbox: "独立创作工作台",
    skills: "技能工作台",
    plugins: "插件中心",
    referral: "邀请返利",
    wallet: "积分中心",
    "test-voice-recording": "语音录制实验",
    share: "公开分享",
    "ark-portrait": "供应商素材库",
};

export function studioUnavailableRoute(pathname: string): string | undefined {
    return unavailableRoutes[pathname.split("/").filter(Boolean)[0]];
}

export function studioSettingsSectionAvailable(section: string): boolean {
    return ["profile", "preferences", "models", "storage"].includes(section);
}
