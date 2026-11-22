const labels: Record<string, string> = {
    size: "尺寸",
    imageSize: "分辨率",
    resolution: "分辨率",
    quality: "画质",
    aspectRatio: "画幅",
    aspect_ratio: "画幅",
    ratio: "画幅",
    duration: "时长",
    seconds: "时长",
    generate_audio: "生成音频",
    n: "生成数量",
    output_format: "输出格式",
    output_compression: "压缩率",
    background: "背景",
    seed: "随机种子",
};

export function creativeParameterLabel(pointer: string, title?: string) {
    const leaf = pointer.split("/").at(-1) || pointer;
    return labels[leaf] || title || leaf;
}
