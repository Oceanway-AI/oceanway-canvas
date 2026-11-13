import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { AssetLibraryPickerModal, remoteAssetToPickerItem, type AssetLibraryPickerItem } from "./asset-library-picker-modal";
import { useUserStore } from "@/stores/use-user-store";
import { getActiveUserScope } from "@/lib/user-scope";
import { insertLibraryAsset } from "@/services/platform-material-selection";
import { platformLibraryAsset, uploadLibraryMaterial } from "@/services/platform-library-assets";
import type { InsertAssetPayload } from "@/components/canvas/asset-picker-modal";

type Props = {
    open: boolean;
    multiple?: boolean;
    imageReferences?: boolean;
    title?: string;
    confirmLabel?: (count: number) => string;
    disabledReason?: (kind: string) => string | undefined;
    onClose: () => void;
    onInsert: (items: InsertAssetPayload[]) => Promise<unknown> | void;
};
export function PlatformMaterialPicker(props: Props) {
    const userId = useUserStore((state) => state.user?.id);
    return <MaterialPicker key={userId} {...props} />;
}
function MaterialPicker({ open, multiple = true, imageReferences = false, title, confirmLabel, disabledReason, onClose, onInsert }: Props) {
    const { t } = useTranslation();
    const queryClient = useQueryClient();
    const scope = getActiveUserScope();
    const assertCurrent = () => {
        if (getActiveUserScope() !== scope) throw new DOMException("素材操作已因账号切换取消", "AbortError");
    };
    const upload = async (files: FileList) => {
        const uploaded: AssetLibraryPickerItem[] = [];
        try {
            for (const file of Array.from(files)) {
                assertCurrent();
                if (!(imageReferences ? /^image\// : /^(image|video|audio)\//).test(file.type)) throw new Error(t("library.picker.notApplicable"));
                const reason = disabledReason?.(file.type.split("/")[0]);
                if (reason) throw new Error(reason);
                const asset = platformLibraryAsset(await uploadLibraryMaterial(file));
                assertCurrent();
                if (asset.kind !== "document") uploaded.push(remoteAssetToPickerItem(asset, undefined, true));
            }
            return uploaded;
        } finally {
            if (getActiveUserScope() === scope) {
                await Promise.all([["asset-library"], ["asset-picker"], ["canvas-workspace-assets"], ["platform-storage"], ["account-file-storage-usage"]].map((queryKey) => queryClient.invalidateQueries({ queryKey })));
            }
        }
    };
    return (
        <AssetLibraryPickerModal
            open={open}
            remoteLibrary
            platformLibrary
            acceptRemoteItems
            multiple={multiple}
            items={[]}
            categoryLabels={{}}
            remoteKind={imageReferences ? "image" : undefined}
            remoteItemDisabledReason={disabledReason && ((asset) => disabledReason(asset.kind))}
            title={title ?? (imageReferences ? t("library.assets.platform.reference") : t("library.picker.title"))}
            confirmLabel={confirmLabel ?? ((count) => (imageReferences ? t("library.assets.platform.addReferences", { count }) : t("canvasA.assetPicker.insertSelectedCount", { count })))}
            upload={{ accept: imageReferences ? "image/*" : "image/*,video/*,audio/*", description: t("library.assets.platform.upload"), onUpload: upload }}
            onClose={onClose}
            onConfirm={async (ids, picked = []) => {
                const byId = new Map(picked.map((item) => [item.id, item.asset]));
                const results: InsertAssetPayload[] = [];
                for (const id of ids) {
                    assertCurrent();
                    const asset = byId.get(id);
                    if (!asset || (imageReferences && asset.kind !== "image")) throw new Error(t("library.picker.notApplicable"));
                    const reason = disabledReason?.(asset.kind);
                    if (reason) throw new Error(reason);
                    results.push(await insertLibraryAsset(asset));
                }
                assertCurrent();
                await onInsert(results);
                onClose();
            }}
        />
    );
}
