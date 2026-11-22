import { readFileSync } from "node:fs";
import { gunzipSync } from "node:zlib";
import type { NativeMediaVariant } from "@/lib/platform-native-media";

export function catalogVariants(snapshot: string): NativeMediaVariant[] {
    const path = new URL(`../../../backend/internal/app/testdata/platform-partial-catalog-20261122/${snapshot}.json.gz`, import.meta.url);
    return JSON.parse(gunzipSync(readFileSync(path)).toString()).data.model.media_generation_profile.variants;
}
